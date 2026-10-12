package experiment

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvidenceID_ExactIDsOnly(t *testing.T) {
	for _, c := range []struct {
		text, id string
		want     bool
	}{
		{"`concept-alpha`", "concept-alpha", true},
		{"/notes/concept-alpha.md", "concept-alpha", true},
		{"concept-alpha-more", "concept-alpha", false},
		{"other-concept-alpha", "concept-alpha", false},
		{"concept-alpha_more", "concept-alpha", false},
		{"Concept-alpha", "concept-alpha", false},
		{"The Governing Principle", "note-title", false},
		{"concept-alpha-more, then concept-alpha.", "concept-alpha", true},
		{"éconcept-alpha", "concept-alpha", false},
		{"(概念-甲)", "概念-甲", true},
		{"anything", "", false},
	} {
		t.Run(c.text, func(t *testing.T) { assert.Equal(t, c.want, exactEvidenceID(c.text, c.id)) })
	}
}

func TestEvidencePrompt_OnlyHumanTurnBoundaries(t *testing.T) {
	for _, c := range []struct {
		raw  string
		want bool
	}{
		{`"Hello"`, true}, {`[{"type":"text","text":"Hello"},{"type":"image"}]`, true},
		{`[{"type":"tool_result","content":"hello"}]`, false},
		{`"<task-notification>done"`, false}, {`"<local-command-stdout>done"`, false},
		{`"<command-message>done"`, false}, {`"<system-reminder>hello"`, false},
		{`" "`, false}, {`{}`, false}, {`null`, false},
	} {
		t.Run(c.raw, func(t *testing.T) { assert.Equal(t, c.want, isEvidencePrompt(json.RawMessage(c.raw))) })
	}
}

func TestAssistantEvidence_ValuesOnly(t *testing.T) {
	assert.Equal(t, []string{"a", "b", "c"}, assistantEvidenceText(json.RawMessage(`[{"type":"text","text":"a"},{"type":"tool_use","id":"ignored","input":{"key":["b",{"name":"c"},7,true]}}]`)))
	assert.Empty(t, assistantEvidenceText(json.RawMessage(`{}`)))
	assert.Empty(t, assistantEvidenceText(json.RawMessage(`[{"type":"tool_use","input":null},{"type":"thinking","text":"ignored"}]`)))
}

func TestEvidenceTurn_BeforePromptAndAcrossTurns(t *testing.T) {
	base := time.Now().UTC()
	turns := []evidenceTurn{
		{id: "one", start: base, last: base.Add(time.Minute), next: base.Add(2 * time.Minute)},
		{id: "two", start: base.Add(2 * time.Minute), last: base.Add(3 * time.Minute)},
	}
	for _, c := range []struct {
		at   time.Time
		want string
	}{
		{base.Add(-time.Second), "one"}, {base.Add(30 * time.Second), "one"},
		{base.Add(119 * time.Second), "two"}, {base.Add(2 * time.Minute), "two"},
	} {
		assert.Equal(t, c.want, evidenceTurnFor(turns, c.at).id)
	}
	assert.Nil(t, evidenceTurnFor(nil, base))
}

func TestEvidenceTranscript_PartialTailAndMissingTimestamps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "transcript.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(`{"type":"user","sessionId":"s","timestamp":"2026-10-12T10:00:00Z","message":{"role":"user","content":"Hello"}}
{"type":"assistant","sessionId":"s","message":{"role":"assistant","content":"no timestamp"}}
{"type":"assistant","sessionId":"s","timestamp":"2026-10-12T10:00:01Z","message":{"role":"user","content":"wrong role"}}
{"type":"assistant","sessionId":"s","timestamp":"2026-10-12T10:00:02Z","message":{"role":"assistant","content":"note-id"}}
{"type":"assistant"`), 0o600))
	transcript, err := readEvidenceTranscript(path)
	require.NoError(t, err)
	require.Len(t, transcript.turns, 1)
	assert.Equal(t, "line-1", transcript.turns[0].id)
	require.Len(t, transcript.turns[0].texts, 1)
	assert.Equal(t, "note-id", transcript.turns[0].texts[0])
}
