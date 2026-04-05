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
