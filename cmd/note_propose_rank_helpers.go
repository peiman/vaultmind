package cmd

import (
	"context"
	"sync"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/noisefloor"
	"github.com/peiman/vaultmind/internal/propose"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/spf13/cobra"
)

// askQuestionRanker ranks the original question the way ask does.
// The model loads on the first pass and serves the second. An existing
// draft never asks, so the model stays unloaded. Cleanup runs once, from
// Close, including when the check fails after the model loaded.
type askQuestionRanker struct {
	cmd       *cobra.Command
	db        *index.DB
	vaultPath string

	once                    sync.Once
	ret                     query.AutoRetrieverResult
	limit, forward, salient int
}

func newAskRanker(cmd *cobra.Command, db *index.DB, vaultPath string) propose.QuestionRanker {
	return &askQuestionRanker{cmd: cmd, db: db, vaultPath: vaultPath}
}

func (a *askQuestionRanker) ensure(ctx context.Context) {
	a.once.Do(func() {
		a.ret = query.BuildAutoRetrieverFull(ctx, a.db)
		a.limit, a.forward, a.salient = askRankingBounds(a.cmd)
	})
}

// Close releases the model. It is safe before the model loads and safe twice.
// BuildAutoRetrieverFull's own cleanup leaves the shared ONNX session loaded,
// and a write cannot embed while that session is held, so close the shared
// one here too.
func (a *askQuestionRanker) Close() error {
	if a.ret.Cleanup != nil {
		a.ret.Cleanup()
		a.ret.Cleanup = nil
	}
	query.CloseSharedEmbedder()
	return nil
}

func (a *askQuestionRanker) RankQuestion(ctx context.Context, question string, includeDrafts bool) (propose.QuestionRank, error) {
	a.ensure(ctx)
	retriever := a.ret.Retriever
	// A zero filter struct already excludes drafts. Wrapping it would not
	// change the search, and a wrapper that includes drafts must not be
	// reused for the pass that excludes them.
	filters := askSearchFilters(a.cmd, includeDrafts)
	if filters != (index.SearchFilters{}) {
		retriever = query.FilteredRetriever{Base: a.ret.Retriever, Filters: filters}
	}
	cfg := query.AskConfig{
		Query:              question,
		SearchLimit:        a.limit,
		Embedder:           a.ret.Embedder,
		QueryForwardTokens: a.forward,
		QuerySalientTerms:  a.salient,
	}
	if a.ret.Embedder != nil {
		cfg.NoiseFloor, cfg.NoiseFloorSigma, cfg.VaultLowContrast = resolveNoiseFloor(ctx, a.vaultPath, a.ret.Embedder.Dims())
		cfg.HasNoiseFloor = true
	}
	result, err := query.Rank(ctx, retriever, a.db, cfg)
	if err != nil {
		return propose.QuestionRank{}, err
	}
	return propose.QuestionRank{
		Hits:   bandsFor(result, a.ret),
		Mode:   retrievalModeLabel(a.ret),
		Window: a.limit,
	}, nil
}

func bandsFor(result *query.AskResult, ret query.AutoRetrieverResult) []propose.RankedHit {
	if result == nil {
		return nil
	}
	floor := 0.0
	if ret.Embedder != nil {
		floor = noisefloor.DefaultNoiseFloor(ret.Embedder.Dims())
	}
	hits := make([]propose.RankedHit, 0, len(result.TopHits))
	for _, hit := range result.TopHits {
		hits = append(hits, propose.RankedHit{ID: hit.ID, Band: query.BandFor(result, hit.ID, floor)})
	}
	return hits
}
