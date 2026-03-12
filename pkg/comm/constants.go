// Package comm implements the communication protocol between Master and Agents.
package comm

import "time"

// Network timeouts.
const (
	// ConnectTimeout is the maximum time allowed for a single outgoing TCP connection.
	ConnectTimeout = 5 * time.Second

	// MasterConnectTimeout is the maximum time an agent waits to reach the master.
	MasterConnectTimeout = 10 * time.Second

	// ChannelIdleTimeout is the maximum time without a message before a channel is considered dead.
	ChannelIdleTimeout = 30 * time.Second
)

// Protocol version used in all messages.
const ProtocolVersion = "1.0"
