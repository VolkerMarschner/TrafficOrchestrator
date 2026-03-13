package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ParseExtendedConfigV2 parses a config file supporting both simple and
// SOURCE→DEST extended formats.
//
// New in v0.3.0:
//   - TTL=<seconds> in the [MASTER] section tells agents how long to honour
//     cached instructions without a master connection (0 = never expires).
//   - Traffic rule lines now capture the SOURCE field (parts[1]) so the master
//     can route "connect" rules to source agents and "listen" rules to dest agents.
//
// PSK must be set in the config file (PSK=...) or via TRAFFICORCH_PSK env var.
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

			case "TTL":
				ttl, err := strconv.Atoi(value)
				if err != nil || ttl < 0 {
					return nil, fmt.Errorf("line %d: invalid TTL value %q (must be >= 0 seconds)", lineNum, value)
				}
				config.TTL = ttl

			case "CONFIG":
				// Handled by CLI layer; ignore here.
				continue

			default:
				// Assume it's a target definition (e.g. FILESERVER=10.0.0.1)
				targetName := strings.TrimSpace(line[:idx])
				if len(targetName) > 0 && allAlphaNumeric(targetName) {
					config.TargetMap[targetName] = value
				}
			}

			continue
		}

		// ── Traffic rule line ────────────────────────────────────────────────
		// Supported formats:
		//   Simple:   PROTOCOL  TARGET              PORT  INTERVAL  COUNT [#name]
		//   Extended: PROTOCOL  SOURCE   DEST        PORT  COUNT    [#name]
		//
		// Distinguish by field count: simple has 5+ fields with an INTERVAL column;
		// extended has 5+ fields but the 4th field is a port (no interval).
		// We detect the format heuristically: if parts[3] looks like a port number
		// and parts[4] is count/loop, it's extended; otherwise simple.

		parts := strings.Fields(line)
		if len(parts) < 5 {
			return nil, fmt.Errorf("line %d: invalid format (need at least 5 fields), got %q", lineNum, line)
		}

		protocol := strings.ToUpper(parts[0])
		if protocol != "TCP" && protocol != "UDP" {
			return nil, fmt.Errorf("line %d: invalid protocol %q (must be TCP or UDP)", lineNum, parts[0])
		}

		// Determine whether this is simple (4-column) or extended (SOURCE DEST) format.
		// Heuristic: if parts[3] is a valid port AND parts[4] is count/"loop", it's extended.
		isExtended := false
		if len(parts) >= 5 {
			if p, err := strconv.Atoi(parts[3]); err == nil && p > 0 && p <= maxPort {
				if strings.ToLower(parts[4]) == "loop" {
					isExtended = true
				} else if _, err := strconv.Atoi(parts[4]); err == nil {
					isExtended = true
				}
			}
		}

		var rule *TrafficRule

		if isExtended {
			// Extended: PROTOCOL SOURCE DEST PORT COUNT [#name]
			sourceName := parts[1]
			destName := parts[2]

			port, err := strconv.Atoi(parts[3])
			if err != nil || port <= 0 || port > maxPort {
				return nil, fmt.Errorf("line %d: invalid PORT %q", lineNum, parts[3])
			}

			count := countLoop
			if strings.ToLower(parts[4]) != "loop" {
				c, err := strconv.Atoi(parts[4])
				if err != nil || c <= 0 {
					return nil, fmt.Errorf("line %d: invalid COUNT %q", lineNum, parts[4])
				}
				count = c
			}

			rule = &TrafficRule{
				Protocol: protocol,
				Source:   sourceName, // resolved to IP later (second pass)
				Target:   destName,   // resolved to IP later
				Port:     port,
				Count:    count,
			}
		} else {
			// Simple: PROTOCOL TARGET PORT INTERVAL COUNT [#name]
			if len(parts) < 5 {
				return nil, fmt.Errorf("line %d: simple format requires 5 fields", lineNum)
			}

			targetName := parts[1]

			port, err := strconv.Atoi(parts[2])
			if err != nil || port <= 0 || port > maxPort {
				return nil, fmt.Errorf("line %d: invalid PORT %q", lineNum, parts[2])
			}

			interval, err := strconv.Atoi(parts[3])
			if err != nil || interval < 0 {
				return nil, fmt.Errorf("line %d: invalid INTERVAL %q (must be >= 0)", lineNum, parts[3])
			}

			count := countLoop
			if strings.ToLower(parts[4]) != "loop" {
				c, err := strconv.Atoi(parts[4])
				if err != nil || c <= 0 {
					return nil, fmt.Errorf("line %d: invalid COUNT %q", lineNum, parts[4])
				}
				count = c
			}

			rule = &TrafficRule{
				Protocol: protocol,
				Target:   targetName, // resolved to IP later
				Port:     port,
				Interval: interval,
				Count:    count,
			}
		}

		// Optional name from trailing inline comment  (# SMB)
		if idx := strings.Index(line, "#"); idx != -1 {
			name := strings.TrimSpace(line[idx+1:])
			if name != "" {
				rule.Name = name
			}
		}

		config.TrafficRules = append(config.TrafficRules, rule)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	// Second pass: resolve target / source names to IPs.
	for _, rule := range config.TrafficRules {
		if resolved, ok := config.TargetMap[rule.Target]; ok {
			rule.Target = resolved
		}
		if rule.Source != "" {
			if resolved, ok := config.TargetMap[rule.Source]; ok {
				rule.Source = resolved
			}
		}
	}

	// SEC-1: Fail loudly if PSK is still missing after file + env var.
	if config.PSK == "" {
		return nil, fmt.Errorf(
			"PSK is not set: add 'PSK=<key>' to %s or set TRAFFICORCH_PSK environment variable",
			filePath,
		)
	}

	return config, nil
}
