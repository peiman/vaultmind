// Package schema provides the type registry and frontmatter validation.
package schema

import (
	"sort"
	"strings"

	"github.com/peiman/vaultmind/internal/vault"
)

// coreFields gates domain-note classification — every domain note MUST
// have id and type. Files lacking either are classified non-domain by
// parser.ClassifyNote and skipped from validation entirely (surfaced
// separately by doctor as DoctorIssues.NotesMissingIDOrType).
//
// Per manifesto principle 1 (truth-seeking), this list reflects what
// vaultmind actually enforces. Earlier versions also listed `created`,
// `updated`, `vm_updated` here, but those are not gated at parser
// classification and were never enforced by validators — pure
// declaration without teeth, which principle 9 calls "suggestions"
// (suggestions don't survive time pressure). They're now categorized
// honestly: `created` and `vm_updated` as vaultmindOwnedFields (auto-
// maintained by vaultmind, unset-protected); `updated` as
// humanCompatFields (recognized for Obsidian compat, not maintained,
// not required — file mtime is the SSOT for "edited").
var coreFields = []string{"id", "type"}

// vaultmindOwnedFields are auto-maintained by vaultmind. Listed here
// to be unset-protected by the mutation guard (the user can't unset
// them — vaultmind owns them and would just refill on next mutation,
// so unset is meaningless) and to be recognized in IsFieldAllowed.
// Vaultmind itself maintains presence and freshness via auto-write
// paths: template (init), mutator (every frontmatter set/unset/merge),
// `vaultmind frontmatter fix --backfill` (migration tooling).
//
//   - `created`: when the note was first created. Auto-filled on init
//     and on `frontmatter fix --backfill` (from git first-commit when
//     possible, file mtime, or today's date). Read by no logic; useful
//     to humans for context.
//   - `vm_updated`: when vaultmind last wrote this note's frontmatter.
//     Distinct from file mtime (which catches any edit, including
//     direct vim/Obsidian/sed). Vault-portable processing tracker —
//     survives DB destruction, machine moves, git transfers. Used by
//     doctor to surface "edited since vaultmind processed" drift.
var vaultmindOwnedFields = []string{"created", "vm_updated"}

// humanCompatFields are recognized but neither required nor auto-
// maintained. Tolerated for backward compat with Obsidian-style
// frontmatter on existing user notes. File mtime is the SSOT for
// "last edited" (principle 7); `updated` in frontmatter is duplicate
// data that drifts. Vaultmind doesn't write it; vaultmind doesn't
// require it; users who already have it in their notes are not forced
// to remove it.
var humanCompatFields = []string{"updated"}

// graphFields are recognized graph-tier metadata used by retrieval
// and graph traversal. Tolerated on any type via IsFieldAllowed.
var graphFields = []string{"title", "status", "aliases", "tags", "parent_id", "related_ids", "source_ids"}

// VMUpdatedFormat is the canonical format for the vm_updated field —
// RFC3339 second-precision UTC. Per manifesto principle 7 (SSOT),
// every write site for vm_updated MUST format with this constant:
//
//   - internal/mutation/mutator.go (auto-bump on every operation)
//   - internal/template/process.go (init / scaffold)
//   - internal/initvault/initvault.go (vault scaffold dates)
//
// The format has sub-day precision because doctor's drift detector
// (commit 5 in this chain) compares file mtime against vm_updated
// to surface "edited since vaultmind processed" — date-only would
// produce false-positive drift within the same calendar day.
//
// Note: contains colons, so YAML serialization auto-quotes the
// emitted value. That's correct YAML; readers should strip surrounding
// quotes for parse comparisons.
const VMUpdatedFormat = "2006-01-02T15:04:05Z"

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

// RequiredFields returns the union of fields that mutation must protect
// from unset: coreFields (id, type — gating classification),
// vaultmindOwnedFields (created, vm_updated — vaultmind auto-maintains;
// unsetting is meaningless because vaultmind would just refill them),
// and the type's td.Required (the type's user-supplied contract).
//
// NOT used by validators for the missing-required-field rule —
// validators iterate td.Required only (vaultmind-owned fields are
// auto-maintained, not user-required; humanCompatFields are tolerated,
// not required). Used by mutation/validate.go's unset guard.
func (r *Registry) RequiredFields(typeName string) []string {
	fields := append([]string{}, coreFields...)
	fields = append(fields, vaultmindOwnedFields...)
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
// a type — coreFields, vaultmindOwnedFields, humanCompatFields, graphFields,
// type-required, or type-optional. Used by IsFieldAllowed both directly
// (for canonical lookup) and as the allow-list check when resolving
// aliases.
func (r *Registry) isFieldCanonicallyAllowed(typeName, field string) bool {
	for _, f := range coreFields {
		if f == field {
			return true
		}
	}
	for _, f := range vaultmindOwnedFields {
		if f == field {
			return true
		}
	}
	for _, f := range humanCompatFields {
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
