package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ParseExtendedConfig reads and parses extended config format with SOURCE/DEST
func ParseExtendedConfig(filePath string) (*ExtendedConfig, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	config := &ExtendedConfig{
		ConfigPath: filePath,
		Rules:      make([]*ExtendedTrafficRule, 0),
		TargetMap:  make(map[string]string),
	}
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Skip INI section headers [MASTER], [AGENT]
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			continue
		}

		// Check for TARGET/SOURCE/DEST definition (name=IP format) or INI values
		if idx := strings.Index(line, "="); idx != -1 {
			parts := strings.SplitN(line, "=", 2)
			name := strings.TrimSpace(parts[0])

			// Parse INI-style key=value for PORT and PSK
			if strings.ToUpper(name) == "PORT" || strings.ToUpper(name) == "PSK" || strings.ToUpper(name) == "CONFIG" {
				keyName := strings.ToUpper(name)
				
				// Strip comments from value
				valueWithComment := strings.TrimSpace(parts[1])
				var value string
				if commentIdx := strings.Index(valueWithComment, "#"); commentIdx != -1 {
					value = strings.TrimSpace(valueWithComment[:commentIdx])
				} else {
					value = valueWithComment
				}

				// Set values in config struct
				if keyName == "PORT" && len(value) > 0 {
					port, err := strconv.Atoi(value)
					if err != nil || port <= 0 || port > 65535 {
						return nil, fmt.Errorf("invalid PORT value: %s", value)
					}
					config.Port = port
				} else if keyName == "PSK" && len(value) > 0 {
					config.PSK = value
				} else if keyName == "CONFIG" {
					config.ConfigPath = value // Already set, but good to have as reference
				}
				
				continue
			}

			// Check if this looks like a rule line (starts with TCP/UDP) - skip it
			if strings.HasPrefix(strings.TrimSpace(line[:idx]), "TCP") || strings.HasPrefix(strings.TrimSpace(line[:idx]), "UDP") {
				continue
			}

			// Strip comments from IP value
			ipWithComment := strings.TrimSpace(parts[1])
			var ip string
			if commentIdx := strings.Index(ipWithComment, "#"); commentIdx != -1 {
				ip = strings.TrimSpace(ipWithComment[:commentIdx])
			} else {
				ip = ipWithComment
			}

			// Validate name format (alphanumeric + underscore) for targets
			if len(name) > 0 && allAlphaNumeric(name) {
				config.TargetMap[name] = ip
				continue
			}
		}

		// Try to parse extended traffic rule line: PROTOCOL SOURCE DEST PORT COUNT [NAME]
		rule, err := parseExtendedTrafficLine(line, config.TargetMap)
		if err != nil {
			return nil, fmt.Errorf("error at line %d: %w", lineNum, err)
		}

		if rule != nil {
			config.Rules = append(config.Rules, rule)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	return config, nil
}

// parseExtendedTrafficLine parses a single extended traffic rule line
func parseExtendedTrafficLine(line string, targetMap map[string]string) (*ExtendedTrafficRule, error) {
	parts := strings.Fields(line)

	if len(parts) < 5 {
		return nil, fmt.Errorf("invalid format: expected PROTOCOL SOURCE DEST PORT COUNT [NAME] (got %d parts)", len(parts))
	}

	protocol := strings.ToUpper(parts[0])
	if protocol != "TCP" && protocol != "UDP" {
		return nil, fmt.Errorf("invalid protocol: %s (must be TCP or UDP)", parts[0])
	}

	sourceOrName := parts[1]
	destOrName := parts[2]

	// Resolve source name to IP if it exists in targetMap
	var sourceIP string
	if sourceIP = resolveTarget(sourceOrName, targetMap); sourceIP == "" {
		return nil, fmt.Errorf("unknown source: %s (not defined in targets list)", sourceOrName)
	}

	// Resolve dest name to IP if it exists in targetMap
	var destIP string
	if destIP = resolveTarget(destOrName, targetMap); destIP == "" {
		return nil, fmt.Errorf("unknown destination: %s (not defined in targets list)", destOrName)
	}

	port, err := strconv.Atoi(parts[3])
	if err != nil || port <= 0 || port > 65535 {
		return nil, fmt.Errorf("invalid port: %s (value from parts[3])", parts[3])
	}

	count := -1 // Default to loop forever
	if parts[4] != "loop" {
		c, err := strconv.Atoi(parts[4])
		if err != nil || c <= 0 {
			return nil, fmt.Errorf("invalid count: %s (must be number or 'loop')", parts[4])
		}
		count = c
	}

	rule := &ExtendedTrafficRule{
		Protocol: protocol,
		Source:   sourceIP,
		Dest:     destIP,
		Port:     port,
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

// resolveTarget resolves a target name to IP address using the targetMap
func resolveTarget(targetOrName string, targetMap map[string]string) string {
	if ip, ok := targetMap[targetOrName]; ok {
		return ip
	}
	// Check if it's already an IP address (simple check)
	if isIPAddress(targetOrName) {
		return targetOrName
	}
	return ""
}
