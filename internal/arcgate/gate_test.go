package arcgate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeArcFixture(t *testing.T) (vault, triggers string) {
	t.Helper()
	vault = t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(vault, "arc.md"), []byte("---\nid: arc-half-check\ntype: arc\nstatus: accepted\ntitle: The Half I Thought to Check\n---\n## Principle\nCheck the thing before acting.\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(vault, "arc-plain.md"), []byte("---\nid: arc-plain\ntype: arc\nstatus: accepted\ntitle: Plain Words Are for His Decision\n---\n## Principle\nUse plain words when a decision is needed.\n"), 0o600))
	triggers = filepath.Join(vault, "triggers.yaml")
	return vault, triggers
}

func writeTriggers(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
}

func pretoolInput(t *testing.T, session, tool string, input map[string]any) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{"session_id": session, "tool_name": tool, "tool_input": input})
	require.NoError(t, err)
	return b
}

func stopInput(t *testing.T, session, msg string, active bool) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{"session_id": session, "stop_hook_active": active, "last_assistant_message": msg})
	require.NoError(t, err)
	return b
}

func TestGate_PreToolUseDenyOnceAndLog(t *testing.T) {
	vault, triggers := writeArcFixture(t)
	writeTriggers(t, triggers, "- arc: arc-half-check\n  on: pretooluse\n  tools: [Write]\n  match: '(?i)check'\n")
	state, logs := t.TempDir(), t.TempDir()
	now := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	input := pretoolInput(t, "session-1", "Write", map[string]any{"content": "check this"})
	out, errText := Gate(input, vault, "pretooluse", triggers, state, logs, now)
	require.Empty(t, errText)
	assert.Equal(t, "{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"Before this step, your arc The Half I Thought to Check (arc-half-check):\\nCheck the thing before acting.\\nIf it does not apply here, say why and continue.\"}}", out)
	out, errText = Gate(input, vault, "pretooluse", triggers, state, logs, now)
	assert.Empty(t, errText)
	assert.Empty(t, out)
	logBody, readErr := os.ReadFile(filepath.Join(logs, "arc-gate.jsonl"))
	require.NoError(t, readErr)
	assert.Equal(t, 2, strings.Count(string(logBody), "\n"))
	assert.Contains(t, string(logBody), "\"decision\":\"deny\"")
	assert.Contains(t, string(logBody), "\"decision\":\"allow-repeat\"")
}

func TestGate_DefaultAndEnvironmentTriggerPaths(t *testing.T) {
	vault, _ := writeArcFixture(t)
	defaultPath := filepath.Join(vault, ".vaultmind", "arc-triggers.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(defaultPath), 0o700))
	writeTriggers(t, defaultPath, "- arc: arc-half-check\n  on: stop\n  match: check\n")
	out, errText := Gate(stopInput(t, "default", "check", false), vault, "stop", "", t.TempDir(), t.TempDir(), time.Now())
	assert.Empty(t, errText)
	assert.Contains(t, out, "\"decision\":\"block\"")

	override := filepath.Join(t.TempDir(), "override.yaml")
	writeTriggers(t, override, "- arc: arc-plain\n  on: stop\n  match: choose\n")
	t.Setenv("VAULTMIND_ARC_TRIGGERS", override)
	out, errText = Gate(stopInput(t, "override", "choose", false), vault, "stop", "", t.TempDir(), t.TempDir(), time.Now())
	assert.Empty(t, errText)
	assert.Contains(t, out, "arc-plain")
}

func TestGate_PreToolUseFiltersToolAndUsesCompactJSON(t *testing.T) {
	vault, triggers := writeArcFixture(t)
	writeTriggers(t, triggers, "- arc: arc-half-check\n  on: pretooluse\n  tools: [Write]\n  match: '\"content\":\"check this\"'\n")
	input := pretoolInput(t, "s", "Edit", map[string]any{"content": "check this"})
	out, errText := Gate(input, vault, "pretooluse", triggers, t.TempDir(), t.TempDir(), time.Now())
	assert.Empty(t, errText)
	assert.Empty(t, out)
	input = pretoolInput(t, "s", "Write", map[string]any{"content": "check this"})
	out, errText = Gate(input, vault, "pretooluse", triggers, t.TempDir(), t.TempDir(), time.Now())
	assert.Empty(t, errText)
	assert.Contains(t, out, "\"permissionDecision\":\"deny\"")
}

func TestGate_StopPlainWordsAndActiveAllows(t *testing.T) {
	vault, triggers := writeArcFixture(t)
	writeTriggers(t, triggers, "- arc: arc-plain\n  on: stop\n  check: plain-words\n")
	state, logs := t.TempDir(), t.TempDir()
	message := "The change is small. It is easy to review. " + strings.Repeat("plain ", 72)
	out, errText := Gate(stopInput(t, "s", message, false), vault, "stop", triggers, state, logs, time.Now())
	assert.Empty(t, errText)
	assert.Contains(t, out, "\"decision\":\"block\"")
	assert.Contains(t, out, "What tripped it: no decision in the first two sentences.")
	out, errText = Gate(stopInput(t, "s", message, true), vault, "stop", triggers, state, logs, time.Now())
	assert.Empty(t, errText)
	assert.Empty(t, out)
}

func TestGate_RequireArmAllowsUntilArmedWithoutConsumingState(t *testing.T) {
	vault, triggers := writeArcFixture(t)
	writeTriggers(t, triggers, "- arc: arc-half-check\n  on: pretooluse\n  tools: [Write]\n  match: check\n")
	state, armed, logs := t.TempDir(), t.TempDir(), t.TempDir()
	out, errText := GateWithArmRequirement(pretoolInput(t, "unarmed", "Write", map[string]any{"text": "check"}), vault, "pretooluse", triggers, state, armed, logs, time.Now(), true)
	assert.Empty(t, errText)
	assert.Empty(t, out)
	_, err := os.Stat(filepath.Join(state, "unarmed"))
	assert.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(logs, "arc-gate.jsonl"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestGate_RequireArmBlocksAfterArm(t *testing.T) {
	vault, triggers := writeArcFixture(t)
	writeTriggers(t, triggers, "- arc: arc-half-check\n  on: pretooluse\n  tools: [Write]\n  match: check\n")
	state, armed, logs := t.TempDir(), t.TempDir(), t.TempDir()
	require.NoError(t, Arm(pretoolInput(t, "armed", "Write", map[string]any{"text": "check"}), armed))
	out, errText := GateWithArmRequirement(pretoolInput(t, "armed", "Write", map[string]any{"text": "check"}), vault, "pretooluse", triggers, state, armed, logs, time.Now(), true)
	assert.Empty(t, errText)
	assert.Contains(t, out, "\"permissionDecision\":\"deny\"")
}

func TestArm_WritesSanitizedMarkerAndRejectsBadInput(t *testing.T) {
	armed := t.TempDir()
	require.NoError(t, Arm([]byte(`{"session_id":"unsafe/session"}`), armed))
	_, err := os.Stat(filepath.Join(armed, "unsafe_session"))
	require.NoError(t, err)
	entries, err := os.ReadDir(armed)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Error(t, Arm([]byte("{"), armed))
}

func TestGate_StopUsesTranscriptFallback(t *testing.T) {
	vault, triggers := writeArcFixture(t)
	writeTriggers(t, triggers, "- arc: arc-half-check\n  on: stop\n  match: '(?i)before I merge'\n")
	transcript := filepath.Join(t.TempDir(), "conversation.jsonl")
	require.NoError(t, os.WriteFile(transcript, []byte("{\"type\":\"assistant\",\"message\":{\"content\":[{\"type\":\"text\",\"text\":\"old\"}]}}\n{\"type\":\"assistant\",\"message\":{\"content\":[{\"type\":\"text\",\"text\":\"Before I merge this.\"}]}}\n"), 0o600))
	b, err := json.Marshal(map[string]any{"session_id": "s", "transcript_path": transcript})
	require.NoError(t, err)
	out, errText := Gate(b, vault, "stop", triggers, t.TempDir(), t.TempDir(), time.Now())
	assert.Empty(t, errText)
	assert.Contains(t, out, "\"decision\":\"block\"")
}

func TestGate_StopRegexMatchesLastAssistantMessage(t *testing.T) {
	vault, triggers := writeArcFixture(t)
	writeTriggers(t, triggers, "- arc: arc-half-check\n  on: stop\n  match: '(?i)before I merge'\n")
	out, errText := Gate(stopInput(t, "s", "Before I merge this.", false), vault, "stop", triggers, t.TempDir(), t.TempDir(), time.Now())
	assert.Empty(t, errText)
	assert.Contains(t, out, "\"decision\":\"block\"")
}

func TestGate_FailurePathsAllowWithDiagnostic(t *testing.T) {
	vault, triggers := writeArcFixture(t)
	writeTriggers(t, triggers, "- arc: arc-half-check\n  on: stop\n  match: check\n")
	badYAML := filepath.Join(t.TempDir(), "bad.yaml")
	writeTriggers(t, badYAML, "- arc: [bad\n")
	tests := []struct {
		name, event, vault, triggers, state string
		input                               []byte
	}{
		{name: "missing session", event: "stop", vault: vault, triggers: triggers, state: t.TempDir(), input: []byte("{\"last_assistant_message\":\"check\"}")},
		{name: "bad json", event: "stop", vault: vault, triggers: triggers, state: t.TempDir(), input: []byte("{")},
		{name: "missing vault", event: "stop", vault: filepath.Join(t.TempDir(), "nope"), triggers: triggers, state: t.TempDir(), input: stopInput(t, "s", "check", false)},
		{name: "missing triggers", event: "stop", vault: vault, triggers: filepath.Join(t.TempDir(), "nope"), state: t.TempDir(), input: stopInput(t, "s", "check", false)},
		{name: "invalid event", event: "other", vault: vault, triggers: triggers, state: t.TempDir(), input: stopInput(t, "s", "check", false)},
		{name: "invalid yaml", event: "stop", vault: vault, triggers: badYAML, state: t.TempDir(), input: stopInput(t, "s", "check", false)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, errText := Gate(tc.input, tc.vault, tc.event, tc.triggers, tc.state, t.TempDir(), time.Now())
			assert.Empty(t, out)
			assert.NotEmpty(t, errText)
		})
	}
}

func TestGate_ValidationErrorsNameEntryAndArc(t *testing.T) {
	vault, triggers := writeArcFixture(t)
	for _, body := range []string{
		"- on: stop\n  match: x\n",
		"- arc: arc-half-check\n  on: nope\n  match: x\n",
		"- arc: arc-half-check\n  on: stop\n  match: '['\n",
		"- arc: arc-half-check\n  on: stop\n",
		"- arc: arc-half-check\n  on: stop\n  match: x\n  check: plain-words\n",
		"- arc: arc-half-check\n  on: pretooluse\n  check: plain-words\n",
		"- arc: arc-half-check\n  on: stop\n  check: nope\n",
		"- arc: missing\n  on: stop\n  match: x\n",
	} {
		writeTriggers(t, triggers, body)
		_, errText := Gate(stopInput(t, "s", "x", false), vault, "stop", triggers, t.TempDir(), t.TempDir(), time.Now())
		assert.Regexp(t, "entry 0|arc", errText)
	}
}

func TestGate_StateFileFormatAndIndependentSessions(t *testing.T) {
	vault, triggers := writeArcFixture(t)
	writeTriggers(t, triggers, "- arc: arc-half-check\n  on: stop\n  match: check\n- arc: arc-plain\n  on: stop\n  match: check\n")
	state := t.TempDir()
	for _, session := range []string{"one", "two"} {
		out, errText := Gate(stopInput(t, session, "check", false), vault, "stop", triggers, state, t.TempDir(), time.Now())
		assert.Empty(t, errText)
		assert.Contains(t, out, "\"decision\":\"block\"")
	}
	body, err := os.ReadFile(filepath.Join(state, "one"))
	require.NoError(t, err)
	assert.Equal(t, "arc-half-check\n", string(body))
	assert.NotContains(t, string(body), ".tmp")
}

func TestGate_LaterArcCanFireAfterAnEarlierArc(t *testing.T) {
	vault, triggers := writeArcFixture(t)
	writeTriggers(t, triggers, "- arc: arc-half-check\n  on: stop\n  match: check\n- arc: arc-plain\n  on: stop\n  match: check\n")
	state := t.TempDir()
	out, errText := Gate(stopInput(t, "s", "check", false), vault, "stop", triggers, state, t.TempDir(), time.Now())
	require.Empty(t, errText)
	assert.Contains(t, out, "arc-half-check")
	out, errText = Gate(stopInput(t, "s", "check", false), vault, "stop", triggers, state, t.TempDir(), time.Now())
	require.Empty(t, errText)
	assert.Contains(t, out, "arc-plain")
}

func TestGate_UnwritableStateAllows(t *testing.T) {
	vault, triggers := writeArcFixture(t)
	writeTriggers(t, triggers, "- arc: arc-half-check\n  on: stop\n  match: check\n")
	stateFile := filepath.Join(t.TempDir(), "state-file")
	require.NoError(t, os.WriteFile(stateFile, []byte("not a directory"), 0o600))
	out, errText := Gate(stopInput(t, "s", "check", false), vault, "stop", triggers, stateFile, t.TempDir(), time.Now())
	assert.Empty(t, out)
	assert.Contains(t, errText, "state")
}
