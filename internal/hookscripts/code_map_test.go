package hookscripts_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// vault-code-map.sh: when the agent reads or edits a code file, the notes whose
// `paths:` cover that file arrive as a short map — title, id, one line — at the
// moment the knowledge applies, found by the path the agent is already reading.

const codeMapScript = "vault-code-map.sh"

// coveringJSON is what `vaultmind tree --for … --json` prints when one note in
// one vault covers the file.
func coveringJSON(vault string) string {
	return fmt.Sprintf(`{"status":"ok","result":{"for":"repo:internal/a.go","vaults":[{"vault":%q,"total":1,`+
		`"root":{"path":"","total":1,"dirs":[{"path":"decisions","total":1,"notes":[`+
		`{"id":"decision-a","path":"decisions/a.md","title":"Why a is built so","type":"decision","line":"Because b."}]}]}}]}}`, vault)
}

const emptyCoveringJSON = `{"status":"ok","result":{"for":"repo:README.md","vaults":[{"vault":"v","total":0,"root":{"path":"","total":0}}]}}`

// codeMapStub answers `tree` with treeOut (exit treeCode) and logs every call.
func codeMapStub(t *testing.T, treeOut string, treeCode int) (bin, log string) {
	t.Helper()
	bin = t.TempDir()
	log = filepath.Join(bin, "calls.log")
	body := fmt.Sprintf("#!/bin/bash\necho \"$*\" >> %s\nif [ \"$1\" = tree ]; then printf '%%s\\n' %s; exit %d; fi\nexit 0\n",
		shellSingleQuote(log), shellSingleQuote(treeOut), treeCode)
	require.NoError(t, os.WriteFile(filepath.Join(bin, "vaultmind"), []byte(body), 0o755))
	return bin, log
}

type codeMapEnv struct {
	project, vault, file string
	env                  []string
}

func newCodeMapEnv(t *testing.T, bin string) codeMapEnv {
	t.Helper()
	project := t.TempDir()
	vault := filepath.Join(project, "vaultmind-vault")
	require.NoError(t, os.MkdirAll(filepath.Join(vault, "decisions"), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(project, "internal"), 0o750))
	file := filepath.Join(project, "internal", "a.go")
	require.NoError(t, os.WriteFile(file, []byte("package internal\n"), 0o600))
	return codeMapEnv{project: project, vault: vault, file: file, env: []string{
		"PATH=" + bin + ":/usr/bin:/bin",
		"HOME=" + t.TempDir(),
		"XDG_STATE_HOME=" + t.TempDir(),
		"CLAUDE_PROJECT_DIR=" + project,
		"VAULTMIND_VAULT=" + vault,
	}}
}

func codeMapPayload(tool, file, session string) string {
	b, _ := json.Marshal(map[string]any{
		"tool_name": tool, "session_id": session,
		"tool_input": map[string]any{"file_path": file},
	})
	return string(b)
}

func TestCodeMap_ReadingCoveredCodeBringsItsNotes(t *testing.T) {
	bin, log := codeMapStub(t, coveringJSON("kb"), 0)
	e := newCodeMapEnv(t, bin)

	out, _ := runHookScript(t, codeMapScript, e.env, codeMapPayload("Read", e.file, "s1"))

	var got struct {
		HookSpecificOutput map[string]any `json:"hookSpecificOutput"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &got), out)
	ctx, _ := got.HookSpecificOutput["additionalContext"].(string)
	assert.Contains(t, ctx, "Why a is built so (decision-a) — Because b.")
	assert.Contains(t, ctx, "vaultmind note get")
	assert.NotContains(t, got.HookSpecificOutput, "permissionDecision", "it informs, it never decides")
	calls, err := os.ReadFile(log) //nolint:gosec // test fixture path
	require.NoError(t, err)
	assert.Contains(t, string(calls), "tree --vault "+e.vault+" --for "+e.file+" --json")
}

func TestCodeMap_TheSameFileIsMappedOncePerSession(t *testing.T) {
	bin, _ := codeMapStub(t, coveringJSON("kb"), 0)
	e := newCodeMapEnv(t, bin)

	first, _ := runHookScript(t, codeMapScript, e.env, codeMapPayload("Read", e.file, "s1"))
	again, _ := runHookScript(t, codeMapScript, e.env, codeMapPayload("Edit", e.file, "s1"))
	other, _ := runHookScript(t, codeMapScript, e.env, codeMapPayload("Read", e.file, "s2"))

	assert.NotEmpty(t, first)
	assert.Empty(t, again, "a map already shown this session is not shown again")
	assert.NotEmpty(t, other, "a new session gets it again")
}

func TestCodeMap_UncoveredCodeIsSilent(t *testing.T) {
	bin, _ := codeMapStub(t, emptyCoveringJSON, 0)
	e := newCodeMapEnv(t, bin)

	out, _ := runHookScript(t, codeMapScript, e.env, codeMapPayload("Read", e.file, "s1"))
	assert.Empty(t, out)
}

func TestCodeMap_AVaultNoteIsNotCode(t *testing.T) {
	bin, log := codeMapStub(t, emptyCoveringJSON, 0)
	e := newCodeMapEnv(t, bin)
	note := filepath.Join(e.vault, "decisions", "a.md")

	out, _ := runHookScript(t, codeMapScript, e.env, codeMapPayload("Read", note, "s1"))
	assert.Empty(t, out)
	_, err := os.Stat(log)
	assert.True(t, os.IsNotExist(err), "reading a vault note must not query the vault about it")
}

func TestCodeMap_AnOlderBinaryIsSilent(t *testing.T) {
	bin, _ := codeMapStub(t, "Error: unknown flag: --for", 1)
	e := newCodeMapEnv(t, bin)

	out, _ := runHookScript(t, codeMapScript, e.env, codeMapPayload("Read", e.file, "s1"))
	assert.Empty(t, out)
}

func TestCodeMap_AsksEveryConfiguredVault(t *testing.T) {
	bin, log := codeMapStub(t, emptyCoveringJSON, 0)
	e := newCodeMapEnv(t, bin)
	vaults := e.vault + "," + filepath.Join(e.project, "desk")
	env := append(append([]string{}, e.env...), "VAULTMIND_VAULTS="+vaults)

	runHookScript(t, codeMapScript, env, codeMapPayload("Read", e.file, "s1"))
	calls, err := os.ReadFile(log) //nolint:gosec // test fixture path
	require.NoError(t, err)
	assert.True(t, strings.Contains(string(calls), "tree --vaults "+vaults+" --for "+e.file+" --json"), string(calls))
}

// When the file changed after a note about it was committed, the map says so
// under that note, in the words `tree` chose — the hook adds none of its own.
func TestCodeMap_ShowsWhenTheCodeChangedAfterANote(t *testing.T) {
	summary := `code changed since this note: 2 commits, latest 2026-09-20 "fix: a" — check it still holds`
	stale := fmt.Sprintf(`{"status":"ok","result":{"for":"repo:internal/a.go","vaults":[{"vault":"kb","total":1,`+
		`"root":{"path":"","total":1,"dirs":[{"path":"decisions","total":1,"notes":[`+
		`{"id":"decision-a","path":"decisions/a.md","title":"Why a is built so","type":"decision",`+
		`"code_changed":{"commits":2,"date":"2026-09-20","latest":"fix: a","summary":%q}}]}]}}]}}`, summary)
	bin, _ := codeMapStub(t, stale, 0)
	e := newCodeMapEnv(t, bin)

	out, _ := runHookScript(t, codeMapScript, e.env, codeMapPayload("Read", e.file, "s1"))
	var got struct {
		HookSpecificOutput map[string]any `json:"hookSpecificOutput"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &got), out)
	ctx, _ := got.HookSpecificOutput["additionalContext"].(string)
	assert.Contains(t, ctx, "Why a is built so (decision-a)\n    "+summary)
}

// Notes from two vaults: `note get --vaults` with the configured list opens
// any of them, where a single --vault could name only one.
func TestCodeMap_NotesFromSeveralVaultsPointAtThemAll(t *testing.T) {
	two := `{"status":"ok","result":{"for":"repo:internal/a.go","vaults":[` +
		`{"vault":"kb","total":1,"root":{"path":"","total":1,"notes":[{"id":"decision-a","path":"a.md","title":"A","type":"decision"}]}},` +
		`{"vault":"desk","total":1,"root":{"path":"","total":1,"notes":[{"id":"journal-b","path":"b.md","title":"B","type":"journal"}]}}]}}`
	bin, _ := codeMapStub(t, two, 0)
	e := newCodeMapEnv(t, bin)
	vaults := e.vault + "," + filepath.Join(e.project, "desk")
	env := append(append([]string{}, e.env...), "VAULTMIND_VAULTS="+vaults)

	out, _ := runHookScript(t, codeMapScript, env, codeMapPayload("Read", e.file, "s1"))
	assert.Contains(t, out, "vaultmind note get <id> --vaults "+vaults)
	assert.NotContains(t, out, "<vault>")
}
