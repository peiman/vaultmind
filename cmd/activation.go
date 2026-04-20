// ckeletin:allow-custom-command
package cmd

import (
	"context"
	"time"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// defaultActivationDelta is the spreading activation weight when similarities
// are available. Configurable via experiments.activation.delta.
const defaultActivationDelta = 0.2

// rankedItem pairs a note ID with its position in a result list.
type rankedItem struct {
	ID   string
	Rank int
}

// computeActivationScores returns activation scores for the primary experiment
// variant. Returns nil if the experiment session is absent, the activation
// experiment is not enabled, or no notes have been accessed.
// similarities is optional — when provided (from search results), enables
// spreading activation (Delta > 0) in the scoring model.
func computeActivationScores(ctx context.Context, similarities map[string]float64, delta float64) map[string]float64 {
	session := experiment.FromContext(ctx)
	if session == nil {
		return nil
	}
	exps := loadExperimentDefs()
	actDef, ok := exps["activation"]
	if !ok || !actDef.Enabled {
		return nil
	}
	gamma, ok := experiment.VariantGamma(actDef.Primary)
	if !ok {
		log.Warn().Str("variant", actDef.Primary).Msg("unrecognized activation variant; skipping activation scoring")
		return nil
	}
	params := experiment.DefaultActivationParams(gamma)
	if similarities != nil {
		if delta > 0 {
			params.Delta = delta
		} else {
			params.Delta = defaultActivationDelta
		}
	}
	accessedNotes, err := session.DB.AccessedNoteIDs()
	if err != nil {
		log.Debug().Err(err).Msg("failed to load accessed notes for activation scoring")
		return nil
	}
	if len(accessedNotes) == 0 {
		return nil
	}
	scores, _, err := experiment.ComputeBatchScores(session.DB, accessedNotes, params, similarities)
	if err != nil {
		log.Debug().Err(err).Msg("failed to compute activation scores")
		return nil
	}
	return scores
}

// logAskExperiment logs an ask event with retrieval results and, when the
// activation experiment is enabled, shadow-scored variants alongside.
// retrievalMode is the retriever label (e.g. "hybrid", "keyword") used as the
// variant key for the actual retrieval hits.
func logAskExperiment(cmd *cobra.Command, queryText, vaultPath, retrievalMode string, result *query.AskResult) {
	session := experiment.FromContext(cmd.Context())
	if session == nil {
		return
	}
	session.SetVaultPath(vaultPath)

	var shadowVariants map[string]any
	primary := ""
	actEnabled := false
	if actDef, ok := loadExperimentDefs()["activation"]; ok && actDef.Enabled && result.Context != nil {
		actEnabled = true
		primary = actDef.Primary
		items := make([]rankedItem, len(result.Context.Context))
		for i, item := range result.Context.Context {
			items[i] = rankedItem{ID: item.ID, Rank: i + 1}
		}
		shadowVariants = buildVariantResults(session, actDef, items)
	}

	if _, err := session.LogAskEvent(queryText, buildAskEventData(result, retrievalMode, shadowVariants, primary, actEnabled)); err != nil {
		log.Debug().Err(err).Msg("failed to log ask experiment event")
	}
}

// buildAskEventData composes the event payload for an ask event. The retrieval
// mode is carried as a variant under `variants.{mode}` so downstream LinkOutcomes
// and shadow-scoring consumers see the retrieved notes. When activation is
// enabled, shadow variants are merged into the same variants map — activation
// variant names ("compressed-0.2", "wall-clock", "none", "compressed-0.5") do
// not collide with retrieval-mode names.
func buildAskEventData(result *query.AskResult, retrievalMode string, shadowVariants map[string]any, primaryVariant string, actEnabled bool) map[string]any {
	variants := experiment.BuildVariantPayload(retrievalMode, askRetrievalHits(result.TopHits))
	for name, payload := range shadowVariants {
		variants[name] = payload
	}
	data := map[string]any{
		"top_hits": len(result.TopHits),
		"variants": variants,
	}
	if actEnabled {
		data["primary_variant"] = primaryVariant
	}
	return data
}

// askRetrievalHits maps ask top hits to the experiment payload input type.
func askRetrievalHits(hits []query.ScoredResult) []experiment.RetrievalHit {
	out := make([]experiment.RetrievalHit, len(hits))
	for i, h := range hits {
		out[i] = experiment.RetrievalHit{
			NoteID:   h.ID,
			Rank:     i + 1,
			Score:    h.Score,
			NoteType: h.Type,
			Path:     h.Path,
		}
	}
	return out
}

// buildVariantResults computes activation features for all experiment variants
// across the given ranked items. Used for shadow-scoring comparison in
// experiment event logs.
func buildVariantResults(session *experiment.Session, actDef experiment.ExperimentDef, items []rankedItem) map[string]any {
	accessedNotes, err := session.DB.AccessedNoteIDs()
	if err != nil {
		log.Debug().Err(err).Msg("failed to load accessed notes for variant results")
		return map[string]any{}
	}
	accessMap, err := session.DB.BatchNoteAccessTimes(accessedNotes)
	if err != nil {
		log.Debug().Err(err).Msg("failed to load access times for variant results")
		return map[string]any{}
	}
	windows, err := session.DB.RecentSessionWindows(100)
	if err != nil {
		log.Debug().Err(err).Msg("failed to load session windows for variant results")
		return map[string]any{}
	}
	now := time.Now().UTC()

	variantResults := make(map[string]any, len(actDef.AllVariants()))
	for _, variant := range actDef.AllVariants() {
		gamma, ok := experiment.VariantGamma(variant)
		if !ok {
			log.Debug().Str("variant", variant).Msg("skipping unrecognized variant in shadow scoring")
			continue
		}
		params := experiment.DefaultActivationParams(gamma)
		_, feats := experiment.ScoreFromData(accessedNotes, accessMap, windows, now, params, nil)

		results := make([]any, 0, len(items))
		for _, item := range items {
			r := map[string]any{"note_id": item.ID, "rank": item.Rank}
			if f, fOK := feats[item.ID]; fOK {
				r["features"] = f
			}
			results = append(results, r)
		}
		variantResults[variant] = map[string]any{"results": results}
	}
	return variantResults
}
