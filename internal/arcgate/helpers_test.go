package arcgate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArcGateHelpers_HandleShortAndMalformedInputs(t *testing.T) {
	_, err := readHead(filepath.Join(t.TempDir(), "missing.md"))
	assert.Error(t, err)
	assert.Nil(t, cheapFrontmatter([]byte("---\nid: unfinished\n")))
	fm := cheapFrontmatter([]byte("---\nid: arc-x\ntype: arc\ntitle: X\n---\n"))
	assert.Equal(t, "arc-x", fm["id"])
	assert.Nil(t, cheapFrontmatter([]byte("plain note")))

	assert.Equal(t, "", sanitizeSession(""))
	assert.Equal(t, "a_b", sanitizeSession("a/b"))
	assert.Len(t, sanitizeSession(strings.Repeat("x", 120)), 100)
	assert.Equal(t, "", firstTwoUnits(" \n\n"))
	assert.True(t, isListBullet("1. choose this"))
	assert.False(t, isListBullet("1.5 is a number"))
}

func TestArcGateHelpers_TranscriptAndToolInputFailures(t *testing.T) {
	text, err := hookText(eventPreToolUse, hookPayload{})
	require.NoError(t, err)
	assert.Empty(t, text)
	text, err = hookText(eventStop, hookPayload{})
	require.NoError(t, err)
	assert.Empty(t, text)
	_, err = lastAssistantMessage(filepath.Join(t.TempDir(), "missing.jsonl"))
	assert.Error(t, err)
	_, err = compactJSON([]byte("{"))
	assert.Error(t, err)
	assert.Equal(t, "plain words", trimToken("\"plain words\""))
}

func TestArcGateHelpers_ReadArcFallbacksAndSidecarBestEffort(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "arc.md")
	require.NoError(t, os.WriteFile(path, []byte("---\nid: arc-x\ntype: arc\nstatus: accepted\n---\nOpening prose is the fallback.\n"), 0o600))
	text, title, err := readArc(arcNote{ID: "arc-x", Path: path})
	require.NoError(t, err)
	assert.Equal(t, "arc-x", title)
	assert.Contains(t, text, "Opening prose")
	_, _, err = readArc(arcNote{ID: "arc-x", Path: filepath.Join(dir, "gone.md")})
	assert.Error(t, err)

	fileDir := filepath.Join(dir, "not-a-directory")
	require.NoError(t, os.WriteFile(fileDir, []byte("x"), 0o600))
	writeSidecar(fileDir, testNow(), "s", Trigger{Arc: "arc-x", On: eventStop, Match: "x"}, nil, "deny")
}

func testNow() (now time.Time) { return time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC) }
