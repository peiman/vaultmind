package query_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Track A.2 of the post-2026-04-29 zoom-out: extend RecordNoteAccess
// firing from just the context-pack target to also cover (a) every
// neighbor in the assembled context pack and (b) the note returned
// from a successful note-get resolution. The reinforcement signal
// today only fires once per Ask; it should fire once per note that
// genuinely entered the agent's working context. These tests pin
// the contract.

// TestAsk_RecordsAccessForContextPackNeighbors verifies that every
// note included in the context pack — target plus neighbors — gets
// its access_count incremented. Before A.2 only the target was
// tracked; the neighbors were "warmed up" but invisible to the
// reinforcement layer.
func TestAsk_RecordsAccessForContextPackNeighbors(t *testing.T) {
	db, _ := smallIndexedVault(t)
	retriever := &query.FTSRetriever{DB: db}
	resolver := graph.NewResolver(db)

	// "Alpha" should return concept-alpha as top hit; context-pack
	// from concept-alpha pulls in proj-beta via its related_ids edge.
	result, err := query.Ask(context.Background(), retriever, resolver, db, query.AskConfig{
		Query:       "Alpha",
		Budget:      4000,
		MaxItems:    8,
		SearchLimit: 5,
	})
	require.NoError(t, err)
	require.NotNil(t, result.Context, "context pack expected for the smallIndexedVault Alpha hit")
	require.NotEmpty(t, result.Context.Context,
		"alpha→beta edge should produce at least one context neighbor")

	// Target gets +1 (existing slice 1 contract).
	target, err := index.LookupNoteAccess(db, result.Context.TargetID)
	require.NoError(t, err)
	assert.Equal(t, 1, target.AccessCount,
		"target %q should have access_count=1 after one Ask", result.Context.TargetID)

	// Every neighbor in the pack gets +1 too — A.2's new contract.
	for _, item := range result.Context.Context {
		stats, err := index.LookupNoteAccess(db, item.ID)
		require.NoError(t, err, "looking up neighbor %q", item.ID)
		assert.Equal(t, 1, stats.AccessCount,
			"context-pack neighbor %q should have access_count=1 after one Ask", item.ID)
	}
}

// TestRunNoteGet_RecordsAccessForResolvedNote verifies that an
// explicit `vaultmind note get <id>` increments the resolved note's
// access_count. Direct ID lookup is the strongest non-ambiguous
// recall signal; the reinforcement layer should see it.
func TestRunNoteGet_RecordsAccessForResolvedNote(t *testing.T) {
	db, dir := smallIndexedVault(t)

	before, err := index.LookupNoteAccess(db, "concept-alpha")
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, query.RunNoteGet(db, query.NoteGetConfig{
		Input: "concept-alpha", JSONOutput: true, VaultPath: dir,
	}, &buf))

	after, err := index.LookupNoteAccess(db, "concept-alpha")
	require.NoError(t, err)
	assert.Equal(t, before.AccessCount+1, after.AccessCount,
		"note get on concept-alpha should increment access_count by exactly 1")
	assert.NotEmpty(t, after.LastAccessedAt,
		"last_accessed_at should be set after a successful note get")
}

// TestRunNoteGet_DoesNotRecordAccessForUnknownID verifies that an
// unresolved id does NOT mutate access counts — there's no real note
// to associate the access event with, and incrementing a non-existent
// row would silently no-op anyway. This pins the negative case.
func TestRunNoteGet_DoesNotRecordAccessForUnknownID(t *testing.T) {
	db, dir := smallIndexedVault(t)

	before, err := index.LookupNoteAccess(db, "concept-alpha")
	require.NoError(t, err)

	var buf bytes.Buffer
	// Unknown id — RunNoteGet writes a not_found envelope, doesn't error.
	require.NoError(t, query.RunNoteGet(db, query.NoteGetConfig{
		Input: "concept-does-not-exist", JSONOutput: true, VaultPath: dir,
	}, &buf))

	after, err := index.LookupNoteAccess(db, "concept-alpha")
	require.NoError(t, err)
	assert.Equal(t, before.AccessCount, after.AccessCount,
		"unrelated note's access_count must not change on a failed lookup")
}
