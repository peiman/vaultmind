package experiment

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
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
	transcript, err := readEvidenceTranscriptContext(context.Background(), path)
	require.NoError(t, err)
	require.Len(t, transcript.turns, 1)
	assert.Equal(t, 4, transcript.records, "only completely parsed records count toward the watermark")
	assert.Equal(t, "line-1", transcript.turns[0].id)
	require.Len(t, transcript.turns[0].texts, 1)
	assert.Equal(t, "note-id", transcript.turns[0].texts[0])
}

// A reader that delivers a prefix, then stalls until cancellation, models a
// slow transcript without requiring a huge fixture or timing disk throughput.
type slowEvidenceReader struct {
	prefix io.Reader
	ctx    context.Context
}

func (r *slowEvidenceReader) Read(p []byte) (int, error) {
	n, err := r.prefix.Read(p)
	if err != io.EOF {
		return n, err
	}
	<-r.ctx.Done()
	return 0, r.ctx.Err()
}
func TestEvidenceTranscript_DeadlineDiscardsPartialRead(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	reader := &slowEvidenceReader{ctx: ctx, prefix: strings.NewReader(`{"type":"user","sessionId":"s","timestamp":"2026-10-12T10:00:00Z","message":{"content":"Prompt"}}
{"type":"assistant","sessionId":"s","timestamp":"2026-10-12T10:00:01Z","message":{"content":"note-id"}}
`)}
	transcript, err := parseEvidenceTranscript(ctx, reader)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Nil(t, transcript, "a timeout cannot expose partial citation evidence for writing")
}

func TestEvidenceTranscript_CanceledBeforeRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	transcript, err := parseEvidenceTranscript(ctx, strings.NewReader("malformed"))
	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, transcript)
}

func TestUseEvidence_DeadlineRollsBackPartialWrites(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "evidence.db"))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	sid, err := db.StartSessionWithCaller("/vault", "vaultmind-reach-hook", map[string]any{MetaUserSessionID: "evidence-session"})
	require.NoError(t, err)
	event, err := db.LogEvent(Event{SessionID: sid, Type: EventAsk, VaultPath: "/vault", Data: BuildAskEventData(AskEventParams{RetrievalMode: "keyword", TopHits: []RetrievalHit{{NoteID: "note-cited"}, {NoteID: "note-tool"}}})})
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE events SET timestamp='2026-10-12T10:00:02Z' WHERE event_id=?`, event)
	require.NoError(t, err)
	// The first evidence write completes. The second runs until SQLite is
	// interrupted by the deadline, so the test exercises transaction rollback.
	_, err = db.Exec(`CREATE TRIGGER slow_second_evidence BEFORE INSERT ON events
 WHEN NEW.event_type='use_evidence' AND EXISTS (SELECT 1 FROM events WHERE event_type='use_evidence')
 BEGIN SELECT (WITH RECURSIVE slow(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM slow WHERE n<100000000) SELECT sum(n) FROM slow); END`)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	require.Error(t, db.recordUseEvidence(ctx, "testdata/use-evidence/turns.jsonl"))
	require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM events WHERE event_type='use_evidence'`).Scan(&count))
	assert.Zero(t, count, "even the completed first write is rolled back on timeout")
}
