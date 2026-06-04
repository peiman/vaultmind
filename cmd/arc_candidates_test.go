package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/distill"
	"github.com/peiman/vaultmind/internal/episode"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The renderer→parser wire contract is duplicated (ADR-009 forbids infra→infra
// imports), so this round-trip drift guard lives in cmd, where both packages are
// importable. An Episode rendered by internal/episode and re-parsed by
// internal/distill must preserve verbatim turns — including assistant markdown
// headings, which must NOT truncate the turn (the C1 regression).
func TestEpisodeRenderParseRoundTrip(t *testing.T) {
	src := &episode.Episode{
		ID:         "episode-2026-06-01-roundtrip",
		SessionID:  "roundtrip",
		StartedAt:  "2026-06-01T10:00:00.000Z",
		ToolCounts: map[string]int{},
		UserMessages: []episode.Message{
			{Timestamp: "2026-06-01T10:00:00.000Z", Text: "you have full autonomy, dont need to ask me"},
		},
		AssistantMessages: []episode.Message{
			{Timestamp: "2026-06-01T10:01:00.000Z", Text: "I did the work.\n\n## Principle\n\nEnforce only what is read.\n\n### a subheading\n\nDone."},
		},
	}
	dir := t.TempDir()
	path := filepath.Join(dir, src.ID+".md")
	require.NoError(t, os.WriteFile(path, []byte(episode.RenderMarkdown(src)), 0o600))

	parsed, err := distill.ParseEpisodeFile(path)
	require.NoError(t, err)
	require.Len(t, parsed.UserTurns, 1)
	assert.Equal(t, "you have full autonomy, dont need to ask me", parsed.UserTurns[0].Text,
		"user verbatim round-trips clean (blockquote stripped)")
	require.Len(t, parsed.AssistantTurns, 1, "assistant markdown headings must not split the turn (C1)")
	body := parsed.AssistantTurns[0].Text
	assert.Contains(t, body, "## Principle")
	assert.Contains(t, body, "### a subheading")
	assert.Contains(t, body, "Done.")

	// Closing the loop: the round-tripped user turn fires the authority-grant rule.
	cands := distill.ExtractCandidates(parsed)
	require.Len(t, cands, 1)
	assert.Equal(t, distill.RuleAuthorityGrant, cands[0].Rule)
}
