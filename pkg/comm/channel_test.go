package comm

import (
	"encoding/json"
	"testing"
)

func TestSerializeDeserialize(t *testing.T) {
	msg := &BaseMessage{
		Type:      MsgHeartbeat,
		Timestamp: 1234567890,
		Version:   "1.0",
	}

	data, err := Serialize(msg)
	if err != nil {
		t.Fatalf("Failed to serialize message: %v", err)
	}

	deserialized, err := Deserialize(data)
	if err != nil {
		t.Fatalf("Failed to deserialize message: %v", err)
	}

	if deserialized.Type != msg.Type {
		t.Errorf("Expected type %s, got %s", msg.Type, deserialized.Type)
	}
	if deserialized.Version != msg.Version {
		t.Errorf("Expected version %s, got %s", msg.Version, deserialized.Version)
	}
}

func TestRegisterMessageJSON(t *testing.T) {
	msg := &RegisterMessage{
		BaseMessage: BaseMessage{
			Type:      MsgRegister,
			Timestamp: 1234567890,
			Version:   "1.0",
		},
		AgentID:  "test-agent-1",
		Hostname: "myhost.local",
		Platform: "linux/amd64",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal register message: %v", err)
	}

	var deserialized RegisterMessage
	if err := json.Unmarshal(data, &deserialized); err != nil {
		t.Fatalf("Failed to unmarshal register message: %v", err)
	}

	if deserialized.AgentID != "test-agent-1" {
		t.Errorf("Expected agent_id test-agent-1, got %s", deserialized.AgentID)
	}
	if deserialized.Hostname != "myhost.local" {
		t.Errorf("Expected hostname myhost.local, got %s", deserialized.Hostname)
	}
	if deserialized.Platform != "linux/amd64" {
		t.Errorf("Expected platform linux/amd64, got %s", deserialized.Platform)
	}
}

func TestTrafficRuleSerialization(t *testing.T) {
	rule := &TrafficRule{
		Protocol: "TCP",
		Target:   "192.168.1.100",
		Port:     445,
		Interval: 5,
		Count:    -1, // loop
		Name:     "SMB",
	}

	data, err := Serialize(rule)
	if err != nil {
		t.Fatalf("Failed to serialize traffic rule: %v", err)
	}

	var deserialized TrafficRule
	if err := json.Unmarshal(data, &deserialized); err != nil {
		t.Fatalf("Failed to unmarshal traffic rule: %v", err)
	}

	if deserialized.Protocol != rule.Protocol {
		t.Errorf("Expected protocol %s, got %s", rule.Protocol, deserialized.Protocol)
	}
	if deserialized.Target != rule.Target {
		t.Errorf("Expected target %s, got %s", rule.Target, deserialized.Target)
	}
	if deserialized.Port != rule.Port {
		t.Errorf("Expected port %d, got %d", rule.Port, deserialized.Port)
	}
	if deserialized.Interval != rule.Interval {
		t.Errorf("Expected interval %d, got %d", rule.Interval, deserialized.Interval)
	}
	if deserialized.Count != rule.Count {
		t.Errorf("Expected count %d, got %d", rule.Count, deserialized.Count)
	}
	if deserialized.Name != rule.Name {
		t.Errorf("Expected name %s, got %s", rule.Name, deserialized.Name)
	}
}

func TestHeartbeatMessageJSON(t *testing.T) {
	msg := &HeartbeatMessage{
		BaseMessage: BaseMessage{
			Type:      MsgHeartbeat,
			Timestamp: 1234567890,
			Version:   "1.0",
		},
		AgentID:     "agent-1",
		CPUUsage:    45.5,
		MemoryUsage: 1024 * 1024 * 512, // 512 MB
		ActiveRules: 3,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal heartbeat message: %v", err)
	}

	var deserialized HeartbeatMessage
	if err := json.Unmarshal(data, &deserialized); err != nil {
		t.Fatalf("Failed to unmarshal heartbeat message: %v", err)
	}

	if deserialized.AgentID != "agent-1" {
		t.Errorf("Expected agent_id agent-1, got %s", deserialized.AgentID)
	}
	if deserialized.CPUUsage != 45.5 {
		t.Errorf("Expected cpu_usage 45.5%%, got %.2f%%", deserialized.CPUUsage)
	}
	if deserialized.MemoryUsage != 536870912 {
		t.Errorf("Expected memory %d bytes, got %d bytes", msg.MemoryUsage, deserialized.MemoryUsage)
	}
	if deserialized.ActiveRules != 3 {
		t.Errorf("Expected active rules 3, got %d", deserialized.ActiveRules)
	}

	// Print JSON for debugging
	t.Logf("Heartbeat JSON: %s", string(data))
}

func TestStatusMessageJSON(t *testing.T) {
	msg := &StatusMessage{
		BaseMessage: BaseMessage{
			Type:      MsgStatus,
			Timestamp: 1234567890,
			Version:   "1.0",
		},
		AgentID:         "agent-1",
		State:           "generating",
		ActiveConnections: 10,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal status message: %v", err)
	}

	var deserialized StatusMessage
	if err := json.Unmarshal(data, &deserialized); err != nil {
		t.Fatalf("Failed to unmarshal status message: %v", err)
	}

	if deserialized.AgentID != "agent-1" {
		t.Errorf("Expected agent_id agent-1, got %s", deserialized.AgentID)
	}
	if deserialized.State != "generating" {
		t.Errorf("Expected state generating, got %s", deserialized.State)
	}
	if deserialized.ActiveConnections != 10 {
		t.Errorf("Expected active connections 10, got %d", deserialized.ActiveConnections)
	}

	t.Logf("Status JSON: %s", string(data))
}

func TestErrorMessageJSON(t *testing.T) {
	msg := &ErrorMessage{
		BaseMessage: BaseMessage{
			Type:      MsgError,
			Timestamp: 1234567890,
			Version:   "1.0",
		},
		Code:    "CONFIG_INVALID",
		Message: "Invalid configuration file format",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal error message: %v", err)
	}

	var deserialized ErrorMessage
	if err := json.Unmarshal(data, &deserialized); err != nil {
		t.Fatalf("Failed to unmarshal error message: %v", err)
	}

	if deserialized.Code != "CONFIG_INVALID" {
		t.Errorf("Expected code CONFIG_INVALID, got %s", deserialized.Code)
	}
	if deserialized.Message != "Invalid configuration file format" {
		t.Errorf("Expected message 'Invalid configuration file format', got %s", deserialized.Message)
	}

	t.Logf("Error JSON: %s", string(data))
}
