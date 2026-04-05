package plan

import (
	"testing"

	"github.com/peiman/vaultmind/internal/schema"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
)

func testRegistry() *schema.Registry {
	return schema.NewRegistry(map[string]vault.TypeDef{
		"project":  {Required: []string{"status", "title"}, Statuses: []string{"active", "paused"}},
		"decision": {Required: []string{"title", "status"}, Statuses: []string{"proposed", "accepted"}},
	})
}

func TestValidatePlan_Valid(t *testing.T) {
	p := Plan{Version: 1, Operations: []Operation{
		{Op: OpFrontmatterSet, Target: "proj-1", Key: "status", Value: "paused"},
		{Op: OpNoteCreate, Path: "decisions/test.md", Type: "decision", Frontmatter: map[string]interface{}{"title": "T"}},
	}}
	assert.Empty(t, ValidatePlan(p, testRegistry()))
}

func TestValidatePlan_UnsupportedVersion(t *testing.T) {
	p := Plan{Version: 99, Operations: []Operation{{Op: OpFrontmatterSet, Target: "x", Key: "k", Value: "v"}}}
	errs := ValidatePlan(p, testRegistry())
	assert.Len(t, errs, 1)
	assert.Equal(t, "unsupported_version", errs[0].Code)
}

func TestValidatePlan_UnknownOp(t *testing.T) {
	p := Plan{Version: 1, Operations: []Operation{{Op: "bad_op"}}}
	errs := ValidatePlan(p, testRegistry())
	assert.Equal(t, "unknown_operation", errs[0].Code)
}

func TestValidatePlan_MissingField_Set(t *testing.T) {
	p := Plan{Version: 1, Operations: []Operation{{Op: OpFrontmatterSet, Target: "x"}}}
	errs := ValidatePlan(p, testRegistry())
	assert.Greater(t, len(errs), 0)
	assert.Equal(t, "missing_field", errs[0].Code)
}

func TestValidatePlan_MissingField_Unset(t *testing.T) {
	p := Plan{Version: 1, Operations: []Operation{{Op: OpFrontmatterUnset, Target: "x"}}}
	assert.Equal(t, "missing_field", ValidatePlan(p, testRegistry())[0].Code)
}

func TestValidatePlan_MissingField_Merge(t *testing.T) {
	p := Plan{Version: 1, Operations: []Operation{{Op: OpFrontmatterMerge, Target: "x"}}}
	assert.Equal(t, "missing_field", ValidatePlan(p, testRegistry())[0].Code)
}

func TestValidatePlan_MissingField_Render(t *testing.T) {
	p := Plan{Version: 1, Operations: []Operation{{Op: OpGeneratedRegion, Target: "x"}}}
	assert.Greater(t, len(ValidatePlan(p, testRegistry())), 0)
}

func TestValidatePlan_MissingField_NoteCreate(t *testing.T) {
	p := Plan{Version: 1, Operations: []Operation{{Op: OpNoteCreate, Path: "t.md"}}}
	assert.Greater(t, len(ValidatePlan(p, testRegistry())), 0)
}

func TestValidatePlan_NoteCreate_UnknownType(t *testing.T) {
	p := Plan{Version: 1, Operations: []Operation{
		{Op: OpNoteCreate, Path: "t.md", Type: "nonexistent", Frontmatter: map[string]interface{}{"title": "T"}},
	}}
	errs := ValidatePlan(p, testRegistry())
	assert.Len(t, errs, 1)
	assert.Equal(t, "unknown_type", errs[0].Code)
}

func TestValidatePlan_MultipleErrors(t *testing.T) {
	p := Plan{Version: 1, Operations: []Operation{{Op: "bad"}, {Op: OpFrontmatterSet, Target: "x"}}}
	assert.GreaterOrEqual(t, len(ValidatePlan(p, testRegistry())), 2)
}
