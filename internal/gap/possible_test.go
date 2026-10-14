package gap

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/noisefloor"
	"github.com/peiman/vaultmind/internal/xdg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	xdg.SetAppName("vaultmind")
	// Open measures the hint log, and that resolves the state directory.
	// Without this, a test panics on the real home directory.
	dir, err := os.MkdirTemp("", "vm-gap-state-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "gap tests: temp state dir:", err)
		os.Exit(1)
	}
	if err := os.Setenv("XDG_STATE_HOME", dir); err != nil {
		fmt.Fprintln(os.Stderr, "gap tests: set XDG_STATE_HOME:", err)
		os.Exit(1)
	}
	if err := os.Setenv("XDG_DATA_HOME", dir); err != nil {
		fmt.Fprintln(os.Stderr, "gap tests: set XDG_DATA_HOME:", err)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func isolateGapState(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
}

func TestHashQuery_DependsOnTheInstallSalt(t *testing.T) {
	const query = "same question"
	dirA := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dirA)
	sumA, err := hashQuery(query)
	require.NoError(t, err)
	saltPath := filepath.Join(dirA, "vaultmind", saltFile)
	info, err := os.Stat(saltPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	salt, err := os.ReadFile(saltPath)
	require.NoError(t, err)
	require.Len(t, salt, saltLen)
	again, err := hashQuery(query)
	require.NoError(t, err)
	assert.Equal(t, sumA, again)

	dirB := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dirB)
	sumB, err := hashQuery(query)
	require.NoError(t, err)
	assert.NotEqual(t, sumA, sumB, "the same query must hash differently under two salts")

	planted := bytesRepeat(saltLen, 7)
	plantedPath := filepath.Join(dirB, "vaultmind", saltFile)
	require.NoError(t, os.WriteFile(plantedPath, planted, 0o644))
	got, err := hashQuery(query)
	require.NoError(t, err)
	h := sha256.New()
	_, _ = h.Write(planted)
	_, _ = h.Write([]byte(query))
	assert.Equal(t, hex.EncodeToString(h.Sum(nil)), got)
	info, err = os.Stat(plantedPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestHashQuery_RejectsAShortSalt(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	root := filepath.Join(dir, "vaultmind")
	require.NoError(t, os.MkdirAll(root, 0o700))
	short := []byte("abcd")
	saltPath := filepath.Join(root, saltFile)
	require.NoError(t, os.WriteFile(saltPath, short, 0o600))
	_, err := hashQuery("q")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "4 bytes")
	got, err := os.ReadFile(saltPath)
	require.NoError(t, err)
	assert.Equal(t, short, got)
}

func TestRecordPossible_ErrorsWhenTheSaltIsADirectory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "vaultmind", saltFile), 0o700))
	err := RecordPossible(t.TempDir(), "q", noisefloor.ConfidenceWeak, false, DefaultLogLimit, time.Now())
	require.Error(t, err)
}

func TestRecordPossible_LogStaysOutsideTheVault(t *testing.T) {
	isolateGapState(t)
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o755))
	legacy := filepath.Join(dir, ".vaultmind", "possible-gaps.jsonl")
	const legacyBody = "{\"at\":\"2026-10-01T00:00:00Z\",\"band\":\"weak\"}\n"
	require.NoError(t, os.WriteFile(legacy, []byte(legacyBody), 0o644))

	require.NoError(t, RecordPossible(dir, "secret", noisefloor.ConfidenceWeak, false, DefaultLogLimit, time.Now()))
	path, err := possibleLogPath(dir)
	require.NoError(t, err)
	rel, err := filepath.Rel(dir, path)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(rel, ".."), "log path %s", path)
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	dirInfo, err := os.Stat(filepath.Dir(path))
	require.NoError(t, err)
	assert.True(t, dirInfo.IsDir())
	assert.Equal(t, os.FileMode(0o700), dirInfo.Mode().Perm())

	got, err := os.ReadFile(legacy)
	require.NoError(t, err)
	assert.Equal(t, legacyBody, string(got))
	n, _, err := readPossible(dir)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "the old vault file is not part of the meter")
}

func TestRecordPossible_OneSaltForEveryVault(t *testing.T) {
	isolateGapState(t)
	a, b := t.TempDir(), t.TempDir()
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	const query = "same question"
	require.NoError(t, RecordPossible(a, query, noisefloor.ConfidenceWeak, false, DefaultLogLimit, now))
	require.NoError(t, RecordPossible(b, query, noisefloor.ConfidenceWeak, false, DefaultLogLimit, now))
	pa, err := possibleLogPath(a)
	require.NoError(t, err)
	pb, err := possibleLogPath(b)
	require.NoError(t, err)
	assert.NotEqual(t, pa, pb)
	var recA, recB possibleRecord
	require.NoError(t, json.Unmarshal([]byte(readLines(t, pa)[0]), &recA))
	require.NoError(t, json.Unmarshal([]byte(readLines(t, pb)[0]), &recB))
	assert.Equal(t, recA.QuerySHA256, recB.QuerySHA256)
}

func TestRecordPossible_CompactsPastTwiceTheLimit(t *testing.T) {
	isolateGapState(t)
	dir := t.TempDir()
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 4; i++ {
		require.NoError(t, RecordPossible(dir, fmt.Sprintf("q%d", i), noisefloor.ConfidenceWeak, true, 2, now.Add(time.Duration(i)*time.Second)))
	}
	path, err := possibleLogPath(dir)
	require.NoError(t, err)
	assert.Len(t, readLines(t, path), 4)

	require.NoError(t, RecordPossible(dir, "q4", noisefloor.ConfidenceWeak, true, 2, now.Add(4*time.Second)))
	lines := readLines(t, path)
	require.Len(t, lines, 2)
	assert.Contains(t, lines[0], "q3")
	assert.Contains(t, lines[1], "q4")

	other := t.TempDir()
	for i := 0; i < 5; i++ {
		require.NoError(t, RecordPossible(other, "keep", noisefloor.ConfidenceNoMatch, false, 0, now))
	}
	otherPath, err := possibleLogPath(other)
	require.NoError(t, err)
	assert.Len(t, readLines(t, otherPath), 5, "a non-positive limit keeps the default window")
}

func TestCompactPossible_SlidesAFileThatIsAlreadyOverTheCeiling(t *testing.T) {
	isolateGapState(t)
	dir := t.TempDir()
	path, err := possibleLogPath(dir)
	require.NoError(t, err)
	var b strings.Builder
	for _, q := range []string{"a", "b", "c", "d", "e", "f"} {
		if q == "d" {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "{\"query\":%q}\n", q)
	}
	require.NoError(t, os.WriteFile(path, []byte(b.String()), 0o600))
	require.NoError(t, compactPossible(path, 2))
	lines := readLines(t, path)
	require.Len(t, lines, 2)
	assert.Contains(t, lines[0], `"e"`)
	assert.Contains(t, lines[1], `"f"`)
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func bytesRepeat(n int, b byte) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}
