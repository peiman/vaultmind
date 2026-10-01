package index

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sectionsFailEmbedder embeds notes and fails every batch holding a section
// (only a section's text carries the "title › heading" line).
type sectionsFailEmbedder struct{ fakeEmbedder }

func (s *sectionsFailEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	for _, tx := range texts {
		if strings.Contains(tx, " › ") {
			return nil, errors.New("model failed on a section")
		}
	}
	return s.fakeEmbedder.EmbedBatch(ctx, texts)
}

// A --full run purges every vector, sections included. If it then re-embeds
// the notes but not the sections, the long notes' parts are gone: that is
// the same data loss as failed notes and must not exit 0.
func TestEmbedResolved_FullFailsLoudWhenSectionsAreNotReembedded(t *testing.T) {
	idx, dbPath := buildWhiteboxVault(t)
	var md strings.Builder
	md.WriteString("---\nid: ref-long\ntype: concept\ntitle: Long\n---\n")
	for _, h := range []string{"Install", "Configure"} {
		md.WriteString("## " + h + "\n\n" + strings.Repeat("word ", 1200) + "\n\n")
	}
	for md.Len() < 9000*4 {
		md.WriteString(strings.Repeat("word ", 1200) + "\n\n")
	}
	require.NoError(t, os.WriteFile(filepath.Join(idx.vaultRoot, "long.md"), []byte(md.String()), 0o644))
	_, err := idx.Rebuild()
	require.NoError(t, err)
	_, err = idx.EmbedNotes(context.Background(), dbPath, &fakeEmbedder{dims: 8})
	require.NoError(t, err)

	res, err := idx.embedResolved(context.Background(), dbPath, &sectionsFailEmbedder{fakeEmbedder{dims: 8}}, embedding.ModelMiniLM, true)
	require.Error(t, err, "--full that re-embedded the notes but lost the sections must fail loud")
	assert.Contains(t, err.Error(), "section")
	assert.Positive(t, res.SectionsErrors)
}
