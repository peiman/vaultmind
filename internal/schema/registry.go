// Package schema provides the type registry and frontmatter validation.
package schema

import (
	"sort"
	"strings"

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
	types   map[string]vault.TypeDef
	aliases map[string][]string
}

// NewRegistry creates a Registry from config type definitions with no aliases.
func NewRegistry(types map[string]vault.TypeDef) *Registry {
	return &Registry{types: types}
}

// NewRegistryWithAliases creates a Registry that recognizes per-vault
// frontmatter field aliases. Aliases let migrating users keep their existing
// field names (e.g. `last_updated`) while vaultmind validates against
// canonical names (e.g. `updated`). The map is canonical → list of aliases.
//
// Aliasing is intentionally non-destructive: vaultmind never rewrites
// frontmatter to normalize field names. The alias and the canonical are
// equivalent at validation time only.
func NewRegistryWithAliases(types map[string]vault.TypeDef, aliases map[string][]string) *Registry {
	return &Registry{types: types, aliases: aliases}
}

// Aliases returns the registered aliases for a canonical field name, or nil
// if the canonical name has no aliases.
func (r *Registry) Aliases(canonical string) []string {
	if r.aliases == nil {
		return nil
	}
	return r.aliases[canonical]
}

// FieldNamesForLookup returns the canonical name first, followed by any
// registered aliases. Use this to resolve a field across alternative names
// when looking it up in stores that key by exact field name (e.g. the
// frontmatter_kv table) — try names in order, first non-empty wins.
//
// Canonical-first ordering preserves the canonical-precedence contract:
// when both canonical and alias are present, the canonical value is used.
func (r *Registry) FieldNamesForLookup(canonical string) []string {
	names := []string{canonical}
	names = append(names, r.aliases[canonical]...)
	return names
}

// IsAlias reports whether candidate is registered as an alias for canonical.
// Returns false when candidate equals canonical — a field is not its own alias.
func (r *Registry) IsAlias(canonical, candidate string) bool {
	if canonical == candidate {
		return false
	}
	for _, a := range r.aliases[canonical] {
		if a == candidate {
			return true
		}
	}
	return false
}

// IsFieldPresent reports whether the canonical field OR any of its
// registered aliases is present in fm with a non-empty value. This is the
// alias-aware variant of the package-private fmFieldPresent check used by
// validators — a single point that all required-field checks should call
// instead of inspecting fm directly.
func (r *Registry) IsFieldPresent(fm map[string]interface{}, canonical string) bool {
	if isFmFieldPresent(fm, canonical) {
		return true
	}
	for _, alias := range r.aliases[canonical] {
		if isFmFieldPresent(fm, alias) {
			return true
		}
	}
	return false
}

// isFmFieldPresent reports whether field is present in fm with a non-empty
// value. Empty strings, empty arrays, and empty maps count as absent —
// matching how human-curated frontmatter expresses "I have no value here."
// Lifted to the registry from query/validate_live.go so alias-aware presence
// checks share a single contract.
func isFmFieldPresent(fm map[string]interface{}, field string) bool {
	raw, ok := fm[field]
	if !ok || raw == nil {
		return false
	}
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v) != ""
	case []interface{}:
		return len(v) > 0
	case map[string]interface{}:
		return len(v) > 0
	default:
		return true
	}
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
// Core fields and graph-tier fields are always allowed. Registered aliases
// for any allowed canonical are also allowed — without this, mutation
// (`frontmatter set last_updated=...`) would reject the user's existing
// field name even when the alias was explicitly registered. M1 from the
// 2026-05-02 review.
func (r *Registry) IsFieldAllowed(typeName, field string) bool {
	if r.isFieldCanonicallyAllowed(typeName, field) {
		return true
	}
	// Alias check: is `field` a registered alias for any canonical that is
	// itself allowed for this type?
	for canonical, aliases := range r.aliases {
		for _, a := range aliases {
			if a == field && r.isFieldCanonicallyAllowed(typeName, canonical) {
				return true
			}
		}
	}
	return false
}

// isFieldCanonicallyAllowed checks if a canonical field name is allowed for
// a type — core, graph, type-required, or type-optional. Used by
// IsFieldAllowed both directly (for canonical lookup) and as the
// allow-list check when resolving aliases.
func (r *Registry) isFieldCanonicallyAllowed(typeName, field string) bool {
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
