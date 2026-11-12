package signerservice

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// LIVE-OBSERVED (2026-09-22, dabir on Siavoush's box): the signer died with a
// session reset and every signed send failed "signer unreachable" until it was
// restarted by hand. Mira's own signer had been running unsupervised for a
// month. The only supervised signer on either machine was a hand-written plist.

func spec(t *testing.T) Spec {
	t.Helper()
	return Spec{
		Name:       "mira",
		Binary:     "/Users/x/go/bin/vaultmind",
		KeyPath:    "/Users/x/Library/Application Support/vaultmind/identity-signer.key",
		SocketPath: "/Users/x/Library/Application Support/vaultmind/identity-signer.sock",
		LogDir:     "/Users/x/Library/Logs",
	}
}

// launcherPath returns the expected launcher path for the spec helper's "mira"
// signer inside a given home directory.
func launcherPathFor(home string) string {
	return LauncherAbsPath(home, "mira")
}

// TestLauncherAbsPath_PathFormation verifies the launcher lives in the expected
// Application Support subdirectory and is named after the signer.
func TestLauncherAbsPath_PathFormation(t *testing.T) {
	p := LauncherAbsPath("/Users/x", "daana-mira")
	assert.Equal(t, "/Users/x/Library/Application Support/vaultmind/launchers/VaultMind Signer - daana-mira", p)
}

// TestRenderScript_ValidSyntaxForPaths verifies that a path with an embedded
// space and one with an embedded single quote each produce a syntactically
// valid shell script (sh -n parses it cleanly).
func TestRenderScript_ValidSyntaxForPaths(t *testing.T) {
	for name, mutate := range map[string]func(*Spec){
		"space in key":  func(s *Spec) { s.KeyPath = "/Users/x/Application Support/key.key" },
		"quote in key":  func(s *Spec) { s.KeyPath = "/Users/x/it's/key.key" },
		"space in sock": func(s *Spec) { s.SocketPath = "/Users/x/Application Support/s.sock" },
	} {
		t.Run(name, func(t *testing.T) {
			s := spec(t)
			mutate(&s)
			script, err := RenderScript(s)
			require.NoError(t, err)

			f := filepath.Join(t.TempDir(), "launcher.sh")
			require.NoError(t, os.WriteFile(f, script, 0o755))
			out, err := exec.Command("sh", "-n", f).CombinedOutput()
			require.NoError(t, err, "sh -n: %s", out)
		})
	}
}

// TestRenderScript_ExecArgvRoundTrips verifies that POSIX-quoted exec lines
// reproduce the original paths in argv when the script is run. Uses a fake
// binary that prints each argument on its own line.
func TestRenderScript_ExecArgvRoundTrips(t *testing.T) {
	dir := t.TempDir()
	fakeBin := filepath.Join(dir, "print-args")
	require.NoError(t, os.WriteFile(fakeBin, []byte("#!/bin/sh\nfor a in \"$@\"; do printf '%s\\n' \"$a\"; done\n"), 0o755))

	// key path with an embedded space AND an embedded single quote — both in one run.
	keyPath := "/Users/x/Application Support/key's vault/key.key"
	sockPath := "/tmp/vaultmind.sock"

	s := Spec{
		Name:       "test",
		Binary:     fakeBin,
		KeyPath:    keyPath,
		SocketPath: sockPath,
		LogDir:     "/tmp",
	}
	script, err := RenderScript(s)
	require.NoError(t, err)

	scriptPath := filepath.Join(dir, "launcher.sh")
	require.NoError(t, os.WriteFile(scriptPath, script, 0o755))

	// Syntax check first.
	syntaxOut, syntaxErr := exec.Command("sh", "-n", scriptPath).CombinedOutput()
	require.NoError(t, syntaxErr, "sh -n: %s", syntaxOut)

	// Run the script; exec replaces the shell with fakeBin.
	out, err := exec.Command("sh", scriptPath).Output()
	require.NoError(t, err)

	got := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	want := []string{"identity", "signer", "--signer-key", keyPath, "--signer-socket", sockPath}
	require.Equal(t, want, got, "exec argv must round-trip through POSIX quoting")
}

// TestRenderScript_RejectsRelativePaths verifies that relative binary/key/socket
// paths are rejected: launchd runs with its own environment and would resolve
// them wrongly.
func TestRenderScript_RejectsRelativePaths(t *testing.T) {
	for name, mutate := range map[string]func(*Spec){
		"binary": func(s *Spec) { s.Binary = "vaultmind" },
		"key":    func(s *Spec) { s.KeyPath = "k.key" },
		"socket": func(s *Spec) { s.SocketPath = "s.sock" },
	} {
		t.Run(name, func(t *testing.T) {
			s := spec(t)
			mutate(&s)
			_, err := RenderScript(s)
			require.Error(t, err, "launchd has no working directory worth trusting")
		})
	}
}

// TestRenderPlist_ArgvIsLauncherPath verifies that the plist's ProgramArguments
// has exactly one element — the launcher path — so macOS Login Items shows the
// launcher's name instead of "vaultmind".
func TestRenderPlist_ArgvIsLauncherPath(t *testing.T) {
	launcher := "/Users/x/Library/Application Support/vaultmind/launchers/VaultMind Signer - mira"
	out, err := RenderPlist(spec(t), launcher)
	require.NoError(t, err)
	p := string(out)

	// ProgramArguments has exactly one element: the launcher path.
	assert.Contains(t, p, "    <string>"+launcher+"</string>")

	// The binary and original sub-command args are NOT in the plist; they live
	// in the launcher script so the plist stays readable as a job description.
	assert.NotContains(t, p, "<string>"+spec(t).Binary+"</string>")
	assert.NotContains(t, p, "<string>--signer-key</string>")
	assert.NotContains(t, p, "<string>--signer-socket</string>")

	// Every other plist key is still present.
	assert.Contains(t, p, "<string>"+Label("mira")+"</string>")
	assert.Contains(t, p, "<key>KeepAlive</key><true/>")
	assert.Contains(t, p, "<key>RunAtLoad</key><true/>")
}

// Restart-on-death is the entire point. Without KeepAlive a crash is the same
// outage the reset was.
func TestRenderPlist_StartsAtLoginAndRestartsOnDeath(t *testing.T) {
	launcher := launcherPathFor("/Users/x")
	out, err := RenderPlist(spec(t), launcher)
	require.NoError(t, err)
	p := string(out)
	assert.Contains(t, p, "<key>RunAtLoad</key><true/>")
	assert.Contains(t, p, "<key>KeepAlive</key><true/>")
}

// A path is data, not markup. An ampersand in the launcher path must not
// produce a plist launchd rejects — or one that means something else.
func TestRenderPlist_EscapesPaths(t *testing.T) {
	launcher := "/Users/a&b/launchers/<VaultMind Signer - mira>"
	out, err := RenderPlist(spec(t), launcher)
	require.NoError(t, err)
	assert.Contains(t, string(out), "/Users/a&amp;b/launchers/&lt;VaultMind Signer - mira&gt;")
	assert.NotContains(t, string(out), "a&b/")
}

// TestRenderPlist_RejectsRelativePaths verifies that a relative launcher path
// or log dir is rejected.
func TestRenderPlist_RejectsRelativePaths(t *testing.T) {
	for name, mutate := range map[string]func(*Spec, *string){
		"launcher": func(_ *Spec, lp *string) { *lp = "launchers/VaultMind Signer - mira" },
		"log dir":  func(s *Spec, _ *string) { s.LogDir = "Library/Logs" },
	} {
		t.Run(name, func(t *testing.T) {
			s := spec(t)
			launcher := "/abs/launchers/VaultMind Signer - mira"
			mutate(&s, &launcher)
			_, err := RenderPlist(s, launcher)
			require.Error(t, err, "launchd has no working directory worth trusting")
		})
	}
}

type recorder struct {
	calls      [][]string
	failOnVerb string
}

func (r *recorder) run(_ context.Context, name string, args ...string) error {
	r.calls = append(r.calls, append([]string{name}, args...))
	if len(args) > 0 && args[0] == r.failOnVerb {
		return errors.New(r.failOnVerb + " failed")
	}
	return nil
}

func installer(t *testing.T, r *recorder, live bool) Installer {
	t.Helper()
	return Installer{
		Home:       t.TempDir(),
		UID:        501,
		Run:        r.run,
		SocketLive: func(string) bool { return live },
	}
}

func TestInstall_WritesThePlistAndBootstrapsIt(t *testing.T) {
	r := &recorder{}
	in := installer(t, r, false)

	path, err := in.Install(context.Background(), spec(t))
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(in.Home, "Library", "LaunchAgents", Label("mira")+".plist"), path)
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())

	require.NotEmpty(t, r.calls)
	last := r.calls[len(r.calls)-1]
	assert.Equal(t, []string{"launchctl", "bootstrap", "gui/501", path}, last)

	// Launcher is also written at the correct path with the correct permissions.
	launcherInfo, err := os.Stat(in.LauncherPath("mira"))
	require.NoError(t, err, "launcher script must be written alongside the plist")
	assert.Equal(t, os.FileMode(0o755), launcherInfo.Mode().Perm())
}

// TestInstall_WritesLauncherScript verifies that the launcher script contains
// the binary, signer sub-command and key/socket paths.
func TestInstall_WritesLauncherScript(t *testing.T) {
	r := &recorder{}
	in := installer(t, r, false)

	_, err := in.Install(context.Background(), spec(t))
	require.NoError(t, err)

	content, err := os.ReadFile(in.LauncherPath("mira"))
	require.NoError(t, err)
	body := string(content)
	assert.Contains(t, body, "#!/bin/sh")
	assert.Contains(t, body, spec(t).Binary)
	assert.Contains(t, body, spec(t).KeyPath)
	assert.Contains(t, body, spec(t).SocketPath)
}

// TestInstall_SymlinkAtLauncherPathRefused verifies that a symlink at the
// target launcher path is refused — the installer must not silently follow it.
func TestInstall_SymlinkAtLauncherPathRefused(t *testing.T) {
	r := &recorder{}
	in := installer(t, r, false)

	// Pre-create the launcher dir and plant a symlink at the launcher path.
	launcherPath := in.LauncherPath("mira")
	require.NoError(t, os.MkdirAll(filepath.Dir(launcherPath), 0o700))
	require.NoError(t, os.Symlink("/dev/null", launcherPath))

	_, err := in.Install(context.Background(), spec(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink")
}

// TestInstall_SecondInstallIsNoopOnIdenticalFiles verifies that a re-run with
// the same spec does not rewrite byte-identical files; the job reload still
// happens (behaviour unchanged from before the launcher feature).
func TestInstall_SecondInstallIsNoopOnIdenticalFiles(t *testing.T) {
	r := &recorder{}
	in := installer(t, r, false)

	_, err := in.Install(context.Background(), spec(t))
	require.NoError(t, err)

	launcherPath := in.LauncherPath("mira")
	plistPath := in.PlistPath("mira")

	launcherMtime := fileMtime(t, launcherPath)
	plistMtime := fileMtime(t, plistPath)

	// Brief pause so a rewrite would produce a newer mtime.
	time.Sleep(5 * time.Millisecond)

	_, err = in.Install(context.Background(), spec(t))
	require.NoError(t, err)

	assert.Equal(t, launcherMtime, fileMtime(t, launcherPath), "launcher must not be rewritten when byte-identical")
	assert.Equal(t, plistMtime, fileMtime(t, plistPath), "plist must not be rewritten when byte-identical")
}

func fileMtime(t *testing.T, path string) time.Time {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	return info.ModTime()
}

// Re-running install must replace a loaded job, not fail with "already loaded".
func TestInstall_IsIdempotent(t *testing.T) {
	r := &recorder{failOnVerb: "bootout"} // nothing loaded yet: bootout fails, harmlessly
	in := installer(t, r, false)

	_, err := in.Install(context.Background(), spec(t))
	require.NoError(t, err, "a failing bootout means nothing was loaded — not an error")
	assert.Equal(t, []string{"launchctl", "bootout", "gui/501/" + Label("mira")}, r.calls[0])
}

// THE TRAP. A signer already serving the socket (started by hand) makes the
// launchd copy fail to bind, and KeepAlive turns that into a restart loop that
// looks like a running service. Refuse and say what to do.
func TestInstall_RefusesWhileAnotherSignerServesTheSocket(t *testing.T) {
	r := &recorder{}
	in := installer(t, r, true)

	_, err := in.Install(context.Background(), spec(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), spec(t).SocketPath)
	assert.Contains(t, err.Error(), "stop it")
	assert.Empty(t, r.calls, "nothing may be loaded on the refusal path")
	_, statErr := os.Stat(filepath.Join(in.Home, "Library", "LaunchAgents", Label("mira")+".plist"))
	assert.True(t, os.IsNotExist(statErr), "nothing may be written on the refusal path")
}

func TestInstall_ABootstrapFailureIsReported(t *testing.T) {
	r := &recorder{failOnVerb: "bootstrap"}
	in := installer(t, r, false)

	_, err := in.Install(context.Background(), spec(t))
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "bootstrap"))
}

// TestWriteFileIfChanged_FixesModeWhenBytesMatch verifies that writeFileIfChanged
// corrects a wrong file mode even when the content is byte-identical. Without
// this fix, a file with correct bytes but wrong mode (e.g. 0600 instead of
// 0644) would be silently left with the wrong permissions.
func TestWriteFileIfChanged_FixesModeWhenBytesMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.plist")
	content := []byte("content")

	// Write with wrong mode first.
	require.NoError(t, os.WriteFile(path, content, 0o600))

	// writeFileIfChanged with correct mode: bytes match, but mode must be fixed.
	require.NoError(t, writeFileIfChanged(path, content, 0o644))

	fi, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), fi.Mode().Perm(),
		"mode must be corrected even when bytes are identical")
}

// TestWriteAtomicIfChanged_FixesModeWhenBytesMatch verifies that
// writeAtomicIfChanged corrects a wrong file mode even when the content is
// byte-identical.
func TestWriteAtomicIfChanged_FixesModeWhenBytesMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "launcher")
	content := []byte("content")

	// Write with wrong mode first.
	require.NoError(t, os.WriteFile(path, content, 0o600))

	// writeAtomicIfChanged with correct mode: bytes match, but mode must be fixed.
	require.NoError(t, writeAtomicIfChanged(dir, path, content, 0o755))

	fi, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), fi.Mode().Perm(),
		"mode must be corrected even when bytes are identical")
}

func TestLabel_IsStableAndNamespaced(t *testing.T) {
	assert.Equal(t, "com.vaultmind.signer.mira", Label("mira"))
}

func TestNameFromKeyPath(t *testing.T) {
	assert.Equal(t, "mira", NameFromKeyPath("/a/b/mira.key"))
	assert.Equal(t, "identity-signer", NameFromKeyPath("/a/identity-signer.key"))
}

// TestInstall_RejectsSymlinkedLauncherDir verifies that a symlink at the
// launcher directory is refused — the installer must not silently follow it.
func TestInstall_RejectsSymlinkedLauncherDir(t *testing.T) {
	r := &recorder{}
	in := installer(t, r, false)

	// Plant a symlink where the launcher directory should live.
	launcherDir := filepath.Dir(in.LauncherPath("mira"))
	require.NoError(t, os.MkdirAll(filepath.Dir(launcherDir), 0o700))
	require.NoError(t, os.Symlink("/tmp", launcherDir))

	_, err := in.Install(context.Background(), spec(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink")
	assert.Contains(t, err.Error(), "dir")
}

// TestInstall_PropagatesRenderScriptError verifies that a spec with a relative
// binary path is rejected by RenderScript and the error is propagated.
func TestInstall_PropagatesRenderScriptError(t *testing.T) {
	r := &recorder{}
	in := installer(t, r, false)

	s := spec(t)
	s.Binary = "relative-binary" // not absolute — RenderScript rejects this

	_, err := in.Install(context.Background(), s)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "absolute")
}

// TestInstall_PropagatesRenderPlistError verifies that a spec with a relative
// log dir is rejected by RenderPlist and the error is propagated.
func TestInstall_PropagatesRenderPlistError(t *testing.T) {
	r := &recorder{}
	in := installer(t, r, false)

	s := spec(t)
	s.LogDir = "relative-logs" // not absolute — RenderPlist rejects this

	_, err := in.Install(context.Background(), s)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "absolute")
}

// TestInstall_WriteAtomicFails verifies that an error from writeAtomicIfChanged
// (caused by an unwritable launcher dir) is propagated correctly.
func TestInstall_WriteAtomicFails(t *testing.T) {
	r := &recorder{}
	in := installer(t, r, false)

	// Create the launcher dir and then remove write permission so os.CreateTemp
	// inside writeAtomicIfChanged fails.
	launcherDir := filepath.Dir(in.LauncherPath("mira"))
	require.NoError(t, os.MkdirAll(launcherDir, 0o700))
	require.NoError(t, os.Chmod(launcherDir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(launcherDir, 0o700) })

	_, err := in.Install(context.Background(), spec(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "writing launcher")
}

// TestInstall_PlistDirCreationFails verifies that placing a regular file at the
// LaunchAgents path causes the dir-creation step to fail and the error propagates.
func TestInstall_PlistDirCreationFails(t *testing.T) {
	r := &recorder{}
	in := installer(t, r, false)

	// A file at the LaunchAgents path blocks os.MkdirAll.
	agentsDir := filepath.Join(in.Home, launchAgentDir)
	require.NoError(t, os.MkdirAll(filepath.Dir(agentsDir), 0o755))
	require.NoError(t, os.WriteFile(agentsDir, []byte("block"), 0o600))

	_, err := in.Install(context.Background(), spec(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "creating")
}

// TestInstall_RefusesUnsafeLauncherDirPerms verifies that a pre-existing launcher
// directory with group- or other-writable bits (0777) is refused: an attacker
// who can write there could replace the launcher before launchd runs it.
func TestInstall_RefusesUnsafeLauncherDirPerms(t *testing.T) {
	r := &recorder{}
	in := installer(t, r, false)

	launcherDir := filepath.Dir(in.LauncherPath("mira"))
	require.NoError(t, os.MkdirAll(launcherDir, 0o700))
	// Explicitly set group/other-writable bits; MkdirAll alone is masked by umask.
	require.NoError(t, os.Chmod(launcherDir, 0o777))
	t.Cleanup(func() { _ = os.Chmod(launcherDir, 0o700) })

	_, err := in.Install(context.Background(), spec(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permissions",
		"error must mention permissions so the operator knows what to fix")
}

// TestInstall_PlistWriteFails verifies that an unwritable LaunchAgents directory
// causes the plist write to fail and the error propagates.
func TestInstall_PlistWriteFails(t *testing.T) {
	r := &recorder{}
	in := installer(t, r, false)

	// Create the LaunchAgents dir and then remove write permission.
	agentsDir := filepath.Join(in.Home, launchAgentDir)
	require.NoError(t, os.MkdirAll(agentsDir, 0o755))
	require.NoError(t, os.Chmod(agentsDir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(agentsDir, 0o755) })

	_, err := in.Install(context.Background(), spec(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "writing")
}
