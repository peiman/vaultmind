// Package cmdutil provides shared helpers for CLI command implementations.
package cmdutil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/schema"
	"github.com/peiman/vaultmind/internal/vault"
)

// VaultDB bundles the commonly needed vault resources.
type VaultDB struct {
	DB        *index.DB
	Config    *vault.Config
	Reg       *schema.Registry
	dbPath    string
	indexHash string
}

// Close releases the database connection.
func (v *VaultDB) Close() {
	if v.DB != nil {
		_ = v.DB.Close()
	}
}

// GetIndexHash returns the cached SHA-256 hash of the SQLite database file.
func (v *VaultDB) GetIndexHash() string {
	return v.indexHash
}

// OpenVaultDB loads config, opens the index DB, and creates the type registry.
func OpenVaultDB(vaultPath string) (*VaultDB, error) {
	info, err := os.Stat(vaultPath)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("vault path %q does not exist or is not a directory", vaultPath)
	}

	cfg, err := vault.LoadConfig(vaultPath)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	dbPath := filepath.Join(vaultPath, cfg.Index.DBPath)
	db, err := index.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening index: %w", err)
	}

	vdb := &VaultDB{
		DB:     db,
		Config: cfg,
		Reg:    schema.NewRegistry(cfg.Types),
		dbPath: dbPath,
	}
	vdb.indexHash = vdb.IndexHash()
	return vdb, nil
}

// IndexHash computes the SHA-256 hash of the SQLite database file.
// Uses streaming hash to avoid loading the entire file into memory.
func (v *VaultDB) IndexHash() string {
	f, err := os.Open(v.dbPath)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// WriteJSON writes a JSON envelope to the writer.
func WriteJSON(w io.Writer, command string, result interface{}, vaultPath, indexHash string) error {
	env := envelope.OK(command, result)
	env.Meta.VaultPath = vaultPath
	env.Meta.IndexHash = indexHash
	return json.NewEncoder(w).Encode(env)
}

// WriteJSONError writes a JSON error envelope to the writer.
func WriteJSONError(w io.Writer, command, code, message string) error {
	env := envelope.Error(command, code, message, "")
	return json.NewEncoder(w).Encode(env)
}
