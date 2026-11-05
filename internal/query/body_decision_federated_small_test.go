package query

import (
	"bytes"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A small vault is exempt from the body gate only when it is the only vault
// in the ask. Right after init there is nothing else to compare against, and
// withholding the body returns the right note and refuses to show it.
//
// In a federation the same query was judged by the other vaults. A 19-note
// vault that survives on a weak label — peers said no_match, so the gate kept
// it and it owns the result — is not exempt. Its own label stands. Federated
// is the merged ranking; non-empty is how the result says this was multi-vault.

const federatedSmallBody = "the note body that either appears or does not"

func federatedSmallOwner(confidence string, federated bool) *AskResult {
	r := smallVaultResult(19, confidence, -0.11)
	r.Context = &memory.ContextPackResult{
		TargetID:     r.TopHits[0].ID,
		BudgetTokens: 900,
		UsedTokens:   40,
		Target: &memory.ContextPackTarget{
			Frontmatter: map[string]any{"title": r.TopHits[0].Title},
			Body:        federatedSmallBody,
		},
	}
	if !federated {
		return r
	}
	r.Vault = "vaultmind-mine"
	r.Federated = []FederatedHit{{
		ScoredResult: r.TopHits[0],
		Vault:        "vaultmind-mine",
	}}
	r.FederatedVaults = []FederatedVaultStatus{
		{Name: "vaultmind-identity", Verdict: ConfidenceNoMatch},
		{Name: "vaultmind-mine", Verdict: confidence, Contributed: true},
	}
	return r
}

func renderAsk(t *testing.T, r *AskResult) string {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, formatAskWithOptions(r, &buf, formatOpts{}))
	return buf.String()
}

// Federated, owner too small, label weak: the body is withheld. Today the
// small-vault exemption delivers it, which is what a three-vault recall did
// with an off-topic prompt.
func TestBodyDecision_FederatedSmallVaultWeakWithholdsBody(t *testing.T) {
	r := federatedSmallOwner(ConfidenceWeak, true)

	delivered, reason := r.BodyDecision(false)

	assert.False(t, delivered)
	assert.Equal(t, experiment.SuppressedBelowFloor, reason)

	out := renderAsk(t, r)
	assert.NotContains(t, out, federatedSmallBody)
	header := strings.SplitN(out, "\n", 2)[0]
	assert.NotContains(t, header, "showing the top hit anyway",
		"the header must not claim the body is shown once the label withholds it")
	assert.Contains(t, header, "body suppressed")
}

// Single vault, too small, label weak: the exemption stays. A new user's
// vault has no peer to compare against.
func TestBodyDecision_SingleSmallVaultWeakStillDeliversBody(t *testing.T) {
	r := federatedSmallOwner(ConfidenceWeak, false)

	delivered, reason := r.BodyDecision(false)

	assert.True(t, delivered)
	assert.Empty(t, reason)

	out := renderAsk(t, r)
	assert.Contains(t, out, federatedSmallBody)
	header := strings.SplitN(out, "\n", 2)[0]
	assert.Contains(t, header, "showing the top hit anyway")
}

// Federated, owner too small, label moderate or strong: the normal rules
// below the exemption still deliver. The small-vault gate is not a second
// reason to withhold a hit the label already accepted.
func TestBodyDecision_FederatedSmallVaultConfidentLabelDeliversBody(t *testing.T) {
	for _, confidence := range []string{ConfidenceModerate, ConfidenceStrong} {
		t.Run(confidence, func(t *testing.T) {
			r := federatedSmallOwner(confidence, true)

			delivered, reason := r.BodyDecision(false)

			assert.True(t, delivered)
			assert.Empty(t, reason)
			assert.Contains(t, renderAsk(t, r), federatedSmallBody)
		})
	}
}
