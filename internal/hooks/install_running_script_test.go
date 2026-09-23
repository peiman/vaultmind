package hooks

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// LIVE-OBSERVED (2026-09-23): `hooks install --force` rewrote mesh-watch.sh
// while a watcher was running it. Bash reads a script from its file as it
// goes, so the running watcher continued from its old byte offset into the NEW
// file and died: "syntax error near unexpected token `done'". Anyone upgrading
// with a watcher armed hit this — the release notes told them to reinstall.
//
// Install must replace scripts by rename: a running interpreter keeps the old
// file; the next start gets the new one.
func TestInstall_DoesNotCorruptAScriptThatIsRunning(t *testing.T) {
	dir := t.TempDir()
	scripts := filepath.Join(dir, ".claude", "scripts")
	require.NoError(t, os.MkdirAll(scripts, 0o750))
	target := filepath.Join(scripts, "shell-strip.sh")
	require.NoError(t, os.WriteFile(target, []byte("#!/bin/bash\nsleep 1\necho OLD-VERSION-FINISHED\n"), 0o700)) //nolint:gosec // G306: executable fixture

	cmd := exec.Command("bash", target)
	cmd.Stdin = nil
	out := make(chan []byte, 1)
	go func() { b, _ := cmd.CombinedOutput(); out <- b }()
	time.Sleep(300 * time.Millisecond) // bash is inside `sleep 1`, offset past line 2

	_, err := Install(InstallConfig{ProjectDir: dir, Force: true, Only: []string{"shell-strip.sh"}})
	require.NoError(t, err)

	select {
	case b := <-out:
		assert.Contains(t, string(b), "OLD-VERSION-FINISHED",
			"the running script must finish as the version it started as, not read into the new file")
	case <-time.After(10 * time.Second):
		t.Fatal("running script did not finish")
	}
	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.NotContains(t, string(got), "OLD-VERSION-FINISHED", "the new version is installed for the next start")
}
