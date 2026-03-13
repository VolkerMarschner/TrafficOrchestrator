# Traffic Orchestrator

A lightweight, cross-platform network traffic generator for lab and demo environments.
It creates realistic Layer 3/4 flows (TCP connections, UDP datagrams) between hosts — **no payload content**, just connection-level activity.

---

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Installation](#installation)
- [Usage](#usage)
  - [Master mode](#master-mode)
  - [Agent mode](#agent-mode)
  - [Environment variables](#environment-variables)
- [Configuration file format](#configuration-file-format)
  - [Simple format](#simple-format-legacy)
  - [Extended format](#extended-format-source--dest)
- [Architecture](#architecture)
- [Project layout](#project-layout)
- [Development](#development)
- [Security](#security)
- [Troubleshooting](#troubleshooting)

---

## Overview

Traffic Orchestrator ships as **a single binary** that can run in two modes:

| Mode | Role |
|------|------|
| **Master** | Reads a traffic config file, listens for agents, distributes rules, and handles hot-reload on config changes |
| **Agent** | Connects to the master, receives rules, and generates the actual TCP/UDP connections |

Communication between master and agent is secured with a **pre-shared key (PSK)** over a length-prefixed, HMAC-SHA256-authenticated TCP channel.

---

## Quick Start

```bash
# 1. Build
make build

# 2. Copy configs/traffic-simple.conf.example → traffic.conf and fill in your IPs
cp configs/traffic-simple.conf.example traffic.conf
$EDITOR traffic.conf          # set PSK, IPs, rules

# 3. Start the master
export TRAFFICORCH_PSK=YourKey123
./trafficorch --master --config traffic.conf

# 4. On each agent host — first run (flags are saved automatically to agent.conf)
./trafficorch --agent --master 192.168.1.1 --port 9000 --psk YourKey123 --id host-a

# 5. Every subsequent start on that host — no flags needed
./trafficorch
```

---

## Installation

### Prerequisites

| Tool | Version |
|------|---------|
| Go   | 1.21+   |
| make | any     |

### Build from source

```bash
# Current platform
make build                # → ./trafficorch

# Cross-compile
make build-linux          # → ./trafficorch-linux-amd64
make build-linux-arm64    # → ./trafficorch-linux-arm64
make build-windows        # → ./trafficorch-windows-amd64.exe
make build-all            # all three at once
```

### Deploy to remote hosts

```bash
scp trafficorch-linux-amd64 user@192.168.1.100:/usr/local/bin/trafficorch
scp trafficorch-linux-amd64 user@192.168.1.101:/usr/local/bin/trafficorch
```

No installation step is needed — the binary is self-contained.

---

## Usage

Run without arguments — trafficorch first checks for an `agent.conf` in the
current directory and starts the agent automatically if found.  If no config
file exists, a short guide is printed instead.

```
./trafficorch
```

### Master mode

```
trafficorch --master --config <FILE> [--port <PORT>] [--psk <KEY>]
```

| Flag | Required | Description |
|------|----------|-------------|
| `--config FILE` | **Yes** | Path to traffic config file |
| `--port PORT`   | No  | Override the port from the config file |
| `--psk KEY`     | No  | Override the PSK from the config file (or `TRAFFICORCH_PSK` env var) |

**Example:**

```bash
trafficorch --master --config /etc/trafficorch/traffic.conf
```

The master will:
1. Load traffic rules from the config file
2. Listen for incoming agent connections on the configured port
3. Send the current ruleset to every newly registered agent
4. Watch the config file for changes and push updates to all connected agents automatically

### Agent mode

```
trafficorch --agent --master <HOST> --port <PORT> --psk <KEY> [--id <ID>]
```

| Flag | Required | Description |
|------|----------|-------------|
| `--master HOST` | **Yes** | Master hostname or IP address |
| `--port PORT`   | **Yes** | Master port |
| `--psk KEY`     | **Yes** | Pre-shared key (must match master) |
| `--id ID`       | No  | Agent identifier shown in master logs (default: `agent-unknown`) |

**Example:**

```bash
trafficorch --agent \
  --master 192.168.1.1 \
  --port 9000 \
  --psk YourKey123 \
  --id workstation-01
```

The agent will:
1. Connect and register with the master
2. Receive traffic rules
3. Generate TCP/UDP connections according to those rules
4. Send periodic heartbeats (every 30 s) with basic resource metrics

> **Tip:** When CLI flags are supplied, trafficorch saves them to `agent.conf`
> in the current directory.  On every subsequent start without arguments the
> saved configuration is loaded automatically — no need to repeat the flags.

### Auto-start via agent.conf

Starting from **v0.2.0**, trafficorch supports a persistent agent configuration
file called `agent.conf`.

| Situation | Behaviour |
|-----------|-----------|
| First run with `--agent` flags | Flags are parsed, agent starts, and `agent.conf` is written automatically |
| `--agent` with no flags | Looks for `agent.conf`; starts if found, prints help if not |
| No arguments at all | Same as above |

**agent.conf format** (auto-generated, human-editable):

```ini
# agent.conf — generated by Traffic Orchestrator on 2026-03-13 04:41:00
# Delete this file to reset to interactive startup.

MASTER=192.168.1.1
PORT=9000
PSK=YourKey123
ID=host-a
```

All four keys are supported: `MASTER`, `PORT`, `PSK`, `ID` (optional).
Inline comments (`# …`) are stripped automatically.

### Other flags

```
trafficorch --version   # print version and exit
trafficorch --help      # print full usage
```

### Environment variables

| Variable | Description | Default |
|----------|-------------|---------|
| `TRAFFICORCH_PSK` | Pre-shared key (alternative to `--psk`) | — |
| `TRAFFICORCH_LOG_DIR` | Directory for log files (`traffic.log` / `agent.log`) | `.` (current dir) |

Copy `.env.example` → `.env` and adjust values; then `source .env` before running.

---

## Configuration file format

Two formats are supported. The parser detects which one to use automatically.

### Simple format (legacy)

Best for flat lab environments where one host generates all traffic.

```
# Global settings
[MASTER]
PORT = 9000
PSK  = YourKey123

# Target definitions  (name → IP)
FILESERVER = 192.168.1.100
WEBSERVER  = 192.168.1.102
DNS_SRV    = 192.168.1.105

# Traffic rules:  PROTOCOL  TARGET     PORT  INTERVAL  COUNT  [# comment]
TCP   FILESERVER   445   5    loop   # SMB
TCP   WEBSERVER    80    3    loop   # HTTP
UDP   DNS_SRV      53    2    loop   # DNS
```

| Column | Description |
|--------|-------------|
| `PROTOCOL` | `TCP` or `UDP` |
| `TARGET` | A name from the target map, or a bare IP address |
| `PORT` | 1 – 65535 |
| `INTERVAL` | Seconds between connections (0 = fire immediately) |
| `COUNT` | Number of connections, or `loop` to repeat indefinitely |

A full template is at [`configs/traffic-simple.conf.example`](configs/traffic-simple.conf.example).

---

### Extended format (SOURCE → DEST)

Best for multi-host environments where different agents represent different source hosts.

```
[MASTER]
PORT = 9000
PSK  = YourKey123

# Target definitions
CLIENT    = 10.0.1.10
LINUX_SRV = 10.0.2.10
WIN_SRV   = 10.0.2.20

# Traffic rules:  PROTOCOL  SOURCE    DEST      PORT  COUNT  [# comment]
TCP   CLIENT   LINUX_SRV   80    loop   # HTTP
TCP   CLIENT   WIN_SRV     445   loop   # SMB
UDP   CLIENT   WIN_SRV     53    loop   # DNS
```

| Column | Description |
|--------|-------------|
| `PROTOCOL` | `TCP` or `UDP` |
| `SOURCE` | Agent host that generates traffic (name or IP) |
| `DEST` | Host that receives traffic — its port must be open/listening |
| `PORT` | 1 – 65535 |
| `COUNT` | Number of connections, or `loop` indefinitely |

A full template is at [`configs/traffic-extended.conf.example`](configs/traffic-extended.conf.example).

---

## Architecture

```
  ┌───────────────────────────────────────┐
  │               MASTER                  │
  │                                       │
  │  config file ──► rule loader          │
  │  (hot-reload)         │               │
  │                  rule broadcaster     │
  │                       │               │
  └───────────────────────┼───────────────┘
         PSK-auth TCP channel (HMAC-SHA256)
  ┌──────────────────┬────┴──────────────────┐
  │    AGENT A       │       AGENT B          │
  │                  │                        │
  │  register ───────┘                        │
  │  receive rules                            │
  │  execute traffic ──► TCP/UDP connections  │
  │  heartbeat every 30 s                     │
  └───────────────────────────────────────────┘
```

### Message flow

```
Agent                          Master
  │──── REGISTER ────────────────►│
  │◄─── REGISTER_ACK ─────────────│
  │──── HEARTBEAT (every 30 s) ──►│
  │◄─── TRAFFIC_START / CONFIG_UPDATE ──│
  │──── STATUS / ERROR ──────────►│
```

---

## Project layout

```
TrafficOrchestrator/
├── cmd/                    # Binary entry point
│   ├── main.go             # CLI parsing, mode dispatch
│   ├── master.go           # Master server wrapper (cmd layer)
│   ├── agent.go            # Agent wrapper (cmd layer)
│   └── constants.go        # Timing and defaults
│
├── pkg/
│   ├── comm/               # Master↔Agent protocol
│   │   ├── channel.go      # PSK-auth length-prefixed channel
│   │   ├── messages.go     # All message types (JSON)
│   │   └── constants.go    # Protocol timeouts and version
│   │
│   ├── config/             # Configuration parsing
│   │   ├── parser.go       # CLI arg parsing, legacy file parser
│   │   ├── parser_v2.go    # Primary config parser (extended format)
│   │   ├── parser_extended.go  # Extended SOURCE/DEST format
│   │   ├── parser_smart.go # Auto-detects format, falls back to legacy
│   │   ├── agent_conf.go   # agent.conf load/save (v0.2.0+)
│   │   └── constants.go    # Port defaults, sentinel values
│   │
│   ├── logging/            # Rotating file logger
│   │   └── logger.go
│   │
│   ├── master/             # Master server (pkg layer)
│   │   └── server.go
│   │
│   ├── netutils/           # PSK verification, strength validation
│   │   └── security.go
│   │
│   └── traffic/            # Traffic generation engine
│       └── generator.go
│
├── configs/                # Config templates (safe to commit)
│   ├── traffic-simple.conf.example
│   └── traffic-extended.conf.example
│
├── .env.example            # Environment variable template
├── .gitignore
├── Makefile
└── go.mod
```

---

## Development

```bash
# Run all tests
make test

# Run tests with verbose output
make test-verbose

# Generate HTML coverage report  (opens coverage.html)
make test-cover

# Static analysis
make vet

# Both vet + tests (recommended before commit)
make check

# Show all available make targets
make help
```

### Test coverage

| Package | Tests |
|---------|-------|
| `pkg/comm` | Channel read/write, HMAC validation, timeout |
| `pkg/config` | CLI parsing, file parsing, edge cases |
| `pkg/logging` | File creation, rotation, error fallback |
| `pkg/traffic` | TCP/UDP generation, multi-rule, error paths |

---

## Security

| Control | Implementation |
|---------|---------------|
| Authentication | Every message is signed with HMAC-SHA256 using the PSK |
| No hardcoded secrets | PSK must come from config file or `TRAFFICORCH_PSK` env var; startup fails if absent |
| PSK strength | Minimum 8 characters with upper, lower, and digits — enforced at startup |
| Log path safety | Rejects filenames containing path separators (`..`) |
| No plaintext PSK logging | PSK is never written to logs or stdout |
| Network timeouts | All dials use explicit timeouts; idle channels are reaped |

> **Important:** Keep your `.pem` key files and `.env` files out of version control.
> Both are listed in `.gitignore`. The `TRAFFICORCH_PSK` environment variable is the recommended way to supply the PSK in automated environments.

---

## Troubleshooting

### "PSK does not meet security requirements"

Your key must be at least 8 characters and contain at least one uppercase letter, one lowercase letter, and one digit.

```bash
# ✗ too short / too simple
--psk secret

# ✓
--psk MyLab-Key2024
```

### "PSK is not set"

Add `PSK=<key>` to your config file, or export the environment variable:

```bash
export TRAFFICORCH_PSK=YourKey123
```

### Agent cannot connect to master

1. Confirm the master is running: `ss -tlnp | grep 9000`
2. Check firewall rules on the master host
3. Verify both sides use the **same** PSK
4. Check that the `--port` values match

### Config file not found

Use an absolute path or run from the directory that contains the file:

```bash
trafficorch --master --config /etc/trafficorch/traffic.conf
```

### Agent starts with wrong parameters

If `agent.conf` was saved with incorrect values, either edit it directly or
delete it and re-run with the correct flags:

```bash
rm agent.conf
./trafficorch --agent --master <HOST> --port <PORT> --psk <KEY> --id <ID>
```

### Hot-reload not triggering

The master polls the config file's `mtime` every 5 seconds.
Ensure the file is actually modified (some editors write to a temp file then rename).
A `touch traffic.conf` will force a reload on the next poll cycle.

---

## License

MIT — see `LICENSE` for details.
