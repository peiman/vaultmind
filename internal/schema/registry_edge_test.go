package schema_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/schema"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
)

// ValidStatus on an unregistered type must return false (not crash, not
// true by accident). Callers branch on this to decide whether to prompt
// the user for a type before validating.
func TestRegistry_ValidStatus_UnknownType(t *testing.T) {
	reg := schema.NewRegistry(map[string]vault.TypeDef{
		"concept": {Statuses: []string{"draft"}},
	})
	assert.False(t, reg.ValidStatus("not-a-type", "draft"),
		"unknown type must return false — silently accepting would mask typos")
}

// IsFieldAllowed on an unregistered type returns false. Core + graph-tier
// fields are global, but type-specific fields are gated by the type's
// registration. An unknown type defers to the field's global status.
func TestRegistry_IsFieldAllowed_UnknownTypeCoreFieldStillAllowed(t *testing.T) {
	reg := schema.NewRegistry(map[string]vault.TypeDef{
		"concept": {Required: []string{"title"}},
	})
	// Core fields are allowed for any type (registered or not)
	assert.True(t, reg.IsFieldAllowed("not-a-type", "id"),
		"core field 'id' must be allowed for any type — unknown types included")
}

// IsFieldAllowed on an unregistered type returns false for non-core fields.
// Regression: silently allowing arbitrary fields on unknown types would let
// schema drift accumulate undetected.
func TestRegistry_IsFieldAllowed_UnknownTypeNonCoreFieldRejected(t *testing.T) {
	reg := schema.NewRegistry(map[string]vault.TypeDef{
		"concept": {Required: []string{"title"}},
	})
	assert.False(t, reg.IsFieldAllowed("not-a-type", "url"),
		"non-core field on unknown type must be rejected")
}

// IsFieldAllowed returns true when the field matches the type's required list.
// This is the happy path that registry-tier fields rely on.
func TestRegistry_IsFieldAllowed_RequiredFieldMatches(t *testing.T) {
	reg := schema.NewRegistry(map[string]vault.TypeDef{
		"source": {Required: []string{"url"}, Optional: []string{"year"}},
	})
	assert.True(t, reg.IsFieldAllowed("source", "url"),
		"required field must be allowed for its owning type")
	assert.True(t, reg.IsFieldAllowed("source", "year"),
		"optional field must be allowed for its owning type")
}
