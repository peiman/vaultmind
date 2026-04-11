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

// logAskExperiment logs experiment event with shadow variant scores for an ask command.
func logAskExperiment(cmd *cobra.Command, queryText, vaultPath string, result *query.AskResult) {
	session := experiment.FromContext(cmd.Context())
	if session == nil {
		return
	}
	session.SetVaultPath(vaultPath)
	exps := loadExperimentDefs()
	if actDef, ok := exps["activation"]; ok && actDef.Enabled && result.Context != nil {
		items := make([]rankedItem, len(result.Context.Context))
		for i, item := range result.Context.Context {
			items[i] = rankedItem{ID: item.ID, Rank: i + 1}
		}
		_, err := session.LogAskEvent(queryText, map[string]any{
			"primary_variant": actDef.Primary,
			"top_hits":        len(result.TopHits),
			"variants":        buildVariantResults(session, actDef, items),
		})
		if err != nil {
			log.Debug().Err(err).Msg("failed to log ask experiment event")
		}
	} else {
		_, err := session.LogAskEvent(queryText, map[string]any{
			"top_hits": len(result.TopHits),
			"variants": map[string]any{"none": map[string]any{"results": []any{}}},
		})
		if err != nil {
			log.Debug().Err(err).Msg("failed to log ask experiment event")
		}
	}
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
