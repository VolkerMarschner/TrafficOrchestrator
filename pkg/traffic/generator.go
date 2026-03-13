// Package traffic implements network traffic generation logic.
package traffic

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"time"

	"trafficorch/pkg/config"
)

// payloadSize is the number of random bytes sent in each TCP/UDP packet.
const payloadSize = 64

// errUnsupportedProtocol is a permanent (non-retriable) error for unknown protocols.
var errUnsupportedProtocol = errors.New("unsupported protocol")

// Generator handles creating network connections according to rules.
type Generator struct {
	rules []*config.TrafficRule
}

// NewGenerator creates a new Traffic Generator with the given rules.
func NewGenerator(rules []*config.TrafficRule) *Generator {
	return &Generator{rules: rules}
}

// GenerateTraffic executes traffic generation for all rules.
// This is a blocking call that runs until all connections complete.
func (g *Generator) GenerateTraffic() error {
	if len(g.rules) == 0 {
		return fmt.Errorf("no traffic rules configured")
	}

	var wg sync.WaitGroup

	for _, rule := range g.rules {
		wg.Add(1)
		go func(r *config.TrafficRule) {
			defer wg.Done()
			g.executeRule(r)
		}(rule)
	}

	wg.Wait()

	return nil
}

// executeRule executes a single traffic rule.
func (g *Generator) executeRule(rule *config.TrafficRule) {
	connCount := 0

	for {
		err := g.executeSingleConnection(rule)
		if err != nil {
			// Permanent failure (e.g. unsupported protocol) — exit immediately.
			if errors.Is(err, errUnsupportedProtocol) {
				log.Printf("traffic: rule %s: %v — skipping", rule.Name, err)
				return
			}
			log.Printf("traffic: error creating connection for rule %s: %v", rule.Name, err)
		} else {
			connCount++
		}

		// Check if we should stop.
		if rule.Count > 0 && connCount >= rule.Count {
			break
		}

		// Wait before next connection.
		if rule.Interval > 0 {
			time.Sleep(time.Duration(rule.Interval) * time.Second)
		} else {
			time.Sleep(100 * time.Millisecond)
		}
	}

	log.Printf("traffic: rule %s completed: %d connections", rule.Name, connCount)
}

// executeSingleConnection creates a single connection and sends a random payload.
//
// Bug fix (v0.3.0):
//   - TCP: after dialling, write a random payload so the connection is visible
//     to network monitoring tools and the remote listener receives data.
//   - UDP: net.DialTimeout on UDP only creates a connected socket; no packet is
//     sent until Write() is called. We now call Write(), which triggers the
//     actual UDP datagram transmission.
func (g *Generator) executeSingleConnection(rule *config.TrafficRule) error {
	address := net.JoinHostPort(rule.Target, strconv.Itoa(rule.Port))

	payload := randomPayload(payloadSize)

	switch rule.Protocol {
	case "TCP":
		conn, err := net.DialTimeout("tcp", address, 5*time.Second)
		if err != nil {
			return fmt.Errorf("failed to connect TCP %s: %w", address, err)
		}
		defer conn.Close()

		conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if _, werr := conn.Write(payload); werr != nil {
			log.Printf("traffic: TCP write to %s failed: %v", address, werr)
		}

		log.Printf("traffic: TCP connection established to %s (%d bytes sent)", address, len(payload))
		time.Sleep(10 * time.Millisecond)

	case "UDP":
		conn, err := net.DialTimeout("udp", address, 5*time.Second)
		if err != nil {
			return fmt.Errorf("failed to open UDP socket to %s: %w", address, err)
		}
		defer conn.Close()

		// Without Write(), no UDP datagram is ever transmitted.
		conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if _, werr := conn.Write(payload); werr != nil {
			return fmt.Errorf("failed to send UDP datagram to %s: %w", address, werr)
		}

		log.Printf("traffic: UDP datagram sent to %s (%d bytes)", address, len(payload))

	default:
		return fmt.Errorf("%w: %s", errUnsupportedProtocol, rule.Protocol)
	}

	return nil
}

// randomPayload returns a slice of n pseudo-random printable ASCII bytes.
func randomPayload(n int) []byte {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return b
}
