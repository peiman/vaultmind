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
	// `created` is recognized but not required after the 2026-05-04
	// retraction; `vm_updated` was retired entirely (no read-side
	// consumer survived). RequiredFields = coreFields ∪ td.Required.
	assert.NotContains(t, fields, "created",
		"created is optional/tolerated, not required (post-retraction)")
	assert.NotContains(t, fields, "vm_updated",
		"vm_updated was retired in the 2026-05-04 chain")
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
// taxonomy that emerged from the 2026-05-04 chain + retraction
// (reference-current-context). Per manifesto principles 1, 5, 9:
//
//   - coreFields = [id, type] — gated at parser classification;
//     the only fields RequiredFields reports beyond td.Required.
//   - recognizedFields = [title, status, aliases, tags, parent_id,
//     related_ids, source_ids, created, updated] — recognized on
//     any type via IsFieldAllowed; never required by vaultmind,
//     never auto-maintained.
//
// Earlier shapes declared `vaultmindOwnedFields = [created, vm_updated]`
// (auto-maintained, unset-protected). The dogfood pass retired
// vm_updated entirely (no read-side consumer survived the false-
// positive collapse of mtime drift); `created` joined the tolerated
// tier. Same truth-seeking lens applied recursively.
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

	t.Run("created and updated are tolerated, not required", func(t *testing.T) {
		assert.True(t, reg.IsFieldAllowed("source", "created"))
		assert.True(t, reg.IsFieldAllowed("source", "updated"))
		assert.NotContains(t, reg.RequiredFields("source"), "created")
		assert.NotContains(t, reg.RequiredFields("source"), "updated")
	})

	t.Run("vm_updated is no longer recognized", func(t *testing.T) {
		// Retired in the 2026-05-04 chain. If this passes (i.e. allowed
		// returns false), the retirement held; if it returns true, the
		// field crept back in somewhere.
		assert.False(t, reg.IsFieldAllowed("source", "vm_updated"),
			"vm_updated was retired; recognizedFields must not list it")
	})

	t.Run("graph metadata recognized regardless of type-required/optional", func(t *testing.T) {
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
