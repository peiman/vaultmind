package query_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShownLedger_RemembersWithinTheWindowPerConversation(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	a := query.OpenShownLedger(dir, "conversation-a")

	require.NoError(t, a.Record([]string{"n1", "n2"}, now.Add(-20*time.Minute)))
	require.NoError(t, a.Record([]string{"n3"}, now.Add(-2*time.Minute)))

	assert.Equal(t, map[string]bool{"n3": true}, a.Recent(10*time.Minute, now), "only what was shown inside the window")
	assert.Len(t, a.Recent(time.Hour, now), 3)
	assert.Empty(t, query.OpenShownLedger(dir, "conversation-b").Recent(time.Hour, now),
		"another conversation — a subagent gets a fresh id — never inherits the ledger")
}

func TestShownLedger_WithoutAConversationRemembersNothing(t *testing.T) {
	dir := t.TempDir()
	l := query.OpenShownLedger(dir, "")
	require.NoError(t, l.Record([]string{"n1"}, time.Now()))
	assert.Empty(t, l.Recent(time.Hour, time.Now()))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "nothing is written without a conversation to key it on")
}

func TestShownLedger_AHostileIDStaysInItsDirectory(t *testing.T) {
	dir := t.TempDir()
	l := query.OpenShownLedger(dir, "../../etc/passwd")
	require.NoError(t, l.Record([]string{"n1"}, time.Now()))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.NotContains(t, entries[0].Name(), "/")
	_, err = os.Stat(filepath.Join(dir, "..", "..", "etc", "passwd"))
	assert.True(t, os.IsNotExist(err) || err == nil)
}

func TestShownLedger_IgnoresLinesItCannotRead(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	l := query.OpenShownLedger(dir, "c")
	require.NoError(t, l.Record([]string{"good"}, now))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	path := filepath.Join(dir, entries[0].Name())
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // test path
	require.NoError(t, err)
	_, err = f.WriteString("garbage\nnotanumber\tx\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	assert.Equal(t, map[string]bool{"good": true}, l.Recent(time.Hour, now))
}
