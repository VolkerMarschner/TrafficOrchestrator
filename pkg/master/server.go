// Package master implements the Master server for Traffic Orchestrator.
package master

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"trafficorch/pkg/comm"
	"trafficorch/pkg/config"
)

const version = "0.1.0"

// MasterServer coordinates traffic generation across agents.
type MasterServer struct {
	cfg         *config.MasterConfig
	rules       []*config.TrafficRule
	fileWatcher chan struct{}
	ruleMu      sync.RWMutex
	server      *comm.MasterServer
}

// NewMasterServer creates a new Master server instance.
func NewMasterServer(cfg *config.MasterConfig) (*MasterServer, error) {
	ms := &MasterServer{
		cfg:         cfg,
		rules:       make([]*config.TrafficRule, 0),
		fileWatcher: make(chan struct{}, 1),
	}

	return ms, nil
}

// Start starts the master server and begins listening for agents.
func (ms *MasterServer) Start() error {
	log.Printf("master: starting Traffic Orchestrator Master v%s on port %d", version, ms.cfg.Port)

	if err := ms.loadConfig(); err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	server, err := comm.NewMasterServer(ms.cfg.PSK, ms.cfg.Port, ms.onAgentRegister, nil)
	if err != nil {
		return fmt.Errorf("failed to create master server: %w", err)
	}
	ms.server = server

	go ms.watchConfigFile()

	// Block until the process is killed (acceptLoop runs in a goroutine inside comm.NewMasterServer)
	select {}
}

// Stop stops the master server.
func (ms *MasterServer) Stop() {
	log.Println("master: stopping")
	if ms.server != nil {
		ms.server.CloseAllAgents()
	}
}

// onAgentRegister handles new agent registrations.
func (ms *MasterServer) onAgentRegister(agentID string, hostname string) {
	log.Printf("master: agent registered: %s (hostname: %s)", agentID, hostname)
	// Send current rules to the newly connected agent via broadcast
	ms.notifyAgentsOfUpdate()
}

// watchConfigFile monitors the config file for changes and reloads on modification.
func (ms *MasterServer) watchConfigFile() {
	lastModTime := time.Time{}

	for {
		select {
		case <-time.After(5 * time.Second):
			info, err := os.Stat(ms.cfg.ConfigPath)
			if err != nil {
				log.Printf("master: config file not found: %s", ms.cfg.ConfigPath)
				continue
			}

			modTime := info.ModTime()
			if !modTime.Equal(lastModTime) && !lastModTime.IsZero() {
				log.Println("master: config file changed, reloading")
				ms.loadConfigAndNotify()
			}
			lastModTime = modTime

		case <-ms.fileWatcher:
			log.Println("master: config reload triggered manually")
			ms.loadConfigAndNotify()
		}
	}
}

// loadConfig loads traffic rules from the config file.
func (ms *MasterServer) loadConfig() error {
	rules, targetMap, err := config.LoadTrafficRules(ms.cfg.ConfigPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	ms.ruleMu.Lock()
	ms.rules = rules
	ms.ruleMu.Unlock()

	log.Printf("master: loaded %d traffic rules from %s (%d targets)", len(rules), ms.cfg.ConfigPath, len(targetMap))
	return nil
}

// loadConfigAndNotify loads config and notifies all agents.
func (ms *MasterServer) loadConfigAndNotify() {
	if err := ms.loadConfig(); err != nil {
		log.Printf("master: failed to reload config: %v", err)
		return
	}
	ms.notifyAgentsOfUpdate()
}

// notifyAgentsOfUpdate broadcasts the current rule set to all connected agents.
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
		log.Printf("master: failed to notify agents of config update: %v", err)
	}
}

// GetAgentCount returns the number of currently connected agents.
func (ms *MasterServer) GetAgentCount() int {
	return len(ms.server.GetAgents())
}
