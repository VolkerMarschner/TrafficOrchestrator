package traffic

import (
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

// portListener manages a single port listener (TCP or UDP).
type portListener struct {
	protocol string
	port     int
	stopChan chan struct{}
	once     sync.Once
}

// stop signals the listener to shut down.
func (l *portListener) stop() {
	l.once.Do(func() { close(l.stopChan) })
}

// ListenerManager manages a set of active port listeners.
// Agents use it to open TCP/UDP ports for "listen" role traffic rules.
type ListenerManager struct {
	listeners map[string]*portListener // key: "tcp:8080"
	mu        sync.Mutex
}

// NewListenerManager creates a new, empty ListenerManager.
func NewListenerManager() *ListenerManager {
	return &ListenerManager{
		listeners: make(map[string]*portListener),
	}
}

// StartListener opens a TCP or UDP listener on port.
// If a listener is already active for the same protocol:port, it is a no-op.
func (m *ListenerManager) StartListener(protocol string, port int) error {
	key := fmt.Sprintf("%s:%d", strings.ToLower(protocol), port)

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.listeners[key]; exists {
		return nil // already listening
	}

	l := &portListener{
		protocol: strings.ToUpper(protocol),
		port:     port,
		stopChan: make(chan struct{}),
	}

	var err error
	switch l.protocol {
	case "TCP":
		err = l.startTCP()
	case "UDP":
		err = l.startUDP()
	default:
		return fmt.Errorf("unsupported protocol: %s", protocol)
	}

	if err != nil {
		return err
	}

	m.listeners[key] = l
	return nil
}

// StopAll stops all active listeners.
func (m *ListenerManager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for key, l := range m.listeners {
		l.stop()
		delete(m.listeners, key)
	}
}

// StopListener stops the listener for a specific protocol:port.
func (m *ListenerManager) StopListener(protocol string, port int) {
	key := fmt.Sprintf("%s:%d", strings.ToLower(protocol), port)

	m.mu.Lock()
	defer m.mu.Unlock()

	if l, ok := m.listeners[key]; ok {
		l.stop()
		delete(m.listeners, key)
	}
}

// ─── TCP listener ─────────────────────────────────────────────────────────────

func (l *portListener) startTCP() error {
	addr := fmt.Sprintf(":%d", l.port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("cannot open TCP listener on port %d: %w", l.port, err)
	}

	log.Printf("traffic: TCP listener started on port %d", l.port)

	go func() {
		defer ln.Close()

		// Close the net.Listener when stop is requested so Accept() unblocks.
		// defer ln.Close() above ensures cleanup on any exit path. (N5)
		go func() {
			<-l.stopChan
			ln.Close()
		}()

		for {
			conn, err := ln.Accept()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					log.Printf("traffic: TCP listener on port %d stopped", l.port)
				} else {
					log.Printf("traffic: TCP listener on port %d accept error: %v", l.port, err)
				}
				return
			}
			go handleTCPConn(conn, l.port)
		}
	}()

	return nil
}

func handleTCPConn(conn net.Conn, port int) {
	defer conn.Close()

	buf := make([]byte, 1024)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, _ := conn.Read(buf)

	if n > 0 {
		log.Printf("traffic: TCP port %d received %d bytes from %s", port, n, conn.RemoteAddr())
	} else {
		log.Printf("traffic: TCP port %d: connection from %s (no payload)", port, conn.RemoteAddr())
	}
}

// ─── UDP listener ─────────────────────────────────────────────────────────────

func (l *portListener) startUDP() error {
	addr := fmt.Sprintf(":%d", l.port)
	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		return fmt.Errorf("cannot open UDP listener on port %d: %w", l.port, err)
	}

	log.Printf("traffic: UDP listener started on port %d", l.port)

	go func() {
		defer pc.Close()

		// Close the PacketConn when stop is requested so ReadFrom() unblocks.
		// defer pc.Close() above ensures cleanup on any exit path. (N5)
		go func() {
			<-l.stopChan
			pc.Close()
		}()

		buf := make([]byte, 4096)
		for {
			n, src, err := pc.ReadFrom(buf)
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					log.Printf("traffic: UDP listener on port %d stopped", l.port)
				} else {
					log.Printf("traffic: UDP listener on port %d read error: %v", l.port, err)
				}
				return
			}
			log.Printf("traffic: UDP port %d received %d bytes from %s", l.port, n, src)
		}
	}()

	return nil
}
