package traffic

import (
	"net"
	"strconv"
	"testing"
	"time"

	"trafficorch/pkg/config"
)

// listenPort starts a TCP listener on a random local port, accepts connections until closed,
// and returns the port number plus a cleanup function.
func listenPort(t *testing.T, proto string) int {
	t.Helper()
	switch proto {
	case "tcp":
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("net.Listen tcp: %v", err)
		}
		go func() {
			for {
				conn, err := ln.Accept()
				if err != nil {
					return
				}
				conn.Close()
			}
		}()
		t.Cleanup(func() { ln.Close() })
		_, portStr, _ := net.SplitHostPort(ln.Addr().String())
		port, _ := strconv.Atoi(portStr)
		return port
	case "udp":
		pc, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("net.ListenPacket udp: %v", err)
		}
		t.Cleanup(func() { pc.Close() })
		_, portStr, _ := net.SplitHostPort(pc.LocalAddr().String())
		port, _ := strconv.Atoi(portStr)
		return port
	default:
		t.Fatalf("unknown proto %q", proto)
		return 0
	}
}

func TestNewGenerator(t *testing.T) {
	rules := []*config.TrafficRule{
		{Protocol: "TCP", Target: "127.0.0.1", Port: 1234, Count: 1},
	}
	g := NewGenerator(rules)
	if g == nil {
		t.Fatal("NewGenerator returned nil")
	}
	if len(g.rules) != 1 {
		t.Errorf("expected 1 rule, got %d", len(g.rules))
	}
}

func TestGenerateTraffic_NoRules(t *testing.T) {
	g := NewGenerator(nil)
	if err := g.GenerateTraffic(); err == nil {
		t.Error("expected error for empty rules, got nil")
	}
}

func TestGenerateTraffic_TCP(t *testing.T) {
	port := listenPort(t, "tcp")
	rules := []*config.TrafficRule{
		{
			Protocol: "TCP",
			Target:   "127.0.0.1",
			Port:     port,
			Count:    2,
			Interval: 0,
			Name:     "test-tcp",
		},
	}

	g := NewGenerator(rules)
	done := make(chan error, 1)
	go func() { done <- g.GenerateTraffic() }()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("GenerateTraffic returned unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("GenerateTraffic (TCP) timed out")
	}
}

func TestGenerateTraffic_UDP(t *testing.T) {
	port := listenPort(t, "udp")
	rules := []*config.TrafficRule{
		{
			Protocol: "UDP",
			Target:   "127.0.0.1",
			Port:     port,
			Count:    1,
			Interval: 0,
			Name:     "test-udp",
		},
	}

	g := NewGenerator(rules)
	done := make(chan error, 1)
	go func() { done <- g.GenerateTraffic() }()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("GenerateTraffic (UDP) returned unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("GenerateTraffic (UDP) timed out")
	}
}

func TestGenerateTraffic_UnsupportedProtocol(t *testing.T) {
	rules := []*config.TrafficRule{
		{Protocol: "ICMP", Target: "127.0.0.1", Port: 80, Count: 1},
	}
	g := NewGenerator(rules)
	// Top-level returns nil; unsupported-protocol errors are reported per-connection.
	// Verify the call completes without hanging.
	done := make(chan error, 1)
	go func() { done <- g.GenerateTraffic() }()

	select {
	case <-done:
		// completed — expected
	case <-time.After(3 * time.Second):
		t.Error("GenerateTraffic with unsupported protocol timed out")
	}
}

func TestGenerateTraffic_MultipleRules(t *testing.T) {
	port1 := listenPort(t, "tcp")
	port2 := listenPort(t, "tcp")

	rules := []*config.TrafficRule{
		{Protocol: "TCP", Target: "127.0.0.1", Port: port1, Count: 1, Name: "rule-a"},
		{Protocol: "TCP", Target: "127.0.0.1", Port: port2, Count: 1, Name: "rule-b"},
	}

	g := NewGenerator(rules)
	done := make(chan error, 1)
	go func() { done <- g.GenerateTraffic() }()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("GenerateTraffic (multiple rules) returned unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("GenerateTraffic (multiple rules) timed out")
	}
}
