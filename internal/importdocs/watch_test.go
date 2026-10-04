package importdocs_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/peiman/vaultmind/internal/importdocs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fingerprint(t *testing.T, repo, vault string) string {
	t.Helper()
	fp, err := importdocs.Fingerprint(source(repo), vault)
	require.NoError(t, err)
	return fp
}

func TestFingerprint_ChangesOnlyWithTheFilesAnImportReads(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	base := fingerprint(t, repo, vault)
	assert.Equal(t, base, fingerprint(t, repo, vault), "nothing changed")

	write(t, filepath.Join(repo, "docs", "style.css"), "body { color: red }")
	write(t, filepath.Join(repo, "docs", ".cache", "x.md"), "# hidden\n")
	write(t, filepath.Join(repo, "docs", "node_modules", "lib", "README.md"), "# dep\n")
	assert.Equal(t, base, fingerprint(t, repo, vault), "files the import does not read are not watched")

	write(t, filepath.Join(repo, "docs", "gamma.md"), "# Gamma\n")
	added := fingerprint(t, repo, vault)
	assert.NotEqual(t, base, added, "a new doc")

	write(t, filepath.Join(repo, "docs", "gamma.md"), "# Gamma, longer now\n")
	edited := fingerprint(t, repo, vault)
	assert.NotEqual(t, added, edited, "an edited doc")

	// A filesystem with coarse times can give an edit its file's old mtime;
	// the size still tells.
	g := filepath.Join(repo, "docs", "gamma.md")
	info, err := os.Stat(g)
	require.NoError(t, err)
	write(t, g, "# Gamma, longer still and in the same tick\n")
	require.NoError(t, os.Chtimes(g, info.ModTime(), info.ModTime()))
	assert.NotEqual(t, edited, fingerprint(t, repo, vault), "an edit that kept the mtime")

	require.NoError(t, os.Remove(filepath.Join(repo, "docs", "gamma.md")))
	assert.Equal(t, base, fingerprint(t, repo, vault), "a deleted doc")
}

func TestFingerprint_LeavesOutWhatGitIgnores(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	// go-git, not the git binary: under a git hook GIT_DIR points at the
	// real repository, and `git init` would act there.
	_, err := gogit.PlainInit(repo, false)
	require.NoError(t, err)
	write(t, filepath.Join(repo, ".gitignore"), "docs/build/\n")
	base := fingerprint(t, repo, vault)
	write(t, filepath.Join(repo, "docs", "build", "out.md"), "# generated\n")
	assert.Equal(t, base, fingerprint(t, repo, vault))
}

// steps is a fake clock and source for the watch loop: each wait returns
// at once and moves to the next fingerprint; when they run out the context
// is cancelled.
type steps struct {
	prints []string
	at     int
	cancel context.CancelFunc
}

func (s *steps) wait(ctx context.Context, _ time.Duration) error {
	s.at++
	if s.at >= len(s.prints) {
		s.cancel()
	}
	return ctx.Err()
}

func (s *steps) fingerprint() (string, error) {
	if s.at >= len(s.prints) {
		return s.prints[len(s.prints)-1], nil
	}
	if s.prints[s.at] == "ERR" {
		return "", errors.New("source gone for a moment")
	}
	return s.prints[s.at], nil
}

func runLoop(t *testing.T, prints []string, run func() error) (runs int, errs []error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	s := &steps{prints: prints, cancel: cancel}
	importdocs.WatchLoop(ctx, importdocs.WatchConfig{
		Interval:    2 * time.Second,
		Fingerprint: s.fingerprint,
		Run:         func() error { runs++; return run() },
		Wait:        s.wait,
		OnError:     func(err error) { errs = append(errs, err) },
	})
	return runs, errs
}

func TestWatchLoop_ReSyncsOnceAChangeSettles(t *testing.T) {
	ok := func() error { return nil }
	cases := map[string]struct {
		prints []string
		want   int
	}{
		"quiet":                       {[]string{"a", "a", "a", "a"}, 0},
		"one change":                  {[]string{"a", "b", "b", "b"}, 1},
		"a burst is one re-sync":      {[]string{"a", "b", "c", "d", "d", "d"}, 1},
		"two changes apart":           {[]string{"a", "b", "b", "b", "c", "c", "c"}, 2},
		"still changing when stopped": {[]string{"a", "b", "c"}, 0},
	}
	got := map[string]int{}
	for name, c := range cases {
		runs, _ := runLoop(t, c.prints, ok)
		got[name] = runs
	}
	want := map[string]int{}
	for name, c := range cases {
		want[name] = c.want
	}
	assert.Equal(t, want, got)
}

func TestWatchLoop_AFailureIsSaidAndTheWatchGoesOn(t *testing.T) {
	failed := false
	runs, errs := runLoop(t, []string{"a", "ERR", "b", "b", "c", "c", "c"}, func() error {
		if !failed {
			failed = true
			return errors.New("import failed")
		}
		return nil
	})
	assert.Equal(t, 2, runs, "the first re-sync failed, the second still ran")
	require.Len(t, errs, 2)
	assert.Contains(t, errs[0].Error(), "source gone for a moment")
	assert.Contains(t, errs[1].Error(), "import failed")
}

func TestWatchLoop_WithoutAFingerprintRunsEveryInterval(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	waits, runs := 0, 0
	importdocs.WatchLoop(ctx, importdocs.WatchConfig{
		Interval: time.Hour,
		Run:      func() error { runs++; return nil },
		Wait: func(ctx context.Context, d time.Duration) error {
			assert.Equal(t, time.Hour, d)
			waits++
			if waits > 3 {
				cancel()
			}
			return ctx.Err()
		},
	})
	assert.Equal(t, 3, runs, "a URL is re-imported on each interval")
}

// A zero interval would spin: the loop never waits less than its floor.
func TestWatchLoop_AZeroIntervalDoesNotSpin(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var got time.Duration
	importdocs.WatchLoop(ctx, importdocs.WatchConfig{
		Run: func() error { return nil },
		Wait: func(ctx context.Context, d time.Duration) error {
			got = d
			cancel()
			return ctx.Err()
		},
	})
	assert.Equal(t, importdocs.MinWatchInterval, got)
}
