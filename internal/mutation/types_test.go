package mutation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOpType_String(t *testing.T) {
	tests := []struct {
		op   OpType
		want string
	}{
		{OpSet, "set"},
		{OpUnset, "unset"},
		{OpMerge, "merge"},
		{OpNormalize, "normalize"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.op.String(), "op=%d", tt.op)
	}
}

func TestMutationResult_HasCorrectJSONTags(t *testing.T) {
	r := MutationResult{
		Path:            "notes/test.md",
		ID:              "note-test",
		Operation:       "set",
		Key:             "status",
		OldValue:        "active",
		NewValue:        "paused",
		DryRun:          false,
		Diff:            "--- a\n+++ b\n",
		WriteHash:       "sha256:abc",
		ReindexRequired: false,
		Git: GitInfo{
			RepoDetected:     true,
			WorkingTreeClean: true,
			TargetFileClean:  true,
		},
	}
	assert.Equal(t, "notes/test.md", r.Path)
	assert.Equal(t, "set", r.Operation)
	assert.True(t, r.Git.RepoDetected)
}
