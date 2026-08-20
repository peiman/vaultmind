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
//
// vaultRoot is resolved to an absolute path FIRST. The containment test is a
// string-prefix comparison, and between two relative paths that comparison is
// simply wrong: filepath.Join collapses the leading "./" the prefix depends on.
//
//	ResolveInside(".", ".vaultmind/index.db")
//	  cleanVault = "."
//	  cleanAbs   = ".vaultmind/index.db"        <- Join dropped the "./"
//	  HasPrefix(".vaultmind/index.db", "./")    <- false, so: refused
//
// A path plainly inside the vault read as an escape. Live effect: every command
// failed when the vault was named as `.`, while OMITTING the flag — which means
// the same directory — worked, because that path reaches here already absolute.
//
// This is not a loosening. Resolving first makes the prefix test meaningful
// rather than accidental, and `..` traversal out of a relative root was refused
// before and still is (see TestResolveInside_RelativeRoot). It also makes the
// function keep the promise in its own first line, which said "absolute" while
// returning a relative path for a relative root — and every caller names the
// result absPath or dbPath.
func ResolveInside(vaultRoot, rel string) (string, error) {
	cleanVault, err := filepath.Abs(vaultRoot)
	if err != nil {
		return "", fmt.Errorf("resolving vault root %q: %w", vaultRoot, err)
	}
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
