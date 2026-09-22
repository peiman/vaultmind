package signerservice

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

// The job must launch exactly the signer the operator would start by hand:
// absolute binary, and the key/socket passed EXPLICITLY. Relying on defaults
// inside launchd would resolve them under launchd's environment, not the
// operator's — which is how a native-path-mode signer would quietly come up on
// the XDG paths, serving a socket nobody dials.
func TestRenderPlist_LaunchesTheExactSignerWithExplicitPaths(t *testing.T) {
	out, err := RenderPlist(spec(t))
	require.NoError(t, err)
	p := string(out)

	for _, want := range []string{
		"<string>" + Label("mira") + "</string>",
		"<string>/Users/x/go/bin/vaultmind</string>",
		"<string>identity</string>", "<string>signer</string>",
		"<string>--signer-key</string>",
		"<string>/Users/x/Library/Application Support/vaultmind/identity-signer.key</string>",
		"<string>--signer-socket</string>",
		"<string>/Users/x/Library/Application Support/vaultmind/identity-signer.sock</string>",
	} {
		assert.Contains(t, p, want)
	}
}

// Restart-on-death is the entire point. Without KeepAlive a crash is the same
// outage the reset was.
func TestRenderPlist_StartsAtLoginAndRestartsOnDeath(t *testing.T) {
	out, err := RenderPlist(spec(t))
	require.NoError(t, err)
	p := string(out)
	assert.Contains(t, p, "<key>RunAtLoad</key><true/>")
	assert.Contains(t, p, "<key>KeepAlive</key><true/>")
}

// A path is data, not markup. An ampersand in a home directory must not
// produce a plist launchd rejects — or one that means something else.
func TestRenderPlist_EscapesPaths(t *testing.T) {
	s := spec(t)
	s.KeyPath = "/Users/a&b/<k>.key"
	out, err := RenderPlist(s)
	require.NoError(t, err)
	assert.Contains(t, string(out), "/Users/a&amp;b/&lt;k&gt;.key")
	assert.NotContains(t, string(out), "a&b/<k>")
}

func TestRenderPlist_RejectsRelativePaths(t *testing.T) {
	for name, mutate := range map[string]func(*Spec){
		"binary": func(s *Spec) { s.Binary = "vaultmind" },
		"key":    func(s *Spec) { s.KeyPath = "k.key" },
		"socket": func(s *Spec) { s.SocketPath = "s.sock" },
	} {
		t.Run(name, func(t *testing.T) {
			s := spec(t)
			mutate(&s)
			_, err := RenderPlist(s)
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

func TestLabel_IsStableAndNamespaced(t *testing.T) {
	assert.Equal(t, "com.vaultmind.signer.mira", Label("mira"))
}

func TestNameFromKeyPath(t *testing.T) {
	assert.Equal(t, "mira", NameFromKeyPath("/a/b/mira.key"))
	assert.Equal(t, "identity-signer", NameFromKeyPath("/a/identity-signer.key"))
}
