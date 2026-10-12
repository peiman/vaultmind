//go:build !race

package query

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/peiman/vaultmind/internal/parser"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Wall-clock guards run without race instrumentation, whose SQLite and regexp
// overhead measures instrumentation rather than user-facing selection latency.
func TestPassageSelectionLatency(t *testing.T) {
	intro := "An introductory overview. " + strings.Repeat("padding ", 72) + "ends."
	for name, body := range map[string]string{
		"ordinary_prose":  intro + "\n\n" + strings.Repeat(strings.Repeat("The ordinary explanation has useful background detail. ", 11)+"\n\n", 48) + "\n\nSet WAL_RECOVERY=immutable to recover the database.",
		"short_sentences": intro + "\n\n" + strings.Repeat("A. B. C. D. E. ", 2000) + "\n\nSet WAL_RECOVERY=immutable to recover the database.",
	} {
		t.Run(name, func(t *testing.T) {
			db, err := index.Open(filepath.Join(t.TempDir(), "passage.db"))
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, db.Close()) })
			require.NoError(t, index.StoreNote(db, index.NoteRecord{ID: "target", Type: "reference", BodyRaw: body, BodyText: parser.StripForFTS(body)}))
			today := memory.Excerpt(body, 160)
			start := time.Now()
			result, err := selectPassage(db, "WAL_RECOVERY", body, today, 160)
			elapsed := time.Since(start)
			require.NoError(t, err)
			assert.Contains(t, result.Text, "WAL_RECOVERY=immutable")
			t.Logf("%d bytes: %.3f ms", len(body), float64(elapsed)/float64(time.Millisecond))
			bound := 50 * time.Millisecond // CI allowance; local measurement must be below 20 ms.
			if os.Getenv("VAULTMIND_PASSAGE_LATENCY_LOCAL") != "" {
				bound = 20 * time.Millisecond
			}
			assert.Less(t, elapsed, bound)
		})
	}
}
