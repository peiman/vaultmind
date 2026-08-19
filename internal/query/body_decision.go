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
