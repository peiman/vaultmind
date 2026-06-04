package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOperationType_String(t *testing.T) {
	tests := []struct {
		op   OperationType
		want string
	}{
		{OpRead, "read"},
		{OpDryRun, "dry_run"},
		{OpWrite, "write"},
		{OpWriteCommit, "write_commit"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.op.String())
	}
}

func TestPolicyDecision_String(t *testing.T) {
	tests := []struct {
		d    PolicyDecision
		want string
	}{
		{Allow, "allow"},
		{Warn, "warn"},
		{Refuse, "refuse"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.d.String())
	}
}

func TestParsePolicyDecision(t *testing.T) {
	tests := []struct {
		input string
		want  PolicyDecision
		err   bool
	}{
		{"allow", Allow, false},
		{"warn", Warn, false},
		{"refuse", Refuse, false},
		{"ALLOW", Allow, false},
		{"block", 0, true},
		{"", 0, true},
	}
	for _, tt := range tests {
		got, err := ParsePolicyDecision(tt.input)
		if tt.err {
			assert.Error(t, err, "input: %q", tt.input)
		} else {
			assert.NoError(t, err, "input: %q", tt.input)
			assert.Equal(t, tt.want, got, "input: %q", tt.input)
		}
	}
}
