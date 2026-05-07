package hooks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DetectLegacyHooksJSON exists because Claude Code 2.1.129 stopped
// recognizing the standalone `.claude/hooks.json` file (worked May 2-5
// 2026, broken May 6+). Projects with that file have silently broken
// hooks — they think hooks are firing but they aren't. Doctor needs
// to flag this.

func TestDetectLegacyHooksJSON_TrueWhenFileExists(t *testing.T) {
	projectDir := t.TempDir()
	claudeDir := filepath.Join(projectDir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o750))
	require.NoError(t, os.WriteFile(
		filepath.Join(claudeDir, "hooks.json"),
		[]byte(`{"hooks":{"SessionStart":[]}}`),
		0o600,
	))

	got := DetectLegacyHooksJSON(projectDir)
	assert.True(t, got,
		"DetectLegacyHooksJSON must return true when .claude/hooks.json exists — that file is the silent-breakage shape")
}

func TestDetectLegacyHooksJSON_FalseWhenFileAbsent(t *testing.T) {
	projectDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, ".claude"), 0o750))
	// No hooks.json — only settings.json (the post-migration shape).
	require.NoError(t, os.WriteFile(
		filepath.Join(projectDir, ".claude", "settings.json"),
		[]byte(`{"hooks":{}}`),
		0o600,
	))

	got := DetectLegacyHooksJSON(projectDir)
	assert.False(t, got,
		"DetectLegacyHooksJSON must return false when only settings.json exists — that's the healthy post-migration shape")
}

func TestDetectLegacyHooksJSON_FalseWhenClaudeDirAbsent(t *testing.T) {
	projectDir := t.TempDir()
	// No .claude/ at all — not a Claude Code project.
	got := DetectLegacyHooksJSON(projectDir)
	assert.False(t, got,
		"DetectLegacyHooksJSON must return false when there's no .claude/ at all — nothing to flag")
}

// Edge case: .claude/hooks.json exists as a directory rather than a
// file. Treat as not-the-broken-shape; the broken shape is specifically
// a regular file at that path.
func TestDetectLegacyHooksJSON_FalseWhenHooksJSONIsDirectory(t *testing.T) {
	projectDir := t.TempDir()
	require.NoError(t, os.MkdirAll(
		filepath.Join(projectDir, ".claude", "hooks.json"),
		0o750,
	))
	got := DetectLegacyHooksJSON(projectDir)
	assert.False(t, got,
		"DetectLegacyHooksJSON must return false when hooks.json is a directory — the regression is about a regular file")
}
