package episode_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/episode"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadCursor_DefaultsToZeroWhenAbsent(t *testing.T) {
	line, err := episode.ReadCursor(t.TempDir(), "session-abc")
	require.NoError(t, err)
	assert.Equal(t, 0, line)
}

func TestWriteCursor_ThenReadCursor_RoundTrips(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, episode.WriteCursor(dir, "session-abc", 42))

	line, err := episode.ReadCursor(dir, "session-abc")
	require.NoError(t, err)
	assert.Equal(t, 42, line)
}

func TestWriteCursor_CreatesCursorDirIfMissing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "cursors")

	require.NoError(t, episode.WriteCursor(dir, "session-abc", 7))

	line, err := episode.ReadCursor(dir, "session-abc")
	require.NoError(t, err)
	assert.Equal(t, 7, line)
}

func TestReadCursor_DifferentSessionsAreIndependent(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, episode.WriteCursor(dir, "session-a", 10))
	require.NoError(t, episode.WriteCursor(dir, "session-b", 20))

	a, err := episode.ReadCursor(dir, "session-a")
	require.NoError(t, err)
	b, err := episode.ReadCursor(dir, "session-b")
	require.NoError(t, err)

	assert.Equal(t, 10, a)
	assert.Equal(t, 20, b)
}

func TestReadCursor_ErrorsOnCorruptCursorFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "session-abc.cursor"), []byte("not-a-number"), 0o600))

	_, err := episode.ReadCursor(dir, "session-abc")
	require.Error(t, err, "a corrupt cursor must fail loud, never silently restart from zero and re-blob the transcript")
}

func TestWriteCursor_ErrorsWhenCursorDirCannotBeCreated(t *testing.T) {
	// A regular file sits where a directory is needed, so MkdirAll must fail.
	blocker := filepath.Join(t.TempDir(), "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))

	err := episode.WriteCursor(filepath.Join(blocker, "cursors"), "session-abc", 1)
	require.Error(t, err)
}

func TestWriteCursor_ErrorsWhenDestinationCannotBeWritten(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o500)) // read+execute only, no write
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	err := episode.WriteCursor(dir, "session-abc", 1)
	require.Error(t, err, "a cursor dir that exists but can't be written to must error, not silently no-op")
}
