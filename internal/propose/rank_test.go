package propose

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/noisefloor"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/retrieval"
	tmpl "github.com/peiman/vaultmind/internal/template"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// markerEmbedder maps a fixture token to a near-one-hot vector. Cosine is an
// angle, so the vectors keep a similar magnitude and stay far apart: a tiny
// uniform vector would score as a near match against a one-hot query.
// First match wins. A body carries one token, and the title is not embedded.
type markerEmbedder struct{}

func (markerEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	switch {
	case strings.Contains(text, "quartzkeeper"):
		return []float32{1, 0, 0}, nil
	case strings.Contains(text, "othernote"):
		return []float32{0.6, 0.8, 0}, nil
	case strings.Contains(text, "targetnote"):
		return []float32{0.2, 0, 0.98}, nil
	case strings.Contains(text, "bananas"):
		return []float32{0, 0, 1}, nil
	default:
		return []float32{0, 1, 0}, nil
	}
}

func (e markerEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, text := range texts {
		vec, err := e.Embed(ctx, text)
		if err != nil {
			return nil, err
		}
		out[i] = vec
	}
	return out, nil
}

func (markerEmbedder) Dims() int    { return 3 }
func (markerEmbedder) Close() error { return nil }

// embeddingRanker is the real embedding search with the draft switch honored.
// The included pass asks for drafts; flipping that argument to false drops
// the draft and the rank-1 test goes red.
type embeddingRanker struct {
	db       *index.DB
	emb      embedding.Embedder
	window   int
	snapPath string

	flags     []bool
	questions []string
	passes    []QuestionRank
	snap      []byte
}

func (r *embeddingRanker) RankQuestion(ctx context.Context, question string, includeDrafts bool) (QuestionRank, error) {
	r.flags = append(r.flags, includeDrafts)
	r.questions = append(r.questions, question)
	if r.snap == nil && r.snapPath != "" {
		raw, err := os.ReadFile(r.snapPath)
		if err != nil {
			return QuestionRank{}, err
		}
		r.snap = raw
	}
	base := &query.EmbeddingRetriever{DB: r.db, Embedder: r.emb}
	var ret retrieval.Retriever = base
	if includeDrafts {
		ret = query.FilteredRetriever{Base: base, Filters: index.SearchFilters{IncludeDrafts: true}}
	}
	result, err := query.Rank(ctx, ret, r.db, query.AskConfig{
		Query:       question,
		SearchLimit: r.window,
		Embedder:    r.emb,
	})
	if err != nil {
		return QuestionRank{}, err
	}
	rank := QuestionRank{Mode: "hybrid", Window: r.window}
	if result != nil {
		for _, hit := range result.TopHits {
			rank.Hits = append(rank.Hits, RankedHit{ID: hit.ID, Band: query.BandFor(result, hit.ID, 0)})
		}
	}
	r.passes = append(r.passes, rank)
	return rank, nil
}

func embedIndexedBodies(t *testing.T, db *index.DB, emb embedding.Embedder) {
	t.Helper()
	rows, err := db.Query(`SELECT id, COALESCE(body_text, '') FROM notes`)
	require.NoError(t, err)
	type pair struct{ id, body string }
	var notes []pair
	for rows.Next() {
		var p pair
		require.NoError(t, rows.Scan(&p.id, &p.body))
		notes = append(notes, p)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	require.NotEmpty(t, notes)
	for _, note := range notes {
		vec, err := emb.Embed(context.Background(), note.body)
		require.NoError(t, err)
		require.NoError(t, index.StoreEmbedding(db, note.id, vec))
	}
}

func accessCount(t *testing.T, db *index.DB) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM note_accesses`).Scan(&n))
	return n
}

func embeddingIDs(t *testing.T, db *index.DB, emb embedding.Embedder, question string) []string {
	t.Helper()
	ret := &query.EmbeddingRetriever{DB: db, Embedder: emb}
	hits, _, err := ret.Search(context.Background(), question, 5, 0, index.SearchFilters{})
	require.NoError(t, err)
	ids := make([]string, len(hits))
	for i, hit := range hits {
		ids[i] = hit.ID
	}
	return ids
}

func dropRankLines(raw string) string {
	lines := strings.Split(raw, "\n")
	kept := lines[:0]
	for _, line := range lines {
		if strings.HasPrefix(line, "rank_before:") || strings.HasPrefix(line, "rank_after:") || strings.HasPrefix(line, "rank_checked_at:") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

func asInt(t *testing.T, v interface{}) int {
	t.Helper()
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case uint64:
		return int(n)
	case float64:
		return int(n)
	default:
		t.Fatalf("rank value %T (%v), want a number", v, v)
		return 0
	}
}

func TestPropose_RankCheck_DraftRanksFirst(t *testing.T) {
	v := testVault(t)
	emb := markerEmbedder{}
	const (
		question = "Where does quartzkeeper live"
		finding  = "quartzkeeper lives under the hill"
		targetID = "target-note"
		otherID  = "other-note"
	)
	writeTarget(t, v, "concepts/other.md", otherID, "concept", "Other", "accepted", "othernote is the nearer published note")
	writeTarget(t, v, "concepts/target.md", targetID, "concept", "Target", "accepted", "targetnote is the note a patch would land beside")
	embedIndexedBodies(t, v.DB, emb)
	beforeIDs := embeddingIDs(t, v.DB, emb, "othernote")

	rel := proposalPath("concepts/target.md", targetID, proposeNow)
	ranker := &embeddingRanker{
		db: v.DB, emb: emb, window: 5,
		snapPath: filepath.Join(v.Path, filepath.FromSlash(rel)),
	}
	req, out, _, embeds := reqOf(question, finding)
	req.Ranker = ranker
	req.Embed = func(string) {
		*embeds++
		embedIndexedBodies(t, v.DB, emb)
	}
	ret := &stubClosest{hit: Hit{ID: targetID, Path: "concepts/target.md", Type: "concept", Title: "Target", Band: "strong"}, ok: true}
	require.Equal(t, 0, accessCount(t, v.DB))
	require.NoError(t, Run(context.Background(), v, req, ret))

	id := tmpl.GenerateID(rel, "concept")
	assert.Equal(t, []bool{false, true}, ranker.flags)
	assert.Equal(t, []string{question, question}, ranker.questions)
	require.Len(t, ranker.passes, 2)
	assert.Equal(t, []string{otherID, targetID}, hitIDs(ranker.passes[0].Hits))
	assert.Equal(t, id, ranker.passes[1].Hits[0].ID)
	assert.NotContains(t, hitIDs(ranker.passes[0].Hits), id)

	wantLine := "retrieval check (hybrid): before — best answer " + targetID + " (strong) on the question and the finding; question alone — best answer " + targetID + " at #2; after — this draft at #1 (strong). Measured on the proposal note, which holds the finding."
	assert.Contains(t, out.String(), wantLine)
	assert.Equal(t, 2, *embeds)
	assert.Equal(t, 0, accessCount(t, v.DB))
	assert.Equal(t, beforeIDs, embeddingIDs(t, v.DB, emb, "othernote"))

	fm, _ := parseNote(t, v, rel)
	assert.Equal(t, 2, asInt(t, fm["rank_before"]))
	assert.Equal(t, 1, asInt(t, fm["rank_after"]))
	assert.Equal(t, proposeNow.Format("2006-01-02T15:04:05Z"), fm["rank_checked_at"])
	got, err := os.ReadFile(ranker.snapPath)
	require.NoError(t, err)
	assert.Equal(t, string(ranker.snap), dropRankLines(string(got)))
	assert.Contains(t, string(got), "rank_before:")
	assertNoSchemaErrors(t, v, id)
}

func TestPropose_RankCheck_UnrelatedDraftDoesNotRank(t *testing.T) {
	v := testVault(t)
	emb := markerEmbedder{}
	const (
		question = "Where does quartzkeeper live"
		otherID  = "other-note"
		targetID = "target-note"
	)
	writeTarget(t, v, "concepts/other.md", otherID, "concept", "Other", "accepted", "othernote is the nearer published note")
	writeTarget(t, v, "concepts/target.md", targetID, "concept", "Target", "accepted", "targetnote is the note a patch would land beside")
	embedIndexedBodies(t, v.DB, emb)

	ranker := &embeddingRanker{db: v.DB, emb: emb, window: 2}
	req, out, _, embeds := reqOf(question, "bananas sit in a bowl")
	req.Ranker = ranker
	req.Embed = func(string) {
		*embeds++
		embedIndexedBodies(t, v.DB, emb)
	}
	ret := &stubClosest{hit: Hit{ID: targetID, Path: "concepts/target.md", Type: "concept", Band: "moderate"}, ok: true}
	require.NoError(t, Run(context.Background(), v, req, ret))

	rel := "concepts/where-does-quartzkeeper-live.md"
	assert.Contains(t, out.String(), "mode: new\n")
	assert.Contains(t, out.String(), "retrieval check (hybrid): before — best answer "+targetID+" (moderate) on the question and the finding, which is not close enough for strong, so this is a new note; question alone — best answer "+otherID+" at #1 (strong); after — this draft does not rank in the top 2 for its own question; consider adding the question's key terms or links to "+otherID+", "+targetID)
	assert.NotContains(t, out.String(), "Measured on the proposal note")
	fm, _ := parseNote(t, v, rel)
	assert.Equal(t, 1, asInt(t, fm["rank_before"]))
	assert.Equal(t, 0, asInt(t, fm["rank_after"]))
	assert.Equal(t, proposeNow.Format("2006-01-02T15:04:05Z"), fm["rank_checked_at"])
	assert.Equal(t, 0, accessCount(t, v.DB))
}

func TestPropose_RankCheck_KeywordModeStillRuns(t *testing.T) {
	v := testVault(t)
	writeTarget(t, v, "concepts/memory.md", "concept-memory", "concept", "Memory", "accepted", "published apples")
	ranker := &keywordRanker{db: v.DB, window: 5}
	req, out, _, _ := reqOf("Where does quartzkeeper live", "A keeper note.")
	req.Ranker = ranker
	require.NoError(t, Run(context.Background(), v, req, &stubClosest{}))

	assert.Contains(t, out.String(), "retrieval check (keyword): before — no note on the question and the finding; question alone — no existing note in the top 5; after — this draft at #1")
	rel := "where-does-quartzkeeper-live.md"
	fm, _ := parseNote(t, v, rel)
	assert.Equal(t, 0, asInt(t, fm["rank_before"]))
	assert.Equal(t, 1, asInt(t, fm["rank_after"]))
	assert.Equal(t, "keyword", ranker.mode)
	assert.Equal(t, 0, accessCount(t, v.DB))
}

func TestPropose_RankCheck_JSONContract(t *testing.T) {
	v := testVault(t)
	emb := markerEmbedder{}
	writeTarget(t, v, "concepts/other.md", "other-note", "concept", "Other", "accepted", "othernote is the nearer published note")
	writeTarget(t, v, "concepts/target.md", "target-note", "concept", "Target", "accepted", "targetnote is the note a patch would land beside")
	embedIndexedBodies(t, v.DB, emb)
	rel := proposalPath("concepts/target.md", "target-note", proposeNow)
	ranker := &embeddingRanker{db: v.DB, emb: emb, window: 5}
	req, out, _, _ := reqOf("Where does quartzkeeper live", "quartzkeeper lives under the hill")
	req.JSON = true
	req.Ranker = ranker
	req.Embed = func(string) { embedIndexedBodies(t, v.DB, emb) }
	ret := &stubClosest{hit: Hit{ID: "target-note", Path: "concepts/target.md", Type: "concept", Band: "strong"}, ok: true}
	require.NoError(t, Run(context.Background(), v, req, ret))

	var env struct {
		SchemaVersion string `json:"schema_version"`
		Command       string `json:"command"`
		Status        string `json:"status"`
		Result        struct {
			Mode      string     `json:"mode"`
			ID        string     `json:"id"`
			RankCheck *RankCheck `json:"rank_check"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "v1", env.SchemaVersion)
	assert.Equal(t, "note propose", env.Command)
	assert.Equal(t, "ok", env.Status)
	assert.Equal(t, "patch", env.Result.Mode)
	require.NotNil(t, env.Result.RankCheck)
	assert.Equal(t, "hybrid", env.Result.RankCheck.Mode)
	assert.Equal(t, 5, env.Result.RankCheck.Window)
	assert.Equal(t, "target-note", env.Result.RankCheck.Before.ID)
	assert.Equal(t, 2, env.Result.RankCheck.Before.Rank)
	assert.Empty(t, env.Result.RankCheck.Before.Band)
	assert.Equal(t, tmpl.GenerateID(rel, "concept"), env.Result.RankCheck.After.ID)
	assert.Equal(t, env.Result.ID, env.Result.RankCheck.After.ID)
	assert.Equal(t, 1, env.Result.RankCheck.After.Rank)
	assert.Equal(t, "strong", env.Result.RankCheck.After.Band)
	assert.Equal(t, measuredOnProposal, env.Result.RankCheck.MeasuredOn)
	assert.Empty(t, env.Result.RankCheck.OutrankedBy)
	assert.Equal(t, "target-note", env.Result.RankCheck.Decision.ID)
	assert.Equal(t, 1, env.Result.RankCheck.Decision.Rank)
	assert.Equal(t, "strong", env.Result.RankCheck.Decision.Band)
	assert.Equal(t, decisionOnQuestionAndFinding, env.Result.RankCheck.DecisionOn)
	assert.Equal(t, noisefloor.ConfidenceStrong, env.Result.RankCheck.CloseBand)
}

func TestPropose_RankCheck_ExistingDraftSkipsTheCheck(t *testing.T) {
	v := testVault(t)
	ranker := &countingRanker{}
	req, _, _, _ := reqOf("Where does the day go?", "It goes to the work.")
	req.Ranker = ranker
	var closedAtEmbed []int
	req.Embed = func(string) { closedAtEmbed = append(closedAtEmbed, ranker.closed) }
	require.NoError(t, Run(context.Background(), v, req, &stubClosest{}))
	assert.Equal(t, 2, ranker.calls)
	assert.Equal(t, 2, ranker.closed)
	// The write embeds before the check. The frontmatter write embeds again,
	// and only after the ranker has been closed, so the model session is free.
	assert.Equal(t, []int{0, 1}, closedAtEmbed)

	again, out, _, _ := reqOf("Where does the day go?", "A second finding.")
	again.Ranker = ranker
	require.NoError(t, Run(context.Background(), v, again, &stubClosest{}))
	assert.Equal(t, 2, ranker.calls)
	assert.Equal(t, 3, ranker.closed)
	assert.Contains(t, out.String(), "mode: existing\n")
	assert.NotContains(t, out.String(), "retrieval check")
}

func TestPropose_RankCheck_ErrorNamesTheDraft(t *testing.T) {
	v := testVault(t)
	writeTarget(t, v, "concepts/target.md", "target-note", "concept", "Target", "accepted", "targetnote stays")
	ranker := &failingRanker{}
	req, _, _, _ := reqOf("Where does quartzkeeper live", "quartzkeeper lives under the hill")
	req.Ranker = ranker
	ret := &stubClosest{hit: Hit{ID: "target-note", Path: "concepts/target.md", Type: "concept", Band: "strong"}, ok: true}
	err := Run(context.Background(), v, req, ret)
	rel := proposalPath("concepts/target.md", "target-note", proposeNow)
	id := tmpl.GenerateID(rel, "concept")
	require.Error(t, err)
	assert.Contains(t, err.Error(), id)
	assert.Equal(t, 1, ranker.closed)
	raw, readErr := os.ReadFile(filepath.Join(v.Path, filepath.FromSlash(rel)))
	require.NoError(t, readErr)
	assert.NotContains(t, string(raw), "rank_before:")
	_, statErr := os.Stat(filepath.Join(v.Path, filepath.FromSlash(rel)))
	assert.NoError(t, statErr)
}

func TestFormatRankCheck(t *testing.T) {
	inWindow := RankCheck{
		Mode:   "hybrid",
		Window: 5,
		Before: RankPlace{ID: "concept-memory", Rank: 3, Band: "moderate"},
		After:  RankPlace{ID: "draft", Rank: 1, Band: "strong"},
	}
	cases := []struct {
		name string
		in   RankCheck
		want string
	}{
		{
			name: "in window",
			in:   inWindow,
			want: "retrieval check (hybrid): before — no note on the question and the finding; question alone — best answer concept-memory at #3 (moderate); after — this draft at #1 (strong)",
		},
		{
			name: "patch sentence",
			in: func() RankCheck {
				c := inWindow
				c.MeasuredOn = measuredOnProposal
				return c
			}(),
			want: "retrieval check (hybrid): before — no note on the question and the finding; question alone — best answer concept-memory at #3 (moderate); after — this draft at #1 (strong). Measured on the proposal note, which holds the finding.",
		},
		{
			name: "decision weaker than the close band",
			in: RankCheck{
				Mode:       "hybrid",
				Window:     5,
				Decision:   RankPlace{ID: "target-note", Rank: 1, Band: "moderate"},
				DecisionOn: decisionOnQuestionAndFinding,
				CloseBand:  "strong",
				Before:     RankPlace{ID: "other-note", Rank: 1, Band: "strong"},
				After:      RankPlace{ID: "draft", Rank: 0},
			},
			want: "retrieval check (hybrid): before — best answer target-note (moderate) on the question and the finding, which is not close enough for strong, so this is a new note; question alone — best answer other-note at #1 (strong); after — this draft does not rank in the top 5 for its own question; consider adding the question's key terms",
		},
		{
			name: "decision at least the close band",
			in: RankCheck{
				Mode:      "hybrid",
				Window:    5,
				Decision:  RankPlace{ID: "target-note", Rank: 1, Band: "strong"},
				CloseBand: "moderate",
				Before:    RankPlace{ID: "target-note", Rank: 2},
				After:     RankPlace{ID: "draft", Rank: 1, Band: "strong"},
			},
			want: "retrieval check (hybrid): before — best answer target-note (strong) on the question and the finding; question alone — best answer target-note at #2; after — this draft at #1 (strong)",
		},
		{
			name: "not in the window",
			in: RankCheck{
				Mode:        "hybrid",
				Window:      5,
				Before:      RankPlace{ID: "concept-memory", Rank: 1, Band: "moderate"},
				After:       RankPlace{ID: "draft", Rank: 0},
				OutrankedBy: []string{"concept-a", "concept-b"},
			},
			want: "retrieval check (hybrid): before — no note on the question and the finding; question alone — best answer concept-memory at #1 (moderate); after — this draft does not rank in the top 5 for its own question; consider adding the question's key terms or links to concept-a, concept-b",
		},
		{
			name: "no existing note",
			in: RankCheck{
				Mode:   "keyword",
				Window: 5,
				After:  RankPlace{ID: "draft", Rank: 1},
			},
			want: "retrieval check (keyword): before — no note on the question and the finding; question alone — no existing note in the top 5; after — this draft at #1",
		},
		{
			name: "target missing from the window",
			in: RankCheck{
				Mode:   "hybrid",
				Window: 5,
				Before: RankPlace{ID: "concept-memory", Rank: 0},
				After:  RankPlace{ID: "draft", Rank: 1, Band: "strong"},
			},
			want: "retrieval check (hybrid): before — no note on the question and the finding; question alone — best answer concept-memory is not in the top 5; after — this draft at #1 (strong)",
		},
		{
			name: "empty mode and empty band",
			in: RankCheck{
				Window: 5,
				Before: RankPlace{ID: "concept-memory", Rank: 1},
				After:  RankPlace{ID: "draft", Rank: 2},
			},
			want: "retrieval check: before — no note on the question and the finding; question alone — best answer concept-memory at #1; after — this draft at #2",
		},
		{
			name: "miss with nobody ahead",
			in: RankCheck{
				Mode:   "hybrid",
				Window: 5,
				After:  RankPlace{ID: "draft", Rank: 0},
			},
			want: "retrieval check (hybrid): before — no note on the question and the finding; question alone — no existing note in the top 5; after — this draft does not rank in the top 5 for its own question; consider adding the question's key terms",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, formatRankCheck(tc.in))
		})
	}
}

func hitIDs(hits []RankedHit) []string {
	ids := make([]string, len(hits))
	for i, hit := range hits {
		ids[i] = hit.ID
	}
	return ids
}

type keywordRanker struct {
	db        *index.DB
	window    int
	flags     []bool
	questions []string
	mode      string
}

func (r *keywordRanker) RankQuestion(ctx context.Context, question string, includeDrafts bool) (QuestionRank, error) {
	r.flags = append(r.flags, includeDrafts)
	r.questions = append(r.questions, question)
	r.mode = "keyword"
	base := &query.FTSRetriever{DB: r.db}
	var ret retrieval.Retriever = base
	if includeDrafts {
		ret = query.FilteredRetriever{Base: base, Filters: index.SearchFilters{IncludeDrafts: true}}
	}
	result, err := query.Rank(ctx, ret, r.db, query.AskConfig{Query: question, SearchLimit: r.window})
	if err != nil {
		return QuestionRank{}, err
	}
	rank := QuestionRank{Mode: "keyword", Window: r.window}
	if result != nil {
		for _, hit := range result.TopHits {
			rank.Hits = append(rank.Hits, RankedHit{ID: hit.ID, Band: query.BandFor(result, hit.ID, 0)})
		}
	}
	return rank, nil
}

type countingRanker struct {
	calls  int
	closed int
}

func (c *countingRanker) RankQuestion(context.Context, string, bool) (QuestionRank, error) {
	c.calls++
	return QuestionRank{Mode: "keyword", Window: 5}, nil
}

func (c *countingRanker) Close() error {
	c.closed++
	return nil
}

type failingRanker struct{ closed int }

func (f *failingRanker) RankQuestion(context.Context, string, bool) (QuestionRank, error) {
	return QuestionRank{}, errors.New("rank down")
}

func (f *failingRanker) Close() error {
	f.closed++
	return nil
}
