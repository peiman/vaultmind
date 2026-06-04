package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// contextNoteIDs projects memory.ContextItem to []string of IDs preserving
// order. The rank ordering is the contract the experiment layer relies on —
// losing it would mean shadow-variant comparisons become meaningless.
func TestContextNoteIDs_PreservesOrder(t *testing.T) {
	items := []memory.ContextItem{
		{ID: "a"}, {ID: "b"}, {ID: "c"},
	}
	assert.Equal(t, []string{"a", "b", "c"}, contextNoteIDs(items))
}

func TestContextNoteIDs_EmptyYieldsEmpty(t *testing.T) {
	assert.Empty(t, contextNoteIDs(nil))
	assert.Empty(t, contextNoteIDs([]memory.ContextItem{}))
}

// askRetrievalHits adapts the ask result shape. Nil-in → nil-out so callers
// can pass a failed result through without a conditional.
func TestAskRetrievalHits_NilResultPassThrough(t *testing.T) {
	assert.Nil(t, askRetrievalHits(nil))
}

// A successful result with TopHits produces the same number of
// RetrievalHits with 1-indexed ranks and the original scores preserved.
func TestAskRetrievalHits_RankIsOneIndexedScoresPreserved(t *testing.T) {
	r := &query.AskResult{
		TopHits: []retrieval.ScoredResult{
			{ID: "a", Score: 0.9, Type: "concept", Path: "a.md"},
			{ID: "b", Score: 0.7, Type: "concept", Path: "b.md"},
		},
	}
	got := askRetrievalHits(r)
	require.Len(t, got, 2)
	assert.Equal(t, "a", got[0].NoteID)
	assert.Equal(t, 1, got[0].Rank, "first hit is rank 1, not 0")
	assert.Equal(t, 0.9, got[0].Score)
	assert.Equal(t, 2, got[1].Rank)
	assert.Equal(t, "b", got[1].NoteID)
}

// writeZeroHitDiagnostics fires the keyword-only hint only when mode is
// "keyword" AND hitCount is 0. Regression: firing for hybrid would spam
// users with wrong advice.
func TestWriteZeroHitDiagnostics_FiresOnKeywordZeroHits(t *testing.T) {
	// Build a real index.DB on a temp vault so AllNoteTitles has data.
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"), []byte(`
types:
  concept:
    required: [title]
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "note.md"), []byte(`---
id: c-1
type: concept
title: The Judgment Gap
---
body
`), 0o644))

	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	dbPath := filepath.Join(dir, cfg.Index.DBPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(dbPath), 0o755))
	idxr := index.NewIndexer(dir, dbPath, cfg)
	_, err = idxr.Rebuild()
	require.NoError(t, err)
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	var buf bytes.Buffer
	writeZeroHitDiagnostics(&buf, db, "judgment gap", "keyword", 0)
	out := buf.String()
	assert.Contains(t, out, "no embeddings", "keyword-only hint should name the cause")
	// Title suggestions should also fire because hitCount is 0 and there is
	// a nearby title.
	assert.Contains(t, out, "Judgment Gap", "fuzzy title match should suggest a close title")
}

// With a non-zero hit count, writeZeroHitDiagnostics should not print title
// suggestions (those only help when the user got nothing).
func TestWriteZeroHitDiagnostics_SilentWhenHitsNonZero(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"), []byte(`types: {}`), 0o644))
	cfg, _ := vault.LoadConfig(dir)
	dbPath := filepath.Join(dir, cfg.Index.DBPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(dbPath), 0o755))
	idxr := index.NewIndexer(dir, dbPath, cfg)
	_, _ = idxr.Rebuild()
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	var buf bytes.Buffer
	writeZeroHitDiagnostics(&buf, db, "q", "hybrid", 3)
	assert.Empty(t, buf.String(), "non-zero hits + hybrid must produce no hint")
}

// traceNote end-to-end: with a seeded event that places note n1 at rank 1,
// the JSON trace must carry the note_id and at least one hit. Regression:
// silently returning zero hits for a note that was clearly retrieved would
// make `experiment trace --note` useless for the common case.
func TestExperimentTraceNote_ReturnsHitsForSeededNote(t *testing.T) {
	db, _ := seedExperimentDB(t)
	sid, err := db.StartSession("/vault")
	require.NoError(t, err)
	ts := time.Now().UTC().Format(time.RFC3339)
	blob, _ := json.Marshal(map[string]any{
		"variants": map[string]any{
			"hybrid": map[string]any{"results": []any{map[string]any{"note_id": "n-42", "rank": 1}}},
		},
	})
	_, err = db.Exec(`INSERT INTO events
		(event_id, session_id, event_type, timestamp, vault_path, query_text, query_mode, primary_variant, event_data)
		VALUES ('ev-1', ?, 'ask', ?, '/vault', 'q', 'hybrid', 'hybrid', ?)`, sid, ts, string(blob))
	require.NoError(t, err)
	require.NoError(t, db.Close())

	out, _, err := runRootCmd(t, "experiment", "trace", "--note", "n-42", "--json")
	require.NoError(t, err)

	var env struct {
		Result struct {
			NoteID string `json:"note_id"`
			Hits   []struct {
				Rank int `json:"Rank"`
			} `json:"hits"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "n-42", env.Result.NoteID)
	require.NotEmpty(t, env.Result.Hits, "seeded note must appear in its retrievals")
	assert.Equal(t, 1, env.Result.Hits[0].Rank, "note was seeded at rank 1")
}
