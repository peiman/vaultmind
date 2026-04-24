package episode_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/episode"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCapture_WritesMarkdownToOutputDir(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "episodes")

	outPath, err := episode.Capture(fixturePath, outDir)
	require.NoError(t, err)

	// File exists and lives under outDir with the expected id.
	assert.True(t, strings.HasPrefix(outPath, outDir))
	assert.True(t, strings.HasSuffix(outPath, "episode-2026-04-24-test-ses.md"))

	content, err := os.ReadFile(outPath) // #nosec G304 -- test-controlled path.
	require.NoError(t, err)
	body := string(content)
	assert.Contains(t, body, "type: episode")
	assert.Contains(t, body, "run the tests please")
}

func TestCapture_IsIdempotent(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "episodes")

	first, err := episode.Capture(fixturePath, outDir)
	require.NoError(t, err)
	second, err := episode.Capture(fixturePath, outDir)
	require.NoError(t, err)

	// Same transcript → same path → overwrite, not duplicate.
	assert.Equal(t, first, second)
	entries, err := os.ReadDir(outDir)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

func TestCapture_ErrorsOnBadTranscript(t *testing.T) {
	_, err := episode.Capture("/no/such/transcript.jsonl", t.TempDir())
	require.Error(t, err)
}
