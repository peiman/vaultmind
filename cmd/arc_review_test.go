package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/episode"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeReviewEpisode renders an episode with the REAL renderer (the path
// capture uses), so the review is tested on what capture actually writes. The
// agent speaks at 09:01 and the person at 09:02 on the day of startedAt.
func writeReviewEpisode(t *testing.T, vault, id, day, said string) {
	t.Helper()
	ep := &episode.Episode{
		ID: id, SessionID: id, StartedAt: day + "T09:00:00.000Z", ToolCounts: map[string]int{},
		AssistantMessages: []episode.Message{{Timestamp: day + "T09:01:00.000Z", Text: "I answered from what the session start gave me."}},
		UserMessages:      []episode.Message{{Timestamp: day + "T09:02:00.000Z", Text: said}},
	}
	dir := filepath.Join(vault, "episodes")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, id+".md"), []byte(episode.RenderMarkdown(ep)), 0o600))
}

const (
	reviewOldID = "episode-2026-09-20-old00000"
	reviewNewID = "episode-2026-09-24-new00000"
)

func writeTwoReviewEpisodes(t *testing.T, vault string) {
	t.Helper()
	writeReviewEpisode(t, vault, reviewNewID, "2026-09-24", "did you use your vaults? you have 2")
	writeReviewEpisode(t, vault, reviewOldID, "2026-09-20", "an older session's message")
}

// With no episode named, review opens the OLDEST session nobody has judged —
// a queue worked from the front, not whichever file was touched last.
func TestArcReview_OpensTheOldestSessionAwaitingReview(t *testing.T) {
	vault := t.TempDir()
	writeTwoReviewEpisodes(t, vault)

	out, _, err := runRootCmd(t, "arc", "review", "--vault", vault)
	require.NoError(t, err)
	s := out.String()
	assert.Contains(t, s, reviewOldID)
	assert.Contains(t, s, "an older session's message")
	assert.Contains(t, s, "I answered from what the session start gave me.")
	assert.NotContains(t, s, "did you use your vaults? you have 2")
	assert.Contains(t, s, "desk entry", "says what to do with a turning point")
	assert.Contains(t, s, "--mark-reviewed "+reviewOldID, "says how to record the judgement")
	assert.Contains(t, s, "2 sessions await review")
}

func TestArcReview_MarkingMovesTheQueueOn(t *testing.T) {
	vault := t.TempDir()
	writeTwoReviewEpisodes(t, vault)

	out, _, err := runRootCmd(t, "arc", "review", "--vault", vault, "--mark-reviewed", reviewOldID)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "Marked reviewed: "+reviewOldID)
	assert.Contains(t, out.String(), "1 session awaits review")

	out, _, err = runRootCmd(t, "arc", "review", "--vault", vault)
	require.NoError(t, err)
	assert.Contains(t, out.String(), reviewNewID)
	assert.NotContains(t, out.String(), "an older session's message")

	_, _, err = runRootCmd(t, "arc", "review", "--vault", vault, "--mark-reviewed", reviewNewID)
	require.NoError(t, err)
	out, _, err = runRootCmd(t, "arc", "review", "--vault", vault)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "No session awaits review")
}

func TestArcReview_NamedSessionsAndJSON(t *testing.T) {
	vault := t.TempDir()
	writeTwoReviewEpisodes(t, vault)

	out, _, err := runRootCmd(t, "arc", "review", "--vault", vault, "--episode", reviewNewID, "--json")
	require.NoError(t, err)
	var env struct {
		Command string `json:"command"`
		Result  struct {
			Awaiting int `json:"awaiting"`
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
	assert.Equal(t, arcReviewEnvelope, env.Command)
	assert.Equal(t, 2, env.Result.Awaiting)
	require.Len(t, env.Result.Sessions, 1)
	assert.Equal(t, reviewNewID, env.Result.Sessions[0].Episode)
	require.Len(t, env.Result.Sessions[0].Messages, 1)
	assert.Equal(t, "did you use your vaults? you have 2", env.Result.Sessions[0].Messages[0].Person)
}

func TestArcReview_UnknownSessionIsAnError(t *testing.T) {
	vault := t.TempDir()
	writeTwoReviewEpisodes(t, vault)
	_, _, err := runRootCmd(t, "arc", "review", "--vault", vault, "--episode", "episode-nope")
	require.ErrorContains(t, err, "episode-nope")
	_, _, err = runRootCmd(t, "arc", "review", "--vault", vault, "--mark-reviewed", "episode-nope")
	require.ErrorContains(t, err, "episode-nope")
}

// Session start runs `self`; that is where the queue must be seen, or review
// depends on someone remembering it.
func TestSelf_SaysHowManySessionsAwaitReview(t *testing.T) {
	vault := buildIndexedTestVault(t)
	writeTwoReviewEpisodes(t, vault)

	out, _, err := runRootCmd(t, "self", "--vault", vault)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "2 sessions await review (oldest 2026-09-20)")
	assert.Contains(t, out.String(), "vaultmind arc review --vault "+vault)
}

func TestSelf_SaysNothingAboutReviewWhenNothingAwaits(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "self", "--vault", vault)
	require.NoError(t, err)
	assert.NotContains(t, out.String(), "await review")
}

func writeUnreadableEpisode(t *testing.T, vault string) {
	t.Helper()
	p := filepath.Join(vault, "episodes", "episode-2026-09-18-bad00000.md")
	require.NoError(t, os.WriteFile(p, []byte("---\nid: x\ntags: [unclosed\n---\n\nbody\n"), 0o600))
}

// One unreadable episode must not stop review, and must never make a mark
// that was written look like it failed.
func TestArcReview_AnUnreadableEpisodeIsNamedAndTheRestStillWorks(t *testing.T) {
	vault := t.TempDir()
	writeTwoReviewEpisodes(t, vault)
	writeUnreadableEpisode(t, vault)

	out, _, err := runRootCmd(t, "arc", "review", "--vault", vault)
	require.NoError(t, err)
	assert.Contains(t, out.String(), reviewOldID)
	assert.Contains(t, out.String(), "episode-2026-09-18-bad00000.md", "the skipped file is named")

	out, _, err = runRootCmd(t, "arc", "review", "--vault", vault, "--mark-reviewed", reviewOldID)
	require.NoError(t, err, "the mark was written, so it succeeded")
	assert.Contains(t, out.String(), "Marked reviewed: "+reviewOldID)

	out, _, err = runRootCmd(t, "arc", "review", "--vault", vault, "--episode", reviewNewID, "--json")
	require.NoError(t, err)
	var env struct {
		Status   string `json:"status"`
		Warnings []struct {
			Message string `json:"message"`
		} `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env), out.String())
	assert.Equal(t, "warning", env.Status)
	require.Len(t, env.Warnings, 1)
	assert.Contains(t, env.Warnings[0].Message, "episode-2026-09-18-bad00000.md")
}

// The queue opens and marks an episode by its file, whatever id it carries.
func TestArcReview_ARenamedEpisodeIsOpenedAndMarkedByItsFile(t *testing.T) {
	vault := t.TempDir()
	writeReviewEpisode(t, vault, reviewOldID, "2026-09-20", "an older session's message")
	dir := filepath.Join(vault, "episodes")
	require.NoError(t, os.Rename(filepath.Join(dir, reviewOldID+".md"), filepath.Join(dir, "renamed-copy.md")))

	out, _, err := runRootCmd(t, "arc", "review", "--vault", vault)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "an older session's message")
	assert.Contains(t, out.String(), "--mark-reviewed renamed-copy")

	_, _, err = runRootCmd(t, "arc", "review", "--vault", vault, "--mark-reviewed", "renamed-copy")
	require.NoError(t, err)
	out, _, err = runRootCmd(t, "arc", "review", "--vault", vault)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "No session awaits review")
}

// A vault that captures no sessions is not a vault whose sessions were all
// judged: saying so would send the reader away from the vault that has them.
func TestArcReview_AVaultWithoutEpisodesSaysSoInsteadOfAllClear(t *testing.T) {
	vault := t.TempDir()
	_, _, err := runRootCmd(t, "arc", "review", "--vault", vault)
	require.ErrorContains(t, err, "no episodes folder")
}

func TestArcReview_APartialMarkSaysWhatWasAlreadyMarked(t *testing.T) {
	vault := t.TempDir()
	writeTwoReviewEpisodes(t, vault)
	_, _, err := runRootCmd(t, "arc", "review", "--vault", vault, "--mark-reviewed", reviewOldID+",episode-nope")
	require.ErrorContains(t, err, "episode-nope")
	require.ErrorContains(t, err, "already marked: "+reviewOldID)
}

func TestSelf_SaysWhenEpisodesCouldNotBeRead(t *testing.T) {
	vault := buildIndexedTestVault(t)
	writeTwoReviewEpisodes(t, vault)
	writeUnreadableEpisode(t, vault)
	out, _, err := runRootCmd(t, "self", "--vault", vault)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "2 sessions await review")
	assert.Contains(t, out.String(), "1 episode(s) could not be read")
}
