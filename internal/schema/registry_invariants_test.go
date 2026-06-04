package schema

import (
	"testing"
)

// TestFieldTiers_PairwiseDisjoint enforces the principle-8 invariant
// that the two frontmatter tiers (coreFields, recognizedFields)
// describe DIFFERENT fields. A field in two tiers means two
// enforcement paths claim ownership.
//
// Lives in package schema (not schema_test) so the unexported tier
// arrays are reachable. Per manifesto principle 9, this is the
// closest-to-compile-time enforcement layer for the invariant.
//
// History: there were previously four tiers (coreFields,
// vaultmindOwnedFields, humanCompatFields, graphFields). The 2026-05-04
// dogfood pass retired vaultmindOwnedFields entirely (vm_updated had
// no read-side consumer) and collapsed humanCompatFields + graphFields
// into recognizedFields. Two tiers now: required (coreFields) and
// tolerated (recognizedFields).
func TestFieldTiers_PairwiseDisjoint(t *testing.T) {
	tiers := map[string][]string{
		"coreFields":       coreFields,
		"recognizedFields": recognizedFields,
	}
	seen := make(map[string]string, 16)
	for tierName, tier := range tiers {
		for _, field := range tier {
			if prevTier, dup := seen[field]; dup {
				t.Errorf("field %q appears in both %s and %s — tiers must be pairwise disjoint",
					field, prevTier, tierName)
			}
			seen[field] = tierName
		}
	}
}
