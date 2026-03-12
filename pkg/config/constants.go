// Package config provides configuration parsing and validation for Traffic Orchestrator.
package config

// defaultPort is the TCP port the master listens on when not specified in config.
const defaultPort = 9000

// maxPort is the highest valid TCP/UDP port number.
const maxPort = 65535

// countLoop signals that a traffic rule should repeat indefinitely.
const countLoop = -1
