package vault_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"), []byte(body), 0o644))
	return dir
}

// db_path and template are joined onto the vault root and then OPENED — the db
// path with parent directories created for it. ResolveInside catches `..`
// downstream, but by then the message is about a path the operator never wrote.
// Refuse at load, naming the config key, so the answer is actionable.
func TestLoadConfig_RejectsEscapingDBPath(t *testing.T) {
	dir := writeConfig(t, "index:\n  db_path: ../../evil.db\n")
	_, err := vault.LoadConfig(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db_path", "the error must name the key the operator has to fix")
}

// An absolute db_path does NOT escape — filepath.Join re-roots it under the
// vault (confine_test.go pins that). It is refused anyway, because it silently
// becomes a different path than the one written: `/tmp/index.db` in config
// means `<vault>/tmp/index.db` on disk, and nothing says so. A check that only
// asks "did it escape" would call this fine.
func TestLoadConfig_RejectsAbsoluteDBPath(t *testing.T) {
	dir := writeConfig(t, "index:\n  db_path: /tmp/vaultmind-absolute.db\n")
	_, err := vault.LoadConfig(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db_path")
	assert.Contains(t, err.Error(), "relative", "say what a valid value looks like")
}

func TestLoadConfig_RejectsEscapingTypeTemplate(t *testing.T) {
	dir := writeConfig(t, "types:\n  concept:\n    template: ../../../etc/passwd\n")
	_, err := vault.LoadConfig(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "template")
	assert.Contains(t, err.Error(), "concept", "name the type whose template is wrong")
}

func TestLoadConfig_AcceptsOrdinaryRelativePaths(t *testing.T) {
	dir := writeConfig(t, "index:\n  db_path: .vaultmind/index.db\ntypes:\n  concept:\n    template: templates/concept.md\n")
	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	assert.Equal(t, ".vaultmind/index.db", cfg.Index.DBPath)
	assert.Equal(t, "templates/concept.md", cfg.Types["concept"].Template)
}

// A vault with no config at all still gets working defaults — the guard must
// not turn "no config" into an error.
func TestLoadConfig_MissingConfigStillDefaults(t *testing.T) {
	cfg, err := vault.LoadConfig(t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, ".vaultmind/index.db", cfg.Index.DBPath)
}
