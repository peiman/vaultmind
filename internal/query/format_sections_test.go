package query_test

import (
	"bytes"
	"testing"

	"github.com/peiman/vaultmind/internal/memory"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The text output names the part of a long note that answered: the hit line
// shows the section's id (what `note get` opens), and the context block says
// where in the note the text comes from. A short note renders as before.
func TestFormatAsk_NamesTheSectionOfALongNote(t *testing.T) {
	r := &query.AskResult{
		Query:            "q",
		TopHitConfidence: query.ConfidenceStrong,
		TopHits: []retrieval.ScoredResult{
			{ID: "imported-web-fts5", Section: "imported-web-fts5#6-9-the-optimize-command", Title: "SQLite FTS5 Extension"},
			{ID: "concept-short", Title: "Short"},
		},
		Context: &memory.ContextPackResult{
			TargetID: "imported-web-fts5",
			Target: &memory.ContextPackTarget{
				ID:          "imported-web-fts5",
				Frontmatter: map[string]interface{}{"type": "reference", "title": "SQLite FTS5 Extension"},
				Body:        "This command merges all individual b-trees…",
				Section:     "imported-web-fts5#6-9-the-optimize-command",
				HeadingPath: "6. Special INSERT Commands › 6.9. The 'optimize' Command",
			},
			Context: []memory.ContextItem{{
				ID:          "concept-short",
				Frontmatter: map[string]interface{}{"type": "concept", "title": "Short"},
			}},
		},
	}
	var buf bytes.Buffer
	require.NoError(t, query.FormatAsk(r, &buf))
	out := buf.String()
	assert.Contains(t, out, "imported-web-fts5#6-9-the-optimize-command", "the hit line names the section")
	assert.Contains(t, out, "[reference] SQLite FTS5 Extension › 6. Special INSERT Commands › 6.9. The 'optimize' Command")
	assert.Contains(t, out, "[concept] Short\n", "a short note's line is unchanged")
}
