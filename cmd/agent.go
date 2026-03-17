package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"trafficorch/pkg/comm"
	"trafficorch/pkg/config"
	"trafficorch/pkg/logging"
	"trafficorch/pkg/traffic"
	"trafficorch/pkg/update"
)

// masterConnInfo holds the details needed to (re)connect to a master.
type masterConnInfo struct {
	host string
	port int
	psk  string
}

// Agent handles agent-specific operations.
// It can run in two modes:
//   - Connected: maintains a live channel to the master.
//   - Standalone: executes rules loaded from instructions.conf; reconnects
//     when the TTL expires or the master becomes reachable again.
type Agent struct {
	client       *comm.AgentClient // nil in standalone mode
	agentID      string
	standalone   bool
	masterCfg    masterConnInfo
	currentRules []*config.TrafficRule
	mu           sync.RWMutex
	isRunning    int32 // accessed via sync/atomic
	stopChan     chan struct{}
	listenerMgr  *traffic.ListenerManager

	// connStop is closed when the current TCP connection to the master dies.
	// receiveMessages() closes it on first error; sendHeartbeatLoop() exits on it.
	// A fresh channel is created for each new connection (reconnect).
	connStop chan struct{}

	// v0.4.6: split logging
	slog *logging.Logger // agent-status.log  — start/stop, connect, update events
	tlog *logging.Logger // agent-traffic.log — rule application, connections, listeners
}

// NewAgent creates and registers a connected agent.
// tlog receives traffic events; slog receives operational status events.
// If the master is unreachable it returns an error; the caller may then try
// newStandaloneAgent instead.
func NewAgent(cfg *config.AgentConfig, tlog, slog *logging.Logger) (*Agent, error) {
	client, err := comm.NewAgentClient(cfg.MasterHost, cfg.Port, cfg.PSK)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to master: %w", err)
	}

	hostname, _ := os.Hostname()
	platform := fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)

	slog.Info(fmt.Sprintf("Connecting to master at %s:%d", cfg.MasterHost, cfg.Port))
	if err := client.Register(cfg.AgentID, hostname, platform, version); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to register with master: %w", err)
	}

	return &Agent{
		client:      client,
		agentID:     cfg.AgentID,
		standalone:  false,
		masterCfg:   masterConnInfo{cfg.MasterHost, cfg.Port, cfg.PSK},
		stopChan:    make(chan struct{}),
		connStop:    make(chan struct{}),
		listenerMgr: traffic.NewListenerManager(),
		slog:        slog,
		tlog:        tlog,
	}, nil
}

// newStandaloneAgent creates an agent that operates from a local instructions.conf.
// If instrPath does not exist, an error is returned.
func newStandaloneAgent(instrPath string, fallbackCfg masterConnInfo, agentID string, tlog, slog *logging.Logger) (*Agent, error) {
	instrConf, err := config.LoadInstructionsConf(instrPath)
	if err != nil {
		return nil, fmt.Errorf("no instructions.conf found (%s): %w", instrPath, err)
	}

	slog.Info(fmt.Sprintf("Standalone mode: loaded %d rules from %s (received %s)",
		len(instrConf.Rules), instrPath, instrConf.ReceivedAt.Format(time.RFC3339)))

	if instrConf.TTL > 0 {
		if instrConf.IsExpired() {
			slog.Warn("instructions.conf TTL has already expired — will attempt to reconnect to master")
		} else {
			slog.Info(fmt.Sprintf("Instructions valid for another %s (TTL %ds)",
				instrConf.ExpiresIn().Round(time.Second), instrConf.TTL))
		}
	}

	// Use master conn info from instructions.conf; fall back to CLI-supplied values.
	mCfg := masterConnInfo{
		host: instrConf.MasterHost,
		port: instrConf.MasterPort,
		psk:  instrConf.PSK,
	}
	if fallbackCfg.host != "" {
		mCfg = fallbackCfg
	}

	id := instrConf.AgentID
	if agentID != "" {
		id = agentID
	}
	if id == "" {
		id = "agent-unknown"
	}

	a := &Agent{
		client:       nil,
		agentID:      id,
		standalone:   true,
		masterCfg:    mCfg,
		currentRules: instrConf.Rules,
		stopChan:     make(chan struct{}),
		connStop:     make(chan struct{}),
		listenerMgr:  traffic.NewListenerManager(),
		slog:         slog,
		tlog:         tlog,
	}

	// Schedule TTL-based reconnect if appropriate.
	if instrConf.TTL > 0 {
		go a.ttlReconnectLoop(instrConf)
	}

	return a, nil
}

// Start begins the agent main loop and blocks until shutdown.
func (a *Agent) Start() error {
	a.slog.Info(fmt.Sprintf("Agent %s started (standalone=%v)", a.agentID, a.standalone))
	atomic.StoreInt32(&a.isRunning, 1)

	if !a.standalone {
		go a.receiveMessages(a.connStop)
		go a.sendHeartbeatLoop(a.connStop)
	} else {
		a.applyRules(a.currentRules)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sigChan:
		a.slog.Info("Shutdown signal received")
	case <-a.stopChan:
		a.slog.Info("Agent stopping")
	}

	return a.Stop()
}

// applyRules stops existing listeners, starts new ones for "listen" rules,
// and launches goroutines for "connect" rules.
func (a *Agent) applyRules(rules []*config.TrafficRule) {
	a.listenerMgr.StopAll()

	a.mu.Lock()
	a.currentRules = rules
	a.mu.Unlock()

	var connectRules []*comm.TrafficRule

	for _, rule := range rules {
		r := rule
		if r.Role == "listen" {
			if err := a.listenerMgr.StartListener(r.Protocol, r.Port); err != nil {
				a.tlog.Error(fmt.Sprintf("Failed to open %s listener on port %d: %v", r.Protocol, r.Port, err))
			} else {
				a.tlog.Info(fmt.Sprintf("Listening on %s port %d", r.Protocol, r.Port))
			}
		} else {
			connectRules = append(connectRules, configRuleToComm(r))
		}
	}

	if len(connectRules) > 0 {
		go a.executeTraffic(connectRules)
	}
}

func configRuleToComm(r *config.TrafficRule) *comm.TrafficRule {
	return &comm.TrafficRule{
		Protocol: r.Protocol,
		Source:   r.Source,
		Target:   r.Target,
		Port:     r.Port,
		Interval: r.Interval,
		Count:    r.Count,
		Name:     r.Name,
		Role:     r.Role,
	}
}

func commRulesToConfig(rules []*comm.TrafficRule) []*config.TrafficRule {
	out := make([]*config.TrafficRule, len(rules))
	for i, r := range rules {
		out[i] = &config.TrafficRule{
			Protocol: r.Protocol,
			Source:   r.Source,
			Target:   r.Target,
			Port:     r.Port,
			Interval: r.Interval,
			Count:    r.Count,
			Name:     r.Name,
			Role:     r.Role,
		}
	}
	return out
}

// receiveMessages continuously reads and handles messages from the master.
// connStop is a per-connection channel that is closed when this connection is
// declared dead so that the paired sendHeartbeatLoop exits cleanly.
func (a *Agent) receiveMessages(connStop chan struct{}) {
	for {
		if atomic.LoadInt32(&a.isRunning) == 0 {
			return
		}

		msg, msgBytes, err := a.client.ReadMessage()
		if err != nil {
			a.slog.Error(fmt.Sprintf("Connection to master lost: %v — reconnecting...", err))
			// Close connStop to signal the paired sendHeartbeatLoop to exit,
			// then kick off a reconnect goroutine and leave this goroutine.
			// reconnectToMaster() will start fresh receiveMessages + sendHeartbeatLoop.
			close(connStop)
			a.client.Close()
			go a.reconnectToMaster()
			return
		}

		switch msg.Type {
		case comm.MsgConfigUpdate:
			var configMsg comm.ConfigUpdateMessage
			if err := json.Unmarshal(msgBytes, &configMsg); err == nil {
				a.tlog.Info(fmt.Sprintf("CONFIG_UPDATE received: %d rule(s) (TTL=%ds)", len(configMsg.Rules), configMsg.TTL))
				cfgRules := commRulesToConfig(configMsg.Rules)
				a.applyRules(cfgRules)
				a.saveInstructions(configMsg.TTL, cfgRules)
			}

		case comm.MsgTrafficStart:
			var startMsg comm.TrafficStartMessage
			if err := json.Unmarshal(msgBytes, &startMsg); err == nil {
				a.startTraffic(startMsg.Rules)
			}

		case comm.MsgTrafficStop:
			a.stopTraffic()

		case comm.MsgUpdateAvailable:
			var updateMsg comm.UpdateAvailableMessage
			if err := json.Unmarshal(msgBytes, &updateMsg); err == nil {
				a.slog.Info(fmt.Sprintf("Update available: v%s (current: v%s) — downloading...", updateMsg.NewVersion, version))
				go a.applyUpdate(updateMsg)
			}

		default:
			a.slog.Warn(fmt.Sprintf("Unknown message type: %s", msg.Type))
		}
	}
}

func (a *Agent) applyUpdate(msg comm.UpdateAvailableMessage) {
	exe, err := os.Executable()
	if err != nil {
		a.slog.Error(fmt.Sprintf("Self-update: cannot locate own binary: %v", err))
		return
	}

	// Pass our own platform so the master can serve the correct architecture binary (v0.4.7+).
	platform := fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
	downloadURL := fmt.Sprintf("http://%s:%d/binary?platform=%s",
		a.masterCfg.host, msg.HTTPPort, url.QueryEscape(platform))
	a.slog.Info(fmt.Sprintf("Self-update: downloading v%s for %s from %s", msg.NewVersion, platform, downloadURL))

	restartArgs := make([]string, 0, len(os.Args)-1)
	for _, arg := range os.Args[1:] {
		if arg != "--daemon-child" {
			restartArgs = append(restartArgs, arg)
		}
	}

	if err := update.Apply(downloadURL, msg.SHA256, exe, restartArgs); err != nil {
		a.slog.Error(fmt.Sprintf("Self-update failed: %v", err))
		return
	}
}

func (a *Agent) saveInstructions(ttl int, rules []*config.TrafficRule) {
	instrConf := &config.InstructionsConf{
		ReceivedAt: time.Now(),
		TTL:        ttl,
		MasterHost: a.masterCfg.host,
		MasterPort: a.masterCfg.port,
		PSK:        a.masterCfg.psk,
		AgentID:    a.agentID,
		Rules:      rules,
	}
	if err := config.SaveInstructionsConf(config.InstructionsConfFile, instrConf); err != nil {
		a.slog.Warn(fmt.Sprintf("Could not save instructions.conf: %v", err))
	} else {
		a.slog.Info("Instructions saved to instructions.conf")
	}
}

func (a *Agent) startTraffic(rules []*comm.TrafficRule) {
	if atomic.LoadInt32(&a.isRunning) != 0 {
		go a.executeTraffic(rules)
	}
}

func (a *Agent) executeTraffic(rules []*comm.TrafficRule) {
	a.tlog.Info(fmt.Sprintf("Starting traffic generation for %d rule(s)", len(rules)))

	var wg sync.WaitGroup
	for _, rule := range rules {
		if rule.Role == "listen" {
			continue
		}
		wg.Add(1)
		go func(r *comm.TrafficRule) {
			defer wg.Done()
			a.executeSingleRule(r)
		}(rule)
	}

	wg.Wait()
	a.tlog.Info(fmt.Sprintf("Traffic generation completed for %d rule(s)", len(rules)))
}

func (a *Agent) executeSingleRule(rule *comm.TrafficRule) {
	address := net.JoinHostPort(rule.Target, strconv.Itoa(rule.Port))
	connCount := 0

	a.tlog.Info(fmt.Sprintf("Rule start: %s (%s %s)", rule.Name, rule.Protocol, address))

	for {
		if atomic.LoadInt32(&a.isRunning) == 0 {
			return
		}

		var err error
		switch rule.Protocol {
		case "TCP":
			err = a.dialTCP(address, rule.Name)
		case "UDP":
			err = a.dialUDP(address, rule.Name)
		default:
			a.tlog.Error(fmt.Sprintf("Unsupported protocol: %s", rule.Protocol))
			return
		}

		if err != nil {
			a.tlog.Warn(fmt.Sprintf("Connection failed %s %s (%s): %v", rule.Protocol, address, rule.Name, err))
		} else {
			connCount++
		}

		if rule.Count > 0 && connCount >= rule.Count {
			break
		}

		if rule.Interval > 0 {
			time.Sleep(time.Duration(rule.Interval) * time.Second)
		} else {
			time.Sleep(defaultConnectionDelay)
		}
	}

	a.tlog.Info(fmt.Sprintf("Rule complete: %s — %d connection(s)", rule.Name, connCount))
}

func (a *Agent) dialTCP(address, ruleName string) error {
	conn, err := net.DialTimeout("tcp", address, connectTimeout)
	if err != nil {
		return err
	}
	defer conn.Close()

	payload := randomPayload(64)
	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, werr := conn.Write(payload); werr != nil {
		a.tlog.Warn(fmt.Sprintf("TCP write error to %s: %v", address, werr))
	}

	time.Sleep(tcpHoldDuration)
	a.tlog.Info(fmt.Sprintf("TCP %s (%s): %d bytes sent", address, ruleName, len(payload)))
	return nil
}

func (a *Agent) dialUDP(address, ruleName string) error {
	conn, err := net.DialTimeout("udp", address, connectTimeout)
	if err != nil {
		return err
	}
	defer conn.Close()

	payload := randomPayload(64)
	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, werr := conn.Write(payload); werr != nil {
		return fmt.Errorf("UDP send error: %w", werr)
	}

	a.tlog.Info(fmt.Sprintf("UDP %s (%s): %d bytes sent", address, ruleName, len(payload)))
	return nil
}

func (a *Agent) stopTraffic() {
	a.tlog.Info("Stopping traffic generation...")
}

// sendHeartbeatLoop sends periodic heartbeats to the master.
// connStop is closed by receiveMessages() when the connection dies, causing
// this loop to exit so it doesn't keep sending on a dead socket.
func (a *Agent) sendHeartbeatLoop(connStop <-chan struct{}) {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if atomic.LoadInt32(&a.isRunning) == 0 {
				return
			}

			cpuUsage, memUsage := a.getSystemStats()

			a.mu.RLock()
			activeRules := len(a.currentRules)
			a.mu.RUnlock()

			if err := a.client.SendHeartbeat(version, cpuUsage, memUsage, activeRules); err != nil {
				a.slog.Warn(fmt.Sprintf("Failed to send heartbeat: %v", err))
			}

		case <-connStop:
			// Connection died — receiveMessages() will handle the reconnect.
			return

		case <-a.stopChan:
			return
		}
	}
}

func (a *Agent) getSystemStats() (float64, int64) {
	return 0.0, 0
}

func (a *Agent) ttlReconnectLoop(instrConf *config.InstructionsConf) {
	waitDuration := instrConf.ExpiresIn()
	if waitDuration > 0 {
		a.slog.Info(fmt.Sprintf("TTL reconnect scheduled in %s", waitDuration.Round(time.Second)))
		select {
		case <-time.After(waitDuration):
		case <-a.stopChan:
			return
		}
	}

	a.slog.Info("TTL expired — attempting to reconnect to master...")
	a.reconnectToMaster()
}

func (a *Agent) reconnectToMaster() {
	for attempt := 1; ; attempt++ {
		if atomic.LoadInt32(&a.isRunning) == 0 {
			return
		}

		a.slog.Info(fmt.Sprintf("Reconnect attempt %d to %s:%d...",
			attempt, a.masterCfg.host, a.masterCfg.port))

		client, err := comm.NewAgentClient(a.masterCfg.host, a.masterCfg.port, a.masterCfg.psk)
		if err != nil {
			a.slog.Warn(fmt.Sprintf("Reconnect attempt %d failed: %v — retrying in %s", attempt, err, masterReconnectDelay))
			select {
			case <-time.After(masterReconnectDelay):
				continue
			case <-a.stopChan:
				return
			}
		}

		hostname, _ := os.Hostname()
		platform := fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)

		if err := client.Register(a.agentID, hostname, platform, version); err != nil {
			client.Close()
			a.slog.Warn(fmt.Sprintf("Reconnect registration failed: %v — retrying in %s", err, masterReconnectDelay))
			select {
			case <-time.After(masterReconnectDelay):
				continue
			case <-a.stopChan:
				return
			}
		}

		a.slog.Info(fmt.Sprintf("Reconnected to master after %d attempt(s) — switching to connected mode", attempt))

		// Create a fresh connStop for this new connection so the new goroutines
		// can be signalled independently from any previous (now-dead) connection.
		newConnStop := make(chan struct{})

		a.mu.Lock()
		a.client = client
		a.connStop = newConnStop
		a.standalone = false
		a.mu.Unlock()

		go a.receiveMessages(newConnStop)
		go a.sendHeartbeatLoop(newConnStop)
		return
	}
}

// warnIfNonRoot logs a privileged-port warning to the status log.
//
// Called EARLY at agent startup — before connecting to master — so it always
// appears in agent-status.log, even when stderr is unavailable (daemon mode).
// The warning reminds operators that listen rules on ports 1–1023 require root
// (or CAP_NET_BIND_SERVICE on Linux).
func warnIfNonRoot(agentID string, client *comm.AgentClient, slog *logging.Logger) {
	if runtime.GOOS == "windows" {
		return
	}
	if os.Getuid() == 0 {
		return
	}

	msg := fmt.Sprintf(
		"Agent %s running as non-root (uid=%d): listen rules on privileged ports "+
			"(1-1023) will fail. Ensure assigned profiles only use ports > 1023, "+
			"or restart with sudo / CAP_NET_BIND_SERVICE.",
		agentID, os.Getuid(),
	)

	fmt.Fprintf(os.Stderr, "WARNING: %s\n", msg)
	slog.Warn(msg)

	if client != nil {
		if err := client.SendWarning(agentID, "NON_ROOT", msg); err != nil {
			slog.Warn(fmt.Sprintf("Could not forward non-root warning to master: %v", err))
		}
	}
}

func (a *Agent) Stop() error {
	a.slog.Info("Shutting down agent...")
	atomic.StoreInt32(&a.isRunning, 0)

	a.listenerMgr.StopAll()

	if a.client != nil {
		return a.client.Close()
	}

	return nil
}

func randomPayload(n int) []byte {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	r := time.Now().UnixNano()
	for i := range b {
		r = r*6364136223846793005 + 1442695040888963407
		b[i] = charset[((r>>33)^r)%int64(len(charset))]
	}
	return b
}

func (a *Agent) updateRules(rules []*comm.TrafficRule) {
	cfgRules := commRulesToConfig(rules)
	a.applyRules(cfgRules)
}
