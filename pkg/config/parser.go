// Package config provides configuration parsing and validation for Traffic Orchestrator.
package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// DebugMode enables verbose logging when set to true
var DebugMode = false

func logf(format string, args ...interface{}) {
	if DebugMode {
		fmt.Printf("[CONFIG] "+format+"\n", args...)
	} else {
		// Silent in production mode - no output
	}
}

// TrafficRule defines a single traffic generation rule.
type TrafficRule struct {
	Protocol string `json:"protocol"`          // "TCP" or "UDP"
	Source   string `json:"source,omitempty"`  // Source IP (extended format only; empty = all agents)
	Target   string `json:"target,omitempty"`  // Destination IP (empty for "listen" rules)
	Port     int    `json:"port"`              // Port number
	Interval int    `json:"interval"`          // Seconds between connections (0 = immediate)
	Count    int    `json:"count"`             // Number of connections (-1 = loop forever)
	Name     string `json:"name,omitempty"`    // Optional human-readable name (e.g., "SMB")
	Role     string `json:"role,omitempty"`    // "connect" (dial out) or "listen" (open port); default = "connect"
}

// ExtendedTrafficRule defines a traffic rule with SOURCE and DESTINATION
type ExtendedTrafficRule struct {
	Protocol string // "TCP" or "UDP"
	Source   string // Source IP (traffic generator)
	Dest     string // Destination IP (listener - port must be open)
	Port     int    // Port on destination to listen on
	Count    int    // Number of connections (-1 = loop forever)
	Name     string // Optional human-readable name
}

// ExtendedConfig holds configuration with extended rules
type ExtendedConfig struct {
	Port         int
	PSK          string
	ConfigPath   string
	Rules        []*ExtendedTrafficRule
	TargetMap    map[string]string // name -> IP mapping
}

// MasterConfig holds configuration for the master mode.
type MasterConfig struct {
	Port         int
	PSK          string
	TTL          int               // Seconds agents should cache instructions (0 = no expiry)
	ConfigPath   string
	TrafficRules []*TrafficRule    // Loaded from config file
	TargetMap    map[string]string // name → IP mapping (for SOURCE/DEST routing)

	// v0.4.0: Profile system
	ProfileDir  string              // Directory containing .profile files (PROFILE_DIR key)
	Assignments map[string][]string // host/IP or target name → []profile names ([ASSIGNMENTS])
	TagMap      map[string][]string // tag name → []IP addresses (from #tag: annotations)
	Profiles    map[string]*Profile // loaded profiles (populated after config parse)
}

// AgentConfig holds configuration for the agent mode.
type AgentConfig struct {
	MasterHost string
	Port       int
	PSK        string
	AgentID    string
}

// INIConfig holds parsed INI-style configuration with [MASTER], [AGENT] sections
type INIConfig struct {
	MasterSection map[string]string
	AgentSection  map[string]string
}

// ParseMasterArgs parses command-line arguments for master mode.
func ParseMasterArgs(args []string) (*MasterConfig, error) {
	cfg := &MasterConfig{}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		
		var key, value string
		if idx := strings.Index(arg, "="); idx != -1 {
			key = arg[:idx]
			value = arg[idx+1:]
		} else {
			key = arg
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return nil, fmt.Errorf("%s requires a value", key)
			}
			value = args[i+1]
			i++
		}

		switch key {
		case "--port":
			port, err := strconv.Atoi(value)
			if err != nil || port <= 0 || port > 65535 {
				return nil, fmt.Errorf("invalid port number: %s", value)
			}
			cfg.Port = port
		case "--psk":
			cfg.PSK = value
			if cfg.PSK == "" {
				return nil, fmt.Errorf("--psk cannot be empty")
			}
		case "--config":
			cfg.ConfigPath = value
		default:
			return nil, fmt.Errorf("unknown master option: %s", key)
		}
	}

	// NOTE: We no longer auto-load config file here - let handleMasterMode call ParseExtendedConfigV2
	// This allows us to use the new extended format parser instead of the old legacy one

	// Validate required fields (CLI takes precedence over config)
	// BUT if config path is provided, we can skip port/psk validation since they'll come from config file
	if cfg.ConfigPath != "" {
		// Config file will provide port and psk, so don't require them on CLI
		cfg.ConfigPath, _ = filepath.Abs(cfg.ConfigPath)
		return cfg, nil
	}

	if cfg.Port == 0 {
		return nil, fmt.Errorf("--port is required")
	}
	if cfg.PSK == "" {
		return nil, fmt.Errorf("--psk is required")
	}
	if cfg.ConfigPath == "" {
		return nil, fmt.Errorf("--config is required")
	}

	cfg.ConfigPath, _ = filepath.Abs(cfg.ConfigPath)
	return cfg, nil
}

// ParseAgentArgs parses command-line arguments for agent mode.
func ParseAgentArgs(args []string) (*AgentConfig, error) {
	cfg := &AgentConfig{}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		
		var key, value string
		if idx := strings.Index(arg, "="); idx != -1 {
			key = arg[:idx]
			value = arg[idx+1:]
		} else {
			key = arg
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return nil, fmt.Errorf("%s requires a value", key)
			}
			value = args[i+1]
			i++
		}

		switch key {
		case "--master":
			cfg.MasterHost = value
		case "--port":
			port, err := strconv.Atoi(value)
			if err != nil || port <= 0 || port > 65535 {
				return nil, fmt.Errorf("invalid port number: %s", value)
			}
			cfg.Port = port
		case "--psk":
			cfg.PSK = value
		case "--id":
			cfg.AgentID = value
		default:
			return nil, fmt.Errorf("unknown agent option: %s", key)
		}
	}

	// Validate required fields
	if cfg.MasterHost == "" {
		return nil, fmt.Errorf("--master is required")
	}
	if cfg.Port == 0 {
		return nil, fmt.Errorf("--port is required")
	}
	if cfg.PSK == "" {
		return nil, fmt.Errorf("--psk is required")
	}
	if cfg.AgentID == "" {
		cfg.AgentID = "agent-unknown" // Default ID if not specified
	}

	return cfg, nil
}

// LoadTrafficRules reads and parses a traffic configuration file.
func LoadTrafficRules(filePath string) ([]*TrafficRule, map[string]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	var rules []*TrafficRule
	targetMap := make(map[string]string) // name -> IP mapping
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Skip INI section headers [MASTER], [AGENT], etc.
		if (strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]")) {
			continue // Completely skip INI sections
		}

		// Check for TARGET definition (name=IP format)
		if idx := strings.Index(line, "="); idx != -1 {
			parts := strings.SplitN(line, "=", 2)
			name := strings.TrimSpace(parts[0])
			
			// Skip INI-style key=value (PORT=9000, PSK=xxx, CONFIG=xxx)
			if strings.ToUpper(name) == "PORT" || strings.ToUpper(name) == "PSK" || strings.ToUpper(name) == "CONFIG" {
				continue // Skip these - they're handled by INI parser
			}
			
			// Check if this looks like a TARGET definition (not TCP/UDP line)
			if !strings.HasPrefix(strings.TrimSpace(line[:idx]), "TCP") && !strings.HasPrefix(strings.TrimSpace(line[:idx]), "UDP") {
				// Strip comments from IP value (everything after #)
				ipWithComment := strings.TrimSpace(parts[1])
				var ip string
				if commentIdx := strings.Index(ipWithComment, "#"); commentIdx != -1 {
					ip = strings.TrimSpace(ipWithComment[:commentIdx])
				} else {
					ip = ipWithComment
				}
				
				// Validate name format (alphanumeric + underscore)
				if len(name) > 0 && allAlphaNumeric(name) {
					targetMap[name] = ip
					continue // Skip to next line
				}
			}
		}

		rule, err := parseTrafficLine(line, targetMap)
		if err != nil {
			return nil, nil, fmt.Errorf("error at line %d: %w", lineNum, err)
		}

		if rule != nil {
			rules = append(rules, rule)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("error reading config file: %w", err)
	}

	return rules, targetMap, nil
}

// parseTrafficLine parses a single line from the traffic config file.
func parseTrafficLine(line string, targetMap map[string]string) (*TrafficRule, error) {
	parts := strings.Fields(line)

	if len(parts) < 5 {
		return nil, fmt.Errorf("invalid format: expected PROTOCOL TARGET PORT INTERVAL COUNT [NAME]")
	}

	protocol := strings.ToUpper(parts[0])
	if protocol != "TCP" && protocol != "UDP" {
		return nil, fmt.Errorf("invalid protocol: %s (must be TCP or UDP)", parts[0])
	}

	targetOrName := parts[1]
	
	// Resolve target name to IP if it exists in targetMap
	targetIP, ok := targetMap[targetOrName]
	if !ok {
		// Check if it's already an IP address (simple check)
		if isIPAddress(targetOrName) {
			targetIP = targetOrName
		} else {
			return nil, fmt.Errorf("unknown target: %s (not defined in targets list)", targetOrName)
		}
	}

	port, err := strconv.Atoi(parts[2])
	if err != nil || port <= 0 || port > 65535 {
		return nil, fmt.Errorf("invalid port: %s", parts[2])
	}

	interval, err := strconv.Atoi(parts[3])
	if err != nil || interval < 0 {
		return nil, fmt.Errorf("invalid interval: %s (must be >= 0)", parts[3])
	}

	count := -1 // Default to loop forever
	if parts[4] != "loop" {
		c, err := strconv.Atoi(parts[4])
		if err != nil || c <= 0 {
			return nil, fmt.Errorf("invalid count: %s (must be number or 'loop')", parts[4])
		}
		count = c
	}

	rule := &TrafficRule{
		Protocol: protocol,
		Target:   targetIP, // Store resolved IP
		Port:     port,
		Interval: interval,
		Count:    count,
		Name:     "", // Optional name parsed from comment if present
	}

	// Try to parse optional name from trailing comment
	if idx := strings.Index(line, "#"); idx != -1 {
		namePart := strings.TrimSpace(line[idx+1:])
		if len(namePart) > 0 {
			rule.Name = namePart
		}
	}

	return rule, nil
}

// isIPAddress checks if string looks like an IP address (simple heuristic)
func isIPAddress(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}
	for _, part := range parts {
		num := 0
		for _, c := range part {
			if c < '0' || c > '9' {
				return false
			}
			num = num*10 + int(c-'0')
		}
		if num < 0 || num > 255 {
			return false
		}
	}
	return true
}

// allAlphaNumeric checks if string contains only alphanumeric chars and underscores
func allAlphaNumeric(s string) bool {
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return len(s) > 0
}

// parseINIConfig parses an INI-style config file with [MASTER], [AGENT] sections
func parseINIConfig(filePath string) *INIConfig {
	file, err := os.Open(filePath)
	if err != nil {
		return nil
	}
	defer file.Close()

	result := &INIConfig{
		MasterSection: make(map[string]string),
		AgentSection:  make(map[string]string),
	}

	scanner := bufio.NewScanner(file)
	currentSection := ""

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Check for section header [MASTER] or [AGENT]
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section := strings.ToUpper(strings.TrimPrefix(strings.TrimSuffix(line, "]"), "["))
			switch section {
			case "MASTER":
				currentSection = "master"
			case "AGENT":
				currentSection = "agent"
			default:
				currentSection = ""
			}
			continue
		}

		// Parse key=value in current section
		if idx := strings.Index(line, "="); idx != -1 && currentSection != "" {
			key := strings.TrimSpace(strings.ToUpper(line[:idx]))
			value := strings.TrimSpace(line[idx+1:])
			
			switch currentSection {
			case "master":
				result.MasterSection[key] = value
			case "agent":
				result.AgentSection[key] = value
			}
		}
	}

	return result
}

// loadConfigFileOnly loads the config file without enforcing all CLI params
func loadConfigFileOnly(filePath string) ([]*TrafficRule, map[string]string, error) {
	rules, targetMap, err := LoadTrafficRules(filePath)
	return rules, targetMap, err
}
