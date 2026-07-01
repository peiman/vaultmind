package distill

import (
	"regexp"
	"strings"
)

// RuleID names a candidate-extraction rule (the high-precision mechanical ones
// from the 2026-05-31 distillation review). Rule 2 (recurrence→structural) and
// arc DRAFTING are deliberately NOT here — they need semantic judgment the
// review found unreliable to mechanize, so they stay with the mind.
type RuleID string

const (
	// RuleAuthorityGrant — the partner transfers standing decision authority
	// ("you decide", "I trust you", "you are the one to judge"), not a bare
	// per-task approval ("go for it"). These mark relationship-shaping moments.
	RuleAuthorityGrant RuleID = "authority-grant"
	// RuleManifestoLens — the user turn invokes the manifesto as a decision lens
	// ("manifesto lens on", "remember the manifesto") OR cites a numbered
	// principle ("principle 5"). A frequent precursor to a lens-redirected
	// decision — an arc shape, when the lens actually overrides an instinct.
	RuleManifestoLens RuleID = "manifesto-lens"
	// RuleEvidenceGate — the partner makes proceeding conditional on the agent's
	// OWN confidence or the evidence ("if you're confident, merge"; "only if
	// you're sure"). A sibling of authority-grant: standing trust gated on the
	// agent's judgment rather than transferred outright. Marks the moment the
	// agent's evidence-bar becomes the deciding factor — a recurring arc
	// precursor that no authority-grant phrase catches (added after the Siavoush
	// content-machine field report, 2026-06-19: the detector missed an "if you
	// are confident we merge" arc precisely because it isn't a "you decide").
	RuleEvidenceGate RuleID = "evidence-gate"
)

// Candidate is a surfaced transformation moment — a propose-only pointer into
// an episode, never an arc. The mind drafts and approves; this just finds.
type Candidate struct {
	Rule      RuleID
	EpisodeID string
	TurnIndex int
	Timestamp string
	Verbatim  string // the user turn that triggered the rule (for the mind to quote)
	Trigger   string // the exact phrase that fired the rule (shows WHY; aids judgment)
}

// compactionMarker prefixes the machine-injected context-compaction summary,
// which the episode parser captures as a "user message". It is NOT a real
// partner push — it summarizes a prior session and so spuriously contains
// trigger phrases. Turns starting with it are skipped (a real-corpus probe
// found them to be the dominant false-positive source).
const compactionMarker = "This session is being continued from a previous conversation"

// authorityGrantLexemes are autonomy-TRANSFER phrases — a standing "you decide",
// not a per-task approval. Tightened per the 2026-05-31 review: bare approval
// tokens ("go for it", "yes please", "ok") deliberately match none of these, so
// they don't fire. Matched case-insensitively as substrings.
var authorityGrantLexemes = []string{
	"full autonomy",
	"you decide",
	"you have autonomy",
	"dont need to ask", "don't need to ask",
	"dont need to review", "don't need to review",
	"dont need my", "don't need my",
	"trust you",
	"as you see fit",
	"you are the one", // "you are the one who should evaluate / decide"
	"you should decide", "you should evaluate",
	"your call",
	"i dont mind", "i don't mind",
	"do as you",
}

// manifestoLensLexemes invoke the manifesto as a decision lens. The bare "the
// lens" was dropped after a real-corpus probe (2026-06-01): it matched the
// assistant's own reflective narration ("the lens redirected me"), not a push —
// lower precision than the spec guessed. A separate, broader signal —
// principleNRe ("principle <digit>") — also fires this rule; it can match
// incidental prose, accepted as a high-recall, human-filtered candidate.
var manifestoLensLexemes = []string{
	"manifesto lens",
	"remember the manifesto",
	"manifesto on",
}

var principleNRe = regexp.MustCompile(`principle\s+\d`)

// evidenceGateLexemes are confidence-conditional delegation phrases — the
// partner hands off the decision contingent on the agent's own certainty
// ("if you're confident we merge"). Kept deliberately tight (the conditional +
// a confidence word together) so it stays high-precision like the other rules:
// bare "confident"/"sure" prose does not fire — only the "if you're <conf>"
// construction. Matched case-insensitively as substrings.
var evidenceGateLexemes = []string{
	"if you're confident", "if you are confident",
	"if you're sure", "if you are sure",
	"if you feel confident",
	"as long as you're confident", "as long as you are confident",
	"as long as you're sure", "as long as you are sure",
	"only if you're sure", "only if you are sure",
}

// ExtractCandidates applies the mechanical rules to an episode's USER turns
// only. Commit/build/TDD noise lives in ASSISTANT turns, which are never
// scanned, so it is excluded by construction (no rule can fire on it). A turn
// matching both rules yields one candidate per rule.
func ExtractCandidates(ep *Episode) []Candidate {
	var out []Candidate
	for _, t := range ep.UserTurns {
		if strings.HasPrefix(strings.TrimSpace(t.Text), compactionMarker) {
			continue // machine-injected summary, not a real push
		}
		lower := strings.ToLower(t.Text)
		if m := matchAny(lower, authorityGrantLexemes); m != "" {
			out = append(out, candidate(RuleAuthorityGrant, ep, t, m))
		}
		if m := matchAny(lower, manifestoLensLexemes); m != "" {
			out = append(out, candidate(RuleManifestoLens, ep, t, m))
		} else if loc := principleNRe.FindString(lower); loc != "" {
			out = append(out, candidate(RuleManifestoLens, ep, t, loc))
		}
		if m := matchAny(lower, evidenceGateLexemes); m != "" {
			out = append(out, candidate(RuleEvidenceGate, ep, t, m))
		}
	}
	return out
}

func candidate(rule RuleID, ep *Episode, t Turn, trigger string) Candidate {
	return Candidate{
		Rule: rule, EpisodeID: ep.ID, TurnIndex: t.Index,
		Timestamp: t.Timestamp, Verbatim: t.Text, Trigger: trigger,
	}
}

// matchAny returns the first matching needle (the trigger), or "" if none.
func matchAny(haystack string, needles []string) string {
	for _, n := range needles {
		if strings.Contains(haystack, n) {
			return n
		}
	}
	return ""
}
