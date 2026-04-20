package experiment

import "github.com/rs/zerolog/log"

// AskEventParams is the boundary-clean input for composing an ask event
// payload. The cmd layer converts retrieval/result types into this shape so
// the experiment package stays agnostic of retrieval implementation details
// (no import of internal/query).
type AskEventParams struct {
	// RetrievalMode is the retriever label used as the variant key for the
	// actual retrieval hits (e.g. "hybrid", "keyword").
	RetrievalMode string
	// TopHits are the retrieval results, in rank order.
	TopHits []RetrievalHit
	// ShadowVariants are the shadow-scored variant results from
	// BuildShadowVariantResults. Merged into the event's variants map.
	ShadowVariants map[string]any
	// PrimaryVariant is the activation experiment's chosen variant name
	// (recorded only when ActivationOn is true).
	PrimaryVariant string
	// ActivationOn reports whether the activation experiment is enabled for
	// this event. When false, PrimaryVariant is omitted from the payload.
	ActivationOn bool
	// RetrievalErr, when non-nil, causes BuildRetrievalEventData to populate
	// the "error" field so failed retrievals are distinguishable from
	// zero-hit successes.
	RetrievalErr error
}

// BuildAskEventData composes the event_data payload for an ask event.
// Retrieval hits are carried as a variant under variants.{RetrievalMode};
// shadow variants are merged into the same map. A collision between a shadow
// variant name and the retrieval mode emits a warn log and the shadow payload
// wins (deterministic, documented behavior — rename one set if it matters).
func BuildAskEventData(p AskEventParams) map[string]any {
	variants := BuildVariantPayload(p.RetrievalMode, p.TopHits)
	for name, payload := range p.ShadowVariants {
		if _, clash := variants[name]; clash {
			log.Warn().Str("variant", name).Msg("shadow variant name collides with retrieval mode; retrieval payload overwritten")
		}
		variants[name] = payload
	}
	data := BuildRetrievalEventData(variants, len(p.TopHits), p.RetrievalErr)
	data["top_hits"] = len(p.TopHits)
	if p.ActivationOn {
		data["primary_variant"] = p.PrimaryVariant
	}
	return data
}
