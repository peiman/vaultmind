package query

import "github.com/peiman/vaultmind/internal/experiment"

// BodyDecision reports whether an ask result will hand the caller note BODIES,
// and when it will not, which rule withheld them.
//
// The decision itself is not new — formatAskWithOptions has always auto-degraded
// a weak or no-match result to pointers. What is new is that the decision is a
// VALUE instead of a side effect on a local, so the formatter and the telemetry
// read the same one. Recomputing it in two places is how a log grows to disagree
// with what the agent was actually shown, and a metric built on that log would
// measure the disagreement rather than the delivery.
//
// callerAsked is the caller's own --pointers-only. It wins, and it is recorded
// separately: "the hook asked for ids" and "the tool judged the hit too weak to
// show" are different facts with opposite remedies.
// DeliveredTo reports whether note TEXT actually reached the caller, given the
// output mode — which is the question telemetry needs and BodyDecision does not
// answer.
//
// BodyDecision describes what the TEXT formatter will render. The JSON envelope
// serializes bodies regardless of --pointers-only, so the two diverge there:
// `ask --pointers-only --json` was measured returning a 5,770-character target
// body while telemetry recorded body_delivered=false, and IsActivationSignal
// then discarded a genuine read — reopening the phantom loop from the other
// side.
//
// It also refuses to credit a delivery when no pack exists. On a context-pack
// error the result carries a nil Context, and the previous call site credited
// delivery anyway.
func (r *AskResult) DeliveredTo(callerAsked, jsonOutput bool) (delivered bool, reason string) {
	if r == nil || !r.packHasText() {
		return false, experiment.SuppressedBelowFloor
	}
	if jsonOutput {
		// The envelope carries bodies whatever the text formatter would do.
		return true, ""
	}
	return r.BodyDecision(callerAsked)
}

// packHasText reports whether the assembled pack actually holds note prose, as
// opposed to frontmatter alone. "The pack exists" and "the pack has content" are
// different facts, and only the second is a delivery.
func (r *AskResult) packHasText() bool {
	if r.Context == nil {
		return false
	}
	if r.Context.Target != nil && r.Context.Target.Body != "" {
		return true
	}
	for _, item := range r.Context.Context {
		if item.Body != "" {
			return true
		}
	}
	return false
}

func (r *AskResult) BodyDecision(callerAsked bool) (delivered bool, reason string) {
	if callerAsked {
		return false, experiment.SuppressedByCaller
	}
	if r == nil {
		return false, experiment.SuppressedBelowFloor
	}
	// A vault too small to calibrate is exempt: there the low-confidence verdict
	// is an artifact of judging a handful of notes against a floor built for a
	// large corpus, not a finding about the hit.
	if r.tooSmallToJudge() {
		return true, ""
	}
	switch r.TopHitConfidence {
	case ConfidenceNoMatch:
		return false, experiment.SuppressedBelowFloor
	case ConfidenceWeak:
		// A tight vault's notes are so self-similar that a correct hit cannot
		// rise far above the floor, so genuine matches read "weak". That is a
		// fact about the VAULT, not evidence about the hit — and it holds more
		// strongly the better curated the vault is, because curation is what
		// makes a vault tight.
		//
		// This branch used to suppress, which meant an identity vault — tight by
		// construction, every note being about one agent — could not deliver a
		// body at all: all four reach-hook trigger queries measure inside this
		// band (z = +0.17, +1.25, −0.32, +0.32). The formatter was already
		// printing "a weak top hit here is often the best available correct
		// match ... use --read 1 for the body" and then withholding the body,
		// telling the agent to fetch by hand what the pack had already
		// assembled and paid for.
		//
		// Genuinely irrelevant hits are unaffected: they land at/below the floor
		// as ConfidenceNoMatch above and stay suppressed in every vault.
		if r.LowContrastVault {
			return true, ""
		}
		return false, experiment.SuppressedBelowFloor
	}
	return true, ""
}
