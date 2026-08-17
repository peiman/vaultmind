package vault

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// ErrEscapesVault reports a path that resolves outside the vault root.
//
// A sentinel rather than a typed error because the callers have incompatible
// contracts: plan and mutation must answer with their own `path_traversal`
// error codes, index and cmdutil return plain errors, and a shared error type
// would force one of them to leak the other's vocabulary. errors.Is is the
// whole interface.
var ErrEscapesVault = errors.New("path escapes vault")

// ResolveInside joins rel onto vaultRoot and returns the cleaned absolute path,
// or ErrEscapesVault if the result would land outside the vault.
//
// This is the CONFINEMENT check, and it is deliberately not the symlink check
// (see SkipSymlink). They answer different questions and neither implies the
// other: `notes.md -> ~/.zshrc` passes this function — notes.md really is
// inside the vault — and writing to it still lands outside. Anything that opens
// a path built from vault content or vault config wants both.
//
// What it catches is `..` traversal. `filepath.Join` re-roots an absolute
// second argument inside the first (Join("/v", "/etc/passwd") is
// "/v/etc/passwd"), so an absolute rel is contained rather than escaping — but
// it is also silently not the path the operator wrote, which is its own
// problem; reject those at the point they are configured, not here.
//
// The vault root itself is allowed: some callers resolve "." to mean the root.
func ResolveInside(vaultRoot, rel string) (string, error) {
	cleanVault := filepath.Clean(vaultRoot)
	cleanAbs := filepath.Clean(filepath.Join(cleanVault, rel))

	if cleanAbs != cleanVault && !strings.HasPrefix(cleanAbs, cleanVault+string(filepath.Separator)) {
		return "", fmt.Errorf("%q: %w", rel, ErrEscapesVault)
	}
	return cleanAbs, nil
}

// ValidSegment reports whether s is safe to use as a SINGLE path segment —
// letters, digits, underscore, dash, and nothing else.
//
// For values that name a directory or file component rather than a path:
// a note's `type` from its own frontmatter, a section key from its markers.
// Those are attacker-controlled in any vault you did not write yourself, and
// they get joined into a path. ResolveInside would catch the resulting escape,
// but a whitelist is the better answer one level up: a type called
// "../../../etc" is not a legal type in the first place, whatever it resolves
// to, and rejecting it names the real problem instead of a path error.
func ValidSegment(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}
