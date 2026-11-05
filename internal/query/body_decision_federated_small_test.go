package query

import (
	"bytes"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/peiman/vaultmind/internal/noisefloor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A small vault is exempt from the body gate when no other vault can judge
// the query. Right after init there is nothing else to compare against, and
// withholding the body returns the right note and refuses to show it. Two
// fresh vaults are the same case.
//
// The exemption drops only when another vault is calibrated: at least
// noisefloor.MinCalibNotes notes and a noise-floor verdict. CalibratedPeer
// is that fact. A 19-note owner that survives on a weak label while such a
// peer said no_match is not exempt. Its own label stands.

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
	// The original bug: a peer judged the query. The all-small case clears this.
	r.CalibratedPeer = true
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

// Federated, every vault too small to calibrate, owner label weak: the body
// is still delivered. No peer has the notes to judge the query, so this is
// the same onboarding case as a single fresh vault — identity plus desk,
// both just created. Withholding here repeats the failure the exemption
// exists to prevent.
func TestBodyDecision_FederatedAllVaultsTooSmallWeakStillDeliversBody(t *testing.T) {
	r := federatedSmallOwner(ConfidenceWeak, true)
	// Identity and desk, both under the calibration gate. No peer can judge.
	r.CalibratedPeer = false

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

// HasCalibratedPeer is the one place the peer gate is computed. The owner's
// own size is not a peer. An empty verdict is unmeasured, not a judgement.
func TestHasCalibratedPeer(t *testing.T) {
	const owner = "vaultmind-mine"
	floor := noisefloor.MinCalibNotes
	cases := []struct {
		name     string
		counts   map[string]int
		verdicts map[string]string
		want     bool
	}{
		{
			name:     "other vault at the gate with a verdict",
			counts:   map[string]int{owner: 19, "vaultmind-identity": floor},
			verdicts: map[string]string{"vaultmind-identity": ConfidenceNoMatch},
			want:     true,
		},
		{
			name:     "other vault one note under the gate",
			counts:   map[string]int{owner: 19, "vaultmind-identity": floor - 1},
			verdicts: map[string]string{"vaultmind-identity": ConfidenceNoMatch},
			want:     false,
		},
		{
			name:     "large vault with an empty verdict is unmeasured",
			counts:   map[string]int{owner: 19, "vaultmind-identity": 100},
			verdicts: map[string]string{"vaultmind-identity": ""},
			want:     false,
		},
		{
			name:     "only the owner is calibrated",
			counts:   map[string]int{owner: 100, "vaultmind-desk": 5},
			verdicts: map[string]string{owner: ConfidenceNoMatch, "vaultmind-desk": ConfidenceWeak},
			want:     false,
		},
		{
			name:     "owner small and another vault calibrated",
			counts:   map[string]int{owner: 19, "vaultmind-identity": floor, "vaultmind-desk": 4},
			verdicts: map[string]string{"vaultmind-identity": ConfidenceWeak, "vaultmind-desk": ConfidenceWeak},
			want:     true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, HasCalibratedPeer(owner, tc.counts, tc.verdicts))
		})
	}
}
