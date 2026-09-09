package query

import (
	"bytes"
	"testing"

	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/stretchr/testify/require"
)

// Federated retrieval exists because a single --vault is not how an agent's
// memory is actually shaped. Mine is three vaults — identity, desk, research —
// and the recall hooks search exactly one. Measured cost, 2026-08-23: of four
// real findings from a working week, ONE was retrievable. Two of the three
// misses were written down, in the desk, where they retrieve at z=+1.81 and
// z=+3.08. Unreachable solely because no hook searches a second vault.
//
// Worse than absent: `--vault A --vault B` silently used B and returned a
// confident no-match having never looked in A.

func hit(id string, score float64) retrieval.ScoredResult {
	return retrieval.ScoredResult{ID: id, Title: id, Score: score}
}

// Scores are NOT comparable across vaults — each has its own noise floor and
// dispersion — but ranks are. Cross-vault RRF is the same math VaultMind
// already runs over its four lanes, promoted one level.
func TestMergeByRRF_RankBasedNotScoreBased(t *testing.T) {
	// Vault A's top hit has a numerically tiny score; vault B's runner-up has a
	// larger one. Score concatenation would put B's second place first.
	merged := mergeByRRFWithRelevance(map[string][]retrieval.ScoredResult{
		"identity": {hit("a-top", 0.001), hit("a-second", 0.0005)},
		"desk":     {hit("b-top", 0.9), hit("b-second", 0.8)},
	}, defaultRRFK, nil)

	require.Len(t, merged, 4)
	require.ElementsMatch(t,
		[]string{"a-top", "b-top"},
		[]string{merged[0].ID, merged[1].ID},
		"both vaults' rank-1 hits must outrank both rank-2 hits, whatever the raw scores say")
}

func TestMergeByRRF_AttributesEveryHitToItsVault(t *testing.T) {
	merged := mergeByRRFWithRelevance(map[string][]retrieval.ScoredResult{
		"identity": {hit("a-top", 0.5)},
		"desk":     {hit("b-top", 0.4)},
	}, defaultRRFK, nil)

	byID := map[string]string{}
	for _, m := range merged {
		byID[m.ID] = m.Vault
	}
	require.Equal(t, "identity", byID["a-top"], "origin must survive the merge — 'why is this here?' is answerable")
	require.Equal(t, "desk", byID["b-top"])
}

// A note present in several vaults is stronger evidence, not an accident to
// deduplicate away. RRF amplifies it, and the merged entry keeps the vault it
// ranked highest in so delivery has one unambiguous owner.
func TestMergeByRRF_SharedNoteIsAmplifiedAndOwnedOnce(t *testing.T) {
	merged := mergeByRRFWithRelevance(map[string][]retrieval.ScoredResult{
		"identity": {hit("filler-1", 0.9), hit("shared", 0.5)},
		"desk":     {hit("filler-2", 0.9), hit("shared", 0.4)},
	}, defaultRRFK, nil)

	var count int
	var owner string
	for _, m := range merged {
		if m.ID == "shared" {
			count++
			owner = m.Vault
		}
	}
	require.Equal(t, 1, count, "one entry per note id, not one per vault")
	require.NotEmpty(t, owner)
	require.Equal(t, "shared", merged[0].ID,
		"appearing in both vaults at rank 2 must beat appearing in one vault at rank 1")
}

func TestMergeByRRF_EmptyVaultContributesNothingAndDoesNotPanic(t *testing.T) {
	merged := mergeByRRFWithRelevance(map[string][]retrieval.ScoredResult{
		"identity": {hit("only", 0.5)},
		"empty":    {},
	}, defaultRRFK, nil)
	require.Len(t, merged, 1)
	require.Equal(t, "identity", merged[0].Vault)
}

func TestMergeByRRF_IsDeterministicForTiedRanks(t *testing.T) {
	in := map[string][]retrieval.ScoredResult{
		"b-vault": {hit("z", 0.5)},
		"a-vault": {hit("y", 0.5)},
	}
	first := mergeByRRFWithRelevance(in, defaultRRFK, nil)
	for i := 0; i < 20; i++ {
		again := mergeByRRFWithRelevance(in, defaultRRFK, nil)
		require.Equal(t, first[0].ID, again[0].ID,
			"map iteration order must not leak into results — a flaky ranking is an unreproducible measurement")
	}
}

// A federated answer that does not say WHERE each hit came from is barely an
// answer: "why is this here?" is the first question a three-vault agent asks,
// and the origin is also what tells you which vault to go write to.
func TestFormatAsk_FederatedHitsCarryTheirVaultTag(t *testing.T) {
	res := &AskResult{
		Query: "spreading activation",
		Vault: "vaultmind-mine",
		TopHits: []retrieval.ScoredResult{
			{ID: "journal-x", Title: "A Desk Entry", Type: "journal", Score: 0.02},
		},
		Federated: []FederatedHit{
			{ScoredResult: retrieval.ScoredResult{ID: "journal-x", Title: "A Desk Entry", Type: "journal"},
				Vault: "vaultmind-mine", RRFScore: 0.03},
			{ScoredResult: retrieval.ScoredResult{ID: "concept-y", Title: "A Research Note", Type: "concept"},
				Vault: "vaultmind-vault", RRFScore: 0.016},
		},
	}
	var buf bytes.Buffer
	require.NoError(t, FormatAsk(res, &buf))
	out := buf.String()

	require.Contains(t, out, "vaultmind-mine", "the owning vault must be visible")
	require.Contains(t, out, "vaultmind-vault", "a hit from another vault must name that vault")
	require.Contains(t, out, "concept-y",
		"the federated ranking must be shown, not just the owning vault's own hits")
}

func TestFormatAsk_SingleVaultOutputIsUnchanged(t *testing.T) {
	res := &AskResult{
		Query:   "spreading activation",
		TopHits: []retrieval.ScoredResult{{ID: "concept-z", Title: "Z", Type: "concept", Score: 0.02}},
	}
	var buf bytes.Buffer
	require.NoError(t, FormatAsk(res, &buf))
	require.NotContains(t, buf.String(), "vault:",
		"no federation ⇒ no federation furniture; existing output must not change")
}

// DOGFOOD FINDING (2026-09-09, first live federated query): pure RRF is
// relevance-BLIND. Every vault's rank-1 earns identical credit, so a vault
// with nothing relevant still wins the merge with its best irrelevant note.
// Observed: querying three vaults for a desk finding put the identity vault's
// z=-1.25 no_match note first and the correct desk entry second.
//
// The fix is the noise floor we already have, applied per vault: a vault whose
// own calibration says "nothing here" has nothing to contribute, and must not
// outrank a vault that does. Ranks are comparable across vaults; RELEVANCE is
// what each vault's own floor decides.

func TestGateByOwnFloor_DropsVaultsWithNothingRelevant(t *testing.T) {
	in := map[string][]retrieval.ScoredResult{
		"identity": {hit("irrelevant-top", 0.02)},
		"desk":     {hit("the-answer", 0.015)},
	}
	verdicts := map[string]string{
		"identity": ConfidenceNoMatch, // its own floor says nothing here
		"desk":     ConfidenceModerate,
	}

	kept := gateByOwnFloor(in, verdicts)
	require.NotContains(t, kept, "identity", "a vault below its own floor contributes nothing")
	require.Contains(t, kept, "desk")

	merged := mergeByRRFWithRelevance(kept, defaultRRFK, nil)
	require.Equal(t, "the-answer", merged[0].ID,
		"the vault that actually has the answer must win, whatever the other vault's rank-1 scored")
}

// If EVERY vault says no_match the answer is genuinely "nothing anywhere" —
// and the merge must still return the candidates rather than an empty list, so
// the caller renders the honest no-match path instead of a blank.
func TestGateByOwnFloor_AllNoMatchKeepsEverythingForTheHonestNoMatchPath(t *testing.T) {
	in := map[string][]retrieval.ScoredResult{
		"identity": {hit("a", 0.02)},
		"desk":     {hit("b", 0.01)},
	}
	verdicts := map[string]string{"identity": ConfidenceNoMatch, "desk": ConfidenceNoMatch}

	kept := gateByOwnFloor(in, verdicts)
	require.Len(t, kept, 2, "nothing relevant anywhere is a finding, not an empty result set")
}

// A vault with no verdict (no embedder, so no floor to judge against) is kept:
// absence of a measurement is not evidence of irrelevance.
func TestGateByOwnFloor_UnmeasuredVaultIsKept(t *testing.T) {
	in := map[string][]retrieval.ScoredResult{
		"identity": {hit("a", 0.02)},
		"keyword":  {hit("b", 0.01)},
	}
	kept := gateByOwnFloor(in, map[string]string{"identity": ConfidenceNoMatch})
	require.Contains(t, kept, "keyword", "unmeasured must not be treated as unworthy")
}

// DOGFOOD FINDING #2 (same live run): when two vaults each contribute a
// rank-1, their RRF scores TIE, and the tie broke on alphabetical vault name.
// Measured: identity (z=+0.39) beat desk (z=+0.93) for a desk-owned answer
// purely because "identity" sorts before "mine". A tie-break must be a
// judgement, not an accident of naming.
func TestMergeByRRF_TiesBreakOnRelevanceNotAlphabet(t *testing.T) {
	perVault := map[string][]retrieval.ScoredResult{
		"aaa-vault": {hit("weakly-relevant", 0.02)},
		"zzz-vault": {hit("strongly-relevant", 0.02)},
	}
	// Both rank-1 ⇒ identical RRF. The vaults' own floors disagree sharply.
	relevance := map[string]float64{"aaa-vault": 0.39, "zzz-vault": 0.93}

	merged := mergeByRRFWithRelevance(perVault, defaultRRFK, relevance)
	require.Equal(t, "strongly-relevant", merged[0].ID,
		"the vault whose own floor says this is a better match must win a rank tie")
}

func TestMergeByRRF_RelevanceNeverOverridesRank(t *testing.T) {
	perVault := map[string][]retrieval.ScoredResult{
		"low-z":  {hit("rank-1-here", 0.02)},
		"high-z": {hit("filler", 0.9), hit("rank-2-here", 0.5)},
	}
	relevance := map[string]float64{"low-z": 0.10, "high-z": 3.00}

	merged := mergeByRRFWithRelevance(perVault, defaultRRFK, relevance)
	pos := map[string]int{}
	for i, m := range merged {
		pos[m.ID] = i
	}
	// The high-z vault's OWN rank-1 ("filler") may win the tie against the
	// low-z vault's rank-1 — that is the tie-break working. What must never
	// happen is a rank-2 climbing over a rank-1 on relevance alone, which
	// would rebuild the cross-vault score comparison RRF exists to avoid.
	require.Less(t, pos["rank-1-here"], pos["rank-2-here"],
		"relevance breaks TIES only; it must not let a rank-2 outrank a rank-1")
}

// The first live federated run printed "federated across 1 vaults" while
// searching three — it counted vaults that SURVIVED the relevance gate, not
// vaults searched. A reader could not tell "we only looked in one" from "we
// looked in three and two had nothing", which are completely different facts
// about your memory. Say both, and say which vaults were dropped and why.
func TestFormatAsk_FederationHeaderReportsSearchedAndContributing(t *testing.T) {
	res := &AskResult{
		Query: "the mv alias trap",
		Vault: "vaultmind-mine",
		TopHits: []retrieval.ScoredResult{
			{ID: "journal-x", Title: "Desk Entry", Type: "journal", Score: 0.02},
		},
		Federated: []FederatedHit{
			{ScoredResult: retrieval.ScoredResult{ID: "journal-x", Title: "Desk Entry"}, Vault: "vaultmind-mine"},
		},
		FederatedVaults: []FederatedVaultStatus{
			{Name: "vaultmind-identity", Verdict: ConfidenceNoMatch, Contributed: false},
			{Name: "vaultmind-mine", Verdict: ConfidenceModerate, Contributed: true},
			{Name: "vaultmind-vault", Verdict: ConfidenceNoMatch, Contributed: false},
		},
	}
	var buf bytes.Buffer
	require.NoError(t, FormatAsk(res, &buf))
	out := buf.String()

	require.Contains(t, out, "3 vaults", "the number SEARCHED must be visible")
	require.NotContains(t, out, "1 vaults", "…and must not be replaced by the number that survived the gate")
	require.Contains(t, out, "vaultmind-identity",
		"a searched-but-empty vault must be named, or 'we looked and found nothing there' is indistinguishable from 'we never looked'")
	require.Contains(t, out, "nothing above its own floor")
}

// LIVE FINDING on the real ORT binary (2026-09-09): hugot allows exactly ONE
// embedder session per process. Federation opened every vault at once, so the
// first vault took the embedder and the rest silently fell back to KEYWORD
// search — reporting "unmeasured, no embedder" for vaults that have full
// BGE-M3 indexes. Worse, my earlier measurement was taken on a non-ORT test
// build where that limit does not exist, so the numbers described a binary
// nobody runs.
//
// The merge therefore takes COLLECTED results, not live retrievers: the caller
// searches one vault at a time and releases each embedder before opening the
// next.
func TestMergeFederated_TakesCollectedResultsNotLiveRetrievers(t *testing.T) {
	merged := MergeFederated(
		map[string][]retrieval.ScoredResult{
			"identity": {hit("irrelevant", 0.02)},
			"desk":     {hit("the-answer", 0.02)},
		},
		map[string]string{"identity": ConfidenceNoMatch, "desk": ConfidenceModerate},
		map[string]float64{"identity": -1.25, "desk": 1.05},
	)
	require.NotEmpty(t, merged)
	require.Equal(t, "the-answer", merged[0].ID)
	require.Equal(t, "desk", merged[0].Vault)
}

// MergeFederated must GATE, not merely be able to.
//
// Review mutation M8 unwired gateByOwnFloor from MergeFederated and every test
// stayed green: the gate had thorough unit tests and nothing asserted the
// public entry point actually calls it. That is the third time tonight the
// same shape appeared — a correct unit whose caller throws the answer away —
// so this test goes through MergeFederated deliberately.
func TestMergeFederated_ActuallyAppliesTheGate(t *testing.T) {
	perVault := map[string][]retrieval.ScoredResult{
		"identity": {hit("irrelevant-but-rank-1", 0.02)},
		"desk":     {hit("the-answer", 0.015)},
	}
	verdicts := map[string]string{
		"identity": ConfidenceNoMatch,
		"desk":     ConfidenceModerate,
	}

	merged := MergeFederated(perVault, verdicts, map[string]float64{"identity": -1.25, "desk": 0.93})

	require.NotEmpty(t, merged)
	require.Equal(t, "the-answer", merged[0].ID,
		"a vault its own floor calls no_match must not win on rank alone")
	for _, h := range merged {
		require.NotEqual(t, "identity", h.Vault,
			"the gated vault must contribute nothing through the public entry point")
	}
}

// An unmeasured vault is kept even when another vault IS measured and gated.
//
// The original version of this test had identity=no_match and keyword=unmeasured
// and nothing else. Mutating the gate to also drop unmeasured emptied `kept`,
// which tripped the all-no_match escape hatch and returned everything — so the
// assertion passed while the rule it guards was gone. A third, relevant vault
// keeps `kept` non-empty so the escape hatch cannot mask the mutation.
func TestGateByOwnFloor_UnmeasuredVaultIsKeptEvenWhenAnotherVaultSurvives(t *testing.T) {
	in := map[string][]retrieval.ScoredResult{
		"identity": {hit("a", 0.02)},
		"keyword":  {hit("b", 0.01)},
		"desk":     {hit("c", 0.03)},
	}
	verdicts := map[string]string{
		"identity": ConfidenceNoMatch,
		"desk":     ConfidenceModerate,
		// "keyword" has NO entry: unmeasured, because it has no embedder.
	}

	kept := gateByOwnFloor(in, verdicts)

	require.Contains(t, kept, "desk", "the measured, relevant vault survives")
	require.NotContains(t, kept, "identity", "the measured, irrelevant vault is dropped")
	require.Contains(t, kept, "keyword",
		"unmeasured is not irrelevant — an absent measurement must not be read as evidence")
}

// When a note ties at the same rank in two vaults, the MORE RELEVANT vault
// owns it — the same rule that breaks ordering ties.
//
// Ownership is not cosmetic: the owner delivers the body, records the access,
// and receives the plasticity update. Review mutation M6 (`<` to `<=`) handed
// ownership to whichever vault happened to be iterated last and no test
// noticed, which would have quietly moved reinforcement into the wrong vault.
func TestMergeByRRF_TiedOwnershipGoesToTheMoreRelevantVault(t *testing.T) {
	// BOTH orderings, deliberately. The first version of this test gave the
	// higher relevance to the vault that was also visited LAST, so "relevance
	// decides" and "last one wins" produced the same answer and the mutation
	// survived. A tie-break test has to include the case where the winner is
	// visited FIRST, or it is only asserting iteration order.
	for _, tc := range []struct {
		name      string
		relevance map[string]float64
		wantOwner string
	}{
		{"more relevant vault sorts first", map[string]float64{"alpha": 2.50, "beta": 0.10}, "alpha"},
		{"more relevant vault sorts last", map[string]float64{"alpha": 0.10, "beta": 2.50}, "beta"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			perVault := map[string][]retrieval.ScoredResult{
				"alpha": {hit("shared-note", 0.02)},
				"beta":  {hit("shared-note", 0.02)},
			}

			merged := mergeByRRFWithRelevance(perVault, defaultRRFK, tc.relevance)

			require.Len(t, merged, 1, "the same note in two vaults is one hit, amplified")
			require.Equal(t, tc.wantOwner, merged[0].Vault,
				"a rank tie is decided by relevance, not by which vault was visited last")
			require.Equal(t, map[string]int{"alpha": 1, "beta": 1}, merged[0].Ranks,
				"both vaults' ranks are still recorded, so an amplified hit can show its work")
		})
	}
}
