package plan

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlan_ParseJSON(t *testing.T) {
	raw := `{"version":1,"description":"Test","operations":[{"op":"frontmatter_set","target":"proj-1","key":"status","value":"paused"},{"op":"note_create","path":"decisions/test.md","type":"decision","frontmatter":{"title":"Test"},"body":"# Body"}]}`
	var p Plan
	err := json.Unmarshal([]byte(raw), &p)
	require.NoError(t, err)
	assert.Equal(t, 1, p.Version)
	assert.Len(t, p.Operations, 2)
	assert.Equal(t, "frontmatter_set", p.Operations[0].Op)
	assert.Equal(t, "note_create", p.Operations[1].Op)
}

func TestApplyResult_JSON(t *testing.T) {
	r := ApplyResult{
		PlanDescription: "Test", OperationsTotal: 2, OperationsCompleted: 1,
		Operations: []OpResult{
			{Op: "frontmatter_set", Target: "proj-1", Status: "ok", WriteHash: "sha256:abc"},
			{Op: "note_create", Path: "decisions/test.md", Status: "skipped"},
		},
	}
	data, err := json.Marshal(r)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"operations_completed":1`)
}

func TestOpError_JSON(t *testing.T) {
	e := OpError{Code: "unknown_key", Message: "not allowed"}
	data, _ := json.Marshal(e)
	assert.Contains(t, string(data), `"code":"unknown_key"`)
}
