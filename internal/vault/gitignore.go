package vault

import (
	"fmt"
	"os"
	"path/filepath"
)

// Ignore all derived state, but keep the registry, user-authored section
// templates, project hook setup, and the vault's telemetry fingerprint
// (telemetry.EnsureFingerprint: the vault carries its own identity) available
// to git. Hook files can share this directory when the vault is also the
// project root.
const stateGitignore = "*\n!.gitignore\n!config.yaml\n!fingerprint.txt\n!sections/\n!sections/**\n!scripts/\n!scripts/**\n!vaultmind-profile\n"

// EnsureGitignore creates the vault's state directory and its default ignore
// policy. An existing policy belongs to the user and is never overwritten.
func EnsureGitignore(vaultRoot string) error {
	dir := filepath.Join(vaultRoot, ".vaultmind")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create vault state directory: %w", err)
	}
	// #nosec G304 -- operator-supplied vault root, fixed filename; O_EXCL preserves existing files and symlinks.
	f, err := os.OpenFile(filepath.Join(dir, ".gitignore"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if os.IsExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("create vault .gitignore: %w", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(stateGitignore); err != nil {
		return fmt.Errorf("write vault .gitignore: %w", err)
	}
	return nil
}
