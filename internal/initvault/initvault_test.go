package initvault_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/initvault"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Init scaffolds a fresh vault — the directory it creates must contain
// the .vaultmind/config.yaml + the persona-shaped starter notes. The
// caller-facing contract is "after init you have a working vault that
// vaultmind index can read."
func TestInit_CreatesExpectedFiles(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "my-vault")

	res, err := initvault.Init(dst)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, dst, res.VaultPath)
	assert.Greater(t, res.FilesAdded, 4, "expect at least config + README + 2 starter notes")

	// The skeleton must be there.
	for _, want := range []string{
		".vaultmind/config.yaml",
		"README.md",
		"identity/who-am-i.md",
		"references/current-context.md",
	} {
		_, err := os.Stat(filepath.Join(dst, want))
		assert.NoError(t, err, "expected file %q to exist after init", want)
	}
}

// vault.LoadConfig must accept the scaffolded config — if it doesn't,
// the user runs vaultmind index and gets a parse error on a vault we
// just gave them. That's the most embarrassing possible failure mode.
func TestInit_ConfigIsValid(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "my-vault")
	_, err := initvault.Init(dst)
	require.NoError(t, err)

	cfg, err := vault.LoadConfig(dst)
	require.NoError(t, err, "scaffolded config must parse cleanly")
	require.NotNil(t, cfg)
	assert.NotEmpty(t, cfg.Types, "scaffolded config must declare types")
	assert.Contains(t, cfg.Types, "identity", "identity type must be in default scaffold")
	assert.Contains(t, cfg.Types, "arc", "arc type must be in default scaffold (arcs are vaultmind's atomic unit of persona)")
}

// Each starter note must carry valid frontmatter with today's date
// injected — so the user's fresh vault doesn't have notes pinned to
// whatever date the templates were authored. The injected date drives
// the index's created/updated tracking.
func TestInit_FrontmatterDatesAreToday(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "my-vault")
	_, err := initvault.Init(dst)
	require.NoError(t, err)

	body, err := os.ReadFile(filepath.Join(dst, "identity", "who-am-i.md"))
	require.NoError(t, err)

	// Frontmatter must exist and contain a created: line — the date
	// itself is checked loosely (don't pin to UTC clock skew in CI),
	// just that something looks like an ISO date.
	s := string(body)
	require.True(t, strings.HasPrefix(s, "---\n"), "starter note must start with frontmatter")
	assert.Contains(t, s, "\ncreated: 20", "frontmatter must include a created: 20YY-MM-DD line")
	// vm_updated is YAML-quoted because it contains colons (RFC3339).
	// The bare-prefix shape `vm_updated: "20...T...Z"` confirms the
	// canonical schema.VMUpdatedFormat is in use, distinguishing it
	// from a stale date-only `vm_updated: 2026-05-04` write.
	assert.Contains(t, s, `vm_updated: "20`, "frontmatter must include a quoted RFC3339 vm_updated line")
	assert.Contains(t, s, `T`, "vm_updated must include the RFC3339 'T' time separator")
	assert.Contains(t, s, `Z"`, "vm_updated must end with the UTC 'Z' indicator")
}

// Init refuses to overwrite an existing path. Vaults are stateful —
// silently rewriting someone's existing notes/embeddings/git history
// would be catastrophically destructive.
func TestInit_RefusesExistingPath(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "my-vault")
	require.NoError(t, os.MkdirAll(dst, 0o755))

	res, err := initvault.Init(dst)
	require.Error(t, err)
	assert.Nil(t, res)
	assert.Contains(t, err.Error(), "refuse to overwrite")
}

// README and config files (no leading frontmatter) pass through the
// template renderer unchanged — they don't get a created:/vm_updated:
// injection, since they aren't notes.
func TestInit_NonNoteFilesAreNotMutated(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "my-vault")
	_, err := initvault.Init(dst)
	require.NoError(t, err)

	readme, err := os.ReadFile(filepath.Join(dst, "README.md"))
	require.NoError(t, err)
	assert.NotContains(t, string(readme), "vm_updated:", "README must not get note-style frontmatter injection")

	cfg, err := os.ReadFile(filepath.Join(dst, ".vaultmind", "config.yaml"))
	require.NoError(t, err)
	assert.NotContains(t, string(cfg), "vm_updated:", "config.yaml must not get note-style frontmatter injection")
}
