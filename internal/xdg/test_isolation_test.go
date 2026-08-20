package xdg

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #121: `go test` wrote note_access events into the developer's REAL
// experiments.db. 156 of 227 note_get events in the live log were fixture ids —
// `does-not-exist`, `concept-alpha`, `no-such-id` — arriving 52-per-minute, which
// is `task check`, not a person reading notes. Every measurement taken off that
// log was measuring the test suite.
//
// A guard already existed and worked. It lives in internal/experiment, one
// package, so `go test ./cmd/...` alone sailed straight past it. Replicating it
// per package is the same forgettable convention that let the hook scripts rot.
//
// This guards the choke point instead: all nine non-test call sites reach
// experiments.db through DataDir, so one condition here covers every package that
// exists and every package anyone adds later.
//
// WHY IT PANICS INSTEAD OF RETURNING AN ERROR. Returning an error looks tidier and
// is useless here — cmd/root.go swallows it:
//
//	} else if expDB, expErr := openExperimentDB(); expErr != nil {
//	    log.Debug().Err(expErr).Msg("Experiment DB unavailable")
//
// at debug level, on the highest-traffic write path. The corruption would stop
// and no one would ever be told. A guard that the caller can silence is not a
// guard, which is the lesson this whole session is made of. A panic cannot be
// debug-logged away, and this branch is unreachable outside a test binary, so it
// can never fire for a user.
func TestDataDir_PanicsWhenATestBinaryWouldUseTheRealDataDir(t *testing.T) {
	SetAppName("vaultmind")
	// Empty, not unset: dataBase() treats "" as absent and falls through to the
	// platform default, which is exactly the unisolated case.
	t.Setenv("XDG_DATA_HOME", "")

	defer func() {
		r := recover()
		require.NotNil(t, r, "DataDir returned a path under the user's real data dir instead of refusing")
		msg := fmt.Sprint(r)
		assert.Contains(t, msg, "XDG_DATA_HOME",
			"the panic must name the variable that fixes it")
		assert.Contains(t, msg, "task test",
			"and the command that sets it, so the reader does not have to go looking")
	}()

	dir, err := DataDir()
	t.Fatalf("expected a panic; got dir=%q err=%v", dir, err)
}

// The mirror. An isolated test must be untouched — otherwise the guard makes the
// suite unrunnable rather than making it honest.
func TestDataDir_IsSilentWhenTheTestIsIsolated(t *testing.T) {
	SetAppName("vaultmind")
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	dir, err := DataDir()
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(dir, tmp),
		"an isolated run must resolve inside its temp dir, got %q", dir)
}

// Defence in depth, matching the check this consolidates: XDG_DATA_HOME can be
// SET and still point at the real data directory. The env var being non-empty is
// not the property that matters; where it resolves to is.
func TestDataDir_PanicsWhenXDGPointsAtTheRealDataDir(t *testing.T) {
	SetAppName("vaultmind")
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home directory to test against")
	}
	// Point it at the real per-platform base rather than leaving it unset.
	var realBase string
	switch osName {
	case "darwin":
		realBase = filepath.Join(home, "Library", "Application Support")
	case "windows":
		t.Skip("windows resolves via AppData, covered by the unset case")
	default:
		realBase = filepath.Join(home, ".local", "share")
	}
	t.Setenv("XDG_DATA_HOME", realBase)

	defer func() {
		require.NotNil(t, recover(),
			"XDG_DATA_HOME was set, and set to the real data dir — the env var being "+
				"non-empty proves nothing about isolation")
	}()

	dir, err := DataDir()
	t.Fatalf("expected a panic; got dir=%q err=%v", dir, err)
}

// The guard must fire BEFORE the directory is created. Refusing to hand back the
// path while having already made it is the shape of a check that runs too late —
// the side effect it exists to prevent has already happened.
func TestDataDir_DoesNotCreateTheRealDirectoryBeforeRefusing(t *testing.T) {
	SetAppName("vaultmind-guard-probe-should-never-exist")
	t.Setenv("XDG_DATA_HOME", "")

	base := dataBase()
	probe := filepath.Join(base, "vaultmind-guard-probe-should-never-exist")
	_, statErr := os.Stat(probe)
	require.True(t, os.IsNotExist(statErr), "precondition: the probe dir must not already exist")

	func() {
		defer func() { _ = recover() }()
		_, _ = DataDir()
	}()

	_, statErr = os.Stat(probe)
	assert.True(t, os.IsNotExist(statErr),
		"the guard created %s before refusing — it runs after MkdirAll", probe)
}
