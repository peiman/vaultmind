package experiment

import (
	"time"

	"github.com/rs/zerolog/log"
)

// How an ask reached the process that ran it, recorded so a slow or doubled
// ask can be traced to its path (#406).
const (
	// AskPathServed: the warm server answered it.
	AskPathServed = "served"
	// AskPathFallback: the server was asked but could not take it (none was
	// running, it could not be reached, or it closed without answering), so
	// the ask ran in its own process.
	AskPathFallback = "fallback"
	// AskPathInProcess: the server was not asked (turned off, or not
	// supported here), so the ask ran in its own process.
	AskPathInProcess = "in_process"
)

// Why bodies were withheld from an ask result. These name the branches that
// already exist in the formatter; recording which one fired turns "the agent
// ignored it" and "the tool never handed it over" into separate, countable
// facts.
const (
	// SuppressedByCaller: --pointers-only. The caller asked for ids.
	SuppressedByCaller = "pointers_only"
	// SuppressedBelowFloor: the top hit sat at or under the off-topic noise
	// floor, so the body was judged not worth the tokens.
	SuppressedBelowFloor = "below_floor"
	// SuppressedLowContrast: a tight vault, where the embedder cannot spread
	// scores and a "weak" top hit is often the best available correct match.
	// This is the reason most likely to be withholding something useful.
	SuppressedLowContrast = "low_contrast"
)

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
	// BodyDelivered reports whether the caller actually received note BODIES,
	// as opposed to a list of ids it would have to go and fetch.
	//
	// The distinction is the whole question. A hook that surfaces five perfect
	// pointers and no text has not delivered memory — it has delivered
	// homework, and the agent mid-task does not do homework. Without this
	// field, "surfaced but unused" cannot be told apart from "never actually
	// handed over", and those two have opposite fixes.
	BodyDelivered bool
	// SuppressedReason names why bodies were withheld, when they were. Empty
	// when they were delivered. See SuppressedReason* constants.
	SuppressedReason string
	// RetrievalErr, when non-nil, causes BuildRetrievalEventData to populate
	// the "error" field so failed retrievals are distinguishable from
	// zero-hit successes.
	RetrievalErr error
	// RelevanceZ is the top hit's band-normalized relevance when the noise
	// floor was applied; nil when it was never measured. A pointer because
	// z == 0 is a real measurement (the top hit sat exactly on the floor)
	// and must stay distinguishable from absence — a window criterion on
	// median z was once committed and then found uncomputable because this
	// value lived only in the CLI output, never in the event.
	RelevanceZ *float64
	// AskPath says how the ask reached the process that ran it; one of the
	// AskPath* constants, or empty when not known.
	AskPath string
	// Duration is how long the ask took, from when it reached the process
	// that ran it (for a served ask, when the server read it) to this event;
	// zero when not measured.
	Duration time.Duration
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
	data["body_delivered"] = p.BodyDelivered
	if p.SuppressedReason != "" {
		data["suppressed_reason"] = p.SuppressedReason
	}
	if p.RelevanceZ != nil {
		data["relevance_z"] = *p.RelevanceZ
	}
	if p.ActivationOn {
		data["primary_variant"] = p.PrimaryVariant
	}
	if p.AskPath != "" {
		data["ask_path"] = p.AskPath
	}
	if p.Duration > 0 {
		data["duration_ms"] = p.Duration.Milliseconds()
	}
	return data
}
