package schema_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/schema"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRegistry_FromConfig(t *testing.T) {
	reg := schema.NewRegistry(testTypes())

	assert.True(t, reg.HasType("project"))
	assert.True(t, reg.HasType("concept"))
	assert.False(t, reg.HasType("unknown"))
}

func TestRegistry_RequiredFields(t *testing.T) {
	reg := schema.NewRegistry(map[string]vault.TypeDef{
		"project": {Required: []string{"status", "title"}},
	})

	fields := reg.RequiredFields("project")
	assert.Contains(t, fields, "status")
	assert.Contains(t, fields, "title")
	assert.Contains(t, fields, "id")
	assert.Contains(t, fields, "type")
	assert.Contains(t, fields, "created")
	assert.Contains(t, fields, "vm_updated")
}

func TestRegistry_ValidStatus(t *testing.T) {
	reg := schema.NewRegistry(map[string]vault.TypeDef{
		"project": {Statuses: []string{"active", "paused"}},
	})

	assert.True(t, reg.ValidStatus("project", "active"))
	assert.True(t, reg.ValidStatus("project", "paused"))
	assert.False(t, reg.ValidStatus("project", "unknown"))
}

func TestRegistry_ValidStatus_NoStatuses(t *testing.T) {
	reg := schema.NewRegistry(map[string]vault.TypeDef{
		"concept": {Statuses: []string{}},
	})

	assert.True(t, reg.ValidStatus("concept", "anything"))
}

func TestRegistry_IsFieldAllowed(t *testing.T) {
	reg := schema.NewRegistry(map[string]vault.TypeDef{
		"project": {
			Required: []string{"status"},
			Optional: []string{"owner_id", "tags"},
		},
	})

	assert.True(t, reg.IsFieldAllowed("project", "id"))
	assert.True(t, reg.IsFieldAllowed("project", "type"))
	assert.True(t, reg.IsFieldAllowed("project", "status"))
	assert.True(t, reg.IsFieldAllowed("project", "owner_id"))
	assert.False(t, reg.IsFieldAllowed("project", "random_field"))
}

// TestRegistry_FieldCategorization pins the truthful frontmatter
// taxonomy that emerged from the 2026-05-04 schema audit
// (reference-current-context). Per manifesto principles 1, 5, 9:
//
//   - coreFields = [id, type] — gated at parser classification.
//   - vaultmindOwnedFields = [created, vm_updated] — auto-maintained
//     by vaultmind itself; recognized in IsFieldAllowed; included in
//     RequiredFields (so mutation can't unset them — meaningless
//     since vaultmind would refill).
//   - humanCompatFields = [updated] — Obsidian-compat tolerated, NOT
//     auto-maintained, NOT in RequiredFields (file mtime is the SSOT
//     for "edited" per principle 7).
//   - graphFields = [title, status, aliases, tags, parent_id,
//     related_ids, source_ids] — recognized on any type.
//
// Earlier the schema declared coreFields = [id, type, created,
// updated, vm_updated] but no enforcement teeth existed for the
// dating triplet beyond mutation guards — pure documentation
// drift (manifesto principle 9: "suggestions don't survive time
// pressure"). The new shape makes each category's enforcement
// path explicit.
func TestRegistry_FieldCategorization(t *testing.T) {
	reg := schema.NewRegistry(map[string]vault.TypeDef{
		"source": {Required: []string{"url"}, Optional: []string{"author"}},
	})

	t.Run("coreFields are required and allowed", func(t *testing.T) {
		assert.True(t, reg.IsFieldAllowed("source", "id"))
		assert.True(t, reg.IsFieldAllowed("source", "type"))
		assert.Contains(t, reg.RequiredFields("source"), "id")
		assert.Contains(t, reg.RequiredFields("source"), "type")
	})

	t.Run("vaultmindOwnedFields are allowed and unset-protected", func(t *testing.T) {
		assert.True(t, reg.IsFieldAllowed("source", "created"))
		assert.True(t, reg.IsFieldAllowed("source", "vm_updated"))
		// In RequiredFields so mutation unset-guard blocks them.
		assert.Contains(t, reg.RequiredFields("source"), "created")
		assert.Contains(t, reg.RequiredFields("source"), "vm_updated")
	})

	t.Run("humanCompatFields are allowed but NOT required", func(t *testing.T) {
		assert.True(t, reg.IsFieldAllowed("source", "updated"))
		// Critically: NOT in RequiredFields. mtime is the SSOT for
		// "edited"; vaultmind doesn't write or require this field.
		assert.NotContains(t, reg.RequiredFields("source"), "updated")
	})

	t.Run("graphFields recognized regardless of type-required/optional", func(t *testing.T) {
		assert.True(t, reg.IsFieldAllowed("source", "tags"))
		assert.True(t, reg.IsFieldAllowed("source", "aliases"))
		assert.True(t, reg.IsFieldAllowed("source", "related_ids"))
		assert.True(t, reg.IsFieldAllowed("source", "parent_id"))
	})

	t.Run("td.Required and td.Optional still respected", func(t *testing.T) {
		assert.True(t, reg.IsFieldAllowed("source", "url"))
		assert.True(t, reg.IsFieldAllowed("source", "author"))
		assert.Contains(t, reg.RequiredFields("source"), "url")
	})

	t.Run("unknown fields are rejected", func(t *testing.T) {
		assert.False(t, reg.IsFieldAllowed("source", "random_field_xyz"))
	})
}

func TestRegistry_ListTypes(t *testing.T) {
	reg := schema.NewRegistry(testTypes())

	types := reg.ListTypes()
	require.Len(t, types, 2)
	assert.Equal(t, "concept", types[0])
	assert.Equal(t, "project", types[1])
}

func TestRegistry_GetTypeDef(t *testing.T) {
	reg := schema.NewRegistry(testTypes())

	td, ok := reg.GetTypeDef("project")
	require.True(t, ok)
	assert.Contains(t, td.Required, "status")

	_, ok = reg.GetTypeDef("nonexistent")
	assert.False(t, ok)
}

func testTypes() map[string]vault.TypeDef {
	return map[string]vault.TypeDef{
		"project": {
			Required: []string{"status", "title"},
			Optional: []string{"owner_id", "tags"},
			Statuses: []string{"active", "paused"},
			Template: "templates/project.md",
		},
		"concept": {
			Required: []string{"title"},
			Optional: []string{"aliases", "tags"},
			Statuses: []string{},
			Template: "templates/concept.md",
		},
	}
}
