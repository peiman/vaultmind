package mutation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A value must fit its field's shape. Readers take text fields as strings and
// list fields as a string or a list of strings, dropping anything else — so a
// list written to title, or a number to status, is silently lost on read.
func TestValidateMutation_ValueMustFitTheFieldShape(t *testing.T) {
	reg := testRegistry()
	note := ParsedNoteInfo{ID: "proj-1", Type: "project", IsDomain: true, Keys: []string{"id", "type", "status", "title"}}

	cases := []struct {
		name  string
		key   string
		value interface{}
		ok    bool
	}{
		{"text into title", "title", "Plan", true},
		{"list into title", "title", []interface{}{"a", "b"}, false},
		{"number into title", "title", 2024, false},
		{"number into status", "status", 3, false},
		{"YAML null (~) into title", "title", nil, false},
		{"one tag as text", "tags", "retrieval", true},
		{"tags as a list", "tags", []interface{}{"a", "b"}, true},
		{"a number inside tags", "tags", []interface{}{"a", 3}, false},
		{"a map into tags", "tags", map[string]interface{}{"a": 1}, false},
		{"created as text", "created", "2026-09-26", true},
		{"created as a YAML date", "created", time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC), true},
		{"created as a number", "created", 20260926, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateMutation(MutationRequest{Op: OpSet, Key: c.key, Value: c.value}, note, reg)
			if c.ok {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			me := err.(*MutationError)
			assert.Equal(t, "invalid_type", me.Code)
			assert.Equal(t, c.key, me.Field)
		})
	}
}

func TestValidateMutation_ExtraFieldsTakeAnyValue(t *testing.T) {
	reg := testRegistry()
	note := ParsedNoteInfo{ID: "proj-1", Type: "project", IsDomain: true, Keys: []string{"id", "type"}}
	req := MutationRequest{Op: OpSet, Key: "custom", Value: map[string]interface{}{"any": 1}, AllowExtra: true}
	assert.NoError(t, ValidateMutation(req, note, reg), "a field the schema does not know has no shape to check")
}

func TestValidateMutation_MergeChecksEveryValue(t *testing.T) {
	reg := testRegistry()
	note := ParsedNoteInfo{ID: "proj-1", Type: "project", IsDomain: true, Keys: []string{"id", "type", "status", "title"}}
	req := MutationRequest{Op: OpMerge, Fields: map[string]interface{}{"tags": []interface{}{"a"}, "title": 7}}
	err := ValidateMutation(req, note, reg)
	require.Error(t, err)
	assert.Equal(t, "invalid_type", err.(*MutationError).Code)
}
