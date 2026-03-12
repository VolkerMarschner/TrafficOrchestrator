package main

import (
	"encoding/json"
	"fmt"
	"net"
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
)

// Agent handles agent-specific operations.
type Agent struct {
	client       *comm.AgentClient
	agentID      string
	configPath   string
	currentRules []*config.TrafficRule
	mu           sync.RWMutex
	isRunning    int32 // accessed via sync/atomic
	logger       *logging.Logger
}

// NewAgent creates a new agent instance.
func NewAgent(cfg *config.AgentConfig, logger *logging.Logger) (*Agent, error) {
	client, err := comm.NewAgentClient(
		cfg.MasterHost,
		cfg.Port,
		cfg.PSK,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create agent client: %w", err)
	}

	hostname, _ := os.Hostname()
	platform := fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)

	logger.Info(fmt.Sprintf("Agent connecting to master at %s:%d", cfg.MasterHost, cfg.Port))
	if err := client.Register(cfg.AgentID, hostname, platform); err != nil {
		return nil, fmt.Errorf("failed to register with master: %w", err)
	}

	return &Agent{
		client:     client,
		agentID:    cfg.AgentID,
		configPath: "", // Can be set later if needed
		logger:     logger,
	}, nil
}

// Start begins the agent's main loop.
func (a *Agent) Start() error {
	a.logger.Info(fmt.Sprintf("Agent %s started", a.agentID))
	atomic.StoreInt32(&a.isRunning, 1)

	// Start receiving messages from master
	go a.receiveMessages()

	// Send initial heartbeat
	go a.sendHeartbeatLoop()

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	a.logger.Info("Shutdown signal received")

	return a.Stop()
}

// receiveMessages continuously reads messages from the master.
func (a *Agent) receiveMessages() {
	for {
		if atomic.LoadInt32(&a.isRunning) == 0 {
			break
		}

		msg, msgBytes, err := a.client.ReadMessage()
		if err != nil {
			a.logger.Error(fmt.Sprintf("Error receiving message: %v", err))
			time.Sleep(reconnectDelay)
			continue
		}

		switch msg.Type {
		case comm.MsgConfigUpdate:
			var configMsg comm.ConfigUpdateMessage
			if err := json.Unmarshal(msgBytes, &configMsg); err == nil {
				a.updateRules(configMsg.Rules)
				a.logger.Info(fmt.Sprintf("Updated to %d traffic rules", len(configMsg.Rules)))
			}

		case comm.MsgTrafficStart:
			var startMsg comm.TrafficStartMessage
			if err := json.Unmarshal(msgBytes, &startMsg); err == nil {
				a.startTraffic(startMsg.Rules)
			}

		case comm.MsgTrafficStop:
			a.stopTraffic()

		default:
			a.logger.Warn(fmt.Sprintf("Unknown message type: %s", msg.Type))
		}
	}
}

// updateRules updates the current traffic rules.
func (a *Agent) updateRules(rules []*comm.TrafficRule) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.currentRules = make([]*config.TrafficRule, len(rules))
	for i, rule := range rules {
		a.currentRules[i] = &config.TrafficRule{
			Protocol: rule.Protocol,
			Target:   rule.Target,
			Port:     rule.Port,
			Interval: rule.Interval,
			Count:    rule.Count,
			Name:     rule.Name,
		}
	}
}

// startTraffic begins traffic generation according to the given rules.
func (a *Agent) startTraffic(rules []*comm.TrafficRule) {
	if atomic.LoadInt32(&a.isRunning) != 0 {
		go a.executeTraffic(rules)
	}
}

// executeTraffic executes the actual network connections.
func (a *Agent) executeTraffic(rules []*comm.TrafficRule) {
	a.logger.Info(fmt.Sprintf("Starting traffic generation for %d rules", len(rules)))

	var wg sync.WaitGroup

	for _, rule := range rules {
		wg.Add(1)
		go func(r *comm.TrafficRule) {
			defer wg.Done()
			a.executeSingleRule(r)
		}(rule)
	}

	wg.Wait()
	a.logger.Info(fmt.Sprintf("Traffic generation completed for %d rules", len(rules)))
}

// executeSingleRule executes a single traffic rule.
func (a *Agent) executeSingleRule(rule *comm.TrafficRule) {
	address := net.JoinHostPort(rule.Target, strconv.Itoa(rule.Port))
	connCount := 0

	// Log start of rule execution
	a.logger.Info(fmt.Sprintf("Starting rule: %s (%s to %s)",
		rule.Name, rule.Protocol, address))

	for {
		var conn net.Conn
		var err error

		switch rule.Protocol {
		case "TCP":
			conn, err = net.DialTimeout("tcp", address, connectTimeout)
		case "UDP":
			conn, err = net.DialTimeout("udp", address, connectTimeout)
		default:
			a.logger.Error(fmt.Sprintf("Unsupported protocol: %s", rule.Protocol))
			return
		}

		if err != nil {
			a.logger.Warn(fmt.Sprintf("Connection FAILED to %s (%s): %v",
				address, rule.Protocol, err))
			time.Sleep(time.Duration(rule.Interval) * time.Second)
			continue
		}

		connCount++

		if rule.Protocol == "TCP" {
			a.logger.Info(fmt.Sprintf("TCP connection ESTABLISHED to %s (count: %d)",
				address, connCount))
			time.Sleep(tcpHoldDuration)
		} else if rule.Protocol == "UDP" {
			a.logger.Info(fmt.Sprintf("UDP socket opened to %s", address))
		}

		conn.Close()

		// Check if we should stop
		if rule.Count > 0 && connCount >= rule.Count {
			break
		}

		// Wait before next connection
		if rule.Interval > 0 {
			time.Sleep(time.Duration(rule.Interval) * time.Second)
		} else {
			time.Sleep(defaultConnectionDelay)
		}
	}

	a.logger.Info(fmt.Sprintf("Rule %s COMPLETED: %d connections generated",
		rule.Name, connCount))
}

// stopTraffic stops all ongoing traffic generation.
func (a *Agent) stopTraffic() {
	a.logger.Info("Stopping traffic generation...")
	// TODO: Implement actual stopping logic for active connections
}

// sendHeartbeatLoop sends periodic heartbeats to the master.
func (a *Agent) sendHeartbeatLoop() {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if atomic.LoadInt32(&a.isRunning) == 0 {
				return
			}

			cpuUsage, memUsage := a.getSystemStats()
			activeRules := 0

			a.mu.RLock()
			if a.currentRules != nil {
				activeRules = len(a.currentRules)
			}
			a.mu.RUnlock()

			if err := a.client.SendHeartbeat(cpuUsage, memUsage, activeRules); err != nil {
				a.logger.Warn(fmt.Sprintf("Failed to send heartbeat: %v", err))
			}

		case <-time.After(heartbeatCheck): // Check if still running
			if atomic.LoadInt32(&a.isRunning) == 0 {
				return
			}
		}
	}
}

// getSystemStats returns current CPU and memory usage.
func (a *Agent) getSystemStats() (float64, int64) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return 0.0, int64(memStats.Alloc) // TODO: Implement actual CPU monitoring
}

// Stop gracefully shuts down the agent.
func (a *Agent) Stop() error {
	a.logger.Info("Shutting down Agent...")
	atomic.StoreInt32(&a.isRunning, 0)

	if a.client != nil {
		return a.client.Close()
	}

	return nil
}
