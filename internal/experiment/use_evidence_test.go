package experiment_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func evidenceSession(t *testing.T, db *experiment.DB, user, caller, vault string) string {
	t.Helper()
	sid, err := db.StartSessionWithCaller(vault, caller, map[string]any{experiment.MetaUserSessionID: user})
	require.NoError(t, err)
	return sid
}

func evidenceEvent(t *testing.T, db *experiment.DB, sid, kind, vault string, at time.Time, data map[string]any) string {
	t.Helper()
	id, err := db.LogEvent(experiment.Event{SessionID: sid, Type: kind, VaultPath: vault, Data: data})
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE events SET timestamp = ? WHERE event_id = ?`, at.Format(time.RFC3339Nano), id)
	require.NoError(t, err)
	return id
}

func evidenceStates(t *testing.T, db *experiment.DB) map[string]string {
	t.Helper()
	rows, err := db.Query(`SELECT event_data FROM events WHERE event_type = 'use_evidence'`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var raw string
		require.NoError(t, rows.Scan(&raw))
		var data struct {
			TurnID          string `json:"turn_id"`
			SurfacedEventID string `json:"surfaced_event_id"`
			UserSessionID   string `json:"user_session_id"`
			NoteID          string `json:"note_id"`
			Evidence        string `json:"evidence"`
		}
		require.NoError(t, json.Unmarshal([]byte(raw), &data))
		require.Equal(t, "turn-one", data.TurnID)
		require.NotEmpty(t, data.SurfacedEventID)
		require.Equal(t, "evidence-session", data.UserSessionID)
		out[data.NoteID] = data.Evidence
	}
	require.NoError(t, rows.Err())
	return out
}

func TestUseEvidence_Fixture(t *testing.T) {
	db := openTestExpDB(t)
	at, err := time.Parse(time.RFC3339, "2026-10-12T10:00:02Z")
	require.NoError(t, err)
	sid := evidenceSession(t, db, "evidence-session", "vaultmind-userprompt-hook", "/vault")
	cases := []struct{ id, want string }{
		{"note-opened", "opened"}, {"note-cited", "cited"}, {"note-injected", "unknown"},
		{"note-title", "unknown"}, {"note-tool", "cited"}, {"note-user", "unknown"},
		{"note-result", "unknown"}, {"note-thinking", "unknown"}, {"note-metadata", "unknown"},
		{"note-sidechain", "unknown"}, {"note-other-session", "unknown"}, {"note-prefix", "unknown"},
		{"note-string", "cited"}, {"note-later", "unknown"}, {"note-foreign-read", "unknown"},
		{"note-other-vault", "unknown"}, {"note-ask-access", "unknown"},
	}
	hits := make([]experiment.RetrievalHit, 0, len(cases))
	for _, c := range cases {
		hits = append(hits, experiment.RetrievalHit{NoteID: c.id, Rank: 1})
	}
	evidenceEvent(t, db, sid, experiment.EventAsk, "/vault", at, experiment.BuildAskEventData(experiment.AskEventParams{RetrievalMode: "hybrid", TopHits: hits, BodyDelivered: true}))
	for _, c := range []struct {
		note, user, vault string
		source            experiment.AccessSource
	}{
		{"note-opened", "evidence-session", "/vault", experiment.AccessSourceRead},
		{"note-foreign-read", "other-session", "/vault", experiment.AccessSourceRead},
		{"note-other-vault", "evidence-session", "/other-vault", experiment.AccessSourceRead},
		{"note-ask-access", "evidence-session", "/vault", experiment.AccessSourceAsk},
	} {
		readSID := evidenceSession(t, db, c.user, "claude-code", c.vault)
		evidenceEvent(t, db, readSID, experiment.EventNoteAccess, c.vault, at.Add(2*time.Second), map[string]any{"note_id": c.note, "source": c.source})
	}
	require.NoError(t, db.RecordUseEvidence("testdata/use-evidence/turns.jsonl"))
	states := evidenceStates(t, db)
	require.Len(t, states, len(cases))
	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) { assert.Equal(t, c.want, states[c.id]) })
	}
	require.NoError(t, db.RecordUseEvidence("testdata/use-evidence/turns.jsonl"))
	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM events WHERE event_type='use_evidence'`).Scan(&count))
	assert.Equal(t, len(cases), count, "repeated capture must be idempotent")
}

func TestUseEvidence_LateReadUpgradesAndOpenedWins(t *testing.T) {
	db := openTestExpDB(t)
	at, err := time.Parse(time.RFC3339, "2026-10-12T10:00:02Z")
	require.NoError(t, err)
	sid := evidenceSession(t, db, "evidence-session", "vaultmind-userprompt-hook", "/vault")
	evidenceEvent(t, db, sid, experiment.EventAsk, "/vault", at, experiment.BuildAskEventData(experiment.AskEventParams{RetrievalMode: "keyword", TopHits: []experiment.RetrievalHit{{NoteID: "note-cited"}}}))
	require.NoError(t, db.RecordUseEvidence("testdata/use-evidence/turns.jsonl"))
	assert.Equal(t, "cited", evidenceStates(t, db)["note-cited"])
	readSID := evidenceSession(t, db, "evidence-session", "cli", "/vault")
	evidenceEvent(t, db, readSID, experiment.EventNoteAccess, "/vault", at.Add(6*time.Second), map[string]any{"note_id": "note-cited", "source": experiment.AccessSourceRead})
	require.NoError(t, db.RecordUseEvidence("testdata/use-evidence/turns.jsonl"))
	assert.Equal(t, "opened", evidenceStates(t, db)["note-cited"])
}

func TestUseEvidence_ReportsTranscriptAndDatabaseErrors(t *testing.T) {
	db := openTestExpDB(t)
	require.Error(t, db.RecordUseEvidence(filepath.Join(t.TempDir(), "missing")))
	path := filepath.Join(t.TempDir(), "bad.jsonl")
	require.NoError(t, os.WriteFile(path, []byte("not json\n"), 0o600))
	require.Error(t, db.RecordUseEvidence(path))
	require.NoError(t, db.Close())
	require.Error(t, db.RecordUseEvidence("testdata/use-evidence/turns.jsonl"))
}

func TestMemoryUse_EvidenceRollup(t *testing.T) {
	db := openTestExpDB(t)
	at := time.Now().UTC().Add(-time.Minute)
	sid := evidenceSession(t, db, "rollup-session", "vaultmind-userprompt-hook", "/vault")
	hits := []experiment.RetrievalHit{{NoteID: "opened"}, {NoteID: "cited"}, {NoteID: "unknown"}, {NoteID: "unmeasured"}}
	surface := evidenceEvent(t, db, sid, experiment.EventAsk, "/vault", at, experiment.BuildAskEventData(experiment.AskEventParams{RetrievalMode: "keyword", TopHits: hits, BodyDelivered: true}))
	for _, c := range []struct{ note, state string }{{"opened", "cited"}, {"cited", "cited"}, {"unknown", "unknown"}} {
		evidenceEvent(t, db, sid, "use_evidence", "/vault", at.Add(time.Second), map[string]any{"note_id": c.note, "evidence": c.state, "surfaced_event_id": surface})
	}
	readSID := evidenceSession(t, db, "rollup-session", "cli", "/vault")
	evidenceEvent(t, db, readSID, experiment.EventNoteAccess, "/vault", at.Add(3*time.Second), map[string]any{"note_id": "opened", "source": experiment.AccessSourceRead})
	otherSID := evidenceSession(t, db, "other-session", "cli", "/vault")
	evidenceEvent(t, db, otherSID, experiment.EventNoteAccess, "/vault", at.Add(3*time.Second), map[string]any{"note_id": "unmeasured", "source": experiment.AccessSourceRead})
	use, err := db.MemoryUseSince(7)
	require.NoError(t, err)
	assert.Equal(t, 4, use.Injections)
	assert.Equal(t, 4, use.BodiesDelivered)
	assert.Equal(t, 1, use.Opened)
	assert.Equal(t, 1, use.Cited)
	assert.Equal(t, 2, use.Unknown, "no observation is unknown, never unused")
	require.Len(t, use.PerCaller, 1)
	assert.Equal(t, 1, use.PerCaller[0].Opened)
	assert.Equal(t, 1, use.PerCaller[0].Cited)
	assert.Equal(t, 2, use.PerCaller[0].Unknown)
}

func TestUseEvidence_PartialRecapturePreservesObservedEvidence(t *testing.T) {
	db := openTestExpDB(t)
	at, err := time.Parse(time.RFC3339, "2026-10-12T10:00:02Z")
	require.NoError(t, err)
	sid := evidenceSession(t, db, "evidence-session", "vaultmind-userprompt-hook", "/vault")
	evidenceEvent(t, db, sid, experiment.EventAsk, "/vault", at, experiment.BuildAskEventData(experiment.AskEventParams{RetrievalMode: "hybrid", TopHits: []experiment.RetrievalHit{{NoteID: "note-cited"}}}))
	require.NoError(t, db.RecordUseEvidence("testdata/use-evidence/turns.jsonl"))
	path := filepath.Join(t.TempDir(), "partial.jsonl")
	raw, err := os.ReadFile("testdata/use-evidence/turns.jsonl")
	require.NoError(t, err)
	firstLine := bytes.SplitN(raw, []byte("\n"), 2)[0]
	require.NoError(t, os.WriteFile(path, append(firstLine, '\n'), 0o600))
	require.NoError(t, db.RecordUseEvidence(path))
	assert.Equal(t, "cited", evidenceStates(t, db)["note-cited"], "a stale concurrent capture must not erase observed evidence")
}

func TestUseEvidence_PromptHookSameSecondBelongsToNextTurn(t *testing.T) {
	db := openTestExpDB(t)
	at, err := time.Parse(time.RFC3339, "2026-10-12T10:00:02Z")
	require.NoError(t, err)
	sid := evidenceSession(t, db, "evidence-session", "vaultmind-userprompt-hook", "/vault")
	evidenceEvent(t, db, sid, experiment.EventAsk, "/vault", at, experiment.BuildAskEventData(experiment.AskEventParams{RetrievalMode: "hybrid", TopHits: []experiment.RetrievalHit{{NoteID: "note-id"}}}))
	path := filepath.Join(t.TempDir(), "same-second.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(`{"type":"user","sessionId":"evidence-session","uuid":"old","timestamp":"2026-10-12T10:00:00Z","message":{"content":"First prompt"}}
{"type":"assistant","sessionId":"evidence-session","timestamp":"2026-10-12T10:00:02.100Z","message":{"content":"note-id"}}
{"type":"user","sessionId":"evidence-session","uuid":"new","timestamp":"2026-10-12T10:00:02.900Z","message":{"content":"Second prompt"}}
{"type":"assistant","sessionId":"evidence-session","timestamp":"2026-10-12T10:00:03Z","message":{"content":"No citation"}}
`), 0o600))
	require.NoError(t, db.RecordUseEvidence(path))
	var state, turn string
	require.NoError(t, db.QueryRow(`SELECT json_extract(event_data,'$.evidence'),json_extract(event_data,'$.turn_id') FROM events WHERE event_type='use_evidence'`).Scan(&state, &turn))
	assert.Equal(t, "unknown", state, "second-resolution retrieval timestamps must not borrow a preceding reply")
	assert.Equal(t, "new", turn)
}

func TestUseEvidence_CitationAnywhereInTheSameTurnIsEvidence(t *testing.T) {
	db := openTestExpDB(t)
	at, err := time.Parse(time.RFC3339, "2026-10-12T10:00:08Z")
	require.NoError(t, err)
	sid := evidenceSession(t, db, "evidence-session", "vaultmind-reach-hook", "/vault")
	evidenceEvent(t, db, sid, experiment.EventAsk, "/vault", at, experiment.BuildAskEventData(experiment.AskEventParams{RetrievalMode: "hybrid", TopHits: []experiment.RetrievalHit{{NoteID: "note-tool"}}}))
	require.NoError(t, db.RecordUseEvidence("testdata/use-evidence/turns.jsonl"))
	assert.Equal(t, "cited", evidenceStates(t, db)["note-tool"], "same-turn ID evidence does not claim the injection caused it")
}

// Exercise both capture orders and correction of evidence written before this fix.
func TestUseEvidence_StaleTailAndReattribution(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(fmt.Sprintf("legacy=%t", legacy), func(t *testing.T) {
			db := openTestExpDB(t)
			at := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
			sid := evidenceSession(t, db, "correction-session", "vaultmind-reach-hook", "/vault")
			data := experiment.BuildAskEventData(experiment.AskEventParams{RetrievalMode: "keyword", TopHits: []experiment.RetrievalHit{{NoteID: "note-id"}}})
			first := evidenceEvent(t, db, sid, experiment.EventAsk, "/vault", at.Add(time.Second), data)
			second := evidenceEvent(t, db, sid, experiment.EventAsk, "/vault", at.Add(3*time.Second), data)
			stale := writeEvidenceTurns(t, at, false)
			full := writeEvidenceTurns(t, at, true)
			require.NoError(t, db.RecordUseEvidence(stale))
			assertAppearanceEvidence(t, db, first, "cited", "one")
			if legacy {
				// Simulate the erroneous final-turn attribution from the previous implementation.
				_, err := db.Exec(`UPDATE events SET event_data=json_remove(json_set(event_data,'$.evidence','cited','$.turn_id','one'),'$.capture_through_ns') WHERE event_type='use_evidence' AND json_extract(event_data,'$.surfaced_event_id')=?`, second)
				require.NoError(t, err)
			} else {
				assertAppearanceEvidence(t, db, second, "unknown", "")
			}
			require.NoError(t, db.RecordUseEvidence(full))
			assertAppearanceEvidence(t, db, first, "cited", "one")
			assertAppearanceEvidence(t, db, second, "unknown", "two")
			require.NoError(t, db.RecordUseEvidence(stale))
			assertAppearanceEvidence(t, db, second, "unknown", "two")
		})
	}
}

func writeEvidenceTurns(t *testing.T, at time.Time, full bool) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "turns.jsonl")
	raw := fmt.Sprintf(`{"type":"user","sessionId":"correction-session","uuid":"one","timestamp":%q,"message":{"content":"First prompt"}}
{"type":"assistant","sessionId":"correction-session","timestamp":%q,"message":{"content":"note-id"}}
`, at.Format(time.RFC3339Nano), at.Add(2*time.Second).Format(time.RFC3339Nano))
	if full {
		raw += fmt.Sprintf(`{"type":"user","sessionId":"correction-session","uuid":"two","timestamp":%q,"message":{"content":"Second prompt"}}
{"type":"assistant","sessionId":"correction-session","timestamp":%q,"message":{"content":"No citation"}}
`, at.Add(4*time.Second).Format(time.RFC3339Nano), at.Add(5*time.Second).Format(time.RFC3339Nano))
	}
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o600))
	return path
}

func assertAppearanceEvidence(t *testing.T, db *experiment.DB, event, state, turn string) {
	t.Helper()
	var gotState, gotTurn string
	require.NoError(t, db.QueryRow(`SELECT json_extract(event_data,'$.evidence'),json_extract(event_data,'$.turn_id') FROM events WHERE event_type='use_evidence' AND json_extract(event_data,'$.surfaced_event_id')=?`, event).Scan(&gotState, &gotTurn))
	assert.Equal(t, state, gotState)
	assert.Equal(t, turn, gotTurn)
}

func TestMemoryUse_MappedUnknownWinsOverNextTurnRead(t *testing.T) {
	db := openTestExpDB(t)
	at := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	sid := evidenceSession(t, db, "correction-session", "vaultmind-reach-hook", "/vault")
	surface := evidenceEvent(t, db, sid, experiment.EventAsk, "/vault", at.Add(time.Second), experiment.BuildAskEventData(experiment.AskEventParams{RetrievalMode: "keyword", TopHits: []experiment.RetrievalHit{{NoteID: "not-cited"}}}))
	readSID := evidenceSession(t, db, "correction-session", "cli", "/vault")
	evidenceEvent(t, db, readSID, experiment.EventNoteAccess, "/vault", at.Add(5*time.Second), map[string]any{"note_id": "not-cited", "source": experiment.AccessSourceRead})
	require.NoError(t, db.RecordUseEvidence(writeEvidenceTurns(t, at, true)))
	assertAppearanceEvidence(t, db, surface, "unknown", "one")
	use, err := db.MemoryUseSince(7)
	require.NoError(t, err)
	assert.Equal(t, 0, use.Opened)
	assert.Equal(t, 1, use.Unknown)
	require.Len(t, use.PerCaller, 1)
	assert.Equal(t, 0, use.PerCaller[0].Opened)
	assert.Equal(t, 1, use.PerCaller[0].Unknown)
	assert.Equal(t, 1, use.Consumed, "legacy consumed contract is unchanged")
}

func TestUseEvidence_EqualTimestampPartialRecapture(t *testing.T) {
	db := openTestExpDB(t)
	at := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	sid := evidenceSession(t, db, "correction-session", "vaultmind-reach-hook", "/vault")
	event := evidenceEvent(t, db, sid, experiment.EventAsk, "/vault", at, experiment.BuildAskEventData(experiment.AskEventParams{RetrievalMode: "keyword", TopHits: []experiment.RetrievalHit{{NoteID: "note-id"}}}))
	stale := filepath.Join(t.TempDir(), "stale.jsonl")
	full := filepath.Join(t.TempDir(), "full.jsonl")
	prompt := fmt.Sprintf(`{"type":"user","sessionId":"correction-session","uuid":"one","timestamp":%q,"message":{"content":"Prompt"}}
`, at.Format(time.RFC3339Nano))
	reply := fmt.Sprintf(`{"type":"assistant","sessionId":"correction-session","timestamp":%q,"message":{"content":"note-id"}}
`, at.Format(time.RFC3339Nano))
	require.NoError(t, os.WriteFile(stale, []byte(prompt), 0o600))
	require.NoError(t, os.WriteFile(full, []byte(prompt+reply), 0o600))
	require.NoError(t, db.RecordUseEvidence(full))
	require.NoError(t, db.RecordUseEvidence(stale))
	assertAppearanceEvidence(t, db, event, "cited", "one")
}
