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

// ProfileRule represents a single rule inside a .profile file.
//
// Format (whitespace-flexible):
//
//	PROTOCOL  ROLE     SRC   DST          PORT  INTERVAL  COUNT  [#name]
//	TCP       connect  SELF  group:dc     389   15        3      #ldap-query
//	TCP       listen   SELF  -            389   -         -      #ldap-listener
//
// Field semantics:
//
//	PROTOCOL  TCP or UDP
//	ROLE      connect (dial out) or listen (open port)
//	SRC       SELF, an IP address, or a named target
//	DST       SELF, IP, named target, group:<tag>, ANY, or - (unused for listen)
//	PORT      1-65535
//	INTERVAL  seconds between connections; - means 0 (immediate)
//	COUNT     number of connections; - or loop means repeat forever (-1)
type ProfileRule struct {
	Protocol string // "TCP" or "UDP"
	Role     string // "connect" or "listen"
	Src      string // source placeholder
	Dst      string // destination placeholder
	Port     int
	Interval int    // 0 = immediate
	Count    int    // -1 = loop forever
	Name     string // optional label from trailing #name
}

// ProfileMeta holds the key/value pairs from the [META] section of a profile.
type ProfileMeta struct {
	Name        string
	Description string
	Version     string
	Extends     string   // parent profile name (EXTENDS = base_windows)
	Tags        []string // searchable labels (comma-separated in the file)
}

// Profile is a named, reusable set of traffic rules that can be assigned to hosts.
// Profiles support single-level inheritance via the EXTENDS key.
type Profile struct {
	Meta  ProfileMeta
	Rules []*ProfileRule
	Path  string // absolute path of the source file
}

// LoadProfile parses a single .profile file and returns the Profile.
//
// The file format uses INI-style sections:
//
//	[META]        — metadata (NAME, DESCRIPTION, VERSION, EXTENDS, TAGS)
//	[RULES]       — one rule line per row (see ProfileRule for format)
func LoadProfile(path string) (*Profile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("cannot open profile %s: %w", path, err)
	}
	defer f.Close()

	p := &Profile{Path: path}
	section := ""
	lineNum := 0

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip blank lines and full-line comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Section header
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToUpper(line[1 : len(line)-1])
			continue
		}

		switch section {
		case "META":
			if idx := strings.Index(line, "="); idx != -1 {
				key := strings.TrimSpace(strings.ToUpper(line[:idx]))
				val := strings.TrimSpace(line[idx+1:])
				// Strip trailing inline comment
				if ci := strings.Index(val, " #"); ci >= 0 {
					val = strings.TrimSpace(val[:ci])
				}
				switch key {
				case "NAME":
					p.Meta.Name = val
				case "DESCRIPTION":
					p.Meta.Description = val
				case "VERSION":
					p.Meta.Version = val
				case "EXTENDS":
					p.Meta.Extends = val
				case "TAGS":
					for _, t := range strings.Split(val, ",") {
						if tag := strings.TrimSpace(t); tag != "" {
							p.Meta.Tags = append(p.Meta.Tags, tag)
						}
					}
				}
			}

		case "RULES":
			rule, err := parseProfileRuleLine(line, lineNum)
			if err != nil {
				return nil, fmt.Errorf("%s line %d: %w", filepath.Base(path), lineNum, err)
			}
			p.Rules = append(p.Rules, rule)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading %s: %w", path, err)
	}

	// Derive name from filename if [META] NAME was not provided
	if p.Meta.Name == "" {
		base := filepath.Base(path)
		p.Meta.Name = strings.TrimSuffix(base, filepath.Ext(base))
	}

	return p, nil
}

// parseProfileRuleLine parses one line from the [RULES] section.
func parseProfileRuleLine(line string, lineNum int) (*ProfileRule, error) {
	// Extract optional trailing #name before field splitting
	name := ""
	if idx := strings.Index(line, "#"); idx != -1 {
		name = strings.TrimSpace(line[idx+1:])
		line = strings.TrimSpace(line[:idx])
	}

	parts := strings.Fields(line)
	if len(parts) < 7 {
		return nil, fmt.Errorf(
			"profile rule needs 7 fields (PROTOCOL ROLE SRC DST PORT INTERVAL COUNT), got %d in %q",
			len(parts), line)
	}

	protocol := strings.ToUpper(parts[0])
	if protocol != "TCP" && protocol != "UDP" {
		return nil, fmt.Errorf("invalid protocol %q (must be TCP or UDP)", parts[0])
	}

	role := strings.ToLower(parts[1])
	if role != "connect" && role != "listen" {
		return nil, fmt.Errorf("invalid role %q (must be connect or listen)", parts[1])
	}

	src := parts[2]
	dst := parts[3]

	port, err := strconv.Atoi(parts[4])
	if err != nil || port <= 0 || port > maxPort {
		return nil, fmt.Errorf("invalid PORT %q", parts[4])
	}

	interval := 0
	if parts[5] != "-" {
		interval, err = strconv.Atoi(parts[5])
		if err != nil || interval < 0 {
			return nil, fmt.Errorf("invalid INTERVAL %q (use a number or - for immediate)", parts[5])
		}
	}

	count := countLoop // -1 = loop forever
	if parts[6] != "-" && strings.ToLower(parts[6]) != "loop" {
		count, err = strconv.Atoi(parts[6])
		if err != nil || count <= 0 {
			return nil, fmt.Errorf("invalid COUNT %q (use a number, loop, or -)", parts[6])
		}
	}

	return &ProfileRule{
		Protocol: protocol,
		Role:     role,
		Src:      src,
		Dst:      dst,
		Port:     port,
		Interval: interval,
		Count:    count,
		Name:     name,
	}, nil
}

// LoadProfileDir loads every *.profile file from dir and returns them keyed by profile name.
// Files that fail to parse are reported as errors rather than silently skipped.
func LoadProfileDir(dir string) (map[string]*Profile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("cannot read profile directory %q: %w", dir, err)
	}

	profiles := make(map[string]*Profile)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".profile") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		p, err := LoadProfile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to load %s: %w", entry.Name(), err)
		}
		profiles[p.Meta.Name] = p
	}
	return profiles, nil
}

// FlattenProfile returns the merged rule list for a profile, resolving EXTENDS chains.
// Rules from parent profiles are prepended; the named profile's own rules follow.
// Circular references return an error.
func FlattenProfile(name string, profiles map[string]*Profile, seen map[string]bool) ([]*ProfileRule, error) {
	if seen == nil {
		seen = make(map[string]bool)
	}
	if seen[name] {
		return nil, fmt.Errorf("circular EXTENDS detected involving profile %q", name)
	}
	p, ok := profiles[name]
	if !ok {
		return nil, fmt.Errorf("profile %q not found (check PROFILE_DIR)", name)
	}
	seen[name] = true

	var rules []*ProfileRule

	// Inherit parent rules first
	if p.Meta.Extends != "" {
		parentRules, err := FlattenProfile(p.Meta.Extends, profiles, seen)
		if err != nil {
			return nil, fmt.Errorf("profile %q → EXTENDS %q: %w", name, p.Meta.Extends, err)
		}
		rules = append(rules, parentRules...)
	}

	// Append own rules
	rules = append(rules, p.Rules...)
	return rules, nil
}

// ResolveProfileRules builds a concrete []*TrafficRule list for a specific agent IP
// by resolving SELF, group:<tag>, and ANY placeholders in the assigned profiles.
//
// profileNames  — profiles assigned to this agent (from [ASSIGNMENTS])
// agentIP       — the agent's resolved IP address
// targetMap     — name→IP mapping from the [TARGETS] / key=value section
// tagMap        — tag→[]IP mapping derived from #tag: annotations on target lines
func ResolveProfileRules(
	profiles map[string]*Profile,
	profileNames []string,
	agentIP string,
	targetMap map[string]string,
	tagMap map[string][]string,
) ([]*TrafficRule, error) {

	var result []*TrafficRule

	for _, profileName := range profileNames {
		profileName = strings.TrimSpace(profileName)
		if profileName == "" {
			continue
		}

		rules, err := FlattenProfile(profileName, profiles, nil)
		if err != nil {
			return nil, fmt.Errorf("cannot resolve profile %q: %w", profileName, err)
		}

		for _, pr := range rules {
			switch pr.Role {
			case "listen":
				// Listen rules: the agent opens a port — no source/dest resolution needed.
				result = append(result, &TrafficRule{
					Protocol: pr.Protocol,
					Role:     "listen",
					Port:     pr.Port,
					Name:     pr.Name,
					Count:    countLoop,
				})

			case "connect":
				// Connect rules: resolve Src; skip if this rule belongs to a different agent.
				src := resolvePlaceholder(pr.Src, agentIP, targetMap)
				if pr.Src != "SELF" && src != agentIP {
					// Rule is sourced from a different host — not for this agent.
					continue
				}

				// Expand the destination placeholder to a list of concrete IPs.
				dests := resolveDestination(pr.Dst, agentIP, targetMap, tagMap)
				for _, dst := range dests {
					if dst == "" || dst == "-" {
						continue
					}
					result = append(result, &TrafficRule{
						Protocol: pr.Protocol,
						Role:     "connect",
						Source:   agentIP,
						Target:   dst,
						Port:     pr.Port,
						Interval: pr.Interval,
						Count:    pr.Count,
						Name:     pr.Name,
					})
				}
			}
		}
	}

	return result, nil
}

// LookupAssignments returns the profile names assigned to agentIP.
// It first checks for a direct IP key, then reverses through targetMap to find
// a named alias whose resolved IP matches agentIP.
func LookupAssignments(agentIP string, assignments map[string][]string, targetMap map[string]string) []string {
	// Direct IP match
	if profiles, ok := assignments[agentIP]; ok {
		return profiles
	}
	// Named-target reverse lookup
	for name, ip := range targetMap {
		if ip == agentIP {
			if profiles, ok := assignments[name]; ok {
				return profiles
			}
		}
	}
	return nil
}

// ─── internal helpers ─────────────────────────────────────────────────────────

// resolvePlaceholder converts SELF → agentIP, a target name → its IP, or returns
// the value unchanged (bare IP).
func resolvePlaceholder(value, agentIP string, targetMap map[string]string) string {
	if strings.ToUpper(value) == "SELF" {
		return agentIP
	}
	if ip, ok := targetMap[value]; ok {
		return ip
	}
	return value
}

// resolveDestination expands a destination field into a slice of concrete IPs.
// Recognised placeholders: SELF, - (empty), ANY, group:<tag>, named target, bare IP.
func resolveDestination(dst, agentIP string, targetMap map[string]string, tagMap map[string][]string) []string {
	switch strings.ToUpper(dst) {
	case "SELF":
		return []string{agentIP}
	case "-", "":
		return nil
	case "ANY":
		ips := make([]string, 0, len(targetMap))
		for _, ip := range targetMap {
			ips = append(ips, ip)
		}
		return ips
	}

	// group:<tag>
	lower := strings.ToLower(dst)
	if strings.HasPrefix(lower, "group:") {
		tag := dst[len("group:"):]
		return tagMap[tag]
	}

	// Named target
	if ip, ok := targetMap[dst]; ok {
		return []string{ip}
	}

	// Bare IP or hostname
	return []string{dst}
}
