package cmd

import (
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Doctor names the folders an overview would describe, with the command that
// writes one, and the overviews their folders have outgrown.
func TestWriteOverviews_NamesMissingAndStale(t *testing.T) {
	var b strings.Builder
	require.NoError(t, writeOverviews(&b, &query.OverviewHealth{
		Missing: []query.FolderCount{{Folder: "concepts", Notes: 194}, {Folder: "sources", Notes: 209}},
		Stale:   []query.StaleOverview{{Overview: "people/overview.md", Changed: 12, Notes: 30}},
	}, "/v"))
	out := b.String()
	assert.Contains(t, out, "Overviews:   2 folders without one: concepts/ (194), sources/ (209)")
	// Named after the folder: note create takes the id from the file name, so
	// every folder's overview.md would share one id (found on a real vault).
	assert.Contains(t, out, "vaultmind note create concepts/concepts-overview.md --type overview --field title=\"Concepts\" --body \"<what the folder covers, in one sentence first>\" --vault /v")
	assert.Contains(t, out, "people/overview.md is stale: 12 of its folder's 30 notes changed since")
}

func TestWriteOverviews_SilentWhenHealthy(t *testing.T) {
	var b strings.Builder
	require.NoError(t, writeOverviews(&b, &query.OverviewHealth{}, "/v"))
	require.NoError(t, writeOverviews(&b, nil, "/v"))
	assert.Empty(t, b.String())
}
