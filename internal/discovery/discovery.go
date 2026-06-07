// Package discovery locates VaultMind vaults under a root directory so
// multi-vault commands (e.g. `doctor --all`) can operate over every vault in a
// workspace without the operator enumerating them by hand.
package discovery

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// vaultMarker is the subdirectory whose presence marks a directory as a
// VaultMind vault root. Same marker the rest of the tool keys off
// (vault.LoadConfig reads .vaultmind/config.yaml).
const vaultMarker = ".vaultmind"

// DefaultMaxDepth bounds how deep DiscoverVaults walks below the root. A small
// bound keeps discovery fast and prevents an accidental walk of an enormous
// tree (e.g. a home directory). Depth 0 is the root itself; each nested
// directory adds one. Four levels comfortably covers a workspace/group/project
// layout while staying cheap.
const DefaultMaxDepth = 4

// DiscoverVaults returns the absolute paths of every VaultMind vault found at
// or below root, in deterministic (lexically sorted) order. A directory is a
// vault when it contains a .vaultmind/ SUBDIRECTORY (a file of that name does
// not qualify). Once a vault is found, its subtree is NOT descended — a vault's
// own internals (and any nested vaults) are skipped, so each vault is reported
// exactly once. The walk stops at maxDepth levels below root.
//
// root must exist and be a directory; otherwise an error is returned (an
// unreadable or missing root is a hard failure, not a silent empty result). A
// readable root with no vaults yields an empty, non-nil-error result.
func DiscoverVaults(root string, maxDepth int) ([]string, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolving root %q: %w", root, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("reading root %q: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("root %q is not a directory", root)
	}

	var found []string
	if err := walk(abs, 0, maxDepth, &found); err != nil {
		return nil, err
	}
	sort.Strings(found)
	return found, nil
}

// walk recurses dir, appending vault roots to found. When dir is itself a
// vault, it is recorded and its subtree is pruned (a vault's internals and any
// nested vaults are never descended). Recursion stops once depth exceeds
// maxDepth. Per-directory read errors below the root are surfaced so a
// permission problem isn't silently swallowed.
func walk(dir string, depth, maxDepth int, found *[]string) error {
	if depth > maxDepth {
		return nil
	}
	if isVault(dir) {
		*found = append(*found, dir)
		return nil // prune: do not descend into a discovered vault
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("reading directory %q: %w", dir, err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if err := walk(filepath.Join(dir, e.Name()), depth+1, maxDepth, found); err != nil {
			return err
		}
	}
	return nil
}

// isVault reports whether dir contains a .vaultmind/ SUBDIRECTORY. A file named
// .vaultmind does not qualify the directory as a vault.
func isVault(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, vaultMarker))
	return err == nil && info.IsDir()
}
