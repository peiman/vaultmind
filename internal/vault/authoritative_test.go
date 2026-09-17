package vault_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func configWith(t *testing.T, body string) *vault.Config {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"), []byte(body), 0o600))
	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	return cfg
}

// An undeclared type stays citable. Every vault written before this field
// existed must keep working — the field restricts, it never silently tightens.
func TestIsAuthoritative_UndeclaredTypeIsCitable(t *testing.T) {
	cfg := configWith(t, "types:\n  arc:\n    required: [title]\n")
	assert.True(t, cfg.IsAuthoritative("arc"),
		"a type that says nothing must remain citable, or upgrading would break existing vaults")
}

// Declaring false is what marks raw material as non-evidence.
func TestIsAuthoritative_DeclaredFalseIsNotCitable(t *testing.T) {
	cfg := configWith(t, "types:\n  journal:\n    required: [title]\n    authoritative: false\n")
	assert.False(t, cfg.IsAuthoritative("journal"))
}

// Declaring true is allowed and explicit.
func TestIsAuthoritative_DeclaredTrueIsCitable(t *testing.T) {
	cfg := configWith(t, "types:\n  source:\n    required: [title]\n    authoritative: true\n")
	assert.True(t, cfg.IsAuthoritative("source"))
}

// An unknown type is not this check's business — it is reported elsewhere as
// unknown_type, and claiming it is non-citable would double-report one defect
// as two.
func TestIsAuthoritative_UnknownTypeIsNotThisChecksProblem(t *testing.T) {
	cfg := configWith(t, "types:\n  arc:\n    required: [title]\n")
	assert.True(t, cfg.IsAuthoritative("no-such-type"))
}
