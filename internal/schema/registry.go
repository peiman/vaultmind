// Package schema provides the type registry and frontmatter validation.
package schema

import (
	"sort"

	"github.com/peiman/vaultmind/internal/vault"
)

// Core fields required on all domain notes.
// `updated` is human/Obsidian-managed, `vm_updated` is VaultMind-managed.
// Both are recognized as core fields.
var coreFields = []string{"id", "type", "created", "updated", "vm_updated"}

// Graph-tier fields recognized on any type.
var graphFields = []string{"title", "status", "aliases", "tags", "parent_id", "related_ids", "source_ids"}

// Registry holds the type definitions and provides validation methods.
type Registry struct {
	types map[string]vault.TypeDef
}

// NewRegistry creates a Registry from config type definitions.
func NewRegistry(types map[string]vault.TypeDef) *Registry {
	return &Registry{types: types}
}

// HasType returns whether a type name is registered.
func (r *Registry) HasType(typeName string) bool {
	_, ok := r.types[typeName]
	return ok
}

// RequiredFields returns all required fields for a type, including core fields.
func (r *Registry) RequiredFields(typeName string) []string {
	fields := append([]string{}, coreFields...)
	if td, ok := r.types[typeName]; ok {
		fields = append(fields, td.Required...)
	}
	return fields
}

// ValidStatus checks if a status value is valid for a type.
// Types with no defined statuses accept any value.
func (r *Registry) ValidStatus(typeName, status string) bool {
	td, ok := r.types[typeName]
	if !ok {
		return false
	}
	if len(td.Statuses) == 0 {
		return true
	}
	for _, s := range td.Statuses {
		if s == status {
			return true
		}
	}
	return false
}

// IsFieldAllowed checks if a field name is allowed for a type.
// Core fields and graph-tier fields are always allowed.
func (r *Registry) IsFieldAllowed(typeName, field string) bool {
	for _, f := range coreFields {
		if f == field {
			return true
		}
	}
	for _, f := range graphFields {
		if f == field {
			return true
		}
	}
	td, ok := r.types[typeName]
	if !ok {
		return false
	}
	for _, f := range td.Required {
		if f == field {
			return true
		}
	}
	for _, f := range td.Optional {
		if f == field {
			return true
		}
	}
	return false
}

// ListTypes returns all registered type names, sorted alphabetically.
func (r *Registry) ListTypes() []string {
	names := make([]string, 0, len(r.types))
	for name := range r.types {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// GetTypeDef returns the type definition for a given type name.
func (r *Registry) GetTypeDef(typeName string) (vault.TypeDef, bool) {
	td, ok := r.types[typeName]
	return td, ok
}
