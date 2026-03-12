package config

import "fmt"

// LoadConfigSmart tries the extended SOURCE/DEST format first, then falls back to legacy format.
func LoadConfigSmart(filePath string) (*MasterConfig, error) {
	// Try extended format first
	extCfg, err := ParseExtendedConfig(filePath)
	if err == nil && len(extCfg.Rules) > 0 {
		// Convert extended rules to legacy TrafficRules for compatibility
		rules := make([]*TrafficRule, 0, len(extCfg.Rules))
		for _, rule := range extCfg.Rules {
			rules = append(rules, &TrafficRule{
				Protocol: rule.Protocol,
				Target:   rule.Dest, // Use DEST as target for legacy compatibility
				Port:     rule.Port,
				Count:    rule.Count,
				Name:     fmt.Sprintf("%s -> %s", rule.Source, rule.Name),
			})
		}

		return &MasterConfig{
			Port:         extCfg.Port,
			PSK:          extCfg.PSK,
			ConfigPath:   filePath,
			TrafficRules: rules,
			TargetMap:    extCfg.TargetMap,
		}, nil
	}

	// Fall back to legacy format parsing
	rules, _, err := LoadTrafficRules(filePath)
	if err != nil {
		return nil, err
	}

	return &MasterConfig{
		ConfigPath:   filePath,
		TrafficRules: rules,
	}, nil
}
