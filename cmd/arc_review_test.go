package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/episode"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeReviewEpisode renders an episode with the REAL renderer (the path
// capture uses), so the review is tested on what capture actually writes.
func writeReviewEpisode(t *testing.T, vault, id, said string, age time.Duration) {
	t.Helper()
	ep := &episode.Episode{
		ID: id, SessionID: id, StartedAt: "2026-09-24T09:00:00.000Z", ToolCounts: map[string]int{},
		AssistantMessages: []episode.Message{{Timestamp: "2026-09-24T09:01:00.000Z", Text: "I answered from what the session start gave me."}},
		UserMessages:      []episode.Message{{Timestamp: "2026-09-24T09:02:00.000Z", Text: said}},
	}
	dir := filepath.Join(vault, "episodes")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	p := filepath.Join(dir, id+".md")
	require.NoError(t, os.WriteFile(p, []byte(episode.RenderMarkdown(ep)), 0o600))
	ts := time.Now().Add(-age)
	require.NoError(t, os.Chtimes(p, ts, ts))
}

func TestArcReview_ShowsTheMostRecentSessionByDefault(t *testing.T) {
	vault := t.TempDir()
	writeReviewEpisode(t, vault, "episode-2026-09-20-old00000", "an older session's message", time.Hour)
	writeReviewEpisode(t, vault, "episode-2026-09-24-new00000", "did you use your vaults? you have 2", 0)

	out, _, err := runRootCmd(t, "arc", "candidates", "--vault", vault, "--review")
	require.NoError(t, err)
	s := out.String()
	assert.Contains(t, s, "episode-2026-09-24-new00000")
	assert.Contains(t, s, "did you use your vaults? you have 2")
	assert.Contains(t, s, "I answered from what the session start gave me.")
	assert.NotContains(t, s, "an older session's message")
	assert.Contains(t, s, "desk entry", "says what to do with a turning point")
}

func TestArcReview_NamedSessionsAndJSON(t *testing.T) {
	vault := t.TempDir()
	writeReviewEpisode(t, vault, "episode-2026-09-20-old00000", "an older session's message", time.Hour)
	writeReviewEpisode(t, vault, "episode-2026-09-24-new00000", "did you use your vaults? you have 2", 0)

	out, _, err := runRootCmd(t, "arc", "candidates", "--vault", vault, "--review",
		"--episode", "episode-2026-09-20-old00000", "--json")
	require.NoError(t, err)
	var env struct {
		Result struct {
			Sessions []struct {
				Episode  string `json:"episode"`
				Messages []struct {
					N           int    `json:"n"`
					AgentBefore string `json:"agent_before"`
					Person      string `json:"person"`
				} `json:"messages"`
			} `json:"sessions"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env), out.String())
	require.Len(t, env.Result.Sessions, 1)
	assert.Equal(t, "episode-2026-09-20-old00000", env.Result.Sessions[0].Episode)
	assert.Equal(t, "an older session's message", env.Result.Sessions[0].Messages[0].Person)
}

func TestArcReview_UnknownSessionIsAnError(t *testing.T) {
	vault := t.TempDir()
	writeReviewEpisode(t, vault, "episode-2026-09-24-new00000", "did you use your vaults? you have 2", 0)
	_, _, err := runRootCmd(t, "arc", "candidates", "--vault", vault, "--review", "--episode", "episode-nope")
	require.ErrorContains(t, err, "episode-nope")
}
