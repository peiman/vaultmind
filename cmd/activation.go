// ckeletin:allow-custom-command
package cmd

import (
	"context"
	"time"

	"github.com/peiman/vaultmind/internal/experiment"
)

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
func computeActivationScores(ctx context.Context, similarities map[string]float64) map[string]float64 {
	session := experiment.FromContext(ctx)
	if session == nil {
		return nil
	}
	exps := loadExperimentDefs()
	actDef, ok := exps["activation"]
	if !ok || !actDef.Enabled {
		return nil
	}
	gamma, _ := experiment.VariantGamma(actDef.Primary)
	params := experiment.DefaultActivationParams(gamma)
	if similarities != nil {
		params.Delta = 0.2
	}
	accessedNotes, _ := session.DB.AccessedNoteIDs()
	if len(accessedNotes) == 0 {
		return nil
	}
	scores, _, _ := experiment.ComputeBatchScores(session.DB, accessedNotes, params, similarities)
	return scores
}

// buildVariantResults computes activation features for all experiment variants
// across the given ranked items. Used for shadow-scoring comparison in
// experiment event logs.
func buildVariantResults(session *experiment.Session, actDef experiment.ExperimentDef, items []rankedItem) map[string]any {
	accessedNotes, _ := session.DB.AccessedNoteIDs()
	accessMap, _ := session.DB.BatchNoteAccessTimes(accessedNotes)
	windows, _ := session.DB.RecentSessionWindows(100)
	now := time.Now().UTC()

	variantResults := make(map[string]any, len(actDef.AllVariants()))
	for _, variant := range actDef.AllVariants() {
		gamma, _ := experiment.VariantGamma(variant)
		params := experiment.DefaultActivationParams(gamma)
		_, feats := experiment.ScoreFromData(accessedNotes, accessMap, windows, now, params, nil)

		results := make([]any, 0, len(items))
		for _, item := range items {
			r := map[string]any{"note_id": item.ID, "rank": item.Rank}
			if f, ok := feats[item.ID]; ok {
				r["features"] = f
			}
			results = append(results, r)
		}
		variantResults[variant] = map[string]any{"results": results}
	}
	return variantResults
}
