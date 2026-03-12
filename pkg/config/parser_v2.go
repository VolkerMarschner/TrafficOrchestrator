package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ParseExtendedConfigV2 parses a config file with SOURCE->DEST format support.
// PSK must be set either in the config file (PSK=...) or via the
// TRAFFICORCH_PSK environment variable. The function fails if no PSK is found.
func ParseExtendedConfigV2(filePath string) (*MasterConfig, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	// SEC-1: No hardcoded PSK default. Fall back to env var, then fail.
	psk := os.Getenv("TRAFFICORCH_PSK")

	config := &MasterConfig{
		ConfigPath:   filePath,
		TrafficRules: make([]*TrafficRule, 0),
		Port:         defaultPort,
		PSK:          psk,
		TargetMap:    make(map[string]string),
	}

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and pure comments
		if line == "" || (len(line) > 0 && line[0] == '#') {
			continue
		}

		// Skip INI section headers [MASTER], [AGENT]
		if len(line) >= 2 && line[0] == '[' && line[len(line)-1] == ']' {
			continue
		}

		// Check for key=value format (INI-style or target definitions)
		if idx := strings.Index(line, "="); idx != -1 {
			key := strings.TrimSpace(strings.ToUpper(line[:idx]))
			value := strings.TrimSpace(line[idx+1:])

			// Strip inline comments from value
			if cidx := strings.Index(value, "#"); cidx != -1 {
				value = strings.TrimSpace(value[:cidx])
			}

			switch key {
			case "PORT":
				port, err := strconv.Atoi(value)
				if err != nil || port <= 0 || port > maxPort {
					return nil, fmt.Errorf("line %d: invalid PORT value %q", lineNum, value)
				}
				config.Port = port
			case "PSK":
				config.PSK = value
			case "CONFIG":
				// CONFIG is handled by CLI, skip here
				continue
			default:
				// Assume it's a target definition (e.g. TARGET1=10.0.0.1)
				targetName := strings.TrimSpace(line[:idx])
				if len(targetName) > 0 && allAlphaNumeric(targetName) {
					config.TargetMap[targetName] = value
				}
			}

			continue
		}

		// Try to parse as traffic rule: PROTOCOL SOURCE DEST PORT COUNT [NAME]
		parts := strings.Fields(line)
		if len(parts) < 5 {
			return nil, fmt.Errorf("line %d: invalid format (need at least 5 fields), got %q", lineNum, line)
		}

		protocol := strings.ToUpper(parts[0])
		if protocol != "TCP" && protocol != "UDP" {
			return nil, fmt.Errorf("line %d: invalid protocol %q (must be TCP or UDP)", lineNum, parts[0])
		}

		destName := parts[2]

		port, err := strconv.Atoi(parts[3])
		if err != nil || port <= 0 || port > maxPort {
			return nil, fmt.Errorf("line %d: invalid PORT %q (must be 1-65535)", lineNum, parts[3])
		}

		count := countLoop // Default: loop forever
		if strings.ToLower(parts[4]) != "loop" {
			c, err := strconv.Atoi(parts[4])
			if err != nil || c <= 0 {
				return nil, fmt.Errorf("line %d: invalid COUNT %q (must be positive number or 'loop')", lineNum, parts[4])
			}
			count = c
		}

		rule := &TrafficRule{
			Protocol: protocol,
			Target:   destName,
			Port:     port,
			Count:    count,
		}

		config.TrafficRules = append(config.TrafficRules, rule)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	// Second pass: resolve target names to IPs
	// (definitions may appear after rule lines in the file)
	for _, rule := range config.TrafficRules {
		if resolved, ok := config.TargetMap[rule.Target]; ok {
			rule.Target = resolved
		}
	}

	// SEC-1: Fail loudly if PSK is still missing after file + env var
	if config.PSK == "" {
		return nil, fmt.Errorf(
			"PSK is not set: add 'PSK=<key>' to %s or set TRAFFICORCH_PSK environment variable",
			filePath,
		)
	}

	return config, nil
}
