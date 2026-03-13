// Package main implements the Traffic Orchestrator Master CLI entry point.
package main

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"trafficorch/pkg/comm"
	"trafficorch/pkg/config"
	"trafficorch/pkg/logging"
)

// MasterServer wraps the communication master server.
type MasterServer struct {
	server      *comm.MasterServer
	configPath  string
	cfg         *config.MasterConfig
	rules       []*config.TrafficRule
	ruleMu      sync.RWMutex
	fileWatcher chan struct{}
	logger      *logging.Logger
}

// NewMasterServer creates a new master server instance.
func NewMasterServer(cfg *config.MasterConfig, logger *logging.Logger) (*MasterServer, error) {
	ms := &MasterServer{
		configPath:  cfg.ConfigPath,
		cfg:         cfg,
		fileWatcher: make(chan struct{}, 1),
		logger:      logger,
	}

	var err error
	ms.server, err = comm.NewMasterServer(
		cfg.PSK,
		cfg.Port,
		ms.onAgentRegister,
		ms.onTrafficRequest,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create master server: %w", err)
	}

	// Load initial configuration
	if err := ms.loadConfig(); err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Start file watcher for automatic config reload
	go ms.watchConfigFile()

	ms.logger.Info(fmt.Sprintf("Master server initialised on port %d (TTL=%ds)", cfg.Port, cfg.TTL))

	return ms, nil
}

// onAgentRegister is called when a new agent registers.
// It immediately distributes the current ruleset to the new agent.
func (ms *MasterServer) onAgentRegister(agentID string, hostname string) {
	ms.logger.Info(fmt.Sprintf("New agent registered: %s (%s)", agentID, hostname))

	// Give the channel a moment to settle before pushing config.
	time.Sleep(200 * time.Millisecond)

	ms.distributeRulesToAgent(agentID)
}

// onTrafficRequest handles traffic generation requests from agents.
func (ms *MasterServer) onTrafficRequest(agentID string, rules []*comm.TrafficRule) {
	ms.logger.Info(fmt.Sprintf("Traffic request from %s: %d rules", agentID, len(rules)))
}

// loadConfig re-parses the configuration file from disk and updates the active rule set.
func (ms *MasterServer) loadConfig() error {
	freshCfg, err := config.ParseExtendedConfigV2(ms.configPath)
	if err != nil {
		return fmt.Errorf("failed to parse config file %q: %w", ms.configPath, err)
	}

	ms.ruleMu.Lock()
	ms.rules = freshCfg.TrafficRules
	ms.cfg.TTL = freshCfg.TTL
	ms.ruleMu.Unlock()

	ms.logger.Info(fmt.Sprintf("Loaded %d traffic rules from %s (TTL=%ds)", len(freshCfg.TrafficRules), ms.configPath, freshCfg.TTL))
	return nil
}

// watchConfigFile monitors the config file for changes.
func (ms *MasterServer) watchConfigFile() {
	var lastModTime time.Time

	for {
		select {
		case <-time.After(configWatchInterval):
			info, err := os.Stat(ms.configPath)
			if err != nil {
				ms.logger.Error(fmt.Sprintf("Config file not found: %s", ms.configPath))
				continue
			}

			modTime := info.ModTime()
			if !modTime.Equal(lastModTime) && !lastModTime.IsZero() {
				ms.logger.Info("Config file changed, reloading...")
				go ms.loadConfigAndNotify()
			}
			lastModTime = modTime

		case <-ms.fileWatcher:
			ms.logger.Info("Config reload triggered manually")
			go ms.loadConfigAndNotify()
		}
	}
}

// loadConfigAndNotify reloads config and pushes updates to all agents.
func (ms *MasterServer) loadConfigAndNotify() {
	if err := ms.loadConfig(); err != nil {
		ms.logger.Error(fmt.Sprintf("Failed to reload config: %v", err))
		return
	}
	ms.notifyAllAgents()
}

// notifyAllAgents distributes the current ruleset to every connected agent.
func (ms *MasterServer) notifyAllAgents() {
	agentIPs := ms.server.GetAgentIPs() // agentID → remoteIP
	for agentID := range agentIPs {
		ms.distributeRulesToAgent(agentID)
	}
}

// distributeRulesToAgent builds a per-agent rule set and sends a CONFIG_UPDATE.
//
// Distribution logic (v0.3.0):
//   - Rules with Source=="" (simple format): sent to ALL agents with Role="connect".
//   - Rules with Source!="" (extended format):
//     · If agent IP matches Source → sent with Role="connect".
//     · If agent IP matches Target(Dest) → sent with Role="listen".
//     · Otherwise the rule is not sent to that agent.
func (ms *MasterServer) distributeRulesToAgent(agentID string) {
	agentIPs := ms.server.GetAgentIPs()
	agentIP := agentIPs[agentID]

	ms.ruleMu.RLock()
	allRules := ms.rules
	ttl := ms.cfg.TTL
	ms.ruleMu.RUnlock()

	var agentRules []*comm.TrafficRule

	for _, rule := range allRules {
		if rule.Source == "" {
			// Simple format — every agent acts as traffic generator.
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
			// Extended format — route by IP.
			if agentIP == rule.Source {
				agentRules = append(agentRules, &comm.TrafficRule{
					Protocol: rule.Protocol,
					Source:   rule.Source,
					Target:   rule.Target,
					Port:     rule.Port,
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
			// Agents with neither SOURCE nor DEST IP receive nothing for this rule.
		}
	}

	if len(agentRules) == 0 {
		ms.logger.Debug(fmt.Sprintf("No rules applicable to agent %s (IP=%s)", agentID, agentIP))
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
		ms.logger.Error(fmt.Sprintf("Failed to send config to agent %s: %v", agentID, err))
	} else {
		ms.logger.Info(fmt.Sprintf("Sent %d rules to agent %s (TTL=%ds)", len(agentRules), agentID, ttl))
	}
}

// Start starts the master server and blocks until a shutdown signal is received.
func (ms *MasterServer) Start() error {
	ms.logger.Info(fmt.Sprintf("Starting Traffic Orchestrator Master v%s", version))
	ms.logger.Info(fmt.Sprintf("Listening on port %d with PSK authentication", ms.cfg.Port))
	ms.logger.Info(fmt.Sprintf("Config file: %s (%d rules loaded)", ms.configPath, len(ms.rules)))

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	ms.logger.Info("Shutdown signal received")

	return ms.Stop(ms.logger)
}

// Stop gracefully shuts down the master server.
func (ms *MasterServer) Stop(logger *logging.Logger) error {
	logger.Info("Shutting down Master server...")
	ms.server.CloseAllAgents()
	return nil
}

// GetConfigPath returns the current configuration file path.
func (ms *MasterServer) GetConfigPath() string {
	return ms.configPath
}

// ReloadConfig manually triggers a config reload.
func (ms *MasterServer) ReloadConfig() error {
	ms.fileWatcher <- struct{}{}
	return nil
}
