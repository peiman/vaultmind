package meshpaths

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/xdg"
	"github.com/stretchr/testify/require"
)

// TestFor_FilenameScheme pins the per-agent state filename scheme in ONE place.
//
// Before this package existed there were three hand-maintained watcher scripts
// and a doctor check, each carrying its own idea of where mesh state lives and
// what the files are called. mira's watcher wrote mesh-watch.heartbeat,
// workhorse's wrote mesh-watch-wh.heartbeat, and doctor looked for a third
// spelling in (until the configBase fix) a directory none of them wrote to.
// Four opinions, zero mechanism keeping them aligned.
func TestFor_FilenameScheme(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	xdg.SetAppName("vaultmind")

	p, err := For("mira")
	require.NoError(t, err)

	dir := filepath.Dir(p.Heartbeat)
	require.Equal(t, "mesh-watch-mira.heartbeat", filepath.Base(p.Heartbeat))
	require.Equal(t, "mesh-watch-mira.pid", filepath.Base(p.Pid))
	require.Equal(t, "mesh-watch-mira.lastwake", filepath.Base(p.Lastwake))
	require.Equal(t, "mesh-watch-mira.lastarm", filepath.Base(p.Lastarm))
	require.Equal(t, "mesh-watch-mira.disarm", filepath.Base(p.Disarm))
	require.Equal(t, "mesh-watch-mira.log", filepath.Base(p.Log))

	// Every per-agent file lives in the same directory.
	for _, f := range []string{p.Pid, p.Lastwake, p.Lastarm, p.Disarm, p.Log, p.Listen} {
		require.Equal(t, dir, filepath.Dir(f))
	}

	// mesh-listen.json is deliberately NOT slug-suffixed: it is the shared
	// listen-control SSOT, one file for every agent on this machine.
	require.Equal(t, "mesh-listen.json", filepath.Base(p.Listen))
}

// TestFor_EmptySlugRefuses — a path derived from an empty slug would be a file
// named "mesh-watch-.heartbeat" that some checker would then dutifully report
// on. No slug means the question cannot be answered, and that must be an error,
// not a degenerate path.
func TestFor_EmptySlugRefuses(t *testing.T) {
	xdg.SetAppName("vaultmind")
	_, err := For("")
	require.ErrorIs(t, err, ErrNoSlug)
}

// TestDir_MatchesTheShellDerivation is the enforcement the old design had only
// as a comment. Both watcher scripts carried
// "# matches `vaultmind doctor`'s xdg.ConfigFile path" — false on darwin for
// months, and nothing could fail. This runs the exact expression the shell
// scripts use and asserts the Go side agrees, on whatever platform the test
// runs on.
func TestDir_MatchesTheShellDerivation(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("VAULTMIND_MESH_DIR", "")
	xdg.SetAppName("vaultmind")

	out, err := exec.Command("bash", "-c",
		`printf '%s' "${XDG_CONFIG_HOME:-$HOME/.config}/vaultmind"`).Output()
	require.NoError(t, err)

	got, err := Dir()
	require.NoError(t, err)
	require.Equal(t, strings.TrimSpace(string(out)), got,
		"the shell scripts and the Go SSOT must resolve the same directory")
}

// TestDir_EnvEscapeHatch — $VAULTMIND_MESH_DIR wins outright, for tests and for
// any deployment that needs the state somewhere else.
func TestDir_EnvEscapeHatch(t *testing.T) {
	explicit := t.TempDir()
	t.Setenv("VAULTMIND_MESH_DIR", explicit)
	xdg.SetAppName("vaultmind")

	got, err := Dir()
	require.NoError(t, err)
	require.Equal(t, explicit, got)
}
