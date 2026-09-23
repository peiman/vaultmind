package cmd

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/peiman/vaultmind/internal/identity/signerservice"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --print is the review path: an operator installing onto someone else's
// machine shows the job first. It must write NOTHING and load NOTHING.
func TestIdentitySignerInstall_PrintShowsTheJobAndTouchesNothing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	keyPath := filepath.Join(t.TempDir(), "dabir.key")

	buf, _, err := runRootCmd(t, "identity", "signer", "install", "--print",
		"--signer-key", keyPath, "--signer-socket", "/tmp/vm-s.sock")
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "<string>com.vaultmind.signer.dabir</string>",
		"the job is named after the key so two signers get two jobs")
	assert.Contains(t, out, "<string>"+keyPath+"</string>")
	assert.Contains(t, out, "<string>/tmp/vm-s.sock</string>")
	assert.Contains(t, out, "<key>KeepAlive</key><true/>")

	_, err = os.Stat(filepath.Join(home, "Library", "LaunchAgents"))
	assert.True(t, os.IsNotExist(err), "--print must not write a job")
}

// Relative paths are pinned to absolute at install time: launchd would resolve
// them against its own working directory.
func TestResolveSignerServiceSpec_PinsAbsolutePaths(t *testing.T) {
	t.Chdir(t.TempDir())
	buf, _, err := runRootCmd(t, "identity", "signer", "install", "--print",
		"--signer-key", "rel.key", "--signer-socket", "rel.sock")
	require.NoError(t, err)
	wd, err := os.Getwd()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "<string>"+filepath.Join(wd, "rel.key")+"</string>")
	assert.NotContains(t, buf.String(), "<string>rel.key</string>")
}

func TestSignerSocketLive(t *testing.T) {
	dir, err := os.MkdirTemp("", "vmsl")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "s.sock")

	assert.False(t, signerSocketLive(sock), "no socket, nothing serving")

	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	assert.True(t, signerSocketLive(sock))
}

// fakeLaunchctl puts a `launchctl` on PATH that prints msg and exits code.
func fakeLaunchctl(t *testing.T, msg string, code int) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\necho " + msg + " >&2\nexit " + strconv.Itoa(code) + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "launchctl"), []byte(script), 0o755)) //nolint:gosec // G306: test fixture must be executable
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestRunLaunchctl_FoldsOutputIntoTheError(t *testing.T) {
	fakeLaunchctl(t, "launchd says no", 3)
	err := runLaunchctl(context.Background(), launchctlBin, "bootstrap", "gui/1", "/x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "launchd says no",
		"launchd's own message is usually the only explanation there is")
}

func TestRunLaunchctl_Succeeds(t *testing.T) {
	fakeLaunchctl(t, "ok", 0)
	require.NoError(t, runLaunchctl(context.Background(), launchctlBin, "bootout", "gui/1/x"))
}

// The runner is injectable; it must never become a way to run anything else.
func TestRunLaunchctl_RefusesAnyOtherProgram(t *testing.T) {
	err := runLaunchctl(context.Background(), "sh", "-c", "true")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refusing")
}

func TestWriteSignerInstalled_SaysHowToRemoveIt(t *testing.T) {
	buf := new(bytes.Buffer)
	s := signerservice.Spec{Name: "mira", LogDir: "/L"}
	require.NoError(t, writeSignerInstalled(buf, s, 501, "/H/job.plist"))
	assert.Contains(t, buf.String(), "launchctl bootout gui/501/com.vaultmind.signer.mira")
	assert.Contains(t, buf.String(), "restarts if it dies")
}
