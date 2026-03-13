package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// InstructionsConfFile is the default filename for agent-side cached instructions.
const InstructionsConfFile = "instructions.conf"

// InstructionsConf stores the traffic rules received from the master together
// with metadata required for standalone operation and TTL-based reconnection.
//
// File format: JSON, written atomically (tmp + rename).
// The PSK is stored in plain text — treat the file with the same care as agent.conf.
type InstructionsConf struct {
	ReceivedAt time.Time      `json:"received_at"`
	TTL        int            `json:"ttl"`     // seconds; 0 = never expires
	MasterHost string         `json:"master"`
	MasterPort int            `json:"port"`
	PSK        string         `json:"psk"`
	AgentID    string         `json:"agent_id,omitempty"`
	Rules      []*TrafficRule `json:"rules"`
}

// IsExpired reports whether the instructions have exceeded their TTL.
// Instructions with TTL == 0 never expire.
func (c *InstructionsConf) IsExpired() bool {
	if c.TTL <= 0 {
		return false
	}
	return time.Since(c.ReceivedAt) >= time.Duration(c.TTL)*time.Second
}

// ExpiresIn returns the time remaining until the instructions expire.
// Returns 0 if already expired or TTL is not set.
func (c *InstructionsConf) ExpiresIn() time.Duration {
	if c.TTL <= 0 {
		return 0
	}
	d := time.Until(c.ReceivedAt.Add(time.Duration(c.TTL) * time.Second))
	if d < 0 {
		return 0
	}
	return d
}

// LoadInstructionsConf reads an InstructionsConf from a JSON file.
// Returns os.ErrNotExist (wrapped) if the file is absent.
func LoadInstructionsConf(path string) (*InstructionsConf, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err // caller can use os.IsNotExist
	}

	var conf InstructionsConf
	if err := json.Unmarshal(data, &conf); err != nil {
		return nil, fmt.Errorf("%s: invalid JSON: %w", path, err)
	}

	if conf.MasterHost == "" {
		return nil, fmt.Errorf("%s: missing master host", path)
	}
	if conf.MasterPort == 0 {
		return nil, fmt.Errorf("%s: missing master port", path)
	}
	if conf.PSK == "" {
		return nil, fmt.Errorf("%s: missing PSK", path)
	}

	return &conf, nil
}

// SaveInstructionsConf persists an InstructionsConf to path atomically.
// The file is pretty-printed JSON for human readability.
// Delete the file to force a full re-sync with the master on next start.
func SaveInstructionsConf(path string, conf *InstructionsConf) error {
	data, err := json.MarshalIndent(conf, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot serialise instructions: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("cannot write %s: %w", tmp, err)
	}

	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("cannot save %s: %w", path, err)
	}

	return nil
}
