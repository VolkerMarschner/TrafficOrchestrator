package main

import "time"

// Logging
const (
	defaultLogMaxSizeMB = 10
	defaultLogMaxFiles  = 5

	// Log file names (v0.4.6 — split into status + traffic logs)
	masterStatusLogFile  = "master-status.log" // operational events (start/stop, agent register, config changes)
	masterTrafficLogFile = "master-traffic.log" // rule distribution events (rules sent per agent)
	agentStatusLogFile   = "agent-status.log"   // operational events (start/stop, connect, update)
	agentTrafficLogFile  = "agent-traffic.log"  // traffic execution events (connections, listeners, rules)
)

// Timing
const (
	connectTimeout         = 5 * time.Second
	tcpHoldDuration        = 10 * time.Millisecond
	defaultConnectionDelay = 100 * time.Millisecond
	heartbeatInterval      = 30 * time.Second
	heartbeatCheck         = 1 * time.Minute
	reconnectDelay         = 5 * time.Second  // delay between reconnect attempts (v0.4.7: also used by receiveMessages error path)
	masterReconnectDelay   = 30 * time.Second // longer pause between reconnect attempts when master is confirmed down
	configWatchInterval    = 5 * time.Second
)

// Distribution & registry (v0.4.5)
const (
	// distributionPort is the HTTP port on which the master serves its binary
	// for agent self-update and exposes the agent registry endpoint.
	distributionPort = 9001

	// registryFile is the JSON file written by the master to track all agents.
	registryFile = "agents.json"

	// pidFile is written by a process started with -d/--daemon.
	pidFile = "trafficorch.pid"
)
