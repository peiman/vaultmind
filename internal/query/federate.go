package query

import (
	"context"
	"fmt"
	"sort"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/retrieval"
)

// Federated retrieval — one query, many vaults, one ranked answer.
//
// WHY. An agent's memory is not one vault. Mine is three (identity, desk,
// research) and every recall hook searched exactly one. Measured 2026-08-23:
// of four real findings from a working week, ONE was retrievable — and two of
// the three misses were WRITTEN DOWN, in the desk, retrieving there at z=+1.81
// and z=+3.08. The write path and the read path did not meet. Worse, passing
// `--vault A --vault B` silently kept B and answered "nothing relevant"
// without ever opening A: not a missing feature, a confident wrong answer.
//
// WHY RRF AND NOT SCORES. Each vault calibrates its own noise floor N and
// dispersion σ against its own corpus, so a 0.02 in a tight 12-note desk and a
// 0.02 in a 415-note research vault mean different things. Ranks survive that;
// scores do not. Cross-vault RRF is the same reciprocal-rank fusion VaultMind
// already runs across its four lanes, promoted one level — no new model, no
// new dependency, and a note that surfaces in several vaults is amplified
// rather than deduplicated away.

// defaultRRFK is the standard reciprocal-rank-fusion constant. Matches the
// per-lane fusion inside a vault so the two levels behave the same way.
const defaultRRFK = 60

// FederatedHit is a merged result carrying the vault it came from, so both the
// agent and the delivery path can answer "which vault owns this?".
type FederatedHit struct {
	retrieval.ScoredResult
	// Vault is the name of the vault this note was ranked highest in. One
	// owner per note: delivery, access recording and plasticity all follow it.
	Vault string `json:"vault"`
	// RRFScore is the fused rank score. Deliberately separate from Score,
	// which stays the ORIGINATING vault's own number — overwriting it would
	// hide the per-vault evidence behind a cross-vault artifact.
	RRFScore float64 `json:"rrf_score"`
	// Ranks records the 1-based rank this note achieved in each vault that
	// returned it, so an amplified hit can show its work.
	Ranks map[string]int `json:"ranks,omitempty"`
}

// mergeByRRF fuses per-vault ranked lists into one ranked list.
//
// Iteration is over a SORTED vault list, not the map, because Go randomizes
// map order: without it, tied hits would swap places between runs and every
// measurement taken from this would be unreproducible.
// mergeByRRFWithRelevance is mergeByRRF with a tie-break on each vault's own
// relevance (its top-hit z).
//
// Rank ties are common and their resolution used to be alphabetical: with two
// vaults each contributing a rank-1, "identity" beat "mine" because of the
// letter i. Measured live — identity's z=+0.39 note outranked the desk's
// z=+0.93 note, which actually answered the question. Relevance breaks TIES
// only; it never overrides rank, or we would be back to comparing scores
// across differently-calibrated vaults, which is what RRF is here to avoid.
func mergeByRRFWithRelevance(perVault map[string][]retrieval.ScoredResult, k int, relevance map[string]float64) []FederatedHit {
	vaults := make([]string, 0, len(perVault))
	for name := range perVault {
		vaults = append(vaults, name)
	}
	sort.Strings(vaults)

	type acc struct {
		hit      FederatedHit
		rrf      float64
		bestRank int
	}
	byID := map[string]*acc{}
	order := make([]string, 0)

	for _, vault := range vaults {
		for i, h := range perVault[vault] {
			rank := i + 1
			a, seen := byID[h.ID]
			if !seen {
				a = &acc{
					hit: FederatedHit{
						ScoredResult: h,
						Vault:        vault,
						Ranks:        map[string]int{},
					},
					bestRank: rank,
				}
				byID[h.ID] = a
				order = append(order, h.ID)
			}
			a.rrf += 1.0 / float64(k+rank)
			a.hit.Ranks[vault] = rank
			// The owning vault is the one it ranked highest in — the vault with
			// the strongest claim, and the one whose body/access/plasticity the
			// delivery path will use.
			if rank < a.bestRank {
				a.bestRank = rank
				a.hit.Vault = vault
				a.hit.ScoredResult = h
			}
		}
	}

	out := make([]FederatedHit, 0, len(order))
	for _, id := range order {
		a := byID[id]
		a.hit.RRFScore = a.rrf
		out = append(out, a.hit)
	}
	// Stable sort over a deterministic input order: equal RRF keeps the
	// sorted-vault-then-rank order established above.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].RRFScore != out[j].RRFScore {
			return out[i].RRFScore > out[j].RRFScore
		}
		// Tie: the owning vault that judges its own hit more relevant wins.
		// Absent relevance data both sides are 0 and the deterministic
		// sorted-vault order established above still decides.
		return relevance[out[i].Vault] > relevance[out[j].Vault]
	})
	return out
}

// gateByOwnFloor drops vaults whose OWN calibration says nothing here.
//
// Pure RRF is rank-based, which makes it scale-free across vaults — and
// relevance-blind. Every vault's rank-1 earns the same credit, so a vault with
// nothing relevant still wins with its best irrelevant note. Observed on the
// first live federated query: an identity-vault note at z=-1.25 (no_match)
// outranked the desk entry that actually answered the question.
//
// Each vault already knows whether it has anything: that is exactly what its
// noise floor measures. So RANKS are compared across vaults, and RELEVANCE is
// judged within each. Two rules keep this honest:
//   - a vault with NO verdict (no embedder ⇒ no floor) is kept, because an
//     absent measurement is not evidence of irrelevance;
//   - if every vault says no_match, nothing is dropped — "nothing anywhere" is
//     a finding the caller must be able to render, not an empty result.
func gateByOwnFloor(perVault map[string][]retrieval.ScoredResult, verdicts map[string]string) map[string][]retrieval.ScoredResult {
	kept := make(map[string][]retrieval.ScoredResult, len(perVault))
	for name, hits := range perVault {
		if verdicts[name] == ConfidenceNoMatch {
			continue
		}
		kept[name] = hits
	}
	if len(kept) == 0 {
		return perVault
	}
	return kept
}

// FederatedVaultStatus records what happened to one vault in a federated
// query: was it searched, what did its OWN floor say, did it contribute.
//
// Reported because "federated across 1 vault" while searching three is a lie
// of omission: "we only looked in one" and "we looked in three and two had
// nothing" are different facts about your memory, and only one of them means
// you should go write something down.
type FederatedVaultStatus struct {
	Name        string `json:"name"`
	Verdict     string `json:"verdict,omitempty"`
	Contributed bool   `json:"contributed"`
}

// VaultSource is one vault participating in a federated query: a display name
// and a way to search it. Everything else about the vault (its floor, its DB,
// its resolver) stays with the caller, because delivery happens in the owning
// vault's own pipeline rather than through some merged pseudo-vault.
type VaultSource struct {
	Name      string
	Retriever retrieval.Retriever
	// Verdict is this vault's OWN top-hit confidence, computed against its own
	// noise floor. Empty means unmeasured (no embedder), which is kept rather
	// than assumed irrelevant.
	Verdict string
	// RelevanceZ is this vault's own top-hit z, used only to break RANK TIES
	// between vaults. It is never compared as a score across vaults for
	// ordering — that is precisely what RRF exists to avoid.
	RelevanceZ float64
}

// MergeFederated fuses per-vault results that the CALLER has already
// collected, gating by each vault's own verdict and breaking rank ties on
// relevance.
//
// Taking collected results rather than live retrievers is not a style choice:
// the ORT/hugot embedder allows ONE session per process, so a federation that
// holds every vault open at once leaves all but the first on keyword search —
// silently, while reporting those vaults as "unmeasured". The caller searches
// one vault at a time and releases each embedder before opening the next.
func MergeFederated(perVault map[string][]retrieval.ScoredResult, verdicts map[string]string, relevance map[string]float64) []FederatedHit {
	return mergeByRRFWithRelevance(gateByOwnFloor(perVault, verdicts), defaultRRFK, relevance)
}

// FederatedSearch fans a query out across sources and returns one merged
// ranked list. A source that ERRORS is reported, never silently skipped: a
// federation that quietly drops a vault answers "nothing relevant" while
// having failed to look, which is the exact failure this feature exists to
// end.
func FederatedSearch(ctx context.Context, sources []VaultSource, query string, limit int) ([]FederatedHit, error) {
	if len(sources) == 0 {
		return nil, fmt.Errorf("federated search: no vaults given")
	}
	perVault := make(map[string][]retrieval.ScoredResult, len(sources))
	var failures []string
	for _, src := range sources {
		hits, _, err := src.Retriever.Search(ctx, query, limit, 0, index.SearchFilters{})
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", src.Name, err))
			continue
		}
		perVault[src.Name] = hits
	}
	if len(failures) > 0 && len(perVault) == 0 {
		return nil, fmt.Errorf("federated search: every vault failed: %v", failures)
	}
	verdicts := make(map[string]string, len(sources))
	for _, src := range sources {
		verdicts[src.Name] = src.Verdict
	}
	merged := mergeByRRFWithRelevance(gateByOwnFloor(perVault, verdicts), defaultRRFK, relevanceByVault(sources))
	if len(failures) > 0 {
		// Partial answer, named as partial. The caller decides what to do; what
		// it must not do is mistake this for a complete search.
		return merged, fmt.Errorf("federated search: %d of %d vaults failed: %v",
			len(failures), len(sources), failures)
	}
	return merged, nil
}

// relevanceByVault collects each source's own top-hit z for tie-breaking.
func relevanceByVault(sources []VaultSource) map[string]float64 {
	out := make(map[string]float64, len(sources))
	for _, s := range sources {
		out[s.Name] = s.RelevanceZ
	}
	return out
}
