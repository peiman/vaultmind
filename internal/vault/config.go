// Package vault provides vault scanning and configuration loading.
package vault

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the full .vaultmind/config.yaml
type Config struct {
	Vault  VaultConfig        `yaml:"vault"`
	Types  map[string]TypeDef `yaml:"types"`
	Git    GitPolicyConfig    `yaml:"git"`
	Index  IndexConfig        `yaml:"index"`
	Memory MemoryConfig       `yaml:"memory"`
}

// VaultConfig holds vault scanning settings.
type VaultConfig struct {
	Exclude []string `yaml:"exclude"`
}

// TypeDef defines a note type in the registry.
type TypeDef struct {
	Required []string `yaml:"required"`
	Optional []string `yaml:"optional"`
	Statuses []string `yaml:"statuses"`
	Template string   `yaml:"template"`
}

// GitPolicyConfig holds git policy overrides.
type GitPolicyConfig struct {
	Policy map[string]string `yaml:"policy"`
}

// IndexConfig holds indexing settings.
type IndexConfig struct {
	DBPath string `yaml:"db_path"`
}

// MemoryConfig holds memory engine settings.
type MemoryConfig struct {
	AliasMinLength           int     `yaml:"alias_min_length"`
	TagOverlapThreshold      float64 `yaml:"tag_overlap_threshold"`
	ContextPackDefaultBudget int     `yaml:"context_pack_default_budget"`
}

var defaultExcludes = []string{".git", ".obsidian", ".trash", "node_modules"}

// LoadConfig reads .vaultmind/config.yaml from the vault root.
// Returns defaults if the config file doesn't exist.
func LoadConfig(vaultRoot string) (*Config, error) {
	cfg := &Config{
		Vault: VaultConfig{Exclude: append([]string{}, defaultExcludes...)},
		Types: make(map[string]TypeDef),
		Index: IndexConfig{DBPath: ".vaultmind/index.db"},
		Memory: MemoryConfig{
			AliasMinLength:           3,
			TagOverlapThreshold:      1.0,
			ContextPackDefaultBudget: 4096,
		},
	}

	configPath := filepath.Join(vaultRoot, ".vaultmind", "config.yaml")
	cleanPath := filepath.Clean(configPath)

	data, err := os.ReadFile(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", cleanPath, err)
	}

	if len(cfg.Vault.Exclude) == 0 {
		cfg.Vault.Exclude = append([]string{}, defaultExcludes...)
	}

	return cfg, nil
}
