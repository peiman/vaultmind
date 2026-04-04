// Package cmdutil provides shared helpers for CLI command implementations.
package cmdutil

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"

	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/schema"
	"github.com/peiman/vaultmind/internal/vault"
)

// VaultDB bundles the commonly needed vault resources.
type VaultDB struct {
	DB     *index.DB
	Config *vault.Config
	Reg    *schema.Registry
}

// Close releases the database connection.
func (v *VaultDB) Close() {
	if v.DB != nil {
		_ = v.DB.Close()
	}
}

// OpenVaultDB loads config, opens the index DB, and creates the type registry.
func OpenVaultDB(vaultPath string) (*VaultDB, error) {
	cfg, err := vault.LoadConfig(vaultPath)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	dbPath := filepath.Join(vaultPath, cfg.Index.DBPath)
	db, err := index.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening index: %w", err)
	}

	return &VaultDB{
		DB:     db,
		Config: cfg,
		Reg:    schema.NewRegistry(cfg.Types),
	}, nil
}

// WriteJSON writes a JSON envelope to the writer.
func WriteJSON(w io.Writer, command string, result interface{}, vaultPath string) error {
	env := envelope.OK(command, result)
	env.Meta.VaultPath = vaultPath
	return json.NewEncoder(w).Encode(env)
}

// WriteJSONError writes a JSON error envelope to the writer.
func WriteJSONError(w io.Writer, command, code, message string) error {
	env := envelope.Error(command, code, message, "")
	return json.NewEncoder(w).Encode(env)
}
