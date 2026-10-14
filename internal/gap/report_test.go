package gap

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/noisefloor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMeasure_MeterAndOpenList(t *testing.T) {
	isolateGapState(t)
	v := openVault(t, conceptTypes)
	indexNote(t, v, "gaps/two.md", gapFile("gap-two", statusClosed, "2026-10-01T00:00:00Z", "2026-10-03T00:00:00Z", "Two days"))
	indexNote(t, v, "gaps/four.md", gapFile("gap-four", statusClosed, "2026-10-01T00:00:00Z", "2026-10-05T00:00:00Z", "Four days"))
	indexNote(t, v, "gaps/undated.md", `---
id: gap-undated
type: gap
title: Undated
status: closed
question: Undated
closed_by: concept-x
---
`)
	indexNote(t, v, "gaps/open.md", gapFile("gap-open", statusOpen, "2026-10-10T00:00:00Z", "", "Still open"))
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	require.NoError(t, RecordPossible(v.Path, "secret", noisefloor.ConfidenceStrong, true, DefaultLogLimit, now))
	require.NoError(t, RecordPossible(v.Path, "secret", noisefloor.ConfidenceWeak, false, DefaultLogLimit, time.Date(2026, 10, 2, 1, 0, 0, 0, time.UTC)))
	require.NoError(t, RecordPossible(v.Path, "other", noisefloor.ConfidenceNoMatch, false, DefaultLogLimit, time.Date(2026, 10, 3, 1, 0, 0, 0, time.UTC)))

	got, err := Measure(v.DB, v.Path, now)
	require.NoError(t, err)
	assert.Equal(t, 4, got.Opened)
	assert.Equal(t, 3, got.Closed)
	require.NotNil(t, got.Median)
	assert.InDelta(t, 3, *got.Median, 0.0001)
	require.Len(t, got.Notes, 1)
	assert.Equal(t, "gap-open", got.Notes[0].ID)
	assert.Equal(t, "Still open", got.Notes[0].Question)
	assert.Equal(t, 4, got.Notes[0].AgeDays)
	assert.Equal(t, 2, got.Possible)
	assert.Equal(t, "2026-10-02", got.PossibleSince)
}

func TestMedianAndAge(t *testing.T) {
	assert.Nil(t, median(nil))
	one := median([]float64{2})
	require.NotNil(t, one)
	assert.Equal(t, 2.0, *one)
	even := median([]float64{4, 2})
	require.NotNil(t, even)
	assert.Equal(t, 3.0, *even)
	odd := median([]float64{9, 1, 2})
	require.NotNil(t, odd)
	assert.Equal(t, 2.0, *odd)

	now := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, 0, ageDays(time.Time{}, now))
	assert.Equal(t, 0, ageDays(now.Add(time.Hour), now))
	assert.Equal(t, 0, ageDays(now.Add(48*time.Hour), now))
	assert.Equal(t, 4, ageDays(now.Add(-4*24*time.Hour), now))
	assert.Empty(t, jsonString("123"))
	assert.True(t, parseTime(`"yesterday"`).IsZero())
}

func TestMeasure_NilDBAndUnreadableHintLog(t *testing.T) {
	got, err := Measure(nil, t.TempDir(), time.Now())
	require.NoError(t, err)
	assert.Empty(t, got.Notes)
	assert.Equal(t, 0, got.Possible)

	v := openVault(t, conceptTypes)
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(blocked, []byte("x"), 0o600))
	t.Setenv("XDG_STATE_HOME", blocked)
	got, err = Measure(v.DB, v.Path, time.Now())
	require.NoError(t, err)
	assert.Equal(t, 0, got.Possible)
}

func TestReadPossible_SkipsJunkAndRejectsAHugeLine(t *testing.T) {
	isolateGapState(t)
	dir := t.TempDir()
	path, err := possibleLogPath(dir)
	require.NoError(t, err)
	body := "\n{bad\n{\"at\":\"not-a-date\",\"band\":\"weak\"}\n{\"at\":\"2026-10-01T00:00:00Z\",\"band\":\"weak\"}\n"
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	n, since, err := readPossible(dir)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Equal(t, "2026-10-01", since)

	require.NoError(t, os.WriteFile(path, []byte("{}\n"), 0o600))
	n, since, err = readPossible(dir)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
	assert.Empty(t, since)

	require.NoError(t, os.WriteFile(path, bytes.Repeat([]byte("a"), 70*1024), 0o600))
	_, _, err = readPossible(dir)
	require.Error(t, err)

	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(blocked, []byte("x"), 0o600))
	t.Setenv("XDG_STATE_HOME", blocked)
	_, _, err = readPossible(dir)
	require.Error(t, err)
}

func TestRecordPossible_ReportsAnUnwritableDirectory(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(blocked, []byte("x"), 0o600))
	t.Setenv("XDG_STATE_HOME", blocked)
	err := RecordPossible(t.TempDir(), "q", noisefloor.ConfidenceWeak, false, DefaultLogLimit, time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "opening possible-gap salt")
}

func TestRecordPossible_BandsAndPrivacy(t *testing.T) {
	isolateGapState(t)
	dir := t.TempDir()
	now := time.Date(2026, 10, 1, 3, 4, 5, 0, time.UTC)
	const query = "secret question"
	for _, band := range []string{noisefloor.ConfidenceStrong, noisefloor.ConfidenceModerate, ""} {
		require.NoError(t, RecordPossible(dir, query, band, true, DefaultLogLimit, now))
	}
	logPath, err := possibleLogPath(dir)
	require.NoError(t, err)
	_, err = os.Stat(logPath)
	assert.ErrorIs(t, err, os.ErrNotExist)

	require.NoError(t, RecordPossible(dir, query, noisefloor.ConfidenceWeak, false, DefaultLogLimit, now))
	require.NoError(t, RecordPossible(dir, "café", noisefloor.ConfidenceNoMatch, false, DefaultLogLimit, now.Add(time.Hour)))
	lines := readLines(t, logPath)
	require.Len(t, lines, 2)

	var hidden map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &hidden))
	_, hasQuery := hidden["query"]
	assert.False(t, hasQuery)
	sum, err := hashQuery(query)
	require.NoError(t, err)
	assert.Equal(t, sum, hidden["query_sha256"])
	assert.Equal(t, float64(len(query)), hidden["query_len"])
	assert.Equal(t, noisefloor.ConfidenceWeak, hidden["band"])

	var multibyte map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[1]), &multibyte))
	assert.Equal(t, float64(len("café")), multibyte["query_len"])

	require.NoError(t, RecordPossible(dir, query, noisefloor.ConfidenceWeak, true, DefaultLogLimit, now))
	lines = readLines(t, logPath)
	require.Len(t, lines, 3)
	var shown map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[2]), &shown))
	assert.Equal(t, query, shown["query"])
}

func gapFile(id, status, opened, closed, question string) string {
	s := "---\nid: " + id + "\ntype: gap\ntitle: " + question + "\nstatus: " + status + "\nquestion: " + question + "\n"
	if opened != "" {
		s += "opened_at: \"" + opened + "\"\n"
	}
	if closed != "" {
		s += "closed_at: \"" + closed + "\"\n"
	}
	return s + "---\n"
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var lines []string
	for _, line := range bytesSplit(raw) {
		if len(line) > 0 {
			lines = append(lines, string(line))
		}
	}
	return lines
}

func bytesSplit(raw []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range raw {
		if b == '\n' {
			lines = append(lines, raw[start:i])
			start = i + 1
		}
	}
	if start < len(raw) {
		lines = append(lines, raw[start:])
	}
	return lines
}
