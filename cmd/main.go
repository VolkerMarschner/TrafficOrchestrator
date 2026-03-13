// Package main implements the Traffic Orchestrator CLI entry point.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"trafficorch/pkg/config"
	"trafficorch/pkg/logging"
	"trafficorch/pkg/netutils"
)

const version = "0.3.0"

func main() {
	// No arguments: try agent.conf → try instructions.conf → print help.
	if len(os.Args) < 2 {
		tryAutoStart()
		return
	}

	mode := os.Args[1]

	config.DebugMode = false

	switch mode {
	case "--master", "-m":
		handleMasterMode(os.Args[2:])
	case "--agent", "-a":
		handleAgentMode(os.Args[2:])
	case "--version", "-v":
		fmt.Printf("Traffic Orchestrator v%s\n", version)
		os.Exit(0)
	case "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", mode)
		printUsage()
		os.Exit(1)
	}
}

// tryAutoStart is invoked when no arguments are supplied.
//
// Priority order:
//  1. agent.conf  → start as connected agent (needs master reachable)
//  2. instructions.conf → start in standalone mode (no master required)
//  3. Neither found → print help
func tryAutoStart() {
	// 1. Try agent.conf (requires live master connection).
	if cfg, err := config.LoadAgentConf(config.AgentConfFile); err == nil {
		fmt.Printf("Traffic Orchestrator v%s — loading from %s\n", version, config.AgentConfFile)
		startAgent(cfg)
		return
	}

	// 2. Try instructions.conf (standalone mode, no master required).
	if _, err := config.LoadInstructionsConf(config.InstructionsConfFile); err == nil {
		fmt.Printf("Traffic Orchestrator v%s — loading from %s (standalone mode)\n", version, config.InstructionsConfFile)
		startStandalone(masterConnInfo{}, "")
		return
	}

	// 3. Neither file found — show help.
	fmt.Printf("Traffic Orchestrator v%s\n\n", version)
	fmt.Printf("No mode specified and neither %s nor %s found in current directory.\n\n",
		config.AgentConfFile, config.InstructionsConfFile)
	printUsage()
	os.Exit(0)
}

func printUsage() {
	fmt.Printf(`Traffic Orchestrator — Network Traffic Generator

Version: %s

Usage: trafficorch <mode> [options]

Modes:
  --master, -m    Run as Master (coordinates agents)
    Options:
      --config <FILE>   Path to traffic config file (required)
      --port   <PORT>   Override listen port from config
      --psk    <KEY>    Override pre-shared key from config

  --agent, -a     Run as Agent (generates / receives traffic on command)
    Options:
      --master <HOST>   Master host or IP (required on first run)
      --port   <PORT>   Master port (required on first run)
      --psk    <KEY>    Pre-shared key (required on first run)
      --id     <ID>     Agent identifier (optional)

    First run: supply all flags. They are saved to agent.conf and
    instructions.conf for subsequent starts.

    Subsequent runs: just run  trafficorch  with no arguments.
    The agent reconnects to the master. If the master is unreachable,
    traffic continues from instructions.conf (standalone mode).

    Standalone mode: the agent enforces the last set of rules received
    from the master. After the TTL expires it reconnects automatically.

  --version, -v   Show version information
  --help, -h      Show this help message

Auto-start:
  No arguments:  trafficorch checks agent.conf, then instructions.conf.
  Delete both files to reset to interactive startup.

Non-root warning:
  On Linux/macOS, running as non-root restricts port binding to > 1024.
  A warning is printed and sent to the master.

Environment variables:
  TRAFFICORCH_PSK        Pre-shared key (alternative to --psk)
  TRAFFICORCH_LOG_DIR    Directory for log files (default: current directory)
`, version)
}

// resolveLogPath returns an absolute, safe log file path.
func resolveLogPath(filename string) (string, error) {
	logDir := os.Getenv("TRAFFICORCH_LOG_DIR")
	if logDir == "" {
		logDir = "."
	}

	absDir, err := filepath.Abs(logDir)
	if err != nil {
		return "", fmt.Errorf("invalid log directory %q: %w", logDir, err)
	}

	if filepath.Base(filename) != filename {
		return "", fmt.Errorf("log filename must not contain path separators: %q", filename)
	}

	return filepath.Join(absDir, filename), nil
}

func handleMasterMode(args []string) {
	cfg, err := config.ParseMasterArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintln(os.Stderr, "Run 'trafficorch --help' for usage.")
		os.Exit(1)
	}

	masterCfg, err := config.ParseExtendedConfigV2(cfg.ConfigPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// CLI flags override config-file values.
	if cfg.Port > 0 {
		masterCfg.Port = cfg.Port
	}
	if cfg.PSK != "" {
		masterCfg.PSK = cfg.PSK
	}

	if err := netutils.ValidatePSKStrength(masterCfg.PSK); err != nil {
		fmt.Fprintf(os.Stderr, "Error: PSK does not meet security requirements: %v\n", err)
		os.Exit(1)
	}

	logFile, err := resolveLogPath("traffic.log")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving log path: %v\n", err)
		os.Exit(1)
	}

	logger, err := logging.NewLogger(logFile, defaultLogMaxSizeMB, defaultLogMaxFiles)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialise logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close()

	server, err := NewMasterServer(masterCfg, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create master server: %v\n", err)
		os.Exit(1)
	}
	defer server.Stop(logger)

	logger.Info(fmt.Sprintf("Starting Traffic Orchestrator Master v%s", version))
	if err := server.Start(); err != nil {
		logger.Error(fmt.Sprintf("Master server error: %v", err))
		os.Exit(1)
	}
}

// handleAgentMode parses CLI flags, persists them as agent.conf, then starts
// the agent. Falls back to auto-start if no flags are supplied.
func handleAgentMode(args []string) {
	if len(args) == 0 {
		tryAutoStart()
		return
	}

	cfg, err := config.ParseAgentArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintln(os.Stderr, "Run 'trafficorch --help' for usage.")
		os.Exit(1)
	}

	// Persist for the next run.
	if saveErr := config.SaveAgentConf(config.AgentConfFile, cfg); saveErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not save %s: %v\n", config.AgentConfFile, saveErr)
	} else {
		fmt.Printf("Configuration saved to %s.\n", config.AgentConfFile)
	}

	startAgent(cfg)
}

// startAgent validates the PSK, initialises logging, tries to connect to master
// and — if the master is unreachable — falls back to standalone mode.
func startAgent(cfg *config.AgentConfig) {
	if err := netutils.ValidatePSKStrength(cfg.PSK); err != nil {
		fmt.Fprintf(os.Stderr, "Error: PSK does not meet security requirements: %v\n", err)
		os.Exit(1)
	}

	logFile, err := resolveLogPath("agent.log")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving log path: %v\n", err)
		os.Exit(1)
	}

	logger, err := logging.NewLogger(logFile, defaultLogMaxSizeMB, defaultLogMaxFiles)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialise logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close()

	logger.Info(fmt.Sprintf("Starting Traffic Orchestrator Agent v%s", version))

	// Try connected mode first.
	agent, err := NewAgent(cfg, logger)
	if err != nil {
		logger.Warn(fmt.Sprintf("Cannot connect to master (%v) — trying standalone mode...", err))
		mCfg := masterConnInfo{host: cfg.MasterHost, port: cfg.Port, psk: cfg.PSK}
		startStandaloneWithLogger(mCfg, cfg.AgentID, logger)
		return
	}

	// Non-root check (connected mode — warning sent to master).
	checkNonRootAndWarn(agent.agentID, agent.client, logger)

	if err := agent.Start(); err != nil {
		logger.Error(fmt.Sprintf("Agent error: %v", err))
		os.Exit(1)
	}
}

// startStandalone is the standalone entry point used when no master credentials
// are available from CLI (e.g. auto-start from instructions.conf alone).
func startStandalone(mCfg masterConnInfo, agentID string) {
	logFile, err := resolveLogPath("agent.log")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving log path: %v\n", err)
		os.Exit(1)
	}

	logger, err := logging.NewLogger(logFile, defaultLogMaxSizeMB, defaultLogMaxFiles)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialise logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close()

	logger.Info(fmt.Sprintf("Starting Traffic Orchestrator Agent v%s (standalone)", version))
	startStandaloneWithLogger(mCfg, agentID, logger)
}

// startStandaloneWithLogger creates a standalone agent and starts it.
func startStandaloneWithLogger(mCfg masterConnInfo, agentID string, logger *logging.Logger) {
	agent, err := newStandaloneAgent(config.InstructionsConfFile, mCfg, agentID, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Standalone start failed: %v\n", err)
		logger.Error(fmt.Sprintf("Standalone start failed: %v", err))
		os.Exit(1)
	}

	// Non-root check (standalone mode — warning logged only, no master to notify).
	checkNonRootAndWarn(agent.agentID, nil, logger)

	if err := agent.Start(); err != nil {
		logger.Error(fmt.Sprintf("Agent error: %v", err))
		os.Exit(1)
	}
}
