package mutation

import (
	"testing"

	"github.com/peiman/vaultmind/internal/schema"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testRegistry() *schema.Registry {
	return schema.NewRegistry(map[string]vault.TypeDef{
		"project": {
			Required: []string{"status", "title"},
			Optional: []string{"owner_id", "tags", "aliases"},
			Statuses: []string{"active", "paused", "completed"},
		},
		"concept": {
			Required: []string{"title"},
			Optional: []string{"tags", "aliases"},
		},
	})
}

func TestValidateMutation_ImmutableID(t *testing.T) {
	reg := testRegistry()
	note := ParsedNoteInfo{ID: "proj-1", Type: "project", IsDomain: true, Keys: []string{"id", "type", "status"}}
	req := MutationRequest{Op: OpSet, Key: "id", Value: "new-id"}
	err := ValidateMutation(req, note, reg)
	require.Error(t, err)
	assert.Equal(t, "immutable_field", err.(*MutationError).Code)
}

func TestValidateMutation_ImmutableType(t *testing.T) {
	reg := testRegistry()
	note := ParsedNoteInfo{ID: "proj-1", Type: "project", IsDomain: true, Keys: []string{"id", "type", "status"}}
	req := MutationRequest{Op: OpSet, Key: "type", Value: "concept"}
	err := ValidateMutation(req, note, reg)
	require.Error(t, err)
	assert.Equal(t, "immutable_field", err.(*MutationError).Code)
}

func TestValidateMutation_UnknownKey(t *testing.T) {
	reg := testRegistry()
	note := ParsedNoteInfo{ID: "proj-1", Type: "project", IsDomain: true, Keys: []string{"id", "type", "status"}}
	req := MutationRequest{Op: OpSet, Key: "unknown_field", Value: "val"}
	err := ValidateMutation(req, note, reg)
	require.Error(t, err)
	assert.Equal(t, "unknown_key", err.(*MutationError).Code)
}

func TestValidateMutation_UnknownKey_AllowExtra(t *testing.T) {
	reg := testRegistry()
	note := ParsedNoteInfo{ID: "proj-1", Type: "project", IsDomain: true, Keys: []string{"id", "type", "status"}}
	req := MutationRequest{Op: OpSet, Key: "unknown_field", Value: "val", AllowExtra: true}
	err := ValidateMutation(req, note, reg)
	assert.NoError(t, err)
}

func TestValidateMutation_InvalidStatus(t *testing.T) {
	reg := testRegistry()
	note := ParsedNoteInfo{ID: "proj-1", Type: "project", IsDomain: true, Keys: []string{"id", "type", "status"}}
	req := MutationRequest{Op: OpSet, Key: "status", Value: "invalid_status"}
	err := ValidateMutation(req, note, reg)
	require.Error(t, err)
	assert.Equal(t, "invalid_status", err.(*MutationError).Code)
}

func TestValidateMutation_ValidStatus(t *testing.T) {
	reg := testRegistry()
	note := ParsedNoteInfo{ID: "proj-1", Type: "project", IsDomain: true, Keys: []string{"id", "type", "status"}}
	req := MutationRequest{Op: OpSet, Key: "status", Value: "paused"}
	err := ValidateMutation(req, note, reg)
	assert.NoError(t, err)
}

func TestValidateMutation_UnsetRequiredField(t *testing.T) {
	reg := testRegistry()
	note := ParsedNoteInfo{ID: "proj-1", Type: "project", IsDomain: true, Keys: []string{"id", "type", "status", "title"}}
	req := MutationRequest{Op: OpUnset, Key: "status"}
	err := ValidateMutation(req, note, reg)
	require.Error(t, err)
	assert.Equal(t, "missing_required_field", err.(*MutationError).Code)
}

func TestValidateMutation_UnsetOptionalField(t *testing.T) {
	reg := testRegistry()
	note := ParsedNoteInfo{ID: "proj-1", Type: "project", IsDomain: true, Keys: []string{"id", "type", "status", "tags"}}
	req := MutationRequest{Op: OpUnset, Key: "tags"}
	err := ValidateMutation(req, note, reg)
	assert.NoError(t, err)
}

func TestValidateMutation_NotDomainNote_Set(t *testing.T) {
	reg := testRegistry()
	note := ParsedNoteInfo{ID: "", Type: "", IsDomain: false, Keys: []string{}}
	req := MutationRequest{Op: OpSet, Key: "status", Value: "active"}
	err := ValidateMutation(req, note, reg)
	require.Error(t, err)
	assert.Equal(t, "not_domain_note", err.(*MutationError).Code)
}

func TestValidateMutation_Normalize_AllowsUnstructured(t *testing.T) {
	reg := testRegistry()
	note := ParsedNoteInfo{ID: "", Type: "", IsDomain: false, Keys: []string{}}
	req := MutationRequest{Op: OpNormalize}
	err := ValidateMutation(req, note, reg)
	assert.NoError(t, err)
}

func TestValidateMutation_MergeImmutableField(t *testing.T) {
	reg := testRegistry()
	note := ParsedNoteInfo{ID: "proj-1", Type: "project", IsDomain: true, Keys: []string{"id", "type", "status"}}
	req := MutationRequest{Op: OpMerge, Fields: map[string]interface{}{"id": "new-id", "status": "paused"}}
	err := ValidateMutation(req, note, reg)
	require.Error(t, err)
	assert.Equal(t, "immutable_field", err.(*MutationError).Code)
}

func TestValidateMutation_MergeUnknownKey(t *testing.T) {
	reg := testRegistry()
	note := ParsedNoteInfo{ID: "proj-1", Type: "project", IsDomain: true, Keys: []string{"id", "type", "status"}}
	req := MutationRequest{Op: OpMerge, Fields: map[string]interface{}{"unknown": "val"}}
	err := ValidateMutation(req, note, reg)
	require.Error(t, err)
	assert.Equal(t, "unknown_key", err.(*MutationError).Code)
}

func TestValidateMutation_UnsetImmutableField(t *testing.T) {
	reg := testRegistry()
	note := ParsedNoteInfo{ID: "proj-1", Type: "project", IsDomain: true, Keys: []string{"id", "type", "status"}}
	req := MutationRequest{Op: OpUnset, Key: "id"}
	err := ValidateMutation(req, note, reg)
	require.Error(t, err)
	assert.Equal(t, "immutable_field", err.(*MutationError).Code)
}

// testRegistryWithAliases returns a registry that aliases
// `display_status` to `status` (a type-required field for "project").
// Used by the alias-aware mutation tests — alias-of-required-field
// semantics need a real required-field canonical to test against.
//
// History: this previously aliased `creation_date` → `created` because
// `created` was a vaultmind-owned required field. The 2026-05-04
// retraction moved `created` to recognized-but-not-required, so the
// alias tests were rerouted to a type-required field (`status` for the
// project type). The alias-of-required semantics are unchanged; only
// the canonical the test points at moved.
func testRegistryWithAliases() *schema.Registry {
	return schema.NewRegistryWithAliases(map[string]vault.TypeDef{
		"project": {
			Required: []string{"status", "title"},
			Optional: []string{"owner_id", "tags", "aliases"},
			Statuses: []string{"active", "paused", "completed"},
		},
	}, map[string][]string{
		"status": {"display_status"},
	})
}

// TestValidateMutation_SetAliasField — IsFieldAllowed must accept
// registered aliases. Previously `frontmatter set display_status=...`
// would fail with unknown_key even when the alias was explicitly
// registered (M1 in the review). Migrating users hit this the first
// time they try to update a field they keep under their existing name.
func TestValidateMutation_SetAliasField(t *testing.T) {
	reg := testRegistryWithAliases()
	note := ParsedNoteInfo{ID: "proj-1", Type: "project", IsDomain: true, Keys: []string{"id", "type", "title", "display_status"}}
	req := MutationRequest{Op: OpSet, Key: "display_status", Value: "active"}
	err := ValidateMutation(req, note, reg)
	assert.NoError(t, err, "alias for canonical should be allowed without --allow-extra")
}

// TestValidateMutation_UnsetAliasOnlySatisfaction — unsetting the
// alias of a required canonical, when no other satisfaction exists,
// must block. Otherwise the unset leaves the canonical required-field
// unsatisfied silently (M3 in the review — the silent-failure-across-
// layers shape).
func TestValidateMutation_UnsetAliasOnlySatisfaction(t *testing.T) {
	// `status` is in td.Required for project. Note carries
	// `display_status` only — the alias's only satisfaction. Unset
	// must block.
	reg := testRegistryWithAliases()
	note := ParsedNoteInfo{
		ID: "proj-1", Type: "project", IsDomain: true,
		Keys: []string{"id", "type", "title", "display_status"},
	}
	req := MutationRequest{Op: OpUnset, Key: "display_status"}
	err := ValidateMutation(req, note, reg)
	require.Error(t, err)
	assert.Equal(t, "missing_required_field", err.(*MutationError).Code)
	assert.Equal(t, "display_status", err.(*MutationError).Field)
}

// TestValidateMutation_UnsetAliasWithCanonicalPresent — unsetting an
// alias when the canonical is ALSO present is fine; the field's role
// is still satisfied by canonical after the unset.
func TestValidateMutation_UnsetAliasWithCanonicalPresent(t *testing.T) {
	reg := testRegistryWithAliases()
	note := ParsedNoteInfo{
		ID: "proj-1", Type: "project", IsDomain: true,
		Keys: []string{"id", "type", "title", "status", "display_status"},
	}
	req := MutationRequest{Op: OpUnset, Key: "display_status"}
	err := ValidateMutation(req, note, reg)
	assert.NoError(t, err, "canonical still satisfies the required field after alias unset")
}

// TestValidateMutation_UnsetCanonicalWithAliasPresent — symmetric
// case: unsetting the canonical while an alias is present is allowed,
// because the field's role is satisfied by the alias after unset.
// Vaultmind treats canonical and registered alias as equivalent at
// validation time.
func TestValidateMutation_UnsetCanonicalWithAliasPresent(t *testing.T) {
	reg := testRegistryWithAliases()
	note := ParsedNoteInfo{
		ID: "proj-1", Type: "project", IsDomain: true,
		Keys: []string{"id", "type", "title", "status", "display_status"},
	}
	req := MutationRequest{Op: OpUnset, Key: "status"}
	err := ValidateMutation(req, note, reg)
	assert.NoError(t, err, "alias still satisfies the required field after canonical unset")
}
