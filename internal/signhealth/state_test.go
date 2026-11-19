package signhealth

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/xdg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func isolateState(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	xdg.SetAppName("vaultmind")
}

func TestParseSender_LenientAndGuarded(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
		ok   bool
	}{
		{name: "unknown fields", raw: `{"from_agent":"agent:cm","extra":true}`, want: "agent:cm", ok: true},
		{name: "malformed", raw: `{`, ok: false},
		{name: "missing", raw: `{"body":"hello"}`, ok: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseSender([]byte(tc.raw))
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestRecord_WritesAtomicStateAndOverwrites(t *testing.T) {
	isolateState(t)
	require.NoError(t, Record("agent:cm", "v1", errors.New(strings.Repeat("x", 501))))

	path, err := xdg.StateFile(filepath.Join("sign-health", "agent_cm.json"))
	require.NoError(t, err)
	first, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(first), `"ok":false`)
	assert.Len(t, recordError(first), 500)

	require.NoError(t, Record("agent:cm", "v2", nil))
	second, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(second), `"ok":true`)
	assert.NotContains(t, string(second), `"error"`)
	assert.Contains(t, string(second), `"version":"v2"`)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	dirInfo, err := os.Stat(filepath.Dir(path))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), dirInfo.Mode().Perm())
}

func TestSanitizeAgent_GuardsPathComponents(t *testing.T) {
	assert.Equal(t, "agent_a_.._b", SanitizeAgent("agent:a/../b"))
	assert.Equal(t, "_", SanitizeAgent("."))
	assert.Equal(t, "_", SanitizeAgent(".."))
}

func recordError(raw []byte) string {
	var record State
	if err := json.Unmarshal(raw, &record); err != nil {
		return ""
	}
	return record.Error
}

func TestRead_SortsAndMarksStale(t *testing.T) {
	isolateState(t)
	path, err := xdg.StateFile(filepath.Join("sign-health", "agent_cm.json"))
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(`{"agent":"agent:cm","ok":false,"ts":"2020-01-01T00:00:00Z","error":"bad","version":"v1"}`), 0o600))
	other, err := xdg.StateFile(filepath.Join("sign-health", "agent_mira.json"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(other, []byte(`{"agent":"agent:mira","ok":true,"ts":"2026-11-19T00:00:00Z","version":"v1"}`), 0o600))

	entries, err := Read(time.Date(2026, 11, 19, 1, 0, 0, 0, time.UTC), time.Hour)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, "agent:cm", entries[0].Record.Agent)
	assert.True(t, entries[0].Stale)
	assert.Equal(t, "agent:mira", entries[1].Record.Agent)
	assert.False(t, entries[1].Stale)
}

func stateDir(t *testing.T) string {
	t.Helper()
	dir, err := xdg.StateFile("sign-health")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(dir, 0o700))
	return dir
}

func TestRecord_EmptyAgentWritesNothing(t *testing.T) {
	isolateState(t)
	require.NoError(t, Record("", "v1", errors.New("x")))
	entries, err := Read(time.Now(), 0)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestRecord_TruncatesLongErrorsTo500Bytes(t *testing.T) {
	isolateState(t)
	require.NoError(t, Record("agent:cm", "v1", errors.New(strings.Repeat("e", 900))))
	entries, err := Read(time.Now(), 0)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Len(t, entries[0].Record.Error, 500)
}

func TestRecord_ReportsAnUnwritableStateDir(t *testing.T) {
	isolateState(t)
	dir := stateDir(t)
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	assert.Error(t, Record("agent:cm", "v1", nil))
}

func TestRead_SkipsDirsAndNonJSONAndFlagsBadRecords(t *testing.T) {
	isolateState(t)
	dir := stateDir(t)
	require.NoError(t, os.Mkdir(filepath.Join(dir, "sub.json"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.json"), []byte(`{"ts":"2026-11-19T15:32:47Z"}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.json"), []byte(`{"agent":"agent:b","ok":true,"ts":"yesterday"}`), 0o600))
	require.NoError(t, Record("agent:ok", "v1", nil))

	entries, err := Read(time.Now(), 0)
	require.NoError(t, err)
	require.Len(t, entries, 3)
	assert.Equal(t, "agent:ok", entries[0].Record.Agent, "readable entries sort before unreadable ones")
	assert.True(t, entries[1].Unreadable)
	assert.Contains(t, entries[1].Err, "missing agent or timestamp")
	assert.True(t, entries[2].Unreadable)
	assert.Contains(t, entries[2].Err, "parsing time")
}

func TestRead_UnreadableFileIsAnEntryNotAnError(t *testing.T) {
	isolateState(t)
	dir := stateDir(t)
	path := filepath.Join(dir, "agent_cm.json")
	require.NoError(t, os.WriteFile(path, []byte(`{}`), 0o000))
	entries, err := Read(time.Now(), 0)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.True(t, entries[0].Unreadable)
	assert.Equal(t, path, entries[0].File)
}

func TestRead_UnlistableDirIsAnError(t *testing.T) {
	isolateState(t)
	dir := stateDir(t)
	require.NoError(t, os.Chmod(dir, 0o000))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	_, err := Read(time.Now(), 0)
	assert.Error(t, err)
}
