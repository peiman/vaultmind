package schema_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/schema"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
)

// TestRegistry_NoAliases_BackwardCompat — existing NewRegistry constructor
// continues to work; a registry built without aliases reports nil/empty
// alias data and IsFieldPresent matches direct field presence only.
func TestRegistry_NoAliases_BackwardCompat(t *testing.T) {
	reg := schema.NewRegistry(map[string]vault.TypeDef{
		"research": {Required: []string{"title"}},
	})

	fm := map[string]interface{}{"updated": "2026-05-01"}
	assert.True(t, reg.IsFieldPresent(fm, "updated"))
	assert.False(t, reg.IsFieldPresent(fm, "last_updated"))
	assert.Equal(t, []string(nil), reg.Aliases("updated"))
	assert.False(t, reg.IsAlias("updated", "last_updated"))
}

// TestRegistry_WithAliases_FieldPresent — when a vault registers an alias,
// IsFieldPresent treats the alias as a stand-in for the canonical name.
// This is the load-bearing migration property: shahname-rts notes carry
// `last_updated` and never `updated`; aliasing lets vaultmind accept
// `last_updated` where `updated` is the canonical required name.
func TestRegistry_WithAliases_FieldPresent(t *testing.T) {
	aliases := map[string][]string{
		"updated": {"last_updated"},
	}
	reg := schema.NewRegistryWithAliases(map[string]vault.TypeDef{
		"research": {Required: []string{"title"}},
	}, aliases)

	// Note carries alias only — canonical missing.
	fmAliasOnly := map[string]interface{}{"last_updated": "2026-05-01"}
	assert.True(t, reg.IsFieldPresent(fmAliasOnly, "updated"),
		"alias-only note should satisfy IsFieldPresent for canonical name")

	// Note carries canonical only — alias not needed.
	fmCanonical := map[string]interface{}{"updated": "2026-05-01"}
	assert.True(t, reg.IsFieldPresent(fmCanonical, "updated"))

	// Note carries neither — fails.
	fmNeither := map[string]interface{}{"title": "x"}
	assert.False(t, reg.IsFieldPresent(fmNeither, "updated"))
}

// TestRegistry_MultiAlias_AnyMatches — multiple aliases per canonical;
// any one of them being present satisfies presence.
func TestRegistry_MultiAlias_AnyMatches(t *testing.T) {
	aliases := map[string][]string{
		"updated": {"last_updated", "modified", "date_updated"},
	}
	reg := schema.NewRegistryWithAliases(map[string]vault.TypeDef{
		"research": {Required: []string{"title"}},
	}, aliases)

	cases := []struct {
		name string
		fm   map[string]interface{}
		want bool
	}{
		{"first alias", map[string]interface{}{"last_updated": "x"}, true},
		{"second alias", map[string]interface{}{"modified": "x"}, true},
		{"third alias", map[string]interface{}{"date_updated": "x"}, true},
		{"unknown name", map[string]interface{}{"foo": "x"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, reg.IsFieldPresent(tc.fm, "updated"))
		})
	}
}

// TestRegistry_Aliases_QueryAPI — Aliases() and IsAlias() expose the
// alias mapping for callers (mutation guard, doctor, anything that needs
// to reason about field-name equivalence).
func TestRegistry_Aliases_QueryAPI(t *testing.T) {
	aliases := map[string][]string{
		"updated": {"last_updated", "modified"},
		"created": {"created_at"},
	}
	reg := schema.NewRegistryWithAliases(map[string]vault.TypeDef{
		"research": {Required: []string{"title"}},
	}, aliases)

	assert.ElementsMatch(t, []string{"last_updated", "modified"}, reg.Aliases("updated"))
	assert.ElementsMatch(t, []string{"created_at"}, reg.Aliases("created"))
	assert.Equal(t, []string(nil), reg.Aliases("nonexistent"))

	assert.True(t, reg.IsAlias("updated", "last_updated"))
	assert.True(t, reg.IsAlias("updated", "modified"))
	assert.True(t, reg.IsAlias("created", "created_at"))
	assert.False(t, reg.IsAlias("updated", "created_at"))
	assert.False(t, reg.IsAlias("updated", "updated"), "canonical itself is not its own alias")
}

// TestRegistry_EmptyValueDoesNotSatisfy — a field present with an empty
// string is treated the same as absent (matches existing fmFieldPresent
// semantics — required fields must have non-empty values).
func TestRegistry_EmptyValueDoesNotSatisfy(t *testing.T) {
	aliases := map[string][]string{
		"updated": {"last_updated"},
	}
	reg := schema.NewRegistryWithAliases(map[string]vault.TypeDef{
		"research": {Required: []string{"title"}},
	}, aliases)

	fmEmpty := map[string]interface{}{"last_updated": ""}
	assert.False(t, reg.IsFieldPresent(fmEmpty, "updated"),
		"empty-string alias should not satisfy presence")

	fmNil := map[string]interface{}{"last_updated": nil}
	assert.False(t, reg.IsFieldPresent(fmNil, "updated"))

	fmWhitespace := map[string]interface{}{"last_updated": "   "}
	assert.False(t, reg.IsFieldPresent(fmWhitespace, "updated"),
		"whitespace-only alias should not satisfy presence")
}

// TestRegistry_FieldNamesForLookup — returns canonical first, then aliases
// in registration order. Use case: alias-aware DB-backed validation that
// must iterate alternative key names, canonical-first so canonical wins
// when both are present in the index.
func TestRegistry_FieldNamesForLookup(t *testing.T) {
	t.Run("no aliases registered", func(t *testing.T) {
		reg := schema.NewRegistry(map[string]vault.TypeDef{
			"research": {Required: []string{"title"}},
		})
		assert.Equal(t, []string{"updated"}, reg.FieldNamesForLookup("updated"))
	})

	t.Run("single alias", func(t *testing.T) {
		reg := schema.NewRegistryWithAliases(map[string]vault.TypeDef{
			"research": {Required: []string{"title"}},
		}, map[string][]string{
			"updated": {"last_updated"},
		})
		assert.Equal(t, []string{"updated", "last_updated"}, reg.FieldNamesForLookup("updated"))
	})

	t.Run("multiple aliases preserve order", func(t *testing.T) {
		reg := schema.NewRegistryWithAliases(map[string]vault.TypeDef{
			"research": {Required: []string{"title"}},
		}, map[string][]string{
			"updated": {"last_updated", "modified", "date_updated"},
		})
		assert.Equal(t, []string{"updated", "last_updated", "modified", "date_updated"},
			reg.FieldNamesForLookup("updated"))
	})

	t.Run("canonical with no aliases for that key", func(t *testing.T) {
		reg := schema.NewRegistryWithAliases(map[string]vault.TypeDef{
			"research": {Required: []string{"title"}},
		}, map[string][]string{
			"updated": {"last_updated"},
		})
		// `created` has no aliases registered — returns just [canonical].
		assert.Equal(t, []string{"created"}, reg.FieldNamesForLookup("created"))
	})
}

// TestRegistry_FieldTypes_PresenceSemantics — frontmatter values can be
// strings, lists, maps, scalars (numbers, bools). Empty collections count
// as absent (consistent with how human-curated YAML expresses "no value");
// non-empty collections and any non-empty scalar count as present.
//
// Pins the type-switch contract that was lifted from query/validate_live.go's
// fmFieldPresent into the registry so alias-aware presence checks share it.
func TestRegistry_FieldTypes_PresenceSemantics(t *testing.T) {
	reg := schema.NewRegistry(map[string]vault.TypeDef{
		"research": {Required: []string{"title"}},
	})

	cases := []struct {
		name  string
		value interface{}
		want  bool
	}{
		{"empty list", []interface{}{}, false},
		{"non-empty list", []interface{}{"a"}, true},
		{"empty map", map[string]interface{}{}, false},
		{"non-empty map", map[string]interface{}{"k": "v"}, true},
		{"int scalar", 42, true},
		{"bool scalar", true, true},
		{"float scalar", 3.14, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fm := map[string]interface{}{"title": tc.value}
			assert.Equal(t, tc.want, reg.IsFieldPresent(fm, "title"))
		})
	}
}
