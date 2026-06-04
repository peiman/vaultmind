package experiment_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openSessionDB(t *testing.T) *experiment.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "exp.db")
	db, err := experiment.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// Session.SetVaultPath persists the vault_path update and updates the local
// field. Regression: if the update call were dropped, experiment outcomes
// would be attributed to the wrong vault.
func TestSession_SetVaultPathUpdatesFieldAndPersists(t *testing.T) {
	db := openSessionDB(t)
	sid, err := db.StartSession("")
	require.NoError(t, err)
	s := &experiment.Session{DB: db, ID: sid, OutcomeWindow: 2}

	s.SetVaultPath("/vault/alpha")
	assert.Equal(t, "/vault/alpha", s.VaultPath)

	// The DB row should reflect the update too.
	rows, err := db.Query("SELECT vault_path FROM sessions WHERE session_id = ?", sid)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	require.True(t, rows.Next(), "session row must exist")
	var got string
	require.NoError(t, rows.Scan(&got))
	assert.Equal(t, "/vault/alpha", got)
}

// WithSession / FromContext round-trip a Session through a context. Regression
// against silent nil-returns if the key type changes.
func TestWithSession_FromContextRoundTrip(t *testing.T) {
	db := openSessionDB(t)
	sid, _ := db.StartSession("")
	s := &experiment.Session{DB: db, ID: sid, OutcomeWindow: 2}

	ctx := experiment.WithSession(context.Background(), s)
	got := experiment.FromContext(ctx)
	require.NotNil(t, got)
	assert.Equal(t, s.ID, got.ID)
}

// FromContext on a bare context returns nil so callers can safely no-op
// the experiment pipeline when it's disabled.
func TestFromContext_NoSessionReturnsNil(t *testing.T) {
	assert.Nil(t, experiment.FromContext(context.Background()))
}

// DB.Begin returns a *sql.Tx that can commit — proves the wrapper isn't
// silently nil-returning.
func TestDB_BeginCommitsTransaction(t *testing.T) {
	db := openSessionDB(t)
	tx, err := db.Begin()
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
}
