package hookscripts_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cursor-adapter.sh runs one of the hook scripts for Cursor. Cursor's hooks
// take a different input and give context back in a different envelope than
// Claude Code's, and only sessionStart and postToolUse can add context at all
// (probed live 2026-10-02; beforeSubmitPrompt never fired). The adapter
// translates in both directions so the scripts stay one copy.

const cursorAdapter = "cursor-adapter.sh"

func runCursorAdapter(t *testing.T, env []string, stdin string, args ...string) string {
	t.Helper()
	bashPath := "/bin/bash" // what a stock Mac runs; see runHookScript
	if _, err := os.Stat(bashPath); err != nil {
		var lerr error
		if bashPath, lerr = exec.LookPath("bash"); lerr != nil {
			t.Skip("bash not available")
		}
	}
	script, err := filepath.Abs(cursorAdapter)
	require.NoError(t, err)
	cmd := exec.Command(bashPath, append([]string{script}, args...)...)
	cmd.Env = env
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	require.NoErrorf(t, cmd.Run(), "the adapter must always exit 0 (stderr: %s)", errb.String())
	return out.String()
}

// cursorContext parses the adapter's output: always one JSON object, whose
// additional_context is the context Cursor shows the model ("" when none).
func cursorContext(t *testing.T, out string) string {
	t.Helper()
	var got map[string]any
	require.NoErrorf(t, json.Unmarshal([]byte(out), &got), "Cursor needs one JSON object, got %q", out)
	keys := make([]string, 0, len(got))
	for k := range got {
		keys = append(keys, k)
	}
	assert.Subset(t, []string{"additional_context"}, keys, "only additional_context: postToolUse has no permission to decide")
	s, _ := got["additional_context"].(string)
	return s
}

func cursorToolPayload(tool string, input map[string]any, ids map[string]any) string {
	m := map[string]any{"hook_event_name": "postToolUse", "tool_name": tool, "tool_input": input,
		"tool_output": "", "cursor_version": "2026.10.01"}
	for k, v := range ids {
		m[k] = v
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func TestCursorAdapter_AReadBringsTheCodeMap(t *testing.T) {
	bin, _ := codeMapStub(t, coveringJSON("kb"), 0)
	e := newCodeMapEnv(t, bin)
	in := cursorToolPayload("Read", map[string]any{"file_path": e.file},
		map[string]any{"session_id": "c1", "conversation_id": "c1", "workspace_roots": []string{e.project}})

	ctx := cursorContext(t, runCursorAdapter(t, e.env, in, codeMapScript, "Read|Write"))
	assert.Contains(t, ctx, "Why a is built so (decision-a) — Because b.")
}

// Cursor reports an edit as "Write". The code map covers edits too.
func TestCursorAdapter_AnEditBringsTheCodeMap(t *testing.T) {
	bin, _ := codeMapStub(t, coveringJSON("kb"), 0)
	e := newCodeMapEnv(t, bin)
	in := cursorToolPayload("Write", map[string]any{"file_path": e.file, "content": "x"},
		map[string]any{"session_id": "c1"})

	assert.Contains(t, cursorContext(t, runCursorAdapter(t, e.env, in, codeMapScript, "Read|Write")), "decision-a")
}

// A tool the script is not wired for runs nothing: the vault is not asked.
func TestCursorAdapter_AnotherToolRunsNothing(t *testing.T) {
	bin, log := codeMapStub(t, coveringJSON("kb"), 0)
	e := newCodeMapEnv(t, bin)
	in := cursorToolPayload("Shell", map[string]any{"command": "ls"}, map[string]any{"session_id": "c1"})

	assert.Empty(t, cursorContext(t, runCursorAdapter(t, e.env, in, codeMapScript, "Read|Write")))
	_, err := os.Stat(log)
	assert.True(t, os.IsNotExist(err), "a tool outside the pattern must not query the vault")
}

// The script's once-per-session ledger keys on the session. A payload that
// carries only Cursor's conversation_id still keys on it.
func TestCursorAdapter_TheConversationIsTheSession(t *testing.T) {
	bin, _ := codeMapStub(t, coveringJSON("kb"), 0)
	e := newCodeMapEnv(t, bin)
	read := func(conv string) string {
		in := cursorToolPayload("Read", map[string]any{"file_path": e.file}, map[string]any{"conversation_id": conv})
		return cursorContext(t, runCursorAdapter(t, e.env, in, codeMapScript, "Read|Write"))
	}

	assert.NotEmpty(t, read("conv-a"))
	assert.Empty(t, read("conv-a"), "shown once per conversation")
	assert.NotEmpty(t, read("conv-b"))
}

// Cursor's shell tool is "Shell"; the scripts know Claude Code's "Bash". A
// commit brings the notes about this commit, as reach does in Claude Code.
func TestCursorAdapter_AShellCommitBringsReach(t *testing.T) {
	h := newHookEnv(t, echoStub)
	in := cursorToolPayload("Shell", map[string]any{"command": `git commit -m "fix(hooks): a dry run writes nothing"`},
		map[string]any{"session_id": "c1", "workspace_roots": []string{h.projectDir}})

	assert.Contains(t, cursorContext(t, runCursorAdapter(t, h.env(false), in, "vault-reach.sh", "Shell|Write")),
		"a dry run writes nothing")
}

// A session-start script prints plain text, which Claude Code takes as context.
// Cursor takes it only inside the envelope.
func TestCursorAdapter_SessionStartWrapsPlainText(t *testing.T) {
	project := projectWithVault(t)
	bin := stubVaultmind(t, "Embeddings: dense 50/50 (bge-m3), sparse 50/50, colbert 50/50")
	env := []string{"PATH=" + bin + ":" + healthHookPATH, "HOME=" + t.TempDir()}
	in, _ := json.Marshal(map[string]any{"hook_event_name": "sessionStart", "session_id": "c1",
		"conversation_id": "c1", "workspace_roots": []string{project}, "is_background_agent": false})

	ctx := cursorContext(t, runCursorAdapter(t, env, string(in), "vaultmind-health.sh"))
	assert.Contains(t, ctx, "full BGE-M3 hybrid", "the health line reaches the model inside additional_context")
}

// Nothing to say is "{}", never empty output and never an error.
func TestCursorAdapter_SilenceIsAnEmptyObject(t *testing.T) {
	bin, _ := codeMapStub(t, emptyCoveringJSON, 0)
	e := newCodeMapEnv(t, bin)
	in := cursorToolPayload("Read", map[string]any{"file_path": e.file}, map[string]any{"session_id": "c1"})

	out := runCursorAdapter(t, e.env, in, codeMapScript, "Read|Write")
	assert.Equal(t, "{}", strings.TrimSpace(out))
}

// adapterBesideStub copies the adapter into a fresh dir next to a script that
// prints the payload it was handed, so a test sees the translation itself.
func adapterBesideStub(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile(cursorAdapter)
	require.NoError(t, err)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, cursorAdapter), src, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "echo-input.sh"),
		[]byte("#!/bin/bash\npython3 -c 'import json,sys; print(json.dumps({\"hookSpecificOutput\":{\"additionalContext\":sys.stdin.read()}}))'\n"), 0o600))
	return filepath.Join(dir, cursorAdapter)
}

func runAdapterAt(t *testing.T, adapter string, env []string, stdin string, args ...string) string {
	t.Helper()
	cmd := exec.Command("/bin/bash", append([]string{adapter}, args...)...)
	cmd.Env = env
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.Output()
	require.NoError(t, err, "the adapter must always exit 0")
	return string(out)
}

// The script sees Claude Code's names: Shell becomes Bash, Cursor's
// conversation becomes the session, the workspace root becomes the cwd.
func TestCursorAdapter_TranslatesCursorInputForTheScript(t *testing.T) {
	adapter := adapterBesideStub(t)
	in := cursorToolPayload("Shell", map[string]any{"command": "git push"},
		map[string]any{"conversation_id": "conv-9", "workspace_roots": []string{"/proj"}})

	ctx := cursorContext(t, runAdapterAt(t, adapter, []string{"PATH=/usr/bin:/bin"}, in, "echo-input.sh"))
	var seen map[string]any
	require.NoError(t, json.Unmarshal([]byte(ctx), &seen), ctx)
	assert.Equal(t, "Bash", seen["tool_name"])
	assert.Equal(t, "PreToolUse", seen["hook_event_name"])
	assert.Equal(t, "conv-9", seen["session_id"])
	assert.Equal(t, "/proj", seen["cwd"])
}

func TestCursorAdapter_MalformedInputIsAnEmptyObject(t *testing.T) {
	adapter := adapterBesideStub(t)
	for _, in := range []string{"not json at all", "[1,2]", ""} {
		out := runAdapterAt(t, adapter, []string{"PATH=/usr/bin:/bin"}, in, "echo-input.sh", "Read")
		assert.Equal(t, "{}", strings.TrimSpace(out), "input %q", in)
	}
}

// Without python3 nothing can be translated, and a hook must still not fail.
func TestCursorAdapter_WithoutPython3IsAnEmptyObject(t *testing.T) {
	adapter := adapterBesideStub(t)
	bin := t.TempDir()
	for _, tool := range []string{"cat", "dirname", "head", "tail"} {
		p, err := exec.LookPath(tool)
		require.NoError(t, err)
		require.NoError(t, os.Symlink(p, filepath.Join(bin, tool)))
	}
	in := cursorToolPayload("Read", map[string]any{"file_path": "/x"}, nil)
	out := runAdapterAt(t, adapter, []string{"PATH=" + bin}, in, "echo-input.sh")
	assert.Equal(t, "{}", strings.TrimSpace(out))
}

// A script name that is not a hook script beside the adapter runs nothing.
func TestCursorAdapter_RunsOnlyItsOwnScripts(t *testing.T) {
	e := newCodeMapEnv(t, t.TempDir())
	for _, name := range []string{"../../../bin/sh", "missing.sh", ""} {
		out := runCursorAdapter(t, e.env, `{"hook_event_name":"sessionStart"}`, name)
		assert.Equal(t, "{}", strings.TrimSpace(out), "script %q", name)
	}
}
