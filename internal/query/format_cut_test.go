package query_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// oneNoteVault indexes a single concept note and returns an open database.
func oneNoteVault(t *testing.T, id, body string) *index.DB {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"), []byte(`
types:
  concept:
    required: [title]
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "note.md"), []byte("---\nid: "+id+"\ntype: concept\ntitle: Note\n---\n"+body), 0o644))
	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	dbPath := filepath.Join(dir, cfg.Index.DBPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(dbPath), 0o755))
	_, err = index.NewIndexer(dir, dbPath, cfg).Rebuild()
	require.NoError(t, err)
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func formatPackedAsk(t *testing.T, db *index.DB, id string, budget int) string {
	t.Helper()
	pack, err := memory.ContextPack(graph.NewResolver(db), db, memory.ContextPackConfig{
		Input: id, Budget: budget, ExcerptTokens: 0,
	})
	require.NoError(t, err)
	var buf bytes.Buffer
	require.NoError(t, query.FormatAsk(&query.AskResult{Query: "x", Context: pack}, &buf))
	return buf.String()
}

// A body the budget has to slice is not "delivered in full". The header names
// the cut. ask's default excerpt is 0, which is the path that byte-slices.
func TestFormatAsk_CutBodyIsNotDeliveredInFull(t *testing.T) {
	const budget = 4000
	body := strings.Repeat("abcdefghij", 2000) // 20_000 chars, past a 4000-token budget
	db := oneNoteVault(t, "concept-long", body)
	out := formatPackedAsk(t, db, "concept-long", budget)
	assert.NotContains(t, out, "in full")
	assert.Contains(t, out, "1 delivered, cut to the 4000-token budget")
	assert.NotContains(t, out, body, "the rendered body is the cut prefix, not the note")
}

// Truncated is also set when a body is empty or there is no room left. That
// is not a cut body, and the header must not call a body that arrived whole
// a cut.
func TestFormatAsk_TruncatedWithoutACutBodyStaysInFull(t *testing.T) {
	r := &query.AskResult{
		Query: "x",
		Context: &memory.ContextPackResult{
			TargetID:     "concept-x",
			BudgetTokens: 4000, UsedTokens: 20,
			Truncated: true,
			Target: &memory.ContextPackTarget{
				ID:          "concept-x",
				Frontmatter: map[string]any{"type": "concept", "title": "X"},
				Body:        "a whole body",
			},
		},
	}
	var buf bytes.Buffer
	require.NoError(t, query.FormatAsk(r, &buf))
	out := buf.String()
	assert.Contains(t, out, "1 delivered in full")
	assert.NotContains(t, out, "cut to the")
}

// A body that fits keeps the existing wording.
func TestFormatAsk_BodyThatFitsStaysDeliveredInFull(t *testing.T) {
	db := oneNoteVault(t, "concept-short", "a short body that fits the budget")
	out := formatPackedAsk(t, db, "concept-short", 4000)
	assert.Contains(t, out, "1 delivered in full")
	assert.NotContains(t, out, "cut to the")
}

func TestFormatAsk_CutBodySurvivesJSONRoundTrip(t *testing.T) {
	db := oneNoteVault(t, "concept-long", strings.Repeat("long body ", 2000))
	pack, err := memory.ContextPack(graph.NewResolver(db), db, memory.ContextPackConfig{
		Input: "concept-long", Budget: 4000,
	})
	require.NoError(t, err)
	raw, err := json.Marshal(&query.AskResult{Query: "x", Context: pack})
	require.NoError(t, err)
	var decoded query.AskResult
	require.NoError(t, json.Unmarshal(raw, &decoded))
	var out bytes.Buffer
	require.NoError(t, query.FormatAsk(&decoded, &out))
	header := strings.Split(out.String(), "\n")[2]
	assert.NotContains(t, header, "in full")
	assert.Contains(t, header, "cut to the 4000-token budget")
}

func TestFormatAsk_CutTargetWithExcerptedNeighbor(t *testing.T) {
	r := &query.AskResult{Query: "x", Context: &memory.ContextPackResult{
		TargetID: "long", BudgetTokens: 4000,
		Target:  &memory.ContextPackTarget{ID: "long", Body: "cut body", BodyCutToBudget: true},
		Context: []memory.ContextItem{{ID: "neighbor", Body: "chosen passage", BodyIncluded: true, BodyExcerpted: true}},
	}}
	var out bytes.Buffer
	require.NoError(t, query.FormatAsk(r, &out))
	assert.Contains(t, out.String(), "2 delivered, 1 as excerpt, 1 cut to the 4000-token budget")
}
