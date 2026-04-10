// ckeletin:allow-custom-command
package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// persistTelemetryChoice writes the user's telemetry preference to the config
// file at configPath. If the file already exists, it preserves all existing
// settings. If it doesn't exist, it creates a minimal config with just the
// telemetry key.
func persistTelemetryChoice(telemetry, configPath string) error {
	configPath = filepath.Clean(configPath)
	cfg := make(map[string]any)

	if existing, err := os.ReadFile(configPath); err == nil { //nolint:gosec // path comes from viper.ConfigFileUsed(), not user input
		if err := yaml.Unmarshal(existing, &cfg); err != nil {
			return fmt.Errorf("parse existing config: %w", err)
		}
	}

	experiments, ok := cfg["experiments"].(map[string]any)
	if !ok {
		experiments = make(map[string]any)
	}
	experiments["telemetry"] = telemetry
	cfg["experiments"] = experiments

	out, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return os.WriteFile(configPath, out, 0o600)
}
