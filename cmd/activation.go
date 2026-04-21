// ckeletin:allow-custom-command
package cmd

import (
	"context"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// defaultActivationDelta is the spreading activation weight when similarities
// are available. Configurable via experiments.activation.delta.
const defaultActivationDelta = 0.2

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
		// silent-failure-ok: activation scoring is opt-in telemetry. On
		// failure we fall through to the non-activation path (nil scores),
		// which the retriever handles identically to "activation off".
		log.Debug().Err(err).Msg("failed to load accessed notes for activation scoring")
		return nil
	}
	if len(accessedNotes) == 0 {
		return nil
	}
	scores, _, err := experiment.ComputeBatchScores(session.DB, accessedNotes, params, similarities)
	if err != nil {
		// silent-failure-ok: activation is an experimental enhancement.
		// Nil-score fallback restores baseline (non-activation) retrieval.
		log.Debug().Err(err).Msg("failed to compute activation scores")
		return nil
	}
	return scores
}

// logAskExperiment logs an ask event with retrieval results and, when the
// activation experiment is enabled, shadow-scored variants alongside. Logs
// regardless of ask error so failures are observable — retrievalErr becomes
// the event_data.error field. retrievalMode is the retriever label (e.g.
// "hybrid", "keyword") used as the variant key for the actual retrieval hits.
func logAskExperiment(cmd *cobra.Command, queryText, vaultPath, retrievalMode string, result *query.AskResult, retrievalErr error) {
	session := experiment.FromContext(cmd.Context())
	if session == nil {
		return
	}
	session.SetVaultPath(vaultPath)

	params := experiment.AskEventParams{
		RetrievalMode: retrievalMode,
		TopHits:       askRetrievalHits(result),
		RetrievalErr:  retrievalErr,
	}
	if result != nil {
		if actDef, ok := loadExperimentDefs()["activation"]; ok && actDef.Enabled && result.Context != nil {
			params.ActivationOn = true
			params.PrimaryVariant = actDef.Primary
			params.ShadowVariants = experiment.BuildShadowVariantResults(session, actDef, contextNoteIDs(result.Context.Context))
		}
	}

	if _, err := session.LogAskEvent(queryText, experiment.BuildAskEventData(params)); err != nil {
		// silent-failure-ok: telemetry-only. Failing to log the event
		// must never block the user's ask — the result has already been
		// computed and returned above.
		log.Debug().Err(err).Msg("failed to log ask experiment event")
	}
}

// contextNoteIDs extracts note IDs from context-pack items in rank order.
// Thin adapter between memory.ContextItem and the experiment layer.
func contextNoteIDs(items []memory.ContextItem) []string {
	ids := make([]string, len(items))
	for i, it := range items {
		ids[i] = it.ID
	}
	return ids
}

// askRetrievalHits adapts query top hits into the experiment payload shape.
// Rank is 1-indexed within the result set (ask does not paginate — no offset).
// Returns nil when result is nil so callers can pass straight through.
func askRetrievalHits(result *query.AskResult) []experiment.RetrievalHit {
	if result == nil {
		return nil
	}
	out := make([]experiment.RetrievalHit, len(result.TopHits))
	for i, h := range result.TopHits {
		out[i] = experiment.RetrievalHit{
			NoteID:   h.ID,
			Rank:     i + 1,
			Score:    h.Score,
			NoteType: h.Type,
			Path:     h.Path,
			Scores:   h.Components,
		}
	}
	return out
}
