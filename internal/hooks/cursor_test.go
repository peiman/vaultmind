package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// LIVE-OBSERVED (2026-10-02, cursor-agent 2026.10.01): Cursor reads
// .cursor/hooks.json as {"version": 1, "hooks": {event: [{command, timeout}]}},
// adds context only from sessionStart and postToolUse, keeps the context of
// every hook on an event, and runs project hooks from the project root.

type cursorFile struct {
	Version int                    `json:"version"`
	Hooks   map[string][]cursorCmd `json:"hooks"`
}

type cursorCmd struct {
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
}

func renderCursor(t *testing.T, projectDir, vault string, p Profile) cursorFile {
	t.Helper()
	out, err := CursorHooksStanza(projectDir, vault, p)
	require.NoError(t, err)
	var f cursorFile
	require.NoError(t, json.Unmarshal([]byte(out), &f), out)
	return f
}

// commandFor returns the command on event that runs script, or "".
func cursorCommandFor(f cursorFile, event, script string) string {
	for _, c := range f.Hooks[event] {
		if strings.Contains(c.Command, " "+script+" ") || strings.HasSuffix(c.Command, " "+script) {
			return c.Command
		}
	}
	return ""
}

func TestCursorHooks_KnowledgeProfileWiresWhatCursorCanDeliver(t *testing.T) {
	project := t.TempDir()
	f := renderCursor(t, project, "/v/kb", ProfileKnowledge)

	assert.Equal(t, 1, f.Version)
	assert.NotEmpty(t, cursorCommandFor(f, "sessionStart", hookHealthScript))
	code := cursorCommandFor(f, "postToolUse", hookCodeMapScript)
	assert.Contains(t, code, "'Read|Write'", "the code map runs on reads and edits")
	assert.Contains(t, cursorCommandFor(f, "postToolUse", hookReachScript), "'Shell|Write'")
	assert.Contains(t, cursorCommandFor(f, "postToolUse", hookPreToolUseScript), "'Read'")

	for event := range f.Hooks {
		assert.Contains(t, []string{"sessionStart", "postToolUse"}, event,
			"only the events that can add context are wired")
	}
	assert.Empty(t, cursorCommandFor(f, "sessionStart", hookSessionStartScript), "the knowledge profile has no persona loader")
}

func TestCursorHooks_CommandsRunTheAdapterFromTheProject(t *testing.T) {
	project := t.TempDir()
	f := renderCursor(t, project, "/v/kb", ProfileKnowledge)
	cmd := cursorCommandFor(f, "postToolUse", hookCodeMapScript)

	adapter := filepath.Join(ScriptsDir(project, AgentCursor), cursorAdapterScript)
	assert.Contains(t, cmd, "export "+envProjectDir+"="+singleQuote(project)+"; ", "exported, not a prefix assignment")
	assert.Contains(t, cmd, "bash "+singleQuote(adapter)+" "+hookCodeMapScript)
	assert.Contains(t, cmd, "VAULTMIND_VAULT='/v/kb'")
	for _, c := range append(f.Hooks["sessionStart"], f.Hooks["postToolUse"]...) {
		assert.Positive(t, c.Timeout, "a hook that hangs must not hang the agent: %s", c.Command)
	}
}

func TestCursorHooks_PersonaProfileLoadsThePersona(t *testing.T) {
	f := renderCursor(t, t.TempDir(), "/v/id", ProfilePersona)
	assert.NotEmpty(t, cursorCommandFor(f, "sessionStart", hookSessionStartScript))
}

func TestCursorHooks_FederationReachesTheSearchingHooks(t *testing.T) {
	out, err := CursorHooksStanzaFor(t.TempDir(), "", []string{"/v/a", "/v/b"}, ProfileKnowledge)
	require.NoError(t, err)
	var f cursorFile
	require.NoError(t, json.Unmarshal([]byte(out), &f))
	assert.Contains(t, cursorCommandFor(f, "postToolUse", hookCodeMapScript), "VAULTMIND_VAULTS='/v/a,/v/b'")
}

func writeCursorHooks(t *testing.T, project, body string) string {
	t.Helper()
	p := CursorHooksPath(project)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o750))
	require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	return p
}

// Hooks the user wrote themselves survive, on our events and on others.
func TestMergeIntoCursorHooks_KeepsTheUsersHooks(t *testing.T) {
	project := t.TempDir()
	p := writeCursorHooks(t, project, `{"version":1,"hooks":{
		"postToolUse":[{"command":".cursor/hooks/audit.sh"}],
		"afterFileEdit":[{"command":".cursor/hooks/format.sh"}]}}`)

	res, err := MergeIntoCursorHooks(project, "/v/kb", ProfileKnowledge, false)
	require.NoError(t, err)
	assert.True(t, res.Changed)

	raw, err := os.ReadFile(p) //nolint:gosec // test path
	require.NoError(t, err)
	var f cursorFile
	require.NoError(t, json.Unmarshal(raw, &f))
	assert.Equal(t, ".cursor/hooks/audit.sh", f.Hooks["postToolUse"][0].Command, "the user's hook stays first")
	assert.Equal(t, ".cursor/hooks/format.sh", f.Hooks["afterFileEdit"][0].Command)
	assert.NotEmpty(t, cursorCommandFor(f, "postToolUse", hookCodeMapScript))
}

func TestMergeIntoCursorHooks_ARerunChangesNothing(t *testing.T) {
	project := t.TempDir()
	_, err := MergeIntoCursorHooks(project, "/v/kb", ProfileKnowledge, false)
	require.NoError(t, err)
	before, err := os.ReadFile(CursorHooksPath(project))
	require.NoError(t, err)

	res, err := MergeIntoCursorHooks(project, "/v/kb", ProfileKnowledge, false)
	require.NoError(t, err)
	assert.False(t, res.Changed)
	after, err := os.ReadFile(CursorHooksPath(project))
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after))
}

// Our entries are replaced, not duplicated, when the wiring changes (a moved
// vault, a new profile): an install never leaves an old command behind.
func TestMergeIntoCursorHooks_ReplacesItsOwnStaleEntries(t *testing.T) {
	project := t.TempDir()
	_, err := MergeIntoCursorHooks(project, "/v/old", ProfileKnowledge, false)
	require.NoError(t, err)
	_, err = MergeIntoCursorHooks(project, "/v/new", ProfileKnowledge, false)
	require.NoError(t, err)

	raw, err := os.ReadFile(CursorHooksPath(project))
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "/v/old")
	assert.Equal(t, 1, strings.Count(string(raw), " "+hookCodeMapScript+" "), "one code-map entry, not two")
}

func TestMergeIntoCursorHooks_RefusesAMalformedFile(t *testing.T) {
	project := t.TempDir()
	p := writeCursorHooks(t, project, `{"version":1,"hooks":`)

	_, err := MergeIntoCursorHooks(project, "/v/kb", ProfileKnowledge, false)
	require.Error(t, err)
	raw, rerr := os.ReadFile(p) //nolint:gosec // test path
	require.NoError(t, rerr)
	assert.Equal(t, `{"version":1,"hooks":`, string(raw), "a file we cannot read is left as it was")
}

func TestMergeIntoCursorHooks_DryRunWritesNothing(t *testing.T) {
	project := t.TempDir()
	res, err := MergeIntoCursorHooks(project, "/v/kb", ProfileKnowledge, true)
	require.NoError(t, err)
	assert.True(t, res.Changed)
	assert.NoFileExists(t, CursorHooksPath(project))
}

func TestRemoveFromCursorHooks_StripsOnlyOurs(t *testing.T) {
	project := t.TempDir()
	writeCursorHooks(t, project, `{"version":1,"hooks":{"postToolUse":[{"command":".cursor/hooks/audit.sh"}]}}`)
	_, err := MergeIntoCursorHooks(project, "/v/kb", ProfileKnowledge, false)
	require.NoError(t, err)

	res, err := RemoveFromCursorHooks(project, false)
	require.NoError(t, err)
	assert.True(t, res.Changed)
	raw, err := os.ReadFile(CursorHooksPath(project))
	require.NoError(t, err)
	assert.NotContains(t, string(raw), cursorAdapterScript)
	assert.Contains(t, string(raw), ".cursor/hooks/audit.sh")
	assert.NotContains(t, string(raw), "sessionStart", "an event left empty is dropped")
}

// A Cursor-only project is judged as one: its scripts are the ones
// .cursor/hooks.json runs, not Claude Code's. Found live: status graded such a
// project on .claude/scripts it never had and called 6 scripts out of date.
func TestStatus_ACursorOnlyProjectIsJudgedAsOne(t *testing.T) {
	project := t.TempDir()
	_, err := Install(InstallConfig{ProjectDir: project, Profile: ProfileKnowledge, Agent: AgentCursor})
	require.NoError(t, err)
	_, err = MergeIntoCursorHooks(project, "", ProfileKnowledge, false)
	require.NoError(t, err)

	r, err := Status(project)
	require.NoError(t, err)
	assert.Empty(t, r.Scripts, "no Claude Code scripts are expected of a Cursor project")
	assert.Empty(t, r.Events, "nor Claude Code wiring")
	require.NotNil(t, r.Cursor)
	inSync, drifted, missing := r.Cursor.ScriptCounts()
	assert.Zero(t, drifted+missing)
	assert.Equal(t, len(r.Cursor.Wired)+1, inSync, "every wired script and the adapter")
	assert.Contains(t, r.Cursor.Wired, hookCodeMapScript)
}

func TestStatus_ADriftedCursorScriptShows(t *testing.T) {
	project := t.TempDir()
	_, err := Install(InstallConfig{ProjectDir: project, Profile: ProfileKnowledge, Agent: AgentCursor})
	require.NoError(t, err)
	_, err = MergeIntoCursorHooks(project, "", ProfileKnowledge, false)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(ScriptsDir(project, AgentCursor), hookCodeMapScript),
		[]byte("#!/bin/bash\necho changed\n"), 0o600))

	r, err := Status(project)
	require.NoError(t, err)
	require.NotNil(t, r.Cursor)
	_, drifted, _ := r.Cursor.ScriptCounts()
	assert.Equal(t, 1, drifted)
}

func TestStatus_NoCursorWiringNoCursorSection(t *testing.T) {
	r, err := Status(t.TempDir())
	require.NoError(t, err)
	assert.Nil(t, r.Cursor)
}

// The adapter is Cursor's: a Claude Code or Codex install does not ship it.
func TestInstall_TheAdapterShipsOnlyForCursor(t *testing.T) {
	project := t.TempDir()
	_, err := Install(InstallConfig{ProjectDir: project, Profile: ProfileFull, Agent: AgentClaude})
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(ScriptsDir(project, AgentClaude), cursorAdapterScript))

	res, err := Install(InstallConfig{ProjectDir: project, Profile: ProfileKnowledge, Agent: AgentCursor})
	require.NoError(t, err)
	assert.Contains(t, res.Written, cursorAdapterScript)
	assert.FileExists(t, filepath.Join(ScriptsDir(project, AgentCursor), cursorAdapterScript))
	assert.FileExists(t, filepath.Join(ScriptsDir(project, AgentCursor), hookCodeMapScript))
}

// --only cannot put another agent's script where it is useless: the adapter
// in .claude/scripts would be a file nothing runs.
func TestInstall_OnlyRefusesAnotherAgentsScript(t *testing.T) {
	project := t.TempDir()
	_, err := Install(InstallConfig{ProjectDir: project, Only: []string{cursorAdapterScript}, Agent: AgentClaude})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--agent cursor")
	assert.NoFileExists(t, filepath.Join(ScriptsDir(project, AgentClaude), cursorAdapterScript))

	_, err = Install(InstallConfig{ProjectDir: project, Only: []string{cursorAdapterScript}, Agent: AgentCursor})
	require.NoError(t, err, "for Cursor it is its own script")
}

// Codex and Cursor share .vaultmind/scripts. Removing one agent's hooks with
// --remove-scripts must not break the other's.
func TestRemoveScripts_KeepsScriptsTheOtherAgentStillRuns(t *testing.T) {
	project := t.TempDir()
	_, err := Install(InstallConfig{ProjectDir: project, Profile: ProfileKnowledge, Agent: AgentCursor})
	require.NoError(t, err)
	_, err = MergeIntoCursorHooks(project, "", ProfileKnowledge, false)
	require.NoError(t, err)
	_, err = MergeIntoCodexHooks(project, "", ProfileKnowledge, false)
	require.NoError(t, err)

	res, err := RemoveFromCodexHooks(project, true)
	require.NoError(t, err)
	assert.Empty(t, res.ScriptsDeleted, "Cursor still runs them")
	assert.NotEmpty(t, res.ScriptsKept)
	assert.FileExists(t, filepath.Join(ScriptsDir(project, AgentCursor), hookCodeMapScript))

	res, err = RemoveFromCursorHooks(project, true)
	require.NoError(t, err)
	assert.NotEmpty(t, res.ScriptsDeleted, "no agent runs them any more")
	assert.NoFileExists(t, filepath.Join(ScriptsDir(project, AgentCursor), hookCodeMapScript))
}
