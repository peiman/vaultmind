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
		// rise far above the floor, so genuine matches read "weak". The
		// formatter already says so in the output; recording it separately is
		// what will show how much of the withholding is this case rather than a
		// genuinely bad hit.
		if r.LowContrastVault {
			return false, experiment.SuppressedLowContrast
		}
		return false, experiment.SuppressedBelowFloor
	}
	return true, ""
}
