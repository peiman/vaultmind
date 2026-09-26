package query_test

import (
	"context"
	"testing"

	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A note this conversation already received arrives as its title alone —
// kept, so it is never dropped, but without its text a second time.
func TestAsk_NotesShownBeforeArriveAsTitles(t *testing.T) {
	db := buildIndexedDB(t)
	retriever := &query.FTSRetriever{DB: db}
	resolver := graph.NewResolver(db)
	cfg := query.AskConfig{Query: "spreading activation", Budget: 8000, MaxItems: 8, SearchLimit: 5}

	first, err := query.Ask(context.Background(), retriever, resolver, db, cfg)
	require.NoError(t, err)
	require.NotNil(t, first.Context)
	require.NotEmpty(t, first.Context.Target.Body, "the fixture delivers the target's text")
	require.NotEmpty(t, first.Context.Context)
	neighbour := first.Context.Context[0]
	require.NotEmpty(t, neighbour.Body)

	cfg.ShownBefore = map[string]bool{first.Context.TargetID: true, neighbour.ID: true}
	second, err := query.Ask(context.Background(), retriever, resolver, db, cfg)
	require.NoError(t, err)

	assert.True(t, second.Context.Target.ShownBefore)
	assert.Empty(t, second.Context.Target.Body)
	got := itemByID(second.Context.Context, neighbour.ID)
	require.NotNil(t, got, "a note shown before is kept in the pack")
	assert.True(t, got.ShownBefore)
	assert.Empty(t, got.Body)
	assert.False(t, got.BodyIncluded)
	assert.Less(t, second.Context.UsedTokens, first.Context.UsedTokens, "the withheld text is not counted as sent")
	assert.NotContains(t, second.DeliveredIDs(), first.Context.TargetID, "a title is not a delivery")
	assert.Contains(t, first.DeliveredIDs(), first.Context.TargetID)
}

func itemByID(items []memory.ContextItem, id string) *memory.ContextItem {
	for i := range items {
		if items[i].ID == id {
			return &items[i]
		}
	}
	return nil
}
