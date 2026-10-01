package memory_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sectionPackDB holds two long notes cut into sections: "ref-long" with
// headings Install / Configure, and "ref-other" with Alpha / Beta. Each
// section's text starts with its heading word.
func sectionPackDB(t *testing.T) *index.DB {
	t.Helper()
	db, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	for id, headings := range map[string][]string{"ref-long": {"Install", "Configure"}, "ref-other": {"Alpha", "Beta"}} {
		var md strings.Builder
		for _, h := range headings {
			md.WriteString("## " + h + "\n\n" + h + " " + strings.Repeat("word ", 1200) + "\n\n")
		}
		for md.Len() < 9000*4 {
			md.WriteString(strings.Repeat("filler ", 1000) + "\n\n")
		}
		rec := index.NoteRecord{ID: id, Path: "refs/" + id + ".md", Title: "Title of " + id, Type: "reference",
			BodyText: md.String(), Hash: "h-" + id, IsDomain: true}
		rec.Sections = index.SectionsFor(rec.BodyText, rec.BodyText)
		require.NoError(t, index.StoreNote(db, rec))
	}
	return db
}

// A hit on a section of a long note delivers that section — not the note's
// opening, which is what the agent got before and which held nothing of a
// deep answer. The target says which section, and where it sits.
func TestContextPack_DeliversTheMatchedSection(t *testing.T) {
	db := sectionPackDB(t)
	res, err := memory.ContextPack(graph.NewResolver(db), db, memory.ContextPackConfig{
		Input: "ref-long", Budget: 3000, ExcerptTokens: 80,
		Sections: map[string]string{"ref-long": "ref-long#configure"},
	})
	require.NoError(t, err)
	require.NotNil(t, res.Target)
	assert.True(t, strings.HasPrefix(res.Target.Body, "Configure"), res.Target.Body[:min(60, len(res.Target.Body))])
	assert.Equal(t, "ref-long#configure", res.Target.Section)
	assert.Equal(t, "Configure", res.Target.HeadingPath)
}

// Seeds (later hits) deliver their matched section too.
func TestContextPack_ASeedDeliversItsSection(t *testing.T) {
	db := sectionPackDB(t)
	res, err := memory.ContextPack(graph.NewResolver(db), db, memory.ContextPackConfig{
		Input: "ref-long", Budget: 6000, ExcerptTokens: 80, MaxItems: 3,
		Seeds:    []memory.Seed{{ID: "ref-other", Confidence: "moderate"}},
		Sections: map[string]string{"ref-long": "ref-long#configure", "ref-other": "ref-other#beta"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, res.Context)
	var seed *memory.ContextItem
	for i := range res.Context {
		if res.Context[i].ID == "ref-other" {
			seed = &res.Context[i]
		}
	}
	require.NotNil(t, seed)
	assert.True(t, strings.HasPrefix(seed.Body, "Beta"), seed.Body[:min(60, len(seed.Body))])
	assert.Equal(t, "ref-other#beta", seed.Section)
}

// A section that is unknown or stale falls back to the whole note: the agent
// gets the note it would have got before, never nothing.
func TestContextPack_AnUnknownSectionFallsBackToTheNote(t *testing.T) {
	db := sectionPackDB(t)
	res, err := memory.ContextPack(graph.NewResolver(db), db, memory.ContextPackConfig{
		Input: "ref-long", Budget: 3000, ExcerptTokens: 80,
		Sections: map[string]string{"ref-long": "ref-long#gone"},
	})
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(res.Target.Body, "Install"), "the note's opening, as before")
	assert.Empty(t, res.Target.Section)
}

// With no sections named, the pack is exactly what it was.
func TestContextPack_NoSectionsIsUnchanged(t *testing.T) {
	db := sectionPackDB(t)
	cfg := memory.ContextPackConfig{Input: "ref-long", Budget: 3000, ExcerptTokens: 80}
	before, err := memory.ContextPack(graph.NewResolver(db), db, cfg)
	require.NoError(t, err)
	cfg.Sections = map[string]string{}
	after, err := memory.ContextPack(graph.NewResolver(db), db, cfg)
	require.NoError(t, err)
	assert.Equal(t, before, after)
	assert.Empty(t, before.Target.Section)
}
