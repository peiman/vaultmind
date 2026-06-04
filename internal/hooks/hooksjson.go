// Package hooks — legacy hooks.json detection.
//
// Claude Code 2.1.129 stopped recognizing the standalone
// `.claude/hooks.json` file. Projects with that file have silently
// broken hooks: the JSON is well-formed, the schema is documented, but
// Claude Code doesn't read it anymore. The fix is to migrate the
// contents into `.claude/settings.json` under a top-level `hooks` key.
//
// Live evidence: companion project dogfood 2026-05-06/07. SessionStart
// preread-track sidecar logs prove firing through May 5; absent from
// May 6 forward. Migration to settings.json restored the layer.
package hooks

import (
	"os"
	"path/filepath"
)

// DetectLegacyHooksJSON returns true when `<projectDir>/.claude/hooks.json`
// exists as a regular file. That's the silent-breakage shape on Claude
// Code 2.1.129+ — present-but-ignored.
//
// Returns false on: missing file, missing `.claude/` dir, or `hooks.json`
// existing as a directory rather than a file. Stat errors other than
// not-exist also return false; doctor is a health summary, not a
// filesystem-error reporter.
func DetectLegacyHooksJSON(projectDir string) bool {
	path := filepath.Join(projectDir, ".claude", "hooks.json")
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}
