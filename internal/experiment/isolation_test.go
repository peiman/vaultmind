package experiment_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestXDGDataHomeIsolated is the enforcement half of issue #17: test runs
// must route the experiment DB to a temp directory, not the real user data
// directory, or tests will write fixture IDs into production telemetry.
//
// The leak this prevents: without XDG_DATA_HOME redirected, xdg.DataFile
// resolves `experiments.db` under ~/Library/Application Support/vaultmind/
// on macOS (or ~/.local/share/vaultmind/ on Linux), so any test that opens
// an experiment DB writes into production user data.
//
// Use `task test`, which sets XDG_DATA_HOME automatically. For debugging a
// single test via `go test -run ...`, export XDG_DATA_HOME to a tmp dir first.
func TestXDGDataHomeIsolated(t *testing.T) {
	xdg := os.Getenv("XDG_DATA_HOME")
	if xdg == "" {
		t.Fatal("XDG_DATA_HOME is not set during tests (issue #17 regression).\n" +
			"Tests must not write to the real user data directory. Use `task test` " +
			"(sets XDG_DATA_HOME automatically), or export XDG_DATA_HOME=$(mktemp -d) " +
			"before running `go test` directly.")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return
	}
	abs, err := filepath.Abs(xdg)
	if err != nil {
		t.Fatalf("resolving XDG_DATA_HOME abs path: %v", err)
	}
	// Defense in depth: env set, but pointing at the default user data dir.
	for _, defaultSub := range []string{"Library/Application Support", ".local/share"} {
		if strings.HasPrefix(abs, filepath.Join(home, defaultSub)) {
			t.Fatalf("XDG_DATA_HOME=%s resolves under the user's real data "+
				"directory — test isolation bypassed (issue #17).", abs)
		}
	}
}
