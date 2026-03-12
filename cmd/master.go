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

	ms.logger.Info(fmt.Sprintf("Master server initialized on port %d", cfg.Port))

	return ms, nil
}

// onAgentRegister is called when a new agent registers.
func (ms *MasterServer) onAgentRegister(agentID string, hostname string) {
	ms.logger.Info(fmt.Sprintf("New agent registered: %s (%s)", agentID, hostname))
}

// onTrafficRequest handles traffic generation requests from agents.
func (ms *MasterServer) onTrafficRequest(agentID string, rules []*comm.TrafficRule) {
	ms.ruleMu.RLock()
	currentRules := make([]*config.TrafficRule, len(ms.rules))
	copy(currentRules, ms.rules)
	ms.ruleMu.RUnlock()

	ms.logger.Info(fmt.Sprintf("Traffic request from %s: %d rules", agentID, len(rules)))

	// Execute traffic generation (this would be implemented in the future)
	if err := ms.executeTraffic(agentID, currentRules); err != nil {
		ms.logger.Error(fmt.Sprintf("Error executing traffic: %v", err))
	}
}

// loadConfig re-parses the configuration file from disk and updates the active rule set.
func (ms *MasterServer) loadConfig() error {
	freshCfg, err := config.ParseExtendedConfigV2(ms.configPath)
	if err != nil {
		return fmt.Errorf("failed to parse config file %q: %w", ms.configPath, err)
	}

	ms.ruleMu.Lock()
	ms.rules = freshCfg.TrafficRules
	ms.ruleMu.Unlock()

	ms.logger.Info(fmt.Sprintf("Loaded %d traffic rules from %s", len(freshCfg.TrafficRules), ms.configPath))
	if len(freshCfg.TargetMap) > 0 {
		ms.logger.Debug(fmt.Sprintf("Target map reloaded: %d entries", len(freshCfg.TargetMap)))
	}
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

			// Check if file was modified since last check
			if !modTime.Equal(lastModTime) && !lastModTime.IsZero() {
				ms.logger.Info("Config file changed, reloading...")
				go ms.loadConfigAndNotify()
			}

			lastModTime = modTime

		case <-ms.fileWatcher:
			// Manual trigger from external signal
			ms.logger.Info("Config reload triggered manually")
			go ms.loadConfigAndNotify()
		}
	}
}

// loadConfigAndNotify loads config and notifies all agents.
func (ms *MasterServer) loadConfigAndNotify() {
	if err := ms.loadConfig(); err != nil {
		ms.logger.Error(fmt.Sprintf("Failed to reload config: %v", err))
		return
	}

	ms.notifyAgentsOfUpdate()
}

// notifyAgentsOfUpdate sends config update to all registered agents.
func (ms *MasterServer) notifyAgentsOfUpdate() {
	ms.ruleMu.RLock()
	rules := make([]*comm.TrafficRule, len(ms.rules))
	for i, rule := range ms.rules {
		rules[i] = &comm.TrafficRule{
			Protocol: rule.Protocol,
			Target:   rule.Target,
			Port:     rule.Port,
			Interval: rule.Interval,
			Count:    rule.Count,
			Name:     rule.Name,
		}
	}
	ms.ruleMu.RUnlock()

	msg := &comm.ConfigUpdateMessage{
		BaseMessage: comm.BaseMessage{
			Type:      comm.MsgConfigUpdate,
			Timestamp: time.Now().Unix(),
			Version:   "1.0",
		},
		Rules: rules,
	}

	if err := ms.server.SendToAllAgents(msg); err != nil {
		ms.logger.Error(fmt.Sprintf("Failed to notify agents of config update: %v", err))
	}
}

// executeTraffic executes traffic generation according to the given rules.
func (ms *MasterServer) executeTraffic(agentID string, rules []*config.TrafficRule) error {
	if len(rules) == 0 {
		return fmt.Errorf("no rules provided")
	}

	ms.logger.Info(fmt.Sprintf("Executing %d traffic rules for agent %s", len(rules), agentID))

	// TODO: Implement actual traffic execution logic
	// This would spawn goroutines to create network connections

	return nil
}

// Start starts the master server and begins listening for agents.
func (ms *MasterServer) Start() error {
	ms.logger.Info(fmt.Sprintf("Starting Traffic Orchestrator Master v%s", version))
	ms.logger.Info(fmt.Sprintf("Listening on port %d with PSK authentication", ms.cfg.Port))
	ms.logger.Info(fmt.Sprintf("Config file: %s (%d rules loaded)", ms.configPath, len(ms.rules)))

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	ms.logger.Info("Shutdown signal received")

	return ms.Stop(ms.logger)
}

// Stop gracefully shuts down the master server.
func (ms *MasterServer) Stop(logger *logging.Logger) error {
	logger.Info("Shutting down Master server...")

	// Close all agent connections
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
