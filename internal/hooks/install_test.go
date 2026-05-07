package hooks_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/hooks"
	"github.com/peiman/vaultmind/internal/hookscripts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInstall_FreshDir_WritesAllCanonicalScripts — first install
// into a clean project: every embedded canonical script is written
// to <project>/.claude/scripts/. Pin the contract that this is the
// path users see.
func TestInstall_FreshDir_WritesAllCanonicalScripts(t *testing.T) {
	dir := t.TempDir()

	res, err := hooks.Install(hooks.InstallConfig{ProjectDir: dir})
	require.NoError(t, err)
	require.NotNil(t, res)

	assert.Equal(t, dir, res.ProjectDir)
	assert.Equal(t, filepath.Join(dir, ".claude", "scripts"), res.ScriptsDir)
	assert.Empty(t, res.Conflicts)
	assert.Empty(t, res.Skipped)
	assert.Equal(t, hookscripts.Names(), res.Written,
		"every canonical script must be written on a fresh install")

	// Each written file matches the embedded canonical byte-for-byte.
	for _, name := range res.Written {
		canonical, ok := hookscripts.Get(name)
		require.True(t, ok)
		body, err := os.ReadFile(filepath.Join(res.ScriptsDir, name))
		require.NoError(t, err)
		assert.Equal(t, canonical, body,
			"%s: written bytes must match embedded canonical", name)
		// 0o700: scripts have shebangs and ARE executed (some hook
		// scripts internally invoke other hook scripts via `[ -x … ]`
		// gates — the workhorse 2026-05-07 CRITICAL caught this).
		// 0o700 keeps them owner-private while still runnable.
		info, err := os.Stat(filepath.Join(res.ScriptsDir, name))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o700), info.Mode().Perm(),
			"%s: scripts must be installed 0o700 — internal `[ -x ]` gates require the exec bit", name)
	}
}

// TestInstall_RefusesToOverwriteWithoutForce — user-edited or
// drifted scripts must not get silently clobbered. Per arc-extending-
// not-overwriting; same gate vaultmind init uses.
func TestInstall_RefusesToOverwriteWithoutForce(t *testing.T) {
	dir := t.TempDir()
	scriptsDir := filepath.Join(dir, ".claude", "scripts")
	require.NoError(t, os.MkdirAll(scriptsDir, 0o755))

	// Pre-place a load-persona.sh with different content (simulates
	// user-edited or drifted-from-old-canonical state).
	preExisting := []byte("#!/bin/bash\n# user-edited; do not clobber\n")
	require.NoError(t, os.WriteFile(filepath.Join(scriptsDir, "load-persona.sh"), preExisting, 0o755))

	res, err := hooks.Install(hooks.InstallConfig{ProjectDir: dir})
	require.Error(t, err, "must error when conflicts exist without --force")
	require.NotNil(t, res, "result populated even on conflict so caller can list both written + conflicts")

	assert.Contains(t, res.Conflicts, "load-persona.sh")
	assert.NotContains(t, res.Written, "load-persona.sh",
		"conflicting script must NOT be written")
	// The OTHER scripts still get written even when one conflicts —
	// the user can fix the one conflict + re-run.
	assert.Contains(t, res.Written, "vault-recall.sh",
		"non-conflicting scripts written despite conflict in one")

	// User's edited file is preserved byte-for-byte.
	body, err := os.ReadFile(filepath.Join(scriptsDir, "load-persona.sh"))
	require.NoError(t, err)
	assert.Equal(t, preExisting, body, "user's edited load-persona.sh untouched")
}

// TestInstall_ForceOverwritesEvenOnDrift — refresh path. After a
// vaultmind upgrade, user runs install --force; the embedded
// canonical replaces stale copies.
func TestInstall_ForceOverwritesEvenOnDrift(t *testing.T) {
	dir := t.TempDir()
	scriptsDir := filepath.Join(dir, ".claude", "scripts")
	require.NoError(t, os.MkdirAll(scriptsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(scriptsDir, "load-persona.sh"),
		[]byte("#!/bin/bash\n# stale\n"), 0o755))

	res, err := hooks.Install(hooks.InstallConfig{ProjectDir: dir, Force: true})
	require.NoError(t, err)
	assert.True(t, res.ForceUsed)
	assert.Contains(t, res.Written, "load-persona.sh")
	assert.Empty(t, res.Conflicts)

	canonical, _ := hookscripts.Get("load-persona.sh")
	body, _ := os.ReadFile(filepath.Join(scriptsDir, "load-persona.sh"))
	assert.Equal(t, canonical, body, "force overwrote stale with canonical")
}

// TestInstall_IdempotentOnByteIdenticalCopies — re-running install
// when copies are already identical is a no-op (skipped, not
// rewritten). Future stat-mtime-based change detection in tooling
// shouldn't fire on a re-run with same content.
func TestInstall_IdempotentOnByteIdenticalCopies(t *testing.T) {
	dir := t.TempDir()

	// First install populates everything.
	_, err := hooks.Install(hooks.InstallConfig{ProjectDir: dir})
	require.NoError(t, err)

	// Second install should skip every script (same bytes).
	res2, err := hooks.Install(hooks.InstallConfig{ProjectDir: dir})
	require.NoError(t, err)
	assert.Empty(t, res2.Written, "re-install with no changes should write nothing")
	assert.Empty(t, res2.Conflicts)
	assert.Equal(t, hookscripts.Names(), res2.Skipped,
		"every script reported as skipped (already byte-identical)")
}

// TestCompareInstalled_DetectsDrift — the doctor-side primitive.
// Pin the contract: drifted scripts surface; absent scripts don't;
// matching scripts don't.
func TestCompareInstalled_DetectsDrift(t *testing.T) {
	dir := t.TempDir()
	scriptsDir := filepath.Join(dir, ".claude", "scripts")
	require.NoError(t, os.MkdirAll(scriptsDir, 0o755))

	// Install everything cleanly.
	_, err := hooks.Install(hooks.InstallConfig{ProjectDir: dir})
	require.NoError(t, err)

	// No drift on fresh install.
	drift, err := hooks.CompareInstalled(dir)
	require.NoError(t, err)
	assert.Empty(t, drift, "fresh install has zero drift")

	// Modify one script — simulate user-edit or stale-binary.
	require.NoError(t, os.WriteFile(filepath.Join(scriptsDir, "vault-recall.sh"),
		[]byte("#!/bin/bash\n# diverged\n"), 0o755))

	drift, err = hooks.CompareInstalled(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"vault-recall.sh"}, drift, "drifted script surfaces; others stay clean")

	// Delete another — absent != drifted.
	require.NoError(t, os.Remove(filepath.Join(scriptsDir, "load-persona.sh")))
	drift, err = hooks.CompareInstalled(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"vault-recall.sh"}, drift,
		"deleted script is 'not installed', not 'drifted' — different signal")
}

// TestCompareInstalled_NoScriptsDir_ReturnsEmpty — fresh project
// with no .claude/scripts/ yet. Drift is zero; the "no hooks
// installed" signal is doctor's separate concern.
func TestCompareInstalled_NoScriptsDir_ReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	drift, err := hooks.CompareInstalled(dir)
	require.NoError(t, err)
	assert.Empty(t, drift)
}

// TestInstall_OnlyWritesNamedSubset — workhorse 2026-05-07 MEDIUM:
// `vaultmind hooks install` was all-or-nothing (writes 8 scripts).
// Consumers customizing non-auto-RAG hooks (workhorse customized
// vault-track-read + load-persona) had to accept --force overwrite
// of those customizations. The Only allowlist lets consumers pull
// just the slice they want.
func TestInstall_OnlyWritesNamedSubset(t *testing.T) {
	dir := t.TempDir()
	want := []string{"auto-rag-guard.sh", "shell-strip.sh"}
	res, err := hooks.Install(hooks.InstallConfig{
		ProjectDir: dir,
		Only:       want,
	})
	require.NoError(t, err)
	require.NotNil(t, res)

	// Only the named scripts written.
	assert.Equal(t, want, res.Written,
		"Only must restrict written set to the allowlist")

	// And only those exist on disk.
	scriptsDir := filepath.Join(dir, ".claude", "scripts")
	entries, err := os.ReadDir(scriptsDir)
	require.NoError(t, err)
	got := []string{}
	for _, e := range entries {
		got = append(got, e.Name())
	}
	assert.ElementsMatch(t, want, got,
		"only the named scripts should land on disk; everything else stays untouched")
}

// TestInstall_OnlyRejectsUnknownName — typo-protect: a name that
// isn't in hookscripts.Names() must be rejected at lint-time so
// the consumer fixes it before shipping a settings.json that
// references a non-existent script.
func TestInstall_OnlyRejectsUnknownName(t *testing.T) {
	dir := t.TempDir()
	_, err := hooks.Install(hooks.InstallConfig{
		ProjectDir: dir,
		Only:       []string{"auto-rag-guard.sh", "not-a-real-hook.sh"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not-a-real-hook.sh",
		"unknown name in Only must be surfaced explicitly so consumers fix typos before shipping")
}

// TestInstall_OnlyRejectsDuplicateName — reviewer-found edge in
// MEDIUM follow-up: duplicate names in --only would silently
// double-write the same script and pollute res.Written. Reject at
// lint time so the consumer fixes the malformed list.
func TestInstall_OnlyRejectsDuplicateName(t *testing.T) {
	dir := t.TempDir()
	_, err := hooks.Install(hooks.InstallConfig{
		ProjectDir: dir,
		Only:       []string{"auto-rag-guard.sh", "auto-rag-guard.sh"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate",
		"duplicate names in Only must be rejected explicitly so machine consumers parsing the JSON envelope don't see misleading res.Written counts")
}

// TestInstall_OnlyEmpty_InstallsAll — backward compatibility:
// `Only: nil` and `Only: []` both fall through to "install all 8
// canonical scripts". Existing callers without the flag keep
// working unchanged.
func TestInstall_OnlyEmpty_InstallsAll(t *testing.T) {
	dir := t.TempDir()
	res, err := hooks.Install(hooks.InstallConfig{
		ProjectDir: dir,
		Only:       []string{},
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, hookscripts.Names(), res.Written,
		"empty Only must install all canonical scripts (backward compat with pre-MEDIUM-fix Install)")
}
