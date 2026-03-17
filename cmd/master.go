// Package main implements the Traffic Orchestrator Master CLI entry point.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"trafficorch/pkg/comm"
	"trafficorch/pkg/config"
	"trafficorch/pkg/logging"
	"trafficorch/pkg/registry"
	"trafficorch/pkg/update"
)

// MasterServer wraps the communication master server.
type MasterServer struct {
	server      *comm.MasterServer
	configPath  string
	cfg         *config.MasterConfig
	rules       []*config.TrafficRule
	ruleMu      sync.RWMutex
	fileWatcher chan struct{}

	// v0.4.6: split logging
	slog *logging.Logger // master-status.log  — start/stop, agent events, config changes
	tlog *logging.Logger // master-traffic.log — rule distribution, per-agent rule counts

	// v0.4.5 additions
	reg            *registry.Registry
	httpSrv        *http.Server
	binaryPath     string // path to own executable
	binarySHA      string // pre-computed SHA-256 of own binary (fallback)
	updateNotified sync.Map // agentID → struct{}: agents already sent UPDATE_AVAILABLE

	// v0.4.7: multi-arch distribution
	// Key: "linux/amd64", "linux/arm64", "windows/amd64", …
	platformBinaries map[string]platformBinary

	// v0.4.11: clean shutdown
	done     chan struct{} // closed by Stop() to signal watchConfigFile to exit (H4)
	stopOnce sync.Once    // guards close(done)
	reloadMu sync.Mutex   // serialises loadConfigAndNotify() calls (M3)
}

// platformBinary holds the resolved path and pre-computed SHA-256 for a platform binary.
type platformBinary struct {
	path   string
	sha256 string
}

// NewMasterServer creates a new master server instance.
// tlog receives traffic/rule-distribution events; slog receives operational status events.
func NewMasterServer(cfg *config.MasterConfig, tlog, slog *logging.Logger) (*MasterServer, error) {
	// ── Agent registry ────────────────────────────────────────────────────────
	reg, err := registry.New(registryFile)
	if err != nil {
		return nil, fmt.Errorf("failed to initialise agent registry: %w", err)
	}

	ms := &MasterServer{
		configPath:  cfg.ConfigPath,
		cfg:         cfg,
		fileWatcher: make(chan struct{}, 1),
		slog:        slog,
		tlog:        tlog,
		reg:         reg,
		done:        make(chan struct{}),
	}

	ms.server, err = comm.NewMasterServer(
		cfg.PSK,
		cfg.Port,
		ms.onAgentRegister,
		ms.onTrafficRequest,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create master server: %w", err)
	}

	// Register optional callbacks (v0.4.5).
	ms.server.SetOnHeartbeat(ms.onAgentHeartbeat)
	ms.server.SetOnDisconnect(ms.onAgentDisconnect)

	// Load initial configuration.
	if err := ms.loadConfig(); err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Start the binary distribution HTTP server.
	if err := ms.startDistributionServer(); err != nil {
		ms.slog.Warn(fmt.Sprintf("Distribution server unavailable: %v", err))
	}

	// Start file watcher for automatic config reload.
	go ms.watchConfigFile()

	ms.slog.Info(fmt.Sprintf("Master server initialised on port %d (TTL=%ds)", cfg.Port, cfg.TTL))
	return ms, nil
}

// ─── Callbacks ────────────────────────────────────────────────────────────────

// onAgentRegister is called when a new agent registers.
func (ms *MasterServer) onAgentRegister(agentID string, hostname string) {
	agentIPs := ms.server.GetAgentIPs()
	agentIP := agentIPs[agentID]
	agentVer := ms.server.GetAgentVersion(agentID)
	agentPlatform := ms.server.GetAgentPlatform(agentID)

	ms.slog.Info(fmt.Sprintf("Agent registered: %s (%s) v%s @ %s [%s]",
		agentID, hostname, agentVer, agentIP, agentPlatform))

	// Upsert into persistent registry.
	ms.reg.Upsert(registry.AgentRecord{
		ID:       agentID,
		Hostname: hostname,
		IP:       agentIP,
		Version:  agentVer,
		Platform: agentPlatform,
		LastSeen: time.Now(),
		Status:   "online",
	})

	// Give the channel a moment to settle before pushing config.
	time.Sleep(200 * time.Millisecond)
	ms.distributeRulesToAgent(agentID)
}

// onTrafficRequest handles traffic generation requests from agents.
func (ms *MasterServer) onTrafficRequest(agentID string, rules []*comm.TrafficRule) {
	ms.tlog.Info(fmt.Sprintf("Traffic request from %s: %d rules", agentID, len(rules)))
}

// onAgentHeartbeat is called on every agent heartbeat.
func (ms *MasterServer) onAgentHeartbeat(agentID string, hb comm.HeartbeatMessage) {
	ms.reg.UpdateSeen(agentID, hb.AgentVersion)

	// Check whether the agent needs an update.
	if hb.AgentVersion != "" && needsUpdate(hb.AgentVersion, version) {
		if _, alreadySent := ms.updateNotified.LoadOrStore(agentID, struct{}{}); !alreadySent {
			ms.sendUpdateNotification(agentID)
		}
	}
}

// onAgentDisconnect is called when an agent disconnects.
func (ms *MasterServer) onAgentDisconnect(agentID string) {
	ms.slog.Info(fmt.Sprintf("Agent %s disconnected", agentID))
	ms.reg.SetOffline(agentID)
	ms.updateNotified.Delete(agentID) // allow re-notification on next connect
}

// ─── Update notification ──────────────────────────────────────────────────────

// sendUpdateNotification sends an UPDATE_AVAILABLE message to an agent.
// It looks up the agent's platform and includes the SHA-256 of the binary that will
// actually be served for that platform, so the agent can verify the download correctly.
func (ms *MasterServer) sendUpdateNotification(agentID string) {
	if ms.binarySHA == "" {
		return // distribution server not available
	}

	// Resolve the SHA-256 for the agent's specific platform (v0.4.7+).
	platform := ms.server.GetAgentPlatform(agentID)
	_, sha := ms.binaryForPlatform(platform)

	msg := &comm.UpdateAvailableMessage{
		BaseMessage: comm.BaseMessage{
			Type:      comm.MsgUpdateAvailable,
			Timestamp: time.Now().Unix(),
			Version:   comm.ProtocolVersion,
		},
		NewVersion: version,
		HTTPPort:   distributionPort,
		SHA256:     sha,
	}
	if err := ms.server.SendToAgent(agentID, msg); err != nil {
		ms.slog.Warn(fmt.Sprintf("Failed to send UPDATE_AVAILABLE to %s: %v", agentID, err))
	} else {
		ms.slog.Info(fmt.Sprintf("Sent UPDATE_AVAILABLE to agent %s (platform: %s, new: v%s)", agentID, platform, version))
	}
}

// ─── Distribution HTTP server ─────────────────────────────────────────────────

// platformBinaryName returns the expected filename for a given "os/arch" platform string,
// e.g. "linux/arm64" → "trafficorch-linux-arm64", "windows/amd64" → "trafficorch-windows-amd64.exe".
func platformBinaryName(platform string) string {
	parts := strings.SplitN(platform, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	name := fmt.Sprintf("trafficorch-%s-%s", parts[0], parts[1])
	if parts[0] == "windows" {
		name += ".exe"
	}
	return name
}

// startDistributionServer starts the HTTP server on distributionPort.
// It pre-computes SHA-256 checksums for the master's own binary and for any
// platform-specific binaries found in the same directory (trafficorch-{os}-{arch}[.exe]).
func (ms *MasterServer) startDistributionServer() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot locate own executable: %w", err)
	}
	ms.binaryPath = exe

	sha, err := update.BinaryChecksum(exe)
	if err != nil {
		return fmt.Errorf("cannot compute binary checksum: %w", err)
	}
	ms.binarySHA = sha

	// Scan sibling binaries for all known platforms.
	ms.platformBinaries = make(map[string]platformBinary)
	knownPlatforms := []string{"linux/amd64", "linux/arm64", "windows/amd64"}
	binDir := filepath.Dir(exe)
	for _, p := range knownPlatforms {
		name := platformBinaryName(p)
		candidate := filepath.Join(binDir, name)
		candidateSHA, err := update.BinaryChecksum(candidate)
		if err != nil {
			continue // binary not present for this platform
		}
		ms.platformBinaries[p] = platformBinary{path: candidate, sha256: candidateSHA}
		ms.slog.Info(fmt.Sprintf("Platform binary available: %s → %s (SHA256: %s...)", p, name, candidateSHA[:16]))
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/binary", ms.handleBinaryDownload)
	mux.HandleFunc("/sha256", ms.handleSHA256)
	mux.HandleFunc("/version", ms.handleVersion)
	mux.HandleFunc("/agents", ms.handleAgents)

	ms.httpSrv = &http.Server{
		Addr:         fmt.Sprintf(":%d", distributionPort),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
	}

	go func() {
		ms.slog.Info(fmt.Sprintf("Distribution server listening on port %d (SHA256: %s...)",
			distributionPort, ms.binarySHA[:16]))
		if err := ms.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			ms.slog.Error(fmt.Sprintf("Distribution server error: %v", err))
		}
	}()

	return nil
}

// binaryForPlatform returns the path and SHA-256 for the binary best suited for platform.
// Falls back to the master's own binary if no platform-specific binary is available.
func (ms *MasterServer) binaryForPlatform(platform string) (path, sha string) {
	if pb, ok := ms.platformBinaries[platform]; ok {
		return pb.path, pb.sha256
	}
	return ms.binaryPath, ms.binarySHA
}

func (ms *MasterServer) handleBinaryDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Honour optional ?platform=linux/arm64 query param (v0.4.7+).
	platform := r.URL.Query().Get("platform")
	binaryPath, sha := ms.binaryForPlatform(platform)

	f, err := os.Open(binaryPath)
	if err != nil {
		http.Error(w, "binary unavailable", http.StatusInternalServerError)
		return
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		http.Error(w, "binary unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(stat.Size(), 10))
	w.Header().Set("X-SHA256", sha)
	w.Header().Set("X-Version", version)
	if r.Method == http.MethodHead {
		return
	}
	io.Copy(w, f) //nolint:errcheck
}

func (ms *MasterServer) handleSHA256(w http.ResponseWriter, r *http.Request) {
	platform := r.URL.Query().Get("platform")
	_, sha := ms.binaryForPlatform(platform)
	fmt.Fprintln(w, sha)
}

func (ms *MasterServer) handleVersion(w http.ResponseWriter, _ *http.Request) {
	fmt.Fprintln(w, version)
}

func (ms *MasterServer) handleAgents(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	records := ms.reg.All()
	if err := json.NewEncoder(w).Encode(records); err != nil {
		ms.slog.Warn(fmt.Sprintf("Failed to encode agents response: %v", err))
	}
}

// ─── Config loading ───────────────────────────────────────────────────────────

// loadConfig re-parses the configuration file and updates the active rule set.
func (ms *MasterServer) loadConfig() error {
	freshCfg, err := config.ParseExtendedConfigV2(ms.configPath)
	if err != nil {
		return fmt.Errorf("failed to parse config file %q: %w", ms.configPath, err)
	}

	// Load profiles when PROFILE_DIR is configured.
	// PROFILE_DIR is already resolved to an absolute path by ParseExtendedConfigV2.
	if freshCfg.ProfileDir != "" {
		profiles, err := config.LoadProfileDir(freshCfg.ProfileDir)
		if err != nil {
			ms.slog.Warn(fmt.Sprintf("Could not load profiles from %q: %v", freshCfg.ProfileDir, err))
		} else {
			freshCfg.Profiles = profiles
			ms.slog.Info(fmt.Sprintf("Loaded %d profile(s) from %s", len(profiles), freshCfg.ProfileDir))
		}
	}

	ms.ruleMu.Lock()
	ms.rules = freshCfg.TrafficRules
	ms.cfg.TTL = freshCfg.TTL
	ms.cfg.TargetMap = freshCfg.TargetMap
	ms.cfg.Assignments = freshCfg.Assignments
	ms.cfg.TagMap = freshCfg.TagMap
	ms.cfg.Profiles = freshCfg.Profiles
	ms.cfg.ProfileDir = freshCfg.ProfileDir
	ms.ruleMu.Unlock()

	ms.slog.Info(fmt.Sprintf("Config loaded: %s — %d direct rule(s), %d profile(s), %d assignment(s), TTL=%ds",
		ms.configPath, len(freshCfg.TrafficRules), len(freshCfg.Profiles),
		len(freshCfg.Assignments), freshCfg.TTL))
	return nil
}

// watchConfigFile monitors the config file and the profile directory for changes.
// A reload is triggered whenever the config file or any .profile file is modified,
// added, or removed (checked every configWatchInterval).
//
// H4: exits cleanly when ms.done is closed (during Stop()).
// M2: uses time.NewTicker instead of repeated time.After() to avoid timer leaks.
func (ms *MasterServer) watchConfigFile() {
	ticker := time.NewTicker(configWatchInterval)
	defer ticker.Stop()

	var lastConfigMod time.Time
	var lastProfileSig time.Time // latest mtime across all profile files

	for {
		select {
		case <-ticker.C:
			changed := false

			// ── Check main config file ──────────────────────────────────────
			if info, err := os.Stat(ms.configPath); err != nil {
				ms.slog.Error(fmt.Sprintf("Config file not found: %s", ms.configPath))
			} else {
				if !info.ModTime().Equal(lastConfigMod) && !lastConfigMod.IsZero() {
					ms.slog.Info("Config file changed — reloading...")
					changed = true
				}
				lastConfigMod = info.ModTime()
			}

			// ── Check profile directory ─────────────────────────────────────
			ms.ruleMu.RLock()
			profileDir := ms.cfg.ProfileDir
			ms.ruleMu.RUnlock()

			if profileDir != "" {
				sig := latestProfileMod(profileDir)
				if !lastProfileSig.IsZero() && !sig.Equal(lastProfileSig) {
					ms.slog.Info(fmt.Sprintf("Profile directory changed (%s) — reloading...", profileDir))
					changed = true
				}
				lastProfileSig = sig
			}

			if changed {
				go ms.loadConfigAndNotify()
			}

		case <-ms.fileWatcher:
			ms.slog.Info("Config reload triggered manually")
			go ms.loadConfigAndNotify()

		case <-ms.done:
			return // server is shutting down
		}
	}
}

// latestProfileMod returns the most recent modification time across all .profile
// files in dir. Returns zero time if dir is empty or unreadable.
func latestProfileMod(dir string) time.Time {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return time.Time{}
	}
	var latest time.Time
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".profile" {
			continue
		}
		if info, err := e.Info(); err == nil {
			if info.ModTime().After(latest) {
				latest = info.ModTime()
			}
		}
	}
	return latest
}

// loadConfigAndNotify reloads config and pushes updates to all agents.
// reloadMu ensures that at most one reload runs at a time — concurrent triggers
// (timer + manual) cannot interleave their loadConfig / notifyAllAgents calls. (M3)
func (ms *MasterServer) loadConfigAndNotify() {
	ms.reloadMu.Lock()
	defer ms.reloadMu.Unlock()

	if err := ms.loadConfig(); err != nil {
		ms.slog.Error(fmt.Sprintf("Failed to reload config: %v", err))
		return
	}
	ms.notifyAllAgents()
}

// notifyAllAgents distributes the current ruleset to every connected agent.
func (ms *MasterServer) notifyAllAgents() {
	agentIPs := ms.server.GetAgentIPs()
	for agentID := range agentIPs {
		ms.distributeRulesToAgent(agentID)
	}
}

// ─── Rule distribution ────────────────────────────────────────────────────────

// distributeRulesToAgent builds a per-agent rule set and sends a CONFIG_UPDATE.
//
// Distribution logic (v0.4.0+):
//
//  1. Profile-based (preferred, v0.4.0+):
//     If the agent IP has entries in [ASSIGNMENTS] and profiles are loaded,
//     rules are resolved from the assigned profiles (SELF/group:/ANY expanded).
//
//  2. Direct rule fallback (v0.3.0 / legacy):
//     Rules with Source=="" → sent to ALL agents (connect).
//     Rules with Source!="" → sent to the matching source agent (connect) or
//     the matching destination agent (listen).
//
// Both paths are additive: a config can mix profile assignments and direct rules.
func (ms *MasterServer) distributeRulesToAgent(agentID string) {
	agentIPs := ms.server.GetAgentIPs()
	agentIP := agentIPs[agentID]

	ms.ruleMu.RLock()
	allRules := ms.rules
	ttl := ms.cfg.TTL
	assignments := ms.cfg.Assignments
	tagMap := ms.cfg.TagMap
	targetMap := ms.cfg.TargetMap
	profiles := ms.cfg.Profiles
	ms.ruleMu.RUnlock()

	var agentRules []*comm.TrafficRule

	// ── Profile-based distribution (v0.4.0) ──────────────────────────────────
	if len(profiles) > 0 && len(assignments) > 0 && agentIP == "" {
		ms.tlog.Warn(fmt.Sprintf("Agent %s: profile distribution skipped — agent IP unknown (check that the agent's interface IP is routable to the master)", agentID))
	}
	if len(profiles) > 0 && len(assignments) > 0 && agentIP != "" {
		profileNames := config.LookupAssignments(agentIP, assignments, targetMap)
		if len(profileNames) > 0 {
			resolved, err := config.ResolveProfileRules(profiles, profileNames, agentIP, targetMap, tagMap)
			if err != nil {
				ms.tlog.Error(fmt.Sprintf("Profile resolution failed for agent %s (IP=%s): %v", agentID, agentIP, err))
			} else {
				for _, r := range resolved {
					agentRules = append(agentRules, &comm.TrafficRule{
						Protocol: r.Protocol,
						Role:     r.Role,
						Source:   r.Source,
						Target:   r.Target,
						Port:     r.Port,
						Interval: r.Interval,
						Count:    r.Count,
						Name:     r.Name,
					})
				}
				ms.tlog.Info(fmt.Sprintf("Agent %s (IP=%s): %d rule(s) from profile(s) %v",
					agentID, agentIP, len(agentRules), profileNames))
			}
		} else {
			ms.tlog.Info(fmt.Sprintf("Agent %s (IP=%s): no profile assignment found", agentID, agentIP))
		}
	}

	// ── Direct rule distribution (legacy / additive fallback) ─────────────────
	for _, rule := range allRules {
		if rule.Source == "" {
			agentRules = append(agentRules, &comm.TrafficRule{
				Protocol: rule.Protocol,
				Target:   rule.Target,
				Port:     rule.Port,
				Interval: rule.Interval,
				Count:    rule.Count,
				Name:     rule.Name,
				Role:     "connect",
			})
		} else {
			if agentIP == rule.Source {
				agentRules = append(agentRules, &comm.TrafficRule{
					Protocol: rule.Protocol,
					Source:   rule.Source,
					Target:   rule.Target,
					Port:     rule.Port,
					Interval: rule.Interval, // H6: was missing, causing agents to ignore configured intervals
					Count:    rule.Count,
					Name:     rule.Name,
					Role:     "connect",
				})
			} else if agentIP == rule.Target {
				agentRules = append(agentRules, &comm.TrafficRule{
					Protocol: rule.Protocol,
					Port:     rule.Port,
					Name:     rule.Name,
					Role:     "listen",
				})
			}
		}
	}

	if len(agentRules) == 0 {
		msg := fmt.Sprintf("No rules applicable to agent %s (IP=%s) — "+
			"verify SOURCE/TARGET IPs in config match the agent IP, "+
			"or check [ASSIGNMENTS] entries if using profiles", agentID, agentIP)
		ms.tlog.Warn(msg)
		ms.slog.Warn(msg)
		return
	}

	msg := &comm.ConfigUpdateMessage{
		BaseMessage: comm.BaseMessage{
			Type:      comm.MsgConfigUpdate,
			Timestamp: time.Now().Unix(),
			Version:   "1.0",
		},
		TTL:   ttl,
		Rules: agentRules,
	}

	if err := ms.server.SendToAgent(agentID, msg); err != nil {
		ms.tlog.Error(fmt.Sprintf("Failed to send config to agent %s: %v", agentID, err))
	} else {
		ms.tlog.Info(fmt.Sprintf("Sent %d rule(s) to agent %s (TTL=%ds)", len(agentRules), agentID, ttl))
	}
}

// ─── Lifecycle ────────────────────────────────────────────────────────────────

// Start starts the master server and blocks until a shutdown signal is received.
func (ms *MasterServer) Start() error {
	ms.slog.Info(fmt.Sprintf("Starting Traffic Orchestrator Master v%s", version))
	ms.slog.Info(fmt.Sprintf("Listening on port %d with PSK authentication", ms.cfg.Port))
	ms.slog.Info(fmt.Sprintf("Config file: %s", ms.configPath))

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	ms.slog.Info("Shutdown signal received")

	return ms.Stop(ms.slog)
}

// Stop gracefully shuts down the master server and the HTTP distribution server.
func (ms *MasterServer) Stop(logger *logging.Logger) error {
	logger.Info("Shutting down Master server...")
	ms.stopOnce.Do(func() { close(ms.done) }) // signals watchConfigFile to exit (H4)
	ms.server.CloseAllAgents()
	ms.server.CloseListener() // lets acceptLoop() exit cleanly

	if ms.httpSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := ms.httpSrv.Shutdown(ctx); err != nil {
			logger.Warn(fmt.Sprintf("HTTP server shutdown: %v", err))
		}
	}
	return nil
}

// GetConfigPath returns the current configuration file path.
func (ms *MasterServer) GetConfigPath() string {
	return ms.configPath
}

// ReloadConfig manually triggers a config reload.
// Non-blocking: if a reload is already pending (channel full), this is a no-op. (N2)
func (ms *MasterServer) ReloadConfig() error {
	select {
	case ms.fileWatcher <- struct{}{}:
	default:
		// a reload is already queued
	}
	return nil
}

// ─── Version helpers ──────────────────────────────────────────────────────────

// needsUpdate reports whether agentVer is strictly older than masterVer.
func needsUpdate(agentVer, masterVer string) bool {
	return compareVersions(agentVer, masterVer) < 0
}

// compareVersions compares two "major.minor.patch" version strings.
// Returns -1 if a < b, 0 if equal, 1 if a > b.
func compareVersions(a, b string) int {
	pa := parseVer(a)
	pb := parseVer(b)
	for i := 0; i < 3; i++ {
		if pa[i] < pb[i] {
			return -1
		}
		if pa[i] > pb[i] {
			return 1
		}
	}
	return 0
}

func parseVer(v string) [3]int {
	var maj, min, pat int
	fmt.Sscanf(v, "%d.%d.%d", &maj, &min, &pat)
	return [3]int{maj, min, pat}
}
