package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEpisodeCapture_RecordsUseEvidence(t *testing.T) {
	db, err := experiment.OpenMemory()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	sid, err := db.StartSessionWithCaller("/vault", "vaultmind-userprompt-hook", map[string]any{experiment.MetaUserSessionID: "evidence-session"})
	require.NoError(t, err)
	surface, err := db.LogEvent(experiment.Event{SessionID: sid, Type: experiment.EventAsk, VaultPath: "/vault", Data: experiment.BuildAskEventData(experiment.AskEventParams{RetrievalMode: "hybrid", TopHits: []experiment.RetrievalHit{{NoteID: "note-cited"}}})})
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE events SET timestamp='2026-10-12T10:00:02Z' WHERE event_id=?`, surface)
	require.NoError(t, err)
	cmd := episodeCaptureCmd
	for _, name := range []string{"output-dir", "cursor-dir", "incremental"} {
		value := cmd.Flags().Lookup(name).Value.String()
		t.Cleanup(func() { require.NoError(t, cmd.Flags().Set(name, value)) })
	}
	oldContext := cmd.Context()
	// Restore inheritance, rather than pinning the previously inherited root
	// writer on this child and swallowing later tests' command output.
	t.Cleanup(func() { cmd.SetContext(oldContext); cmd.SetOut(nil) })
	cmd.SetContext(experiment.WithSession(context.Background(), &experiment.Session{DB: db, ID: sid}))
	var out bytes.Buffer
	cmd.SetOut(&out)
	for _, incremental := range []string{"false", "true"} {
		require.NoError(t, cmd.Flags().Set("output-dir", t.TempDir()))
		require.NoError(t, cmd.Flags().Set("cursor-dir", t.TempDir()))
		require.NoError(t, cmd.Flags().Set("incremental", incremental))
		require.NoError(t, cmd.RunE(cmd, []string{"../internal/experiment/testdata/use-evidence/turns.jsonl"}))
	}
	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM events WHERE event_type='use_evidence' AND json_extract(event_data,'$.evidence')='cited'`).Scan(&count))
	assert.Equal(t, 1, count, "the actual capture entrypoint records evidence idempotently")
}

func TestEpisodeEvidence_OffAndFailureDoNotBlockCapture(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background()) // telemetry off: no experiment session
	recordEpisodeUseEvidence(cmd, "nonexistent-transcript")
	db, err := experiment.OpenMemory()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	cmd.SetContext(experiment.WithSession(context.Background(), &experiment.Session{DB: db}))
	recordEpisodeUseEvidence(cmd, "nonexistent-transcript")
	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM events WHERE event_type='use_evidence'`).Scan(&count))
	assert.Zero(t, count)
}

func TestDoctorEvidence_JSONAddsFieldsAndKeepsConsumed(t *testing.T) {
	raw, err := json.Marshal(query.DoctorMemoryUse{Injections: 5, Consumed: 1, Opened: 1, Cited: 2, Unknown: 2})
	require.NoError(t, err)
	var fields map[string]any
	require.NoError(t, json.Unmarshal(raw, &fields))
	for key, want := range map[string]float64{"injections": 5, "consumed": 1, "opened": 1, "cited": 2, "unknown": 2} {
		assert.Equal(t, want, fields[key], key)
	}
}

func TestCollectMemoryUse_EvidenceFields(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	db, err := openExperimentDB()
	require.NoError(t, err)
	sid, err := db.StartSessionWithCaller("/vault", "vaultmind-userprompt-hook", map[string]any{experiment.MetaUserSessionID: "evidence-session"})
	require.NoError(t, err)
	surface, err := db.LogEvent(experiment.Event{SessionID: sid, Type: experiment.EventAsk, VaultPath: "/vault", Data: experiment.BuildAskEventData(experiment.AskEventParams{RetrievalMode: "hybrid", TopHits: []experiment.RetrievalHit{{NoteID: "note-cited"}}})})
	require.NoError(t, err)
	_, err = db.LogEvent(experiment.Event{SessionID: sid, Type: experiment.EventUseEvidence, VaultPath: "/vault", Data: map[string]any{"surfaced_event_id": surface, "note_id": "note-cited", "evidence": "cited"}})
	require.NoError(t, err)
	require.NoError(t, db.Close())
	idb, err := index.Open(":memory:")
	require.NoError(t, err)
	defer func() { _ = idb.Close() }()
	use := collectMemoryUse(&cmdutil.VaultDB{DB: idb})
	require.NotNil(t, use)
	assert.Equal(t, 1, use.Cited)
	assert.Equal(t, 0, use.Opened)
	assert.Equal(t, 0, use.Unknown)
	require.Len(t, use.PerCaller, 1)
	assert.Equal(t, 1, use.PerCaller[0].Cited)
}
