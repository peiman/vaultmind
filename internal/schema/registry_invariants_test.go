package schema

import (
	"testing"
)

// TestFieldTiers_PairwiseDisjoint enforces the principle 8 invariant
// that the four frontmatter tiers (coreFields, vaultmindOwnedFields,
// humanCompatFields, graphFields) describe DIFFERENT fields. A field
// in two tiers means two enforcement paths claim ownership — the
// definition of drift principle 8 separates concerns to prevent.
//
// Without this guard a future "let me move `created` from
// vaultmindOwnedFields to graphFields" PR could land both edits and
// no test would catch the duplicate. Comments rot; tests don't.
//
// Lives in package schema (not schema_test) so the unexported tier
// arrays are reachable. Per manifesto principle 9, this is the
// closest-to-compile-time enforcement layer for the invariant short
// of generics-or-typecheck-tricks that would obscure the four arrays.
func TestFieldTiers_PairwiseDisjoint(t *testing.T) {
	tiers := map[string][]string{
		"coreFields":           coreFields,
		"vaultmindOwnedFields": vaultmindOwnedFields,
		"humanCompatFields":    humanCompatFields,
		"graphFields":          graphFields,
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
