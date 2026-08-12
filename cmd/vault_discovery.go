package cmd

import (
	"os"
	"path/filepath"
	"strings"
)

// vaultEnvVar is the environment variable a consumer can set to point every
// vaultmind command (and the hook scripts) at a vault without passing --vault.
const vaultEnvVar = "VAULTMIND_VAULT"

// discoverVaultPath resolves a vault path when the caller gave no explicit one
// (no --vault flag, config left at the default). Priority:
//
//  1. VAULTMIND_VAULT environment variable (a deliberate override; matches the
//     variable the installed hook scripts already honor).
//  2. The nearest ancestor directory containing a .vaultmind/ folder, found by
//     walking up from startDir (like git locating .git/) — except $HOME, which
//     is never auto-selected. See walkUpForVault.
//  3. fallback (the flag/config default, normally ".").
//
// getenv, startDir, ceiling, and home are injected so the resolution is
// unit-testable without touching the process environment, working directory,
// or the filesystem above the test's temp dir. ceiling="" walks to the
// filesystem root (production); a non-empty ceiling bounds the upward walk.
// home="" means no home directory is known, which disables the home guard.
func discoverVaultPath(fallback string, getenv func(string) string, startDir, ceiling, home string) string {
	if env := strings.TrimSpace(getenv(vaultEnvVar)); env != "" {
		return env
	}
	if found := walkUpForVault(startDir, ceiling, home); found != "" {
		return found
	}
	return fallback
}

// walkUpForVault walks from startDir toward the filesystem root, returning the
// first directory that contains a .vaultmind/ subdirectory (the vault root), or
// "" if none is found. The ceiling directory is the highest one inspected
// (inclusive); ceiling="" walks all the way to the root.
//
// home, when non-empty, is never returned. A .vaultmind/ sitting directly in
// $HOME is nearly always debris from one errant command run there, and because
// this walk reaches the filesystem root that single stray marker would
// otherwise capture every invocation made anywhere beneath $HOME — silently
// answering from the wrong vault, and aiming `index` at the user's entire home
// directory. The walk CONTINUES past $HOME rather than stopping, so the guard
// costs nothing in the ordinary case and a deliberate vault above $HOME still
// resolves. Naming $HOME explicitly (--vault or VAULTMIND_VAULT) still works:
// this blocks the accident, not the choice.
func walkUpForVault(startDir, ceiling, home string) string {
	skip := cleanDirForCompare(home)
	dir := startDir
	for dir != "" {
		// dir is the user's CWD or an ancestor; reading a well-known dotdir
		// under it is the same trust tier as the rest of the CLI.
		if skip == "" || cleanDirForCompare(dir) != skip {
			if info, err := os.Stat(filepath.Join(dir, ".vaultmind")); err == nil && info.IsDir() {
				return dir
			}
		}
		if dir == ceiling {
			return "" // do not inspect above the ceiling
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "" // reached the filesystem root
		}
		dir = parent
	}
	return ""
}

// cleanDirForCompare normalizes a directory path for equality comparison, so a
// legally-set HOME=/Users/me/ matches the /Users/me the walk actually visits.
// An empty input stays empty — the caller reads that as "unknown".
func cleanDirForCompare(dir string) string {
	if strings.TrimSpace(dir) == "" {
		return ""
	}
	return filepath.Clean(dir)
}

// resolveDiscoveredVault applies discoverVaultPath using the real environment
// and working directory, walking to the filesystem root. Used by
// getConfigValueWithFlags when a "vault" read finds only the default value and
// no explicit --vault flag.
func resolveDiscoveredVault(fallback string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return fallback
	}
	// An unreadable home yields "", which disables the guard rather than
	// blocking discovery outright — degrade to the old behaviour, never to none.
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	return discoverVaultPath(fallback, os.Getenv, cwd, "", home)
}
