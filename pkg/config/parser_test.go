package config

import (
	"os"
	"testing"
)

func TestParseMasterArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{
			name: "valid arguments",
			args: []string{"--port=9000", "--psk=testkey123", "--config=/etc/config.conf"},
			wantErr: false,
		},
		{
			name:    "missing port without config",
			args:    []string{"--psk=testkey"},
			wantErr: true,
		},
		{
			name:    "missing psk without config",
			args:    []string{"--port=9000"},
			wantErr: true,
		},
		{
			name:    "invalid port number",
			args:    []string{"--port=99999", "--psk=testkey", "--config=/etc/config.conf"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ParseMasterArgs(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseMasterArgs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && cfg == nil {
				t.Error("Expected config but got nil")
			}
		})
	}
}

func TestParseAgentArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{
			name: "valid arguments",
			args: []string{"--master=192.168.1.1", "--port=9000", "--psk=testkey", "--id=myagent"},
			wantErr: false,
		},
		{
			name:    "missing master",
			args:    []string{"--port=9000", "--psk=testkey"},
			wantErr: true,
		},
		{
			name:    "invalid port",
			args:    []string{"--master=192.168.1.1", "--port=-5", "--psk=testkey", "--id=myagent"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ParseAgentArgs(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseAgentArgs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && cfg == nil {
				t.Error("Expected config but got nil")
			}
		})
	}
}

func TestLoadTrafficRules(t *testing.T) {
	// Create a temporary test file
	tmpFile, err := os.CreateTemp("", "traffic-test-*.conf")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	testContent := `# Test configuration
TARGET1=10.0.0.1
TARGET2=10.0.0.2
TCP     TARGET1     445     5       loop    # SMB traffic
UDP     TARGET2     53      2       loop    # DNS queries
TCP     192.168.1.100   80      10      100     # HTTP to IP

# Empty lines and comments should be ignored
`
	if _, err := tmpFile.WriteString(testContent); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	rules, _, err := LoadTrafficRules(tmpFile.Name())
	if err != nil {
		t.Errorf("LoadTrafficRules() error = %v", err)
		return
	}

	if len(rules) != 3 {
		t.Errorf("Expected 3 rules, got %d", len(rules))
	}

	// Verify first rule
	if rules[0].Protocol != "TCP" || rules[0].Port != 445 || rules[0].Count != -1 {
		t.Errorf("First rule incorrect: %+v", rules[0])
	}

	// Verify third rule (IP address)
	if rules[2].Target != "192.168.1.100" || rules[2].Port != 80 || rules[2].Count != 100 {
		t.Errorf("Third rule incorrect: %+v", rules[2])
	}
}

func TestParseTrafficLine(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		wantErr bool
	}{
		{
			name: "valid TCP line",
			line: "TCP TARGET1 445 5 loop # SMB",
			wantErr: false,
		},
		{
			name: "valid UDP line with IP",
			line: "UDP 192.168.1.100 53 2 loop",
			wantErr: false,
		},
		{
			name:    "invalid protocol",
			line:    "HTTP TARGET1 80 5 loop",
			wantErr: true,
		},
		{
			name:    "invalid port",
			line:    "TCP TARGET1 99999 5 loop",
			wantErr: true,
		},
		{
			name:    "missing fields",
			line:    "TCP TARGET1",
			wantErr: true,
		},
	}

	targetMap := map[string]string{
		"TARGET1": "192.168.1.10",
		"TARGET2": "192.168.1.20",
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule, err := parseTrafficLine(tt.line, targetMap)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseTrafficLine() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && rule == nil {
				t.Error("Expected rule but got nil")
			}
		})
	}
}
