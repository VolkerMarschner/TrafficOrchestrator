package main

import "time"

// Logging
const (
	defaultLogMaxSizeMB = 10
	defaultLogMaxFiles  = 5
)

// Timing
const (
	connectTimeout        = 5 * time.Second
	tcpHoldDuration       = 10 * time.Millisecond
	defaultConnectionDelay = 100 * time.Millisecond
	heartbeatInterval     = 30 * time.Second
	heartbeatCheck        = 1 * time.Minute
	reconnectDelay        = 5 * time.Second
	configWatchInterval   = 5 * time.Second
)
