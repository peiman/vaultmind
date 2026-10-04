package cmd

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeWatchWait replaces the watch's pause: each call runs the next step,
// and when the steps run out the watch is stopped.
func fakeWatchWait(t *testing.T, steps ...func()) *[]time.Duration {
	t.Helper()
	var seen []time.Duration
	prev := watchWait
	t.Cleanup(func() { watchWait = prev })
	i := 0
	watchWait = func(ctx context.Context, d time.Duration) error {
		seen = append(seen, d)
		if i >= len(steps) {
			return context.Canceled
		}
		steps[i]()
		i++
		return ctx.Err()
	}
	return &seen
}

func TestImport_WatchReSyncsAFolderWhenADocChanges(t *testing.T) {
	vault, repo := indexedBaselineVault(t), docsRepo(t)
	docs := filepath.Join(repo, "docs")
	seen := fakeWatchWait(t,
		func() {}, // a quiet poll
		func() {
			require.NoError(t, os.WriteFile(filepath.Join(docs, "gamma.md"), []byte("# Gamma\n\nNew.\n"), 0o600))
		},
		func() {}, // the change holds still: re-sync
		func() {}, // quiet again
	)

	out, errOut, err := runRootCmd(t, "import", docs, "--vault", vault, "--watch")
	require.NoError(t, err)
	assert.LessOrEqual(t, strings.Count(errOut.String(), "never been embedded"), 1, "a hint is said once, not on every re-sync")
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	assert.Contains(t, lines[0], "2 added", "the first import")
	assert.Regexp(t, `(?m)^\d\d:\d\d:\d\d Imported .*: 1 added, 2 unchanged$`, out.String(), "one time-stamped re-sync")
	assert.Equal(t, 2*time.Second, (*seen)[0], "a folder is polled every 2s by default")
	assert.Equal(t, 1, strings.Count(out.String(), "1 added"))
}

func TestImport_WatchRefusesCombinationsItCannotHonour(t *testing.T) {
	vault := indexedBaselineVault(t)
	cases := map[string][]string{
		"--watch prints a line per re-sync; it cannot be combined with --json": {"--watch", "--json"},
		"--watch cannot be combined with --dry-run":                            {"--watch", "--dry-run"},
		"--watch-interval":      {"--watch", "--watch-interval", "soon"},
		"at least 1m for a URL": {"--watch", "--watch-interval", "10s", "https://example.com/"},
	}
	for want, extra := range cases {
		args := []string{"import", t.TempDir(), "--vault", vault}
		if last := extra[len(extra)-1]; strings.HasPrefix(last, "https://") {
			args[1], extra = last, extra[:len(extra)-1]
		}
		_, _, err := runRootCmd(t, append(args, extra...)...)
		if assert.Error(t, err, want) {
			assert.Contains(t, err.Error(), want)
		}
	}
}

func TestWatchInterval_DefaultsByKindOfSource(t *testing.T) {
	got := map[string]time.Duration{}
	for _, arg := range []string{"docs", "https://example.com/"} {
		d, err := watchInterval("", arg)
		require.NoError(t, err)
		got[arg] = d
	}
	assert.Equal(t, map[string]time.Duration{"docs": 2 * time.Second, "https://example.com/": 6 * time.Hour}, got)
}

func TestImport_WatchReSyncsOneFileWhenItChanges(t *testing.T) {
	vault, repo := indexedBaselineVault(t), docsRepo(t)
	file := filepath.Join(repo, "docs", "alpha.md")
	fakeWatchWait(t,
		func() { require.NoError(t, os.WriteFile(file, []byte("# Alpha Guide\n\nRewritten.\n"), 0o600)) },
		func() {},
	)
	out, _, err := runRootCmd(t, "import", file, "--vault", vault, "--watch")
	require.NoError(t, err)
	assert.Regexp(t, `(?m)^\d\d:\d\d:\d\d Imported .*: 1 updated$`, out.String())
}

func TestImport_WatchReImportsAURLEachIntervalAndSaysNothingWhenItIsUnchanged(t *testing.T) {
	vault := indexedBaselineVault(t)
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.Header().Set("Content-Type", "text/markdown")
		_, _ = io.WriteString(w, "# Page\n\nThe same text every time.\n")
	}))
	t.Cleanup(srv.Close)
	seen := fakeWatchWait(t, func() {}, func() {})
	out, _, err := runRootCmd(t, "import", srv.URL+"/page.md", "--vault", vault, "--watch", "--watch-interval", "1m")
	require.NoError(t, err)
	assert.Equal(t, 3, hits, "the first import and two re-imports")
	assert.Equal(t, time.Minute, (*seen)[0])
	assert.NotRegexp(t, `(?m)^\d\d:\d\d:\d\d`, out.String(), "an unchanged page prints nothing")
}

func TestImport_AFailedReSyncIsSaidAndTheWatchGoesOn(t *testing.T) {
	vault, repo := indexedBaselineVault(t), docsRepo(t)
	docs := filepath.Join(repo, "docs")
	fakeWatchWait(t,
		func() { require.NoError(t, os.RemoveAll(docs)) },
		func() {},
	)
	_, errOut, err := runRootCmd(t, "import", docs, "--vault", vault, "--watch")
	require.NoError(t, err, "a watch does not end on a failure")
	assert.Regexp(t, `(?m)^\d\d:\d\d:\d\d .*no such file`, errOut.String())
}

func TestWatchWait_PausesOrStopsWhenTheWatchEnds(t *testing.T) {
	start := time.Now()
	require.NoError(t, watchWait(context.Background(), 10*time.Millisecond))
	assert.GreaterOrEqual(t, time.Since(start), 10*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.ErrorIs(t, watchWait(ctx, time.Hour), context.Canceled)
}
