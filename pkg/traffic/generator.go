// Package traffic implements network traffic generation logic.
package traffic

import (
	"fmt"
	"log"
	"net"
	"strconv"
	"sync"
	"time"

	"trafficorch/pkg/config"
)

// Generator handles creating network connections according to rules.
type Generator struct {
	rules []*config.TrafficRule
}

// NewGenerator creates a new Traffic Generator with the given rules.
func NewGenerator(rules []*config.TrafficRule) *Generator {
	return &Generator{rules: rules}
}

// GenerateTraffic executes traffic generation for all rules.
// This is a blocking call that runs until all connections complete or context is cancelled.
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
		if err := g.executeSingleConnection(rule); err != nil {
			log.Printf("traffic: error creating connection for rule %s: %v", rule.Name, err)
		}

		connCount++

		// Check if we should stop
		if rule.Count > 0 && connCount >= rule.Count {
			break
		}

		// Wait before next connection (if interval specified)
		if rule.Interval > 0 {
			time.Sleep(time.Duration(rule.Interval) * time.Second)
		} else {
			time.Sleep(100 * time.Millisecond) // Small delay for immediate connections
		}
	}

	log.Printf("traffic: rule %s completed: %d connections", rule.Name, connCount)
}

// executeSingleConnection creates a single connection according to the rule.
func (g *Generator) executeSingleConnection(rule *config.TrafficRule) error {
	address := net.JoinHostPort(rule.Target, strconv.Itoa(rule.Port))

	var conn net.Conn
	var err error

	switch rule.Protocol {
	case "TCP":
		conn, err = net.DialTimeout("tcp", address, 5*time.Second)
	case "UDP":
		conn, err = net.DialTimeout("udp", address, 5*time.Second)
	default:
		return fmt.Errorf("unsupported protocol: %s", rule.Protocol)
	}

	if err != nil {
		return fmt.Errorf("failed to connect %s %s: %w", rule.Protocol, address, err)
	}

	defer conn.Close()

	// For TCP: Simulate a quick connection (3-way handshake + teardown)
	if rule.Protocol == "TCP" {
		log.Printf("traffic: TCP connection established to %s", address)
		time.Sleep(10 * time.Millisecond) // Brief connection duration
	} else if rule.Protocol == "UDP" {
		log.Printf("traffic: UDP socket opened to %s", address)
		// For UDP, we just need the connection established (no handshake)
	}

	return nil
}
