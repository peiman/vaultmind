//go:build !race

package query

import (
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/peiman/vaultmind/internal/parser"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// passageLatencyCPUBound is process CPU, not wall time. The fixed path
// tokenizes once: about 6–12 ms of CPU here, coverage included. The pre-fix
// path re-normalized every window into a temporary FTS index. Put back, that
// cost 53–68 ms of CPU on ordinary prose and 2.2–2.5 s on short sentences (the
// historical wall-time range was 127 ms to 3.9 s). 40 ms sits between the
// fixed path and that regression. Wall time is the wrong clock: under load
// the fixed path has been measured at 50–400 ms of scheduler delay, and CPU
// time does not include that wait. The minimum of several samples drops one
// that happened to include a GC cycle.
const (
	passageLatencyCPUBound = 40 * time.Millisecond
	passageLatencySamples  = 5
)

// These guards run without race instrumentation, whose SQLite and regexp
// overhead measures instrumentation rather than selection cost.
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
			var best time.Duration
			var slowestWall time.Duration
			var result passageResult
			for i := 0; i < passageLatencySamples; i++ {
				started := time.Now()
				cpu := processCPU(t)
				result, err = selectPassage(db, "WAL_RECOVERY", body, today, 160)
				cpu = processCPU(t) - cpu
				wall := time.Since(started)
				require.NoError(t, err)
				if i == 0 || cpu < best {
					best = cpu
				}
				if wall > slowestWall {
					slowestWall = wall
				}
			}
			assert.Contains(t, result.Text, "WAL_RECOVERY=immutable")
			t.Logf("%d bytes: min CPU %.3f ms over %d (slowest wall %.3f ms)", len(body), float64(best)/float64(time.Millisecond), passageLatencySamples, float64(slowestWall)/float64(time.Millisecond))
			assert.Less(t, best, passageLatencyCPUBound)
		})
	}
}

func processCPU(t *testing.T) time.Duration {
	t.Helper()
	var ru syscall.Rusage
	require.NoError(t, syscall.Getrusage(syscall.RUSAGE_SELF, &ru))
	return time.Duration(ru.Utime.Sec)*time.Second + time.Duration(ru.Utime.Usec)*time.Microsecond +
		time.Duration(ru.Stime.Sec)*time.Second + time.Duration(ru.Stime.Usec)*time.Microsecond
}
