package autorag_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/hooks"
	"github.com/peiman/vaultmind/internal/hookscripts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Integration tests for the bash auto-rag-guard.sh's DRIFT_CATALOG
// support (slice C2). Each test:
//   1. Materialises the embedded auto-rag-guard.sh + shell-strip.sh
//      to a tempdir (so $(dirname "$0")/shell-strip.sh resolves).
//   2. Spawns bash on auto-rag-guard.sh with a synthetic
//      PreToolUse JSON on stdin.
//   3. Sets DRIFT_CATALOG (or omits it) and asserts on stdout.
//
// AUTORAG_TEST_HARNESS=1 prevents sidecar log writes during the
// run; the canonical engine respects this env (slice B).

// runGuard materialises the canonical auto-rag-guard + shell-strip
// scripts in a tempdir and pipes hookInput to the guard. extraEnv
// extends the inherited environment.
func runGuard(t *testing.T, hookInput string, extraEnv ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"auto-rag-guard.sh", "shell-strip.sh"} {
		body, ok := hookscripts.Get(name)
		require.True(t, ok, "hookscripts.Get(%q) must succeed", name)
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, name), body, 0o700,
		))
	}
	cmd := exec.Command("bash", filepath.Join(dir, "auto-rag-guard.sh"))
	cmd.Stdin = strings.NewReader(hookInput)
	cmd.Env = append(os.Environ(), "AUTORAG_TEST_HARNESS=1")
	cmd.Env = append(cmd.Env, extraEnv...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &bytes.Buffer{} // suppress; the engine writes to stdout for envelope
	require.NoError(t, cmd.Run(), "guard exit must be 0")
	return out.String()
}

// TestGuard_DriftCatalog_BashSignatureMatches — pin the new
// DRIFT_CATALOG support: a Bash signature in the catalog whose
// regex matches the stripped command line fires, with the catalog's
// query/decision flowing through to the envelope.
func TestGuard_DriftCatalog_BashSignatureMatches(t *testing.T) {
	catalog := `[{
		"name":"custom-rebuild",
		"tool":"Bash",
		"match":"go\\s+(build|install)",
		"decision":"inject",
		"query":"don't rebuild this thing"
	}]`
	hookInput := `{"tool_name":"Bash","tool_input":{"command":"go install ./cmd/foo"}}`

	out := runGuard(t, hookInput, "DRIFT_CATALOG="+catalog)
	assert.Contains(t, out, `"hookEventName": "PreToolUse"`,
		"a matching catalog signature must produce a PreToolUse envelope")
	assert.Contains(t, out, "custom-rebuild",
		"the matched signature name must reach the agent's context")
	assert.Contains(t, out, "don't rebuild this thing",
		"the catalog's query string must surface in the canonical guidance line")
}

// TestGuard_DriftCatalog_NoMatch_StaysSilent — a catalog signature
// that doesn't match yields zero output (skip path; zero overhead
// for the agent).
func TestGuard_DriftCatalog_NoMatch_StaysSilent(t *testing.T) {
	catalog := `[{
		"name":"custom",
		"tool":"Bash",
		"match":"NEVER_MATCHES_ANYTHING_xyzzy",
		"decision":"inject",
		"query":"q"
	}]`
	hookInput := `{"tool_name":"Bash","tool_input":{"command":"echo hello"}}`

	out := runGuard(t, hookInput, "DRIFT_CATALOG="+catalog)
	assert.Empty(t, strings.TrimSpace(out),
		"no catalog match + no hardcoded match must produce zero output")
}

// TestGuard_NoCatalog_HardcodedFallbackStillFires — backward-compat:
// without DRIFT_CATALOG, the hardcoded canonical signatures (rebuild-
// vaultmind-binary and -embeddings) still fire. Existing consumers
// who haven't adopted DRIFT_CATALOG yet keep working.
func TestGuard_NoCatalog_HardcodedFallbackStillFires(t *testing.T) {
	hookInput := `{"tool_name":"Bash","tool_input":{"command":"vaultmind index --vault foo --embed"}}`

	out := runGuard(t, hookInput) // no DRIFT_CATALOG
	assert.Contains(t, out, `"hookEventName": "PreToolUse"`,
		"hardcoded rebuild-vaultmind-embeddings signature must still fire when DRIFT_CATALOG is unset")
	assert.Contains(t, out, "rebuild-vaultmind-embeddings",
		"hardcoded signature name must reach the agent's context")
}

// TestGuard_InvalidCatalogJSON_FallsBackToHardcoded — a malformed
// DRIFT_CATALOG must not break the hook. Defensive shape: the agent
// proceeds with the hardcoded canonical signatures (or no match)
// rather than the hook crashing the tool call.
func TestGuard_InvalidCatalogJSON_FallsBackToHardcoded(t *testing.T) {
	hookInput := `{"tool_name":"Bash","tool_input":{"command":"vaultmind index --vault foo --embed"}}`
	out := runGuard(t, hookInput, "DRIFT_CATALOG={not json")
	assert.Contains(t, out, "rebuild-vaultmind-embeddings",
		"invalid DRIFT_CATALOG must fall back to hardcoded signatures, not crash")
}

// TestGuard_DriftCatalog_ShellStripStillApplied — the v0.3
// load-bearing invariant: drift verbs inside heredoc bodies must
// not match catalog regexes either. The bash engine runs
// shell-strip.sh on CMD before applying ANY drift regex (including
// catalog ones), so a catalog signature for `vaultmind index` must
// not fire on a heredoc body containing that string.
func TestGuard_DriftCatalog_ShellStripStillApplied(t *testing.T) {
	catalog := `[{
		"name":"index-embed",
		"tool":"Bash",
		"match":"vaultmind\\s+index.*--embed",
		"decision":"inject",
		"query":"q"
	}]`
	// Drift verb inside a heredoc body — must NOT fire because
	// shell-strip.sh strips heredoc bodies before regex matching.
	hookInput := `{"tool_name":"Bash","tool_input":{"command":"cat <<EOF\ntrue | vaultmind index --vault foo --embed\nEOF"}}`

	out := runGuard(t, hookInput, "DRIFT_CATALOG="+catalog)
	assert.NotContains(t, out, "index-embed",
		"v0.3 false-positive guard: shell-strip must run before catalog regex")
	assert.Empty(t, strings.TrimSpace(out),
		"drift inside heredoc body must produce zero output (shell-strip + catalog both clean)")
}

// TestGuard_DriftCatalog_WriteSignatureMatchesByPath — Write/Edit
// catalog signatures pattern-match against the file path. Pin that
// the dispatch correctly routes Write tool-name to the Write
// branch's catalog check.
func TestGuard_DriftCatalog_WriteSignatureMatchesByPath(t *testing.T) {
	catalog := `[{
		"name":"writes-to-etc",
		"tool":"Write",
		"match":"^/etc/",
		"decision":"deny",
		"query":"don't write to /etc"
	}]`
	hookInput := `{"tool_name":"Write","tool_input":{"file_path":"/etc/hosts","content":"x"}}`

	// Allow the path so the hardcoded cross-project-write doesn't fire
	// (would shadow the catalog match by short-circuiting first). We
	// want to isolate the catalog path here.
	out := runGuard(t, hookInput,
		"DRIFT_CATALOG="+catalog,
		"AUTORAG_ALLOWED_ROOTS=/etc:/tmp",
	)
	assert.Contains(t, out, "writes-to-etc",
		"catalog Write signature must match against file_path")
	assert.Contains(t, out, "deny",
		"catalog decision (deny) must surface in permissionDecision")
}

// TestGuard_DriftCatalog_EditTool — Edit dispatches through the
// same Write/Edit branch but uses a different tool_name in JSON.
// Pin that an Edit catalog entry actually fires (Write was tested
// above; Edit is the symmetric case).
func TestGuard_DriftCatalog_EditTool(t *testing.T) {
	catalog := `[{
		"name":"edits-config",
		"tool":"Edit",
		"match":"\\.config",
		"decision":"inject",
		"query":"q"
	}]`
	hookInput := `{"tool_name":"Edit","tool_input":{"file_path":"/tmp/foo.config","old_string":"a","new_string":"b"}}`

	out := runGuard(t, hookInput, "DRIFT_CATALOG="+catalog)
	assert.Contains(t, out, "edits-config",
		"catalog Edit signature must dispatch through the Write/Edit branch")
}

// TestGuard_DriftCatalog_SecondSignatureMatches — proves the
// per-signature iteration doesn't short-circuit at the first
// non-match. Two signatures; only the second matches.
func TestGuard_DriftCatalog_SecondSignatureMatches(t *testing.T) {
	catalog := `[
		{"name":"first-no-match","tool":"Bash","match":"NEVER_xyzzy","decision":"inject","query":"q1"},
		{"name":"second-match","tool":"Bash","match":"^echo","decision":"inject","query":"q2"}
	]`
	hookInput := `{"tool_name":"Bash","tool_input":{"command":"echo hello"}}`

	out := runGuard(t, hookInput, "DRIFT_CATALOG="+catalog)
	assert.Contains(t, out, "second-match",
		"per-signature iteration must continue past non-matches")
	assert.NotContains(t, out, "first-no-match",
		"non-matching signature name must not appear in the envelope")
}

// TestGuard_DriftCatalog_InvalidRegexInOneSig_DoesNotPoison — a
// broken regex in one signature must be skipped (per-signature
// `except re.error: continue`); subsequent valid signatures
// continue to be evaluated.
func TestGuard_DriftCatalog_InvalidRegexInOneSig_DoesNotPoison(t *testing.T) {
	catalog := `[
		{"name":"broken","tool":"Bash","match":"(unclosed","decision":"inject","query":"q1"},
		{"name":"working","tool":"Bash","match":"^echo","decision":"inject","query":"q2"}
	]`
	hookInput := `{"tool_name":"Bash","tool_input":{"command":"echo hello"}}`

	out := runGuard(t, hookInput, "DRIFT_CATALOG="+catalog)
	assert.Contains(t, out, "working",
		"invalid regex in one signature must not poison the catalog — per-sig skip is load-bearing")
}

// TestGuard_DriftCatalog_EmptyArray — `[]` is legal per the Go
// validator (a consumer with no project-specific drifts who still
// wants the engine installed). Must produce no match and fall
// through to hardcoded.
func TestGuard_DriftCatalog_EmptyArray(t *testing.T) {
	hookInput := `{"tool_name":"Bash","tool_input":{"command":"vaultmind index --vault foo --embed"}}`
	out := runGuard(t, hookInput, "DRIFT_CATALOG=[]")
	// Empty catalog falls through to hardcoded; the rebuild-vaultmind-
	// embeddings signature still fires.
	assert.Contains(t, out, "rebuild-vaultmind-embeddings",
		"empty catalog must fall through to hardcoded canonical signatures")
}

// TestGuard_DriftCatalog_NonListJSON — `{}` or `"string"` instead
// of an array must be rejected by `isinstance(cat, list)` in the
// dispatcher; falls back to hardcoded just like invalid JSON.
func TestGuard_DriftCatalog_NonListJSON(t *testing.T) {
	hookInput := `{"tool_name":"Bash","tool_input":{"command":"vaultmind index --vault foo --embed"}}`
	out := runGuard(t, hookInput, `DRIFT_CATALOG={"not":"a list"}`)
	assert.Contains(t, out, "rebuild-vaultmind-embeddings",
		"non-list catalog JSON must fall back to hardcoded — isinstance(cat, list) guard is load-bearing")
}

// TestGuard_AfterRealInstall_V03ShellStripStillWorks — regression
// guard for the companion-project-found CRITICAL: when scripts were
// installed at 0600 (no exec bit), auto-rag-guard.sh's
// `[ -x "$SHELL_STRIP_SCRIPT" ]` check failed, falling back to
// raw CMD and silently disabling the v0.3 shell-quoting fix for
// every consumer. The earlier `runGuard` helper writes at 0o700
// which masked the bug — this test uses the real `hooks.Install()`
// path so any regression in install permissions surfaces here.
func TestGuard_AfterRealInstall_V03ShellStripStillWorks(t *testing.T) {
	projectDir := t.TempDir()
	res, err := hooks.Install(hooks.InstallConfig{ProjectDir: projectDir})
	require.NoError(t, err)
	require.NotEmpty(t, res.Written)

	guardPath := filepath.Join(res.ScriptsDir, "auto-rag-guard.sh")
	require.FileExists(t, guardPath)

	// Pin the exec-bit explicitly: the companion project CRITICAL was that
	// shell-strip.sh installed without exec bit failed auto-rag-guard's
	// `[ -x ]` gate (since fixed via defense-in-depth `[ -f ]` AND
	// install perms 0o700). This assertion catches regression at install
	// permissions even before the runtime smoke test below.
	stripPath := filepath.Join(res.ScriptsDir, "shell-strip.sh")
	stripInfo, statErr := os.Stat(stripPath)
	require.NoError(t, statErr)
	assert.NotZero(t, stripInfo.Mode().Perm()&0o100,
		"shell-strip.sh must have owner-exec bit after install (0o700) — companion project 2026-05-07 CRITICAL")

	// v0.3 load-bearing case: drift verb inside heredoc body must
	// NOT match. If shell-strip.sh isn't executable, the guard's
	// `[ -x ]` check fails, falls back to raw CMD, and the drift
	// regex matches the unstripped pipe + verb — false positive.
	hookInput := `{"tool_name":"Bash","tool_input":{"command":"cat <<EOF\ntrue | vaultmind index --vault foo --embed\nEOF"}}`
	cmd := exec.Command("bash", guardPath)
	cmd.Stdin = strings.NewReader(hookInput)
	cmd.Env = append(os.Environ(), "AUTORAG_TEST_HARNESS=1")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &bytes.Buffer{}
	require.NoError(t, cmd.Run())

	assert.Empty(t, strings.TrimSpace(out.String()),
		"v0.3 false-positive guard must hold against ACTUALLY-INSTALLED scripts: drift inside heredoc body must skip. Companion project 2026-05-07 dogfood found 0o600 install perms broke this silently.")
}
