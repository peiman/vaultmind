package cmdutil

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// classifyVaultError encodes the string-matching contract that downstream
// JSON callers read via the error code. Each branch must stay distinct so
// scripts branching on code don't misclassify.
func TestClassifyVaultError_BranchesMap(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"vault_not_found", errors.New(`vault path "/x" does not exist or is not a directory`), "vault_not_found"},
		{"config_error", errors.New("loading config: invalid yaml"), "config_error"},
		{"database_locked_keyword", errors.New("database is locked"), "database_locked"},
		{"database_locked_sqlite_busy", errors.New("SQLITE_BUSY: another process"), "database_locked"},
		{"fallback", errors.New("some unrelated failure"), "vault_error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, classifyVaultError(tc.err))
		})
	}
}
