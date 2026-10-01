package query

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type axisEmb struct{ v []float32 }

func (a axisEmb) Embed(context.Context, string) ([]float32, error) { return a.v, nil }
func (a axisEmb) EmbedBatch(context.Context, []string) ([][]float32, error) {
	return nil, nil
}
func (a axisEmb) Dims() int    { return len(a.v) }
func (a axisEmb) Close() error { return nil }

// The relevance verdict on a long note's hit is measured by its best section.
// Measured by the whole-note vector — which covers only the first 8,192
// tokens — a hit deep in the note would read as off-topic and be withheld.
func TestTopHitCosine_MeasuresALongNoteByItsBestSection(t *testing.T) {
	db, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	var md strings.Builder
	for _, h := range []string{"Install", "Configure"} {
		md.WriteString("## " + h + "\n\n" + strings.Repeat("word ", 1200) + "\n\n")
	}
	for md.Len() < 9000*4 {
		md.WriteString(strings.Repeat("word ", 1200) + "\n\n")
	}
	long := index.NoteRecord{ID: "ref-long", Path: "refs/long.md", Title: "Long", BodyText: md.String(), Hash: "h", IsDomain: true}
	long.Sections = index.SectionsFor(long.BodyText, long.BodyText)
	require.NoError(t, index.StoreNote(db, long))
	require.NoError(t, index.StoreEmbedding(db, "ref-long", []float32{1, 0}))

	q := axisEmb{v: []float32{0, 1}}
	cos, ok, err := topHitCosine(context.Background(), "q", "ref-long", q, db)
	require.NoError(t, err)
	require.True(t, ok)
	assert.InDelta(t, 0.0, cos, 1e-6, "before its sections are embedded, the note's own vector")

	_, err = db.Exec(`UPDATE sections SET embedding = ? WHERE anchor = 'configure'`, index.EncodeEmbedding([]float32{0, 1}))
	require.NoError(t, err)
	cos, ok, err = topHitCosine(context.Background(), "q", "ref-long", q, db)
	require.NoError(t, err)
	require.True(t, ok)
	assert.InDelta(t, 1.0, cos, 1e-6, "then its best section")

	_, ok, err = topHitCosine(context.Background(), "q", "no-such-note", q, db)
	assert.Error(t, err)
	assert.False(t, ok)
}
