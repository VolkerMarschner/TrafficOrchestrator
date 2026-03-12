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

const version = "0.1.0"

func main() {
	// FR-01: No mode specified → guide user to run as agent
	if len(os.Args) < 2 {
		fmt.Printf("Traffic Orchestrator v%s\n\n", version)
		fmt.Println("No mode specified. This binary can run as Master or Agent.")
		fmt.Println()
		fmt.Println("To run as Agent (register with a remote Master):")
		fmt.Println("  trafficorch --agent --master <HOST_OR_IP> --port <PORT> --psk <KEY>")
		fmt.Println()
		fmt.Println("To run as Master (coordinate agents):")
		fmt.Println("  trafficorch --master --config <FILE>")
		fmt.Println()
		fmt.Println("Use --help for full usage information.")
		os.Exit(0)
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

func printUsage() {
	fmt.Printf(`Traffic Orchestrator - Network Traffic Generator

Version: %s

Usage: trafficorch <mode> [options]

Modes:
  --master, -m    Run as Master (coordinates agents)
    Options:
      --config <FILE>   Path to traffic config file (required)
      --port   <PORT>   Override listen port from config
      --psk    <KEY>    Override pre-shared key from config

  --agent, -a     Run as Agent (generates traffic on command)
    Options:
      --master <HOST>   Master host or IP (required)
      --port   <PORT>   Master port (required)
      --psk    <KEY>    Pre-shared key (required)
      --id     <ID>     Agent identifier (optional)

  --version, -v   Show version information
  --help, -h      Show this help message

Environment variables:
  TRAFFICORCH_PSK        Pre-shared key (alternative to --psk)
  TRAFFICORCH_LOG_DIR    Directory for log files (default: current directory)
`, version)
}

// resolveLogPath returns an absolute, safe log file path.
// Reads TRAFFICORCH_LOG_DIR env var; defaults to current directory.
func resolveLogPath(filename string) (string, error) {
	logDir := os.Getenv("TRAFFICORCH_LOG_DIR")
	if logDir == "" {
		logDir = "."
	}

	absDir, err := filepath.Abs(logDir)
	if err != nil {
		return "", fmt.Errorf("invalid log directory %q: %w", logDir, err)
	}

	// Reject path traversal in filename
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

	// CLI flags override config-file values
	if cfg.Port > 0 {
		masterCfg.Port = cfg.Port
	}
	if cfg.PSK != "" {
		masterCfg.PSK = cfg.PSK
	}

	// SEC-5: Validate PSK strength before starting
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
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
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

func handleAgentMode(args []string) {
	cfg, err := config.ParseAgentArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintln(os.Stderr, "Run 'trafficorch --help' for usage.")
		os.Exit(1)
	}

	// SEC-5: Validate PSK strength before connecting
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
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close()

	agent, err := NewAgent(cfg, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create agent: %v\n", err)
		os.Exit(1)
	}

	logger.Info(fmt.Sprintf("Starting Traffic Orchestrator Agent v%s", version))
	if err := agent.Start(); err != nil {
		logger.Error(fmt.Sprintf("Agent error: %v", err))
		os.Exit(1)
	}
}
