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
