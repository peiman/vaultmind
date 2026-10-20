package memory

// SessionPinnedBudget is the token ceiling a session start spends on notes
// marked pinned. doctor warns against this same number, and the session-start
// hooks default to it. One constant, so the warning and the hook cannot drift.
const SessionPinnedBudget = 1500

// SessionPinnedExcerpt is how many tokens of a long pinned note the
// session-start hook shows. The hook's VAULTMIND_PINNED_EXCERPT default is
// this number. The command's own --excerpt default stays 0, the whole body.
const SessionPinnedExcerpt = 400

// excerptForBudget is the one decision for "what text, and does it fit".
//
// Recite and pinned notes both deliver bodies under a token ceiling. A second
// copy of the excerpt and the fit check would let them disagree about an
// exact fit (Principle 7 — one excerpt and budget rule). The caller still
// records what it omitted, in its own order: a nil note stays where the
// caller saw it, and a note that does not fit is recorded there too.
//
// excerptTokens <= 0 means the whole body. budget <= 0 means unbounded.
// A note that lands exactly on the remaining budget is kept.
func excerptForBudget(body string, excerptTokens, spent, budget int) (text string, cost int, keep bool) {
	text = body
	if excerptTokens > 0 {
		text = Excerpt(body, excerptTokens)
	}
	cost = EstimateTokens(text)
	keep = budget <= 0 || spent+cost <= budget
	return text, cost, keep
}
