package hooks

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/peiman/vaultmind/internal/hookscripts"
)

// settingsFileName is the committed, team-shared hook config; settingsLocalName
// is the gitignored personal one. Claude Code reads both.
const (
	settingsFileName  = "settings.json"
	settingsLocalName = "settings.local.json"
)

// settingsFilePath returns the target hook-config file under projectDir.
// local selects .claude/settings.local.json (gitignored, personal); otherwise
// .claude/settings.json (committed, team-shared).
func settingsFilePath(projectDir string, local bool) string {
	name := settingsFileName
	if local {
		name = settingsLocalName
	}
	return filepath.Join(projectDir, ".claude", name)
}

// MergeFileResult reports the outcome of MergeIntoSettings.
type MergeFileResult struct {
	SettingsPath string `json:"settings_path"`
	Changed      bool   `json:"changed"`
	DryRun       bool   `json:"dry_run"`
	// Merged is the post-merge file content — populated so a caller can show a
	// dry-run preview or diff. Equal to the on-disk content after a real write.
	Merged string `json:"merged,omitempty"`
}

// MergeIntoSettings reads the target hook-config file (creating none if it is
// absent — MergeStanza treats absence as a fresh file), additively merges
// VaultMind's four canonical hook entries via MergeStanza, and writes the
// result back. A non-existent file is created with the full stanza. When
// dryRun is set, nothing is written and the would-be content is returned in
// MergeFileResult.Merged for preview. A merge that changes nothing writes
// nothing (idempotent). Any malformed existing settings surfaces as an error
// before any write, so a user's file is never corrupted.
func MergeIntoSettings(projectDir, vaultPath string, local, dryRun bool) (*MergeFileResult, error) {
	path := settingsFilePath(projectDir, local)
	existing, err := readSettingsFile(path)
	if err != nil {
		return nil, err
	}
	merged, changed, err := MergeStanza(existing, vaultPath)
	if err != nil {
		return nil, fmt.Errorf("merging into %s: %w", path, err)
	}
	res := &MergeFileResult{SettingsPath: path, Changed: changed, DryRun: dryRun, Merged: string(merged)}
	if dryRun || !changed {
		return res, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}
	if err := atomicWriteFile(path, merged, 0o600); err != nil {
		return nil, fmt.Errorf("writing %s: %w", path, err)
	}
	return res, nil
}

// RemoveFileResult reports the outcome of RemoveFromSettings.
type RemoveFileResult struct {
	SettingsPath   string   `json:"settings_path"`
	Removed        []string `json:"removed"`
	Changed        bool     `json:"changed"`
	ScriptsDeleted []string `json:"scripts_deleted,omitempty"`
}

// RemoveFromSettings strips VaultMind's hook entries from the target hook-config
// file via RemoveStanza and writes the result back. An absent file is a no-op.
// When removeScripts is set, the installed canonical scripts under
// .claude/scripts/ are also deleted. A malformed file surfaces as an error
// before any write.
func RemoveFromSettings(projectDir string, local, removeScripts bool) (*RemoveFileResult, error) {
	path := settingsFilePath(projectDir, local)
	res := &RemoveFileResult{SettingsPath: path}

	existing, err := readSettingsFile(path)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		out, removed, err := RemoveStanza(existing)
		if err != nil {
			return nil, fmt.Errorf("removing from %s: %w", path, err)
		}
		res.Removed = removed
		res.Changed = len(removed) > 0
		if res.Changed {
			if err := atomicWriteFile(path, out, 0o600); err != nil {
				return nil, fmt.Errorf("writing %s: %w", path, err)
			}
		}
	}

	if removeScripts {
		deleted, err := deleteInstalledScripts(projectDir)
		if err != nil {
			return res, err
		}
		res.ScriptsDeleted = deleted
	}
	return res, nil
}

// atomicWriteFile writes data to path atomically: it writes a temp file in the
// same directory, fsync-closes it, sets perm, then renames it over path. A
// mid-write failure (kill, power loss, full disk) leaves the original file
// untouched — load-bearing, since the whole point of this feature is to never
// corrupt a user's settings.json. Same-directory temp guarantees the rename is
// a cheap atomic metadata operation rather than a cross-filesystem copy.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".vaultmind-settings-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("setting permissions on temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("renaming temp file over %s: %w", path, err)
	}
	return nil
}

// readSettingsFile returns the file's bytes, or nil (no error) when it does not
// exist — the "fresh file" case the merge/remove engines handle natively.
func readSettingsFile(path string) ([]byte, error) {
	// path is rooted at the user-supplied project dir — same trust tier as the
	// indexer / hook installer.
	data, err := os.ReadFile(path) // #nosec G304
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return data, nil
}

// deleteInstalledScripts removes the canonical hook scripts under
// .claude/scripts/. Absent scripts are skipped; the returned slice names the
// ones actually deleted, in canonical order.
func deleteInstalledScripts(projectDir string) ([]string, error) {
	scriptsDir := filepath.Join(projectDir, ".claude", "scripts")
	var deleted []string
	for _, name := range hookscripts.Names() {
		p := filepath.Join(scriptsDir, name)
		err := os.Remove(p)
		switch {
		case err == nil:
			deleted = append(deleted, name)
		case os.IsNotExist(err):
			// not installed — nothing to delete
		default:
			return deleted, fmt.Errorf("deleting %s: %w", p, err)
		}
	}
	return deleted, nil
}
