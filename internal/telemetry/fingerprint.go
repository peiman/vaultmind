package telemetry

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FingerprintFile is the relative path within a vault where the
// anonymous per-vault identifier lives.
const FingerprintFile = ".vaultmind/fingerprint.txt"

// EnsureFingerprint returns the vault's anonymous fingerprint, creating
// it on first call. The fingerprint is a 128-bit random hex string
// stored at <vaultPath>/.vaultmind/fingerprint.txt. It is NOT derivable
// from vault content — it's a stable, opaque identifier the federated
// telemetry pipeline uses to group records by vault without learning
// anything about the vault's contents or the user's filesystem.
//
// The fingerprint is per-vault, not per-machine. If a vault is shared
// across machines (committed to git), all machines contribute under
// the same fingerprint. If the user wants per-machine fingerprints,
// they should gitignore .vaultmind/fingerprint.txt — the default
// behavior is "vault carries its own identity."
//
// The file is created with mode 0o600 to keep curious file-system
// scanners from picking it up casually; it's not a secret, but it's
// also not meant to be world-readable.
func EnsureFingerprint(vaultPath string) (string, error) {
	cleanPath := filepath.Clean(vaultPath)
	// nosemgrep: go-path-traversal -- fpPath is derived from a vault path
	// supplied by the operator, not from external user input. Cleaning
	// happens above; the literal FingerprintFile suffix is a constant.
	fpPath := filepath.Clean(filepath.Join(cleanPath, FingerprintFile))

	// #nosec G304 -- fpPath is derived from caller-controlled vaultPath,
	// not user input.
	// nosemgrep: go-path-traversal
	if existing, err := os.ReadFile(fpPath); err == nil {
		fp := strings.TrimSpace(string(existing))
		if isValidFingerprint(fp) {
			return fp, nil
		}
		// File exists but contents are corrupt — overwrite rather than
		// silently propagating garbage to the telemetry pipeline.
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("read fingerprint file: %w", err)
	}

	fp, err := newFingerprint()
	if err != nil {
		return "", fmt.Errorf("generate fingerprint: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(fpPath), 0o750); err != nil {
		return "", fmt.Errorf("create .vaultmind dir: %w", err)
	}
	if err := os.WriteFile(fpPath, []byte(fp+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("write fingerprint file: %w", err)
	}
	return fp, nil
}

// newFingerprint generates a 128-bit random hex string.
func newFingerprint() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// isValidFingerprint validates that a fingerprint is a 32-character
// lowercase hex string. Used on read to guard against corrupted files.
func isValidFingerprint(s string) bool {
	if len(s) != 32 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
