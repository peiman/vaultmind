package distill_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/distill"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Review closes the loop from episode to arc only if it happens without anyone
// having to remember it. An episode awaits review until an agent has judged it;
// the judgement is recorded on the episode itself (a `reviewed` date in its
// frontmatter), the way a desk entry records the arc it became.

func stateEpisode(id, startedAt, extraFM, userText string) string {
	return fmt.Sprintf(`---
id: %s
type: episode
started_at: %s
%s---

# Episode — %s

## User messages (verbatim)

### 1 — %s

> %s

## Assistant responses (verbatim)

### 1 — %s

Done.
`, id, startedAt, extraFM, id, startedAt, userText, startedAt)
}

func writeStateVault(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"episode-2026-09-22-dddd0000": stateEpisode("episode-2026-09-22-dddd0000", "2026-09-22T09:00:00Z", "", "did you use your vaults? you have 2"),
		"episode-2026-09-20-aaaa0000": stateEpisode("episode-2026-09-20-aaaa0000", "2026-09-20T09:00:00Z", "", "decide what you want to do here"),
		"episode-2026-09-21-bbbb0000": stateEpisode("episode-2026-09-21-bbbb0000", "2026-09-21T09:00:00Z", "reviewed: \"2026-09-23\"\n", "this is very disturbing to me"),
		"episode-2026-09-19-cccc0000": stateEpisode("episode-2026-09-19-cccc0000", "2026-09-19T09:00:00Z", "", "yes"),
	}
	for id, body := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, id+".md"), []byte(body), 0o600))
	}
	return dir
}

func pendingIDs(t *testing.T, dir string) []string {
	t.Helper()
	pending, diags, err := distill.PendingReviews(dir)
	require.NoError(t, err)
	require.Empty(t, diags)
	ids := []string{}
	for _, p := range pending {
		ids = append(ids, p.File)
	}
	return ids
}

func TestPendingReviews_OldestFirstSkippingReviewedAndEmpty(t *testing.T) {
	dir := writeStateVault(t)
	assert.Equal(t, []string{"episode-2026-09-20-aaaa0000", "episode-2026-09-22-dddd0000"}, pendingIDs(t, dir),
		"reviewed episodes and episodes with nothing to judge ('yes') are not pending")

	pending, _, err := distill.PendingReviews(dir)
	require.NoError(t, err)
	require.Len(t, pending, 2)
	assert.Equal(t, "2026-09-20", pending[0].Date)
	assert.Equal(t, 1, pending[0].Messages)
}

// YAML reads an unquoted started_at as a timestamp and a quoted one as text;
// either way the queue shows the day.
func TestPendingReviews_DateFromAQuotedStartedAt(t *testing.T) {
	dir := t.TempDir()
	id := "episode-2026-09-22-eeee0000"
	body := stateEpisode(id, `"2026-09-22T09:00:00Z"`, "", "did you use your vaults? you have 2")
	require.NoError(t, os.WriteFile(filepath.Join(dir, id+".md"), []byte(body), 0o600))
	pending, _, err := distill.PendingReviews(dir)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, "2026-09-22", pending[0].Date)
}

func TestMarkReviewed_RemovesTheEpisodeFromPending(t *testing.T) {
	dir := writeStateVault(t)
	require.NoError(t, distill.MarkReviewed(dir, "episode-2026-09-20-aaaa0000", "2026-09-25"))
	assert.Equal(t, []string{"episode-2026-09-22-dddd0000"}, pendingIDs(t, dir))

	raw, err := os.ReadFile(filepath.Join(dir, "episode-2026-09-20-aaaa0000.md"))
	require.NoError(t, err)
	assert.Contains(t, string(raw), "reviewed: \"2026-09-25\"")
	assert.Contains(t, string(raw), "> decide what you want to do here", "the episode itself is untouched")
}

func TestMarkReviewed_AcceptsTheFileNameAndRefusesAnUnknownEpisode(t *testing.T) {
	dir := writeStateVault(t)
	require.NoError(t, distill.MarkReviewed(dir, "episode-2026-09-22-dddd0000.md", "2026-09-25"))
	assert.Equal(t, []string{"episode-2026-09-20-aaaa0000"}, pendingIDs(t, dir))

	assert.Error(t, distill.MarkReviewed(dir, "episode-2026-01-01-ffff0000", "2026-09-25"))
}

func TestMarkReviewed_NeverWritesOutsideTheEpisodesFolder(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "episodes")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	outside := filepath.Join(root, "arc-precious.md")
	original := "---\nid: arc-precious\ntype: arc\n---\n\nbody\n"
	require.NoError(t, os.WriteFile(outside, []byte(original), 0o600))

	assert.Error(t, distill.MarkReviewed(dir, "../arc-precious", "2026-09-25"))
	raw, err := os.ReadFile(outside)
	require.NoError(t, err)
	assert.Equal(t, original, string(raw), "a note outside episodes/ is never written")
}

func TestPendingReviews_AVaultWithoutEpisodesHasNothingPending(t *testing.T) {
	pending, diags, err := distill.PendingReviews(filepath.Join(t.TempDir(), "episodes"))
	require.NoError(t, err)
	assert.Empty(t, pending)
	assert.Empty(t, diags)
}

// One unreadable episode must not block the queue: it is skipped and named,
// the way arc candidates reports a parse error and keeps going.
func TestPendingReviews_AnUnreadableEpisodeIsNamedNotFatal(t *testing.T) {
	dir := writeStateVault(t)
	bad := filepath.Join(dir, "episode-2026-09-18-bad00000.md")
	require.NoError(t, os.WriteFile(bad, []byte("---\nid: x\ntags: [unclosed\n---\n\nbody\n"), 0o600))

	pending, diags, err := distill.PendingReviews(dir)
	require.NoError(t, err)
	require.Len(t, pending, 2, "the readable episodes are still queued")
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0], "episode-2026-09-18-bad00000.md")
}

// The queue opens and marks episodes by FILE, not by the id inside them: a
// renamed or imported episode must neither jam the queue nor mark another file.
func TestPendingReviews_ARenamedEpisodeIsQueuedAndMarkedByItsFile(t *testing.T) {
	dir := t.TempDir()
	body := stateEpisode("episode-2026-09-22-dddd0000", "2026-09-22T09:00:00Z", "", "did you use your vaults? you have 2")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "renamed-copy.md"), []byte(body), 0o600))

	pending, _, err := distill.PendingReviews(dir)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, "renamed-copy", pending[0].File)
	assert.Equal(t, "episode-2026-09-22-dddd0000", pending[0].ID)

	require.NoError(t, distill.MarkReviewed(dir, pending[0].File, "2026-09-25"))
	assert.Empty(t, pendingIDs(t, dir))
}
