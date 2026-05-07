// Package hooks implements `vaultmind hooks install` — the
// command that writes embedded Claude Code hook scripts into a
// user's project. The embedded source-of-truth lives in
// internal/hookscripts/; this package is the consumer-facing
// install path.
//
// SSOT discipline (per Peiman 2026-05-07 "do b but SSOT, can't
// drift"): the embedded scripts are the canonical source. Every
// install copies the SAME bytes. Doctor's hook-drift check
// (separate work) compares installed copies against the embedded
// canonical; mismatches surface with `vaultmind hooks install
// --force` as the resolution.
package hooks

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/peiman/vaultmind/internal/hookscripts"
)

// InstallConfig controls a hooks-install run.
type InstallConfig struct {
	// ProjectDir is the project root. Scripts get written under
	// `<ProjectDir>/.claude/scripts/`. If empty, callers should
	// resolve to CWD before passing.
	ProjectDir string
	// Force overwrites existing scripts. Default false (refuse).
	Force bool
}

// InstallResult is the JSON-serializable output of Install.
type InstallResult struct {
	ProjectDir string   `json:"project_dir"`
	ScriptsDir string   `json:"scripts_dir"`
	Written    []string `json:"written"`
	Skipped    []string `json:"skipped,omitempty"`
	Conflicts  []string `json:"conflicts,omitempty"`
	ForceUsed  bool     `json:"force_used"`
}

// Install writes the embedded canonical hook scripts into the
// configured project's `.claude/scripts/` directory.
//
// Returns InstallResult populated with what was written, skipped
// (already byte-identical), and conflicts (existed with different
// content; only relevant when Force=false). When Force=false and
// there are conflicts, the function writes the non-conflicting
// scripts AND returns a non-nil error naming the conflicts so the
// caller can surface them. Force=true skips the conflict check
// and writes everything.
func Install(cfg InstallConfig) (*InstallResult, error) {
	if cfg.ProjectDir == "" {
		return nil, fmt.Errorf("project-dir is required (current dir works as default if caller resolves)")
	}
	scriptsDir := filepath.Join(cfg.ProjectDir, ".claude", "scripts")
	res := &InstallResult{
		ProjectDir: cfg.ProjectDir,
		ScriptsDir: scriptsDir,
		ForceUsed:  cfg.Force,
		Written:    []string{},
	}
	// MkdirAll is idempotent; safe under both first-install and
	// refresh.
	if err := os.MkdirAll(scriptsDir, 0o750); err != nil {
		return nil, fmt.Errorf("creating %s: %w", scriptsDir, err)
	}

	for _, name := range hookscripts.Names() {
		canonical, ok := hookscripts.Get(name)
		if !ok {
			continue
		}
		dst := filepath.Join(scriptsDir, name)
		// scriptsDir is built from the user-supplied project dir;
		// `name` is from the embedded FS (no path traversal possible
		// per Get's contract). The Stat / ReadFile here read paths
		// rooted at the user's vault — same trust tier as the
		// indexer / mutator.
		existing, err := os.ReadFile(dst) // #nosec G304
		if err != nil && !os.IsNotExist(err) {
			return res, fmt.Errorf("reading existing %s: %w", dst, err)
		}
		if err == nil {
			// File exists — compare. Byte-identical = skip silently
			// (idempotent re-run). Different + Force = overwrite.
			// Different + !Force = conflict.
			if hashEq(existing, canonical) {
				res.Skipped = append(res.Skipped, name)
				continue
			}
			if !cfg.Force {
				res.Conflicts = append(res.Conflicts, name)
				continue
			}
		}
		// 0o700: hook scripts have shebangs and ARE executed —
		// auto-rag-guard.sh internally invokes shell-strip.sh via
		// `[ -x "$SHELL_STRIP_SCRIPT" ]`, which silently fails on
		// 0o600 perms (workhorse 2026-05-07 dogfood CRITICAL).
		// gosec G306 is about data-file permissions; scripts are a
		// separate category. The exec bit is intentional + load-bearing.
		if err := os.WriteFile(dst, canonical, 0o700); err != nil { //nolint:gosec // G306: scripts with shebangs need exec bit; load-bearing per workhorse 2026-05-07 CRITICAL
			return res, fmt.Errorf("writing %s: %w", dst, err)
		}
		res.Written = append(res.Written, name)
	}

	if len(res.Conflicts) > 0 {
		return res, fmt.Errorf(
			"%d hook script(s) exist with different content; re-run with --force to overwrite: %v",
			len(res.Conflicts), res.Conflicts,
		)
	}
	return res, nil
}

// CompareInstalled returns the set of installed-script names whose
// bytes differ from the embedded canonical. Useful for doctor's
// hook-drift check; same shape as DetectContentDrift for vault
// notes but for hook scripts.
//
// Names not present in the user's scripts dir are NOT counted as
// drift — they're "not installed" (a separate signal). Names
// present but matching are not counted (clean state). Only
// differs-from-canonical counts.
func CompareInstalled(projectDir string) ([]string, error) {
	scriptsDir := filepath.Join(projectDir, ".claude", "scripts")
	if _, err := os.Stat(scriptsDir); os.IsNotExist(err) {
		// No scripts dir = nothing installed = nothing to drift.
		// Doctor surfaces "no hooks installed" as a separate signal
		// if it cares to.
		return nil, nil
	}
	drifted := []string{}
	for _, name := range hookscripts.Names() {
		canonical, ok := hookscripts.Get(name)
		if !ok {
			continue
		}
		dst := filepath.Join(scriptsDir, name)
		// dst is built from the (caller-supplied) projectDir + an
		// embed-FS-validated filename. Same trust tier as Install.
		existing, err := os.ReadFile(dst) // #nosec G304
		if err != nil {
			if os.IsNotExist(err) {
				continue // not installed; not drift
			}
			return nil, fmt.Errorf("reading %s: %w", dst, err)
		}
		if !hashEq(existing, canonical) {
			drifted = append(drifted, name)
		}
	}
	return drifted, nil
}

// hashEq returns true when two byte slices have identical content.
// Both sides are already in memory, so bytes.Equal is the right
// primitive — sha256 would burn cycles for no correctness gain.
// (doctor/query's content drift uses sha256 because one side comes
// from a precomputed DB hash; that asymmetry doesn't apply here.)
func hashEq(a, b []byte) bool { return bytes.Equal(a, b) }

// _ assert that hookscripts.FS satisfies fs.FS at compile time —
// guards against accidental signature changes that would break
// CompareInstalled / Install if they depended on it directly.
var _ fs.FS = hookscripts.FS()
