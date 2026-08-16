// Package vault provides vault scanning and configuration loading.
package vault

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ConfigRelPath is the vault's type registry, relative to the vault root, in
// slash form. Its presence is what makes a directory a vault.
//
// The marker used to be the .vaultmind/ directory alone, which is wrong in the
// one place it matters most: the model cache lives at ~/.vaultmind/models, so on
// any machine that has downloaded BGE-M3 weights a bare-directory test answers
// "yes" for the home directory. A registry is written by `init` (or by `index`
// when a vault is explicitly named); a cache never carries one.
const ConfigRelPath = ".vaultmind/config.yaml"

// Config represents the full .vaultmind/config.yaml
type Config struct {
	Vault  VaultConfig        `yaml:"vault"`
	Types  map[string]TypeDef `yaml:"types"`
	Git    GitPolicyConfig    `yaml:"git"`
	Index  IndexConfig        `yaml:"index"`
	Memory MemoryConfig       `yaml:"memory"`
	Schema SchemaConfig       `yaml:"schema"`
}

// SchemaConfig holds per-vault schema settings beyond the type registry.
// Aliases let migrating users keep their existing frontmatter field names
// (e.g. `last_updated`) while vaultmind validates against canonical names
// (e.g. `updated`). The map is canonical → list of aliases. Aliasing is
// non-destructive: vaultmind never rewrites frontmatter to normalize field
// names; the alias and the canonical are equivalent at validation only.
type SchemaConfig struct {
	Aliases map[string][]string `yaml:"aliases"`
}

// VaultConfig holds vault scanning settings.
type VaultConfig struct {
	Exclude []string `yaml:"exclude"`
}

// TypeDef defines a note type in the registry.
type TypeDef struct {
	Required []string `yaml:"required" json:"required"`
	Optional []string `yaml:"optional" json:"optional"`
	Statuses []string `yaml:"statuses" json:"statuses"`
	Template string   `yaml:"template" json:"template"`
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

// defaultExcludes applies when a vault has no config (or an empty exclude list).
// "README.md" is excluded as a file basename — a vault's own README is meta, not
// a knowledge note; indexing it pollutes every query with a blank-titled hit.
// "episodes" is excluded because captured session transcripts are raw material for
// arc distillation, not retrieval targets — large, and their signal lives in the
// arcs distilled from them; embedding them would dominate index cost and noise.
var defaultExcludes = []string{".git", ".obsidian", ".trash", "node_modules", "README.md", "episodes"}

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

	configPath := filepath.Join(vaultRoot, filepath.FromSlash(ConfigRelPath))
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
