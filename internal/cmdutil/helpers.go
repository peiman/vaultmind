// Package cmdutil provides shared helpers for CLI command implementations.
package cmdutil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/schema"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/spf13/cobra"
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

// LoadRegistry loads only the type registry from a vault's .vaultmind/config.yaml
// without opening the index database. Use this when you need schema information
// (e.g., for live validation) but not the index itself.
func LoadRegistry(vaultPath string) (*schema.Registry, error) {
	info, err := os.Stat(vaultPath)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("vault path %q does not exist or is not a directory", vaultPath)
	}
	cfg, err := vault.LoadConfig(vaultPath)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}
	return schema.NewRegistryWithAliases(cfg.Types, cfg.Schema.Aliases), nil
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
		Reg:    schema.NewRegistryWithAliases(cfg.Types, cfg.Schema.Aliases),
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
// It returns ErrAlreadyWritten on success. Every caller does
// `return cmdutil.WriteJSONError(...)`, and while this returned nil on success
// each of those was a command that wrote status "error" and then exited 0 —
// so `vaultmind … --json || handle_failure` never fired. Returning the sentinel
// from here fixes all of them at once and, more to the point, makes the broken
// combination unexpressible rather than merely absent.
func WriteJSONError(w io.Writer, command, code, message string) error {
	return envelope.WriteError(w, envelope.Error(command, code, message, ""))
}

// ErrAlreadyWritten signals that an error envelope was already written. Aliased
// to the envelope package's sentinel so the command layer and the query layer
// raise the same one and main.go needs to recognize only that.
var ErrAlreadyWritten = envelope.ErrAlreadyWritten

func isJSONOutput(cmd *cobra.Command) bool {
	jsonFlag, _ := cmd.Flags().GetBool("json")
	return jsonFlag
}

// classifyVaultError returns a specific error code based on the error message.
// OpenVaultDB wraps errors with fmt.Errorf (not %w), so the original syscall
// error is lost — classification is by string matching on the message.
func classifyVaultError(err error) string {
	msg := err.Error()
	if strings.Contains(msg, "does not exist or is not a directory") {
		return "vault_not_found"
	}
	if strings.Contains(msg, "loading config") {
		return "config_error"
	}
	if strings.Contains(msg, "locked") || strings.Contains(msg, "SQLITE_BUSY") {
		return "database_locked"
	}
	return "vault_error"
}

// vaultMarkerDir is the subdirectory whose presence marks a directory as a
// vault root — the same marker discovery walks up looking for.
const vaultMarkerDir = ".vaultmind"

// vaultEnvVar mirrors cmd.vaultEnvVar. Duplicated rather than imported because
// internal/cmdutil cannot import cmd (which imports it).
const vaultEnvVar = "VAULTMIND_VAULT"

// errGuessedNonVault reports that no vault was named and the guessed path is
// not one. It names both ways out, because a caller in this state either has a
// vault elsewhere or has none at all.
func errGuessedNonVault(vaultPath string) error {
	reason := fmt.Sprintf("no %s", vault.ConfigRelPath)
	// A bare .vaultmind/ here is almost always the tool's own model cache —
	// ~/.vaultmind/models — which used to satisfy the marker check and let the
	// home directory be indexed as a vault. Say so, or the message reads as
	// "your vault is broken" to someone whose home is simply not a vault.
	if _, err := os.Stat(filepath.Join(vaultPath, vaultMarkerDir)); err == nil {
		reason = fmt.Sprintf("it has a %s/ directory but no %s — that is a cache, not a vault",
			vaultMarkerDir, vault.ConfigRelPath)
	}
	return fmt.Errorf(
		"no vault found: %q is not a vault (%s) and none was specified.\n"+
			"  Point at an existing vault:  --vault <path>  (or set %s)\n"+
			"  Create one here:             vaultmind init %s",
		vaultPath, reason, vaultEnvVar, vaultPath)
}

// vaultWasNamed reports whether the caller deliberately chose this vault, via
// the --vault flag or VAULTMIND_VAULT, rather than having it guessed for them
// by discovery's fallback.
func vaultWasNamed(cmd *cobra.Command) bool {
	if cmd.Flags().Changed("vault") {
		return true
	}
	if strings.TrimSpace(os.Getenv(vaultEnvVar)) != "" {
		return true
	}
	// A command with no --vault flag at all never went through discovery, so
	// its path came from its own logic; treat it as named.
	return cmd.Flags().Lookup("vault") == nil
}

// IsVaultRoot reports whether dir is a vault: it must hold the type registry at
// .vaultmind/config.yaml, not merely a .vaultmind/ directory.
//
// The bare-directory test was wrong in the one place it mattered most. The tool
// keeps its model cache at ~/.vaultmind/models, so on any machine that has
// downloaded BGE-M3 weights the marker exists in the home directory — and the
// guessed-vault guard, whose whole job is to refuse a directory the user never
// chose, waved $HOME through and indexed it. Walk-up skips $HOME; the fallback
// to "." did not. A registry is written by `init` (or by `index` when a vault is
// named), never by a cache.
//
// Exported so callers that must require a REAL vault even when the path was
// named — a propose-only reader, say — can ask without spelling the path.
func IsVaultRoot(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(vault.ConfigRelPath)))
	return err == nil && !info.IsDir()
}

// RequireRealVaultIfGuessed enforces the guessed-vs-named rule on its own, for
// commands that act on a vault path without going through OpenVaultDBOrWriteErr.
// With --json set it writes the error envelope and returns ErrAlreadyWritten.
//
// It exists because guarding only the readers left the writer open. `index` is
// the command that CREATES .vaultmind/index.db — the marker every later walk-up
// discovers — so a guess there does not merely answer from the wrong vault once:
// it promotes the directory permanently and captures every future invocation
// from any child of it. The rule was written for `ask` in v0.3.0 and applied
// again to `arc candidates` in v0.4.0, both times without reaching the command
// that mints the marker.
func RequireRealVaultIfGuessed(cmd *cobra.Command, vaultPath, commandName string) error {
	if vaultWasNamed(cmd) || IsVaultRoot(vaultPath) {
		return nil
	}
	err := errGuessedNonVault(vaultPath)
	if isJSONOutput(cmd) {
		_ = WriteJSONError(cmd.OutOrStdout(), commandName, "vault_not_found", err.Error())
		return ErrAlreadyWritten
	}
	return err
}

// OpenVaultDBOrWriteErr opens the vault DB. On failure with --json set,
// writes a JSON error envelope and returns ErrAlreadyWritten.
//
// When no vault was named — no --vault, no VAULTMIND_VAULT — the path is
// whatever discovery fell back to, normally the working directory. Opening a
// non-vault directory in that case is wrong twice over: the command answers
// from a vault the user never chose (reporting "ok" and zero hits, which reads
// as "your vault has nothing on this" rather than "you have no vault"), and
// OpenVaultDB CREATES .vaultmind/index.db there on the way — quietly promoting
// that directory to a vault that every future walk-up will find. So a guessed
// path must prove it is already a vault before it is opened, while a named one
// keeps the permissive behaviour that lets any directory serve as a vault.
func OpenVaultDBOrWriteErr(cmd *cobra.Command, vaultPath, commandName string) (*VaultDB, error) {
	if err := RequireRealVaultIfGuessed(cmd, vaultPath, commandName); err != nil {
		return nil, err
	}

	vdb, err := OpenVaultDB(vaultPath)
	if err != nil {
		if isJSONOutput(cmd) {
			code := classifyVaultError(err)
			_ = WriteJSONError(cmd.OutOrStdout(), commandName, code, err.Error())
			return nil, ErrAlreadyWritten
		}
		return nil, err
	}
	return vdb, nil
}
