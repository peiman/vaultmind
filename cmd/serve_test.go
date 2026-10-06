package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/serve"
	"github.com/peiman/vaultmind/internal/testvault"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// env is the environment a request carries: the test's own, which holds the
// isolated XDG dirs, plus extra.
func env(extra map[string]string) map[string]string {
	out := map[string]string{}
	for _, kv := range os.Environ() {
		for i := 0; i < len(kv); i++ {
			if kv[i] == '=' {
				out[kv[:i]] = kv[i+1:]
				break
			}
		}
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

// A request's answer does not depend on what the server ran before it: not
// on earlier flags, working directory, environment or config file. The server
// resets the CLI's state between requests, so a warm process answers what a
// fresh one would.
func TestServeExec_ARequestIsAnsweredAsInAFreshProcess(t *testing.T) {
	vault := testvault.IndexedFixtureVault(t)
	here, err := os.Getwd()
	require.NoError(t, err)
	plain := serve.Request{Args: []string{"ask", "spreading activation", "--vault", vault}, Dir: here, Env: env(nil)}

	first := serveExec(plain)
	require.Equal(t, 0, first.Code, string(first.Stderr))

	// A project whose config.yaml turns on JSON output, asked from its own
	// directory, with other flags and environment.
	project := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(project, "config.yaml"), []byte("app:\n  output_format: json\n"), 0o600))
	other := serveExec(serve.Request{
		Args: []string{"ask", "memory", "--vault", vault, "--pointers-only", "--max-items", "1"},
		Dir:  project, Env: env(map[string]string{"VAULTMIND_CALLER": "other"}),
	})
	require.Equal(t, 0, other.Code, string(other.Stderr))
	require.Contains(t, string(other.Stdout), `"status"`, "the project's config applied to its own request")

	again := serveExec(plain)
	assert.Equal(t, string(first.Stdout), string(again.Stdout), "no flag, directory or config leaked from the request between")
	assert.Equal(t, 0, again.Code)
}

// The server's own directory and environment are back as they were after a
// request.
func TestServeExec_RestoresTheServersDirAndEnv(t *testing.T) {
	here, err := os.Getwd()
	require.NoError(t, err)
	before := os.Getenv("VAULTMIND_SERVE_PROBE")
	elsewhere := t.TempDir()

	serveExec(serve.Request{Args: []string{"--version"}, Dir: elsewhere, Env: env(map[string]string{"VAULTMIND_SERVE_PROBE": "set"})})

	now, err := os.Getwd()
	require.NoError(t, err)
	assert.Equal(t, here, now)
	assert.Equal(t, before, os.Getenv("VAULTMIND_SERVE_PROBE"))
}

// A command that fails exits non-zero with its message, as main reports it,
// even right after a request in JSON mode: an unknown command fails before
// the output mode is set, so a mode left over would change how it reads.
func TestServeExec_AFailedCommandIsReportedAsMainWould(t *testing.T) {
	here, err := os.Getwd()
	require.NoError(t, err)
	inJSON := serveExec(serve.Request{Args: []string{"--output-format", "json", "ping"}, Dir: here, Env: env(nil)})
	require.Contains(t, string(inJSON.Stdout), `"status"`, "the request before ran in JSON mode")
	resp := serveExec(serve.Request{Args: []string{"no-such-command"}, Dir: here, Env: env(nil)})
	assert.Equal(t, 1, resp.Code)
	assert.Contains(t, string(resp.Stderr), "Error:")
	assert.Empty(t, string(resp.Stdout), "reported as text, not as a JSON envelope")
}

// A request whose directory is gone fails cleanly instead of running
// somewhere else.
func TestServeExec_AMissingDirIsAnError(t *testing.T) {
	resp := serveExec(serve.Request{Args: []string{"--version"}, Dir: filepath.Join(t.TempDir(), "gone"), Env: env(nil)})
	assert.Equal(t, 1, resp.Code)
	assert.Contains(t, string(resp.Stderr), "gone")
}

// After a reset, a command's flags feed its config keys again, as they do in
// a fresh process: viper.Reset drops the bindings made at registration, and
// code reading a key from viper directly would otherwise miss the flag.
func TestResetCLIState_RebindsCommandFlags(t *testing.T) {
	saved := flagBindings
	t.Cleanup(func() { flagBindings = saved; resetCLIState() })
	fs := pflag.NewFlagSet("probe", pflag.ContinueOnError)
	fs.Int("max-items", 3, "")
	flagBindings = []flagBinding{{key: "app.probe.max_items", flag: fs.Lookup("max-items")}}

	resetCLIState()
	require.NoError(t, fs.Set("max-items", "7"))
	assert.Equal(t, 7, viper.GetInt("app.probe.max_items"))
}
