# Traffic Orchestrator — Technical Architecture

**Version:** 0.1 (Draft)  
**Author:** Claudia (Lead Architect)  
**Date:** 2026-03-10  
**Status:** 🚧 Initial Concept — Pending Review

---

## Executive Summary

Traffic Orchestrator (TO) is a lightweight, single-binary system for orchestrating network traffic flows in demo/lab environments (up to 100 hosts). It operates on a Master-Agent architecture where:

- **Master** reads traffic configuration and orchestrates agents
- **Agents** execute traffic generation tasks (TCP/UDP connections)
- **Communication** happens over a PSK-secured control channel

**Key Design Principles:**
- Single binary for both Master and Agent roles ✅
- No dependencies, no installation process ✅
- Flat-file configuration (human-readable) ✅
- Security by default (PSK, input validation, TLS) ✅

---

## 1. System Architecture

### 1.1 High-Level Component Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                        MASTER NODE                          │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  Traffic Orchestrator Binary (--master mode)         │  │
│  ├──────────────────────────────────────────────────────┤  │
│  │  • Config Parser (traffic_list.conf)                 │  │
│  │  • Agent Registry (track connected agents)           │  │
│  │  • Command Dispatcher (send instructions to agents)  │  │
│  │  • File Watcher (monitor config changes)             │  │
│  └──────────────────────────────────────────────────────┘  │
│                            ↓                                │
│                   Control Channel (PSK-secured)             │
│                            ↓                                │
└─────────────────────────────────────────────────────────────┘
                             ↓
        ┌────────────────────┼────────────────────┐
        ↓                    ↓                    ↓
┌───────────────┐   ┌───────────────┐   ┌───────────────┐
│  AGENT 1      │   │  AGENT 2      │   │  AGENT 3      │
│  (192.168.1.1)│   │  (192.168.1.2)│   │  (192.168.1.3)│
├───────────────┤   ├───────────────┤   ├───────────────┤
│ • Listener    │   │ • Listener    │   │ • Listener    │
│ • Connector   │   │ • Connector   │   │ • Connector   │
│ • Reporter    │   │ • Reporter    │   │ • Reporter    │
└───────────────┘   └───────────────┘   └───────────────┘
        ↓                    ↓                    ↓
     Data Plane (Direct TCP/UDP connections between agents)
```

### 1.2 Communication Flow Example

**Scenario:** Master wants Agent A to connect to Agent B via TCP port 443

```
1. Master → Agent B: "LISTEN tcp://0.0.0.0:443 timeout=30s"
2. Agent B → Master: "ACK: Listening on tcp://0.0.0.0:443"
3. Master → Agent A: "CONNECT tcp://192.168.1.2:443 count=5 interval=3s"
4. Agent A → Master: "ACK: Starting connections"
5. Agent A ↔ Agent B: [Data Plane] 5x TCP Handshake + Teardown
6. Agent A → Master: "DONE: 5/5 successful"
7. Master → Agent B: "STOP tcp://0.0.0.0:443"
```

---

## 2. Tech Stack Decisions

### 2.1 Language: **Go**

**Rationale:**
- ✅ Single binary compilation (no runtime dependencies)
- ✅ Excellent networking libraries (`net`, `tcp`, `udp`)
- ✅ Cross-platform support (Linux, Windows)
- ✅ Built-in concurrency (goroutines for handling multiple agents)
- ✅ Strong standard library for crypto/TLS

**Alternative Considered:** Rust (rejected due to steeper learning curve for maintainability)

### 2.2 Key Go Packages

| Package | Purpose |
|---------|---------|
| `net` | TCP/UDP networking (both control channel and data plane) |
| `crypto/tls` | Secure control channel (TLS 1.3 with PSK) |
| `encoding/json` | Command/response serialization (Master ↔ Agent) |
| `bufio` | Line-by-line config file parsing |
| `fsnotify/fsnotify` | File watcher for config hot-reload |
| `spf13/cobra` | CLI argument parsing (--master, --port, --psk) |
| `rs/zerolog` | Structured logging |
| `sync` | Thread-safe state management (RWMutex, atomic) |

### 2.3 Configuration Format Decision

**Config File Format:** ✅ **FLAT FILE** (as per original spec)

**Rationale:**
- ✅ Keeps spec simplicity (no parser complexity)
- ✅ Easy manual editing (vi/nano friendly)
- ✅ Familiar to operators (INI-like syntax)
- ✅ No external dependencies

**Parser Strategy:**
```go
// Line-by-line parsing
// 1. Parse target definitions: TARGET1=192.168.1.100
// 2. Skip comment lines (starting with #)
// 3. Parse flow lines: TCP  TARGET1  445  5  loop
```

**Example Config (from spec):**
```
Target1=192.168.1.100
Target2=192.168.1.102
Target3=192.168.1.103

# --- Fileserver (TARGET1) ---
TCP     TARGET1     445     5       loop    # SMB
TCP     TARGET1     139     10      loop    # NetBIOS

# --- Webserver (TARGET2) ---
TCP     TARGET2     80      3       loop    # HTTP
TCP     TARGET2     443     4       loop    # HTTPS

# --- DNS (TARGET3) ---
UDP     TARGET3     53      2       loop    # DNS
```

**Validation Rules:**
- Target names must match regex: `^TARGET\d+$` or valid IPv4/IPv6
- Protocol must be `TCP` or `UDP`
- Port must be 1-65535
- Interval must be positive integer (seconds)
- Count must be positive integer or literal `loop`

---

## 3. Data Models

### 3.1 Core Types (Go Structs)

```go
// Config represents the traffic configuration file
type Config struct {
    Targets map[string]string  // TARGET1 -> IP mapping
    Flows   []Flow
}

// Flow represents a single traffic generation task
type Flow struct {
    ID       string        // Auto-generated (flow-001, flow-002, ...)
    Protocol string        // "TCP" or "UDP"
    Target   string        // TARGET1 or direct IP
    Port     int           // 1-65535
    Interval time.Duration // Seconds between connections
    Count    int           // -1 for loop, else positive integer
}

// Agent represents a connected agent node
type Agent struct {
    ID         string    // Unique ID (hostname or IP)
    Address    string    // IP:Port of agent
    LastSeen   time.Time // For health checks
    Status     string    // "connected", "busy", "offline"
}

// Command is sent from Master to Agent
type Command struct {
    Type    string            `json:"type"`    // "LISTEN", "CONNECT", "STOP"
    Payload map[string]string `json:"payload"` // Protocol, Port, Target, etc.
}

// Response is sent from Agent to Master
type Response struct {
    Status  string `json:"status"`  // "ACK", "ERROR", "DONE"
    Message string `json:"message"`
}
```

### 3.2 State Management

**Master State:**
- Registry of connected agents (`map[string]*Agent`)
- Active flows (`map[string]*ActiveFlow`)
- Config watcher goroutine

**Agent State:**
- Active listeners (`map[int]*Listener`) — Port → Listener
- Active connections (`[]*Connection`)
- Connection to Master (persistent TCP/TLS)

---

## 4. Security Architecture

### 4.1 Authentication & Encryption

**Control Channel Security:**
- **TLS 1.3** with PSK (Pre-Shared Key)
- PSK is passed via `--psk` flag (validated length ≥32 chars)
- No hardcoded keys — must be provided at runtime

**Implementation:**
```go
// Master side
tlsConfig := &tls.Config{
    MinVersion: tls.VersionTLS13,
    CipherSuites: []uint16{
        tls.TLS_AES_256_GCM_SHA384,
    },
    GetConfigForClient: func(chi *tls.ClientHelloInfo) (*tls.Config, error) {
        return validatePSK(chi, psk), nil
    },
}
```

**Agent Authentication:**
- Agents must provide valid PSK on first connection
- Master validates PSK before registering agent
- Invalid PSK → connection closed immediately

### 4.2 Input Validation

**All external inputs are validated:**

| Input Source | Validation Rules |
|--------------|-----------------|
| Config File | Schema validation (YAML structure), Port range (1-65535), Target IPs (valid IPv4/IPv6) |
| CLI Args | `--master` must be valid hostname/IP, `--port` must be 1-65535, `--psk` minimum length 32 chars |
| Network Commands | JSON schema validation, Protocol must be "TCP" or "UDP", No path traversal in payload |

**Example Validation:**
```go
func validatePort(port int) error {
    if port < 1 || port > 65535 {
        return fmt.Errorf("invalid port %d: must be 1-65535", port)
    }
    return nil
}

func validateProtocol(proto string) error {
    if proto != "TCP" && proto != "UDP" {
        return fmt.Errorf("invalid protocol %s: must be TCP or UDP", proto)
    }
    return nil
}
```

### 4.3 No Hardcoded Secrets

**PSK Distribution Strategy:**
- PSK is **never** stored in code or config files
- Must be passed via CLI flag: `--psk <key>`
- Example: `./trafficorch --master 192.168.1.1 --port 9000 --psk $(cat psk.secret)`

**🤔 OPEN QUESTION FOR REVIEW:**
- Should we support PSK from environment variable (`TO_PSK`) as fallback?
- Kathy: Security implications?

### 4.4 Safe File Operations

**Config File Handling:**
- Canonicalize paths (resolve symlinks, remove `..`)
- Reject any path containing `..` (no traversal)
- Only read from designated config directory

```go
func loadConfig(path string) (*Config, error) {
    // Canonicalize path
    absPath, err := filepath.Abs(path)
    if err != nil {
        return nil, fmt.Errorf("invalid config path: %w", err)
    }
    
    // Reject traversal
    if strings.Contains(absPath, "..") {
        return nil, fmt.Errorf("path traversal detected: %s", path)
    }
    
    // Read and parse
    data, err := os.ReadFile(absPath)
    if err != nil {
        return nil, fmt.Errorf("failed to read config: %w", err)
    }
    
    // Validate YAML structure
    var cfg Config
    if err := yaml.Unmarshal(data, &cfg); err != nil {
        return nil, fmt.Errorf("invalid YAML: %w", err)
    }
    
    return &cfg, nil
}
```

---

## 5. Error Handling Strategy

### 5.1 Typed Errors

**Go Error Wrapping:**
```go
var (
    ErrInvalidConfig   = errors.New("invalid configuration")
    ErrAgentOffline    = errors.New("agent is offline")
    ErrConnectionFailed = errors.New("connection failed")
    ErrTimeout         = errors.New("operation timeout")
)

// Example usage
if err := validateConfig(cfg); err != nil {
    return fmt.Errorf("%w: missing required field 'targets'", ErrInvalidConfig)
}
```

### 5.2 User-Facing Error Messages

**Actionable Errors:**
- ❌ Bad: `"error: nil pointer dereference"`
- ✅ Good: `"Failed to start listener on port 443: port already in use. Please choose a different port or stop the conflicting service."`

**Implementation:**
```go
func startListener(port int) error {
    ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
    if err != nil {
        if strings.Contains(err.Error(), "bind: address already in use") {
            return fmt.Errorf("port %d is already in use — stop the conflicting service or use a different port", port)
        }
        return fmt.Errorf("failed to start listener on port %d: %w", port, err)
    }
    // ...
}
```

### 5.3 Fail-Fast on Invalid Config

**Startup Validation:**
- Config is validated **before** starting Master or Agent
- Invalid config → log clear error + exit with code 1
- **Never** silently fall back to defaults

```go
func main() {
    cfg, err := loadConfig(*configPath)
    if err != nil {
        log.Fatal().Err(err).Msg("Invalid configuration — cannot start")
        os.Exit(1)
    }
    
    if err := validateConfig(cfg); err != nil {
        log.Fatal().Err(err).Msg("Configuration validation failed")
        os.Exit(1)
    }
    
    // Proceed only if config is valid
    // ...
}
```

---

## 6. Master Logic

### 6.1 Startup Sequence

```
1. Parse CLI args (--master, --port, --psk)
2. Load traffic config (traffic_list.yaml)
3. Validate config (targets, flows, ports)
4. Start control channel listener (TLS with PSK)
5. Start file watcher for config hot-reload
6. Wait for agents to register
7. Begin orchestrating traffic flows
```

### 6.2 Agent Registration

**Flow:**
```
Agent → Master: HELLO {id: "agent-01", version: "1.0"}
Master: Validate PSK
Master → Agent: WELCOME {registered: true}
Master: Add agent to registry
```

**Health Checks:**
- Master sends periodic `PING` to all agents (every 30s)
- Agent must respond with `PONG` within 5s
- No response → mark agent as offline (remove from active flows)

### 6.3 Flow Orchestration

**For each flow in config:**
1. Resolve target (TARGET1 → 192.168.1.100)
2. Find available agent for target IP (or any agent if random traffic)
3. Send `LISTEN` command to target agent
4. Wait for `ACK` from target agent
5. Send `CONNECT` command to source agent(s)
6. Monitor completion (`DONE` responses)
7. Send `STOP` command to target agent

**Concurrency:**
- Each flow runs in a separate goroutine
- Synchronization via channels and mutexes

### 6.4 Config Hot-Reload

**Implementation:**
```go
watcher, _ := fsnotify.NewWatcher()
watcher.Add(*configPath)

go func() {
    for {
        select {
        case event := <-watcher.Events:
            if event.Op&fsnotify.Write == fsnotify.Write {
                log.Info().Msg("Config file changed — reloading...")
                newCfg, err := loadConfig(*configPath)
                if err != nil {
                    log.Error().Err(err).Msg("Failed to reload config")
                    continue
                }
                applyNewConfig(newCfg)
            }
        }
    }
}()
```

**Config Change Strategy:**
- Stop old flows gracefully (finish ongoing connections)
- Start new flows from updated config
- Do **not** restart the binary

---

## 7. Agent Logic

### 7.1 Startup Sequence

```
1. Parse CLI args (--master, --port, --psk)
2. Establish TLS connection to Master
3. Send HELLO message (register)
4. Wait for commands from Master
5. Execute commands (LISTEN, CONNECT, STOP)
6. Report results back to Master
```

### 7.2 Command Execution

**LISTEN Command:**
```go
func handleListenCommand(cmd Command) error {
    port := cmd.Payload["port"]
    protocol := cmd.Payload["protocol"]
    
    ln, err := net.Listen(protocol, fmt.Sprintf(":%s", port))
    if err != nil {
        return err
    }
    
    // Store listener (so we can stop it later)
    activeListeners[port] = ln
    
    // Accept connections in background
    go func() {
        for {
            conn, err := ln.Accept()
            if err != nil {
                break  // Listener closed
            }
            conn.Close()  // Immediately tear down (per spec)
        }
    }()
    
    return nil
}
```

**CONNECT Command:**
```go
func handleConnectCommand(cmd Command) error {
    target := cmd.Payload["target"]
    port := cmd.Payload["port"]
    count := parseCount(cmd.Payload["count"])  // "loop" → -1, else integer
    interval := parseDuration(cmd.Payload["interval"])
    
    successCount := 0
    for i := 0; count == -1 || i < count; i++ {
        conn, err := net.Dial("tcp", fmt.Sprintf("%s:%s", target, port))
        if err != nil {
            log.Error().Err(err).Msg("Connection failed")
            continue
        }
        conn.Close()  // Tear down immediately
        successCount++
        
        time.Sleep(interval)
    }
    
    return reportDone(successCount, count)
}
```

**STOP Command:**
```go
func handleStopCommand(cmd Command) error {
    port := cmd.Payload["port"]
    
    ln, exists := activeListeners[port]
    if !exists {
        return fmt.Errorf("no listener on port %s", port)
    }
    
    ln.Close()
    delete(activeListeners, port)
    
    return nil
}
```

---

## 8. Network Protocol Design

### 8.1 Control Channel Message Format

**JSON over TLS:**
```json
{
  "type": "COMMAND",
  "id": "cmd-12345",
  "payload": {
    "action": "LISTEN",
    "protocol": "TCP",
    "port": "443",
    "timeout": "30s"
  }
}
```

**Response:**
```json
{
  "type": "RESPONSE",
  "id": "cmd-12345",
  "status": "ACK",
  "message": "Listening on TCP port 443"
}
```

### 8.2 Timeout Handling

**Master-side:**
- Wait max 10s for agent `ACK` on commands
- Timeout → log warning, mark command as failed

**Agent-side:**
- `LISTEN` commands have optional timeout (default: infinite)
- After timeout, automatically close listener

---

## 9. Testing Strategy

### 9.1 Unit Tests

**Coverage:**
- Config parsing and validation
- Command serialization/deserialization
- Error handling (invalid inputs, network failures)

**Example Test:**
```go
func TestValidateConfig(t *testing.T) {
    tests := []struct {
        name    string
        config  Config
        wantErr bool
    }{
        {
            name: "valid config",
            config: Config{
                Targets: map[string]string{"TARGET1": "192.168.1.1"},
                Flows: []Flow{{Protocol: "TCP", Port: 443}},
            },
            wantErr: false,
        },
        {
            name: "invalid port",
            config: Config{
                Flows: []Flow{{Protocol: "TCP", Port: 99999}},
            },
            wantErr: true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := validateConfig(&tt.config)
            if (err != nil) != tt.wantErr {
                t.Errorf("validateConfig() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

### 9.2 Integration Tests

**Scenarios:**
- Master-Agent handshake with valid/invalid PSK
- LISTEN → CONNECT → STOP flow
- Config hot-reload without restart
- Agent health check (PING/PONG)

**Mock Network:**
- Use Go's `net.Pipe()` for in-memory TCP connections
- Mock file system for config watcher tests

---

## 10. Deployment & Operations

### 10.1 Binary Structure

**Single Binary with Dual Modes:**
```bash
# Master mode
./trafficorch --mode=master --config=traffic_list.yaml --port=9000 --psk=<secret>

# Agent mode (auto-detected if no --mode=master)
./trafficorch --master=192.168.1.1 --port=9000 --psk=<secret>
```

### 10.2 Logging

**Structured Logging with `zerolog`:**
```go
log.Info().
    Str("agent_id", "agent-01").
    Str("flow", "SMB to Fileserver").
    Msg("Flow orchestration started")

log.Error().
    Err(err).
    Int("port", 443).
    Msg("Failed to start listener")
```

**Log Levels:**
- `DEBUG`: Low-level protocol details (command payloads)
- `INFO`: Flow start/stop, agent registration
- `WARN`: Retries, timeouts
- `ERROR`: Failed connections, config errors

### 10.3 Service Deployment (Linux)

**Systemd Unit Example:**
```ini
[Unit]
Description=Traffic Orchestrator Agent
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/trafficorch --master=192.168.1.1 --port=9000 --psk-file=/etc/trafficorch/psk.secret
Restart=always

[Install]
WantedBy=multi-user.target
```

---

## 11. Open Questions & TODOs

### 🤔 Questions — RESOLVED ✅

1. **Config Format:** ✅ **FLAT FILE** (as per spec)
   - Decision: Keep original flat-file format
   - No migration to YAML

2. **PSK Distribution:** ✅ **BOTH** (CLI + env var)
   - CLI flag: `--psk <key>`
   - Env var fallback: `TO_PSK`

3. **Data Plane Encryption:** ✅ **OPTIONAL** (not required, but doesn't hurt)
   - Agent-to-Agent traffic can be plaintext
   - Optional: Add TLS support for data plane in future version

4. **Target Resolution:** ✅ **TARGET1 is a specific agent**
   - `TARGET1=192.168.1.100` means "the agent running on 192.168.1.100"
   - Master must ensure that specific agent is online before orchestrating flow

5. **Loop Termination:** 🚧 **TO BE DESIGNED** (see below)

---

### 🔄 Loop Termination Strategy (Detailed Design)

**Problem:** Flows with `count: loop` run indefinitely — how to stop them gracefully?

**Solution: Multi-Level Shutdown Strategy**

#### Level 1: SIGTERM Handler (Graceful Binary Shutdown)
```go
// Catch SIGTERM/SIGINT
sigChan := make(chan os.Signal, 1)
signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

go func() {
    <-sigChan
    log.Info().Msg("Shutdown signal received — stopping all flows...")
    
    // Master: Send STOP_ALL to all agents
    // Agent: Stop all active loops + listeners
    gracefulShutdown()
    
    os.Exit(0)
}()
```

**Use Case:** Admin runs `systemctl stop trafficorch` or `Ctrl+C`

---

#### Level 2: Master Command (Selective Flow Control)

**New Commands:**

| Command | Description |
|---------|-------------|
| `STOP_FLOW <flow_id>` | Stop a specific flow by ID |
| `STOP_ALL` | Stop all active flows on an agent |
| `PAUSE_FLOW <flow_id>` | Pause a flow (can be resumed later) |
| `RESUME_FLOW <flow_id>` | Resume a paused flow |

**Implementation:**
```go
// Agent side
type FlowController struct {
    flows map[string]*ActiveFlow  // flow_id → flow state
    mu    sync.RWMutex
}

func (fc *FlowController) StopFlow(flowID string) error {
    fc.mu.Lock()
    defer fc.mu.Unlock()
    
    flow, exists := fc.flows[flowID]
    if !exists {
        return fmt.Errorf("flow %s not found", flowID)
    }
    
    // Signal goroutine to stop
    close(flow.stopChan)
    
    // Wait for goroutine to finish (max 5s)
    select {
    case <-flow.doneChan:
        log.Info().Str("flow_id", flowID).Msg("Flow stopped gracefully")
    case <-time.After(5 * time.Second):
        log.Warn().Str("flow_id", flowID).Msg("Flow did not stop in time — force kill")
    }
    
    delete(fc.flows, flowID)
    return nil
}
```

**Use Case:** Master wants to stop a single flow without restarting everything

---

#### Level 3: Config Hot-Reload (Remove Flow from Config)

**Behavior:**
- Admin removes a flow from `traffic_list.conf`
- File watcher detects change
- Master sends `STOP_FLOW` to affected agents
- Flow terminates gracefully

**Implementation:**
```go
func applyConfigChange(oldCfg, newCfg *Config) {
    // Find removed flows
    for _, oldFlow := range oldCfg.Flows {
        if !existsInConfig(oldFlow, newCfg) {
            log.Info().Str("flow", oldFlow.Name).Msg("Flow removed from config — stopping...")
            stopFlow(oldFlow.ID)
        }
    }
    
    // Add new flows
    for _, newFlow := range newCfg.Flows {
        if !existsInConfig(newFlow, oldCfg) {
            log.Info().Str("flow", newFlow.Name).Msg("New flow detected — starting...")
            startFlow(newFlow)
        }
    }
}
```

**Use Case:** Operator edits config file, flow stops automatically (no manual intervention)

---

#### Level 4: Master CLI Interface (Optional, Future)

**Interactive Commands:**
```bash
# Start master in interactive mode
$ ./trafficorch --mode=master --config=traffic_list.conf --interactive

> list flows
ID        Name                  Status    Source      Target      Count
flow-001  SMB to Fileserver     running   agent-02    TARGET1     loop
flow-002  DNS Queries           running   agent-03    TARGET3     loop

> stop flow-001
Stopping flow-001... Done.

> stop all
Stopping all flows... Done.
```

**Implementation:** Simple REPL with `bufio.Scanner`

**Use Case:** Real-time control during demos/debugging

---

### ✅ Recommended Implementation Order:

**Phase 1 (MVP):**
- ✅ SIGTERM handler (graceful shutdown)
- ✅ `STOP_ALL` command from Master

**Phase 2:**
- ✅ Config hot-reload → auto-stop removed flows
- ✅ `STOP_FLOW <id>` for selective control

**Phase 3 (Future):**
- ✅ `PAUSE_FLOW` / `RESUME_FLOW`
- ✅ Interactive CLI

---

### 🔐 Security Consideration:

**Question:** Should STOP commands require authentication?

**Answer:** YES — all Master→Agent commands use the same TLS+PSK channel, so authentication is implicit. No additional auth needed.

---

### 📊 Flow State Machine

```
┌─────────┐    START     ┌─────────┐    PAUSE     ┌────────┐
│ STOPPED │ ──────────> │ RUNNING │ ──────────> │ PAUSED │
└─────────┘              └─────────┘              └────────┘
     ↑                        │                        │
     │         STOP           │         RESUME         │
     └────────────────────────┴────────────────────────┘
```

**States:**
- `STOPPED`: Flow is not active
- `RUNNING`: Flow is generating traffic
- `PAUSED`: Flow is idle, can be resumed

---

### 🎯 Final Recommendation:

**For MVP (v1.0):**
1. **SIGTERM handler** — Catches Ctrl+C and `systemctl stop`
2. **STOP_ALL command** — Master can stop all flows on an agent
3. **Config hot-reload** — Removing a flow from config stops it automatically

**This covers 90% of use cases!** 🚀

Other shutdown methods (PAUSE/RESUME, interactive CLI) can be added in v1.1+.

---

## 12. Next Steps

### Before Coding:

1. ✅ **Review this document** with Kathy (QA perspective)
2. ✅ **Resolve open questions** with Volker
3. ✅ **Finalize tech stack** (confirm Go, package choices)
4. ✅ **Define MVP scope** (which features for v1.0?)

### Implementation Order:

**Phase 1: Core Infrastructure**
- [ ] CLI argument parsing (`cobra`)
- [ ] Config parsing and validation
- [ ] TLS control channel (PSK-based)
- [ ] Basic Master-Agent communication

**Phase 2: Traffic Orchestration**
- [ ] Master: Flow orchestration logic
- [ ] Agent: LISTEN/CONNECT/STOP command handlers
- [ ] Agent registry and health checks

**Phase 3: Advanced Features**
- [ ] Config hot-reload (fsnotify)
- [ ] Structured logging (zerolog)
- [ ] Graceful shutdown (SIGTERM)

**Phase 4: Testing & Deployment**
- [ ] Unit tests (80%+ coverage)
- [ ] Integration tests (E2E scenarios)
- [ ] README and documentation
- [ ] Binary packaging (Linux, Windows)

---

## 13. Conclusion

This architecture provides a **secure, scalable, and maintainable** foundation for the Traffic Orchestrator project. Key highlights:

✅ **Single binary** with dual-mode operation (Master/Agent)  
✅ **PSK-secured TLS** for control channel  
✅ **Flat-file config** with hot-reload support  
✅ **Robust error handling** and input validation  
✅ **Clear separation** of control plane (Master ↔ Agent) and data plane (Agent ↔ Agent)  

**Ready for review by Kathy and Volker!** 🚀

---

**Document Status:** 🟡 Draft — Awaiting feedback  
**Next Reviewer:** Kathy (QA/Security perspective)  
**After Review:** Present to Volker for final approval
