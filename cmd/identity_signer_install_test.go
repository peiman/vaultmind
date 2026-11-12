package cmd

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/peiman/vaultmind/internal/identity/signerservice"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --print is the review path: an operator installing onto someone else's
// machine shows both the launcher script and the plist before anything touches
// the filesystem. It must write NOTHING and load NOTHING.
func TestIdentitySignerInstall_PrintShowsTheJobAndTouchesNothing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	keyPath := filepath.Join(t.TempDir(), "dabir.key")

	buf, _, err := runRootCmd(t, "identity", "signer", "install", "--print",
		"--signer-key", keyPath, "--signer-socket", "/tmp/vm-s.sock")
	require.NoError(t, err)
	out := buf.String()

	// Launcher section: clearly labelled, contains the script.
	assert.Contains(t, out, "=== Launcher", "output must include a labelled launcher section")
	assert.Contains(t, out, "#!/bin/sh", "output must include the launcher script")
	assert.Contains(t, out, keyPath, "key path appears in the launcher script")

	// Plist section: clearly labelled, contains the job definition.
	assert.Contains(t, out, "=== LaunchAgent", "output must include a labelled plist section")
	assert.Contains(t, out, "<string>com.vaultmind.signer.dabir</string>",
		"the job is named after the key so two signers get two jobs")
	assert.Contains(t, out, "<key>KeepAlive</key><true/>")

	// Nothing related to the signer job may be written.
	_, err = os.Stat(filepath.Join(home, "Library", "LaunchAgents"))
	assert.True(t, os.IsNotExist(err), "--print must not write a plist")
	_, err = os.Stat(filepath.Join(home, "Library", "Application Support", "vaultmind", "launchers"))
	assert.True(t, os.IsNotExist(err), "--print must not write a launcher")
}

// Relative paths are pinned to absolute at install time: launchd would resolve
// them against its own working directory.
func TestResolveSignerServiceSpec_PinsAbsolutePaths(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Chdir(t.TempDir())
	buf, _, err := runRootCmd(t, "identity", "signer", "install", "--print",
		"--signer-key", "rel.key", "--signer-socket", "rel.sock")
	require.NoError(t, err)
	wd, err := os.Getwd()
	require.NoError(t, err)
	// The absolute path appears in the launcher script.
	assert.Contains(t, buf.String(), filepath.Join(wd, "rel.key"))
	// The bare relative form does not appear in the plist.
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

// TestIdentitySignerInstall_InstallsTheJob exercises the full install path
// (without --print) using a fake launchctl so the test never touches real
// LaunchAgents and never starts a real signer.
func TestIdentitySignerInstall_InstallsTheJob(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	keyPath := filepath.Join(t.TempDir(), "mira.key")
	fakeLaunchctl(t, "ok", 0)
	// launchctl is faked, so the install path runs on any OS; CI is Linux.
	prevOS := signerInstallOS
	signerInstallOS = runtime.GOOS
	t.Cleanup(func() { signerInstallOS = prevOS })

	buf, _, err := runRootCmd(t, "identity", "signer", "install",
		"--signer-key", keyPath, "--signer-socket", "/tmp/vm-s.sock")
	require.NoError(t, err)
	out := buf.String()

	// Installed message contains the job label and Login Items hint.
	assert.Contains(t, out, "com.vaultmind.signer.mira")
	assert.Contains(t, out, "VaultMind Signer - mira")

	// The launcher script was written.
	launcherPath := filepath.Join(home, "Library", "Application Support",
		"vaultmind", "launchers", "VaultMind Signer - mira")
	_, statErr := os.Stat(launcherPath)
	assert.NoError(t, statErr, "launcher script must exist after install")
}

// TestPrintSignerJob_PropagatesRenderError verifies that printSignerJob returns
// an error when the spec contains a relative binary path (rejected by RenderScript).
func TestPrintSignerJob_PropagatesRenderError(t *testing.T) {
	cmd := &cobra.Command{}
	spec := signerservice.Spec{
		Name:       "mira",
		Binary:     "relative-binary", // not absolute — RenderScript rejects this
		KeyPath:    "/abs/key.key",
		SocketPath: "/abs/sock",
		LogDir:     "/abs/logs",
	}
	err := printSignerJob(cmd, spec, "/abs/launchers/VaultMind Signer - mira")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "absolute")
}

// TestResolveSignerServiceSpec_RejectsInvalidName verifies that names containing
// path separators or other unsafe characters are refused before any filesystem
// operation: today --name ../../x escapes the launchers dir through filepath.Join.
func TestResolveSignerServiceSpec_RejectsInvalidName(t *testing.T) {
	cases := []struct {
		desc    string
		flag    string // non-empty: pass as --name; empty: derive from keyPath
		keyPath string // non-empty basename: use as the key filename
	}{
		{desc: "path traversal", flag: "../x"},
		{desc: "slash in name", flag: "a/b"},
		{desc: "leading dot", flag: ".hidden"},
		// ".key" → NameFromKeyPath → "" (strip the only extension, leaving empty)
		{desc: "empty derived name", keyPath: ".key"},
	}
	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			t.Setenv("XDG_DATA_HOME", t.TempDir())
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			dir := t.TempDir()
			base := "k.key"
			if tc.keyPath != "" {
				base = tc.keyPath
			}
			kp := filepath.Join(dir, base)
			args := []string{
				"identity", "signer", "install", "--print",
				"--signer-key", kp, "--signer-socket", "/tmp/vm-s.sock",
			}
			if tc.flag != "" {
				args = append(args, "--name", tc.flag)
			}
			_, _, err := runRootCmd(t, args...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "--name",
				"error must name the --name flag so the operator knows how to fix it")
		})
	}
}

// TestResolveSignerServiceSpec_AcceptsValidName verifies that a name matching
// the allowed pattern is not rejected.
func TestResolveSignerServiceSpec_AcceptsValidName(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	keyPath := filepath.Join(t.TempDir(), "k.key")
	_, _, err := runRootCmd(t, "identity", "signer", "install", "--print",
		"--name", "daana-mira",
		"--signer-key", keyPath, "--signer-socket", "/tmp/vm-s.sock")
	require.NoError(t, err)
}

func TestWriteSignerInstalled_SaysHowToRemoveIt(t *testing.T) {
	buf := new(bytes.Buffer)
	s := signerservice.Spec{Name: "mira", LogDir: "/L"}
	launcherPath := "/H/launchers/VaultMind Signer - mira"
	require.NoError(t, writeSignerInstalled(buf, s, 501, "/H/job.plist", launcherPath))
	assert.Contains(t, buf.String(), "launchctl bootout gui/501/com.vaultmind.signer.mira")
	assert.Contains(t, buf.String(), "restarts if it dies")
	assert.Contains(t, buf.String(), launcherPath, "remove instructions include the launcher path")
}
