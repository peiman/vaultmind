package hookscripts_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Drive the shipped recall script and real CLI, rather than canned ask output.
// Recreate schema 008 so the hook's ask migrates it without re-indexing.
func TestRecallHook_MigratedIndexStillDeliversBodies(t *testing.T) {
	// Compiled once per process, before the hook. The ask bound below must
	// not include this build: on a slow runner the build alone is most of
	// the test.
	binary := pinnedTestBinary(t)
	h := newHookEnv(t, `exec "$VM_TEST_BINARY" "$@"`)
	root := filepath.Join(h.projectDir, "vaultmind-identity")
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".vaultmind"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".vaultmind", "config.yaml"), []byte("types:\n  concept:\n    required: [title]\n"), 0o600))
	body := "Spreading activation delivers legacy recall bodies."
	require.NoError(t, os.WriteFile(filepath.Join(root, "legacy.md"), []byte("---\nid: legacy\ntitle: Spreading activation\ntype: concept\n---\n"+body+"\n"), 0o600))
	cfg, err := vault.LoadConfig(root)
	require.NoError(t, err)
	dbPath := filepath.Join(root, cfg.Index.DBPath)
	_, err = index.NewIndexer(root, dbPath, cfg).Incremental()
	require.NoError(t, err)
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	_, err = db.Exec("ALTER TABLE notes DROP COLUMN body_raw")
	require.NoError(t, err)
	_, err = db.Exec("DELETE FROM goose_db_version WHERE version_id>=9")
	require.NoError(t, err)
	require.NoError(t, db.Close())
	// VAULTMIND_HOOK_QUERY_TIMEOUT. The production default stays 25s. This
	// test passes in about 10s on an idle machine; 120s is more than ten
	// times that. On a slow runner the 25s bound killed ask and the body
	// never arrived. Where `timeout` is absent the ask is unbounded, as before.
	env := append(h.env(false),
		"VM_TEST_BINARY="+binary,
		"VAULTMIND_NO_SERVE=1",
		"VAULTMIND_HOOK_QUERY_TIMEOUT=120",
		"XDG_DATA_HOME="+t.TempDir(),
		"XDG_STATE_HOME="+t.TempDir(),
	)
	out, _ := runHookScript(t, "vault-recall.sh", env, `{"prompt":"how does spreading activation deliver legacy recall bodies?","session_id":"legacy-upgrade"}`)
	assert.Contains(t, out, "    "+body)
	assert.NotContains(t, out, "no room")
	db, err = index.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	var missing int
	require.NoError(t, db.QueryRow("SELECT count(*) FROM notes WHERE body_raw IS NULL").Scan(&missing))
	assert.Equal(t, 1, missing, "ask must deliver fallback text without mutating the index")
}
