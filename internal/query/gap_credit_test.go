package query

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAsk_DeliversTheGapCreditLine(t *testing.T) {
	db := openGapCreditDB(t)
	opened := time.Date(2026, 10, 14, 15, 4, 0, 0, time.UTC)
	closed := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	long := strings.Repeat("ü", 81)
	seedClosedGap(t, db, "gap-answer", "answer", "how does the index stay current", opened, closed)
	seedClosedGap(t, db, "gap-neighbor", "neighbor", long, opened, closed)

	cases := []struct {
		name    string
		opts    formatOpts
		shown   bool
		lines   int
		wantSub string
	}{
		{name: "target and context item", lines: 2, wantSub: `  ↳ answers a question first investigated 2026-10-14: "how does the index stay current"`},
		{name: "pointers only", opts: formatOpts{pointersOnly: true}, lines: 0},
		{name: "shown earlier", shown: true, lines: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := gapCreditResult()
			if tc.shown {
				markShownBefore(result.Context, map[string]bool{"answer": true, "neighbor": true})
				result.Context.Target.Body = "still here"
				result.Context.Context[0].Body = "still here"
			}
			require.NoError(t, attachGapCredits(db, result))

			require.NotNil(t, result.TopHits[0].AnswersGap)
			assert.Equal(t, "how does the index stay current", result.TopHits[0].AnswersGap.Question)
			assert.True(t, result.TopHits[0].AnswersGap.OpenedAt.Equal(opened))
			assert.Nil(t, result.TopHits[1].AnswersGap, "a normal hit has no answers_gap")
			require.NotNil(t, result.Context.Target.AnswersGap)
			require.NotNil(t, result.Context.Context[0].AnswersGap)
			assert.Equal(t, 81, len([]rune(result.Context.Context[0].AnswersGap.Question)))
			assert.Nil(t, result.Context.Context[1].AnswersGap)

			plain, err := json.Marshal(result.Context.Context[1])
			require.NoError(t, err)
			assert.NotContains(t, string(plain), "answers_gap")
			hit, err := json.Marshal(result.TopHits[0])
			require.NoError(t, err)
			assert.Contains(t, string(hit), `"opened_at":"2026-10-14T15:04:00Z"`)
			assert.Contains(t, string(hit), `"closed_at":"2026-10-16T00:00:00Z"`)

			var buf bytes.Buffer
			require.NoError(t, formatAskWithOptions(result, &buf, tc.opts))
			out := buf.String()
			assert.Equal(t, tc.lines, strings.Count(out, "↳ answers a question"))
			if tc.wantSub != "" {
				assert.Contains(t, out, tc.wantSub)
				assert.Contains(t, out, `"`+strings.Repeat("ü", 80)+`"`)
				assert.NotContains(t, out, strings.Repeat("ü", 81))
				assert.NotContains(t, out, "2026-10-16")
			}
			if tc.shown {
				assert.Contains(t, out, "shown earlier this session")
				assert.NotNil(t, result.Context.Target.AnswersGap, "dedup keeps the JSON credit")
			}
		})
	}
}

func TestAsk_LoadsTheGapCreditMapOnce(t *testing.T) {
	db := openGapCreditDB(t)
	calls := 0
	prev := loadGapClosures
	t.Cleanup(func() { loadGapClosures = prev })
	loadGapClosures = func(q index.RowQuery) (map[string]index.GapCredit, error) {
		calls++
		return prev(q)
	}

	require.NoError(t, attachGapCredits(db, gapCreditResult()))
	assert.Equal(t, 1, calls, "one command loads the map once, however many notes it delivers")

	calls = 0
	got, err := finishAsk(context.Background(), nil, db, AskConfig{}, &AskResult{})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 1, calls)

	loadGapClosures = func(index.RowQuery) (map[string]index.GapCredit, error) {
		return nil, assert.AnError
	}
	got, err = finishAsk(context.Background(), nil, db, AskConfig{}, &AskResult{Query: "still answered"})
	require.NoError(t, err)
	assert.Equal(t, "still answered", got.Query)
}

func TestDoctorGapLoop_JSONKeepsTheFourClaimFields(t *testing.T) {
	raw, err := json.Marshal(DoctorGapLoop{})
	require.NoError(t, err)
	var fields map[string]any
	require.NoError(t, json.Unmarshal(raw, &fields))
	for _, key := range []string{"opened", "closed", "median_days_to_close", "gap_closing_cited"} {
		_, ok := fields[key]
		assert.True(t, ok, "gap_loop.%s", key)
	}
	assert.Nil(t, fields["median_days_to_close"])
	assert.Equal(t, float64(0), fields["gap_closing_cited"])

	use, err := json.Marshal(DoctorMemoryUse{WindowDays: 7, Injections: 1})
	require.NoError(t, err)
	var memory map[string]any
	require.NoError(t, json.Unmarshal(use, &memory))
	assert.Equal(t, float64(0), memory["gap_closing_surfaced"])
	assert.Equal(t, float64(0), memory["gap_closing_cited"])
}

func gapCreditResult() *AskResult {
	return &AskResult{
		Query: "how does the index stay current",
		TopHits: []retrieval.ScoredResult{
			{ID: "answer", Type: "concept", Title: "Answer"},
			{ID: "plain", Type: "concept", Title: "Plain"},
		},
		Context: &memory.ContextPackResult{
			TargetID: "answer",
			Target: &memory.ContextPackTarget{
				ID:          "answer",
				Frontmatter: map[string]any{"type": "concept", "title": "Answer"},
				Body:        "the index is rebuilt from the files",
			},
			Context: []memory.ContextItem{
				{
					ID: "neighbor", BodyIncluded: true, Body: "a neighbor body",
					Frontmatter: map[string]any{"type": "concept", "title": "Neighbor"},
				},
				{
					ID: "plain", BodyIncluded: true, Body: "a plain body",
					Frontmatter: map[string]any{"type": "concept", "title": "Plain"},
				},
			},
		},
	}
}

func openGapCreditDB(t *testing.T) *index.DB {
	t.Helper()
	db, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func seedClosedGap(t *testing.T, db *index.DB, gapID, closedBy, question string, opened, closed time.Time) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO notes (id, path, hash, mtime, status, type) VALUES (?, ?, 'h', 1, 'closed', ?)`,
		gapID, gapID+".md", index.TypeGap)
	require.NoError(t, err)
	for _, kv := range []struct{ key, value string }{
		{"closed_by", closedBy},
		{"question", question},
		{"opened_at", opened.Format(time.RFC3339)},
		{"closed_at", closed.Format(time.RFC3339)},
	} {
		raw, mErr := json.Marshal(kv.value)
		require.NoError(t, mErr)
		_, err = db.Exec(
			`INSERT INTO frontmatter_kv (note_id, key, value_json) VALUES (?, ?, ?)`,
			gapID, kv.key, string(raw))
		require.NoError(t, err)
	}
}
