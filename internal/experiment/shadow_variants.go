package experiment

import (
	"time"

	"github.com/rs/zerolog/log"
)

// BuildShadowVariantResults computes activation features for every variant in
// actDef (primary + shadows) over the given ranked note IDs. Returns a map
// shaped like the event's `variants` substructure:
//
//	{ variant: { results: [ { note_id, rank, features? } ] } }
//
// Rank is 1-indexed from the position of each note ID in noteIDs. Unknown
// variant names are skipped (logged at debug). Used by ask and context-pack
// events to emit shadow-scoring comparisons alongside primary retrieval.
func BuildShadowVariantResults(session *Session, actDef ExperimentDef, noteIDs []string) map[string]any {
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
		gamma, ok := VariantGamma(variant)
		if !ok {
			log.Debug().Str("variant", variant).Msg("skipping unrecognized variant in shadow scoring")
			continue
		}
		params := DefaultActivationParams(gamma)
		_, feats := ScoreFromData(accessedNotes, accessMap, windows, now, params, nil)

		results := make([]any, 0, len(noteIDs))
		for i, id := range noteIDs {
			r := map[string]any{"note_id": id, "rank": i + 1}
			if f, fOK := feats[id]; fOK {
				r["features"] = f
			}
			results = append(results, r)
		}
		variantResults[variant] = map[string]any{"results": results}
	}
	return variantResults
}
