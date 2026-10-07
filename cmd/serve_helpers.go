package cmd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/peiman/vaultmind/internal/serve"
	"github.com/peiman/vaultmind/internal/xdg"
)

// envNoServe turns the warm server off: every command runs in its own
// process, as before the server existed.
const envNoServe = "VAULTMIND_NO_SERVE"

// serveConnectTimeout bounds the wait for a server that is there but slow to
// accept; a missing server fails at once.
const serveConnectTimeout = 200 * time.Millisecond

// serveRequestTimeout bounds a whole forwarded ask. A server that takes longer
// is abandoned and the ask runs here instead. A variable for tests.
var serveRequestTimeout = 10 * time.Second

// serveExec runs one request in this process as a fresh process would: in
// the request's directory, with its environment, from a reset CLI. The
// server's own directory and environment are restored afterwards.
func serveExec(req serve.Request) serve.Response {
	here, err := os.Getwd()
	if err != nil {
		return failed(fmt.Errorf("vaultmind serve: reading its own directory: %w", err))
	}
	saved := os.Environ()
	defer func() {
		_ = os.Chdir(here)
		setEnv(saved)
	}()
	if err := os.Chdir(req.Dir); err != nil {
		return failed(fmt.Errorf("cannot run in %s: %w", req.Dir, err))
	}
	os.Clearenv()
	for k, v := range req.Env {
		_ = os.Setenv(k, v)
	}
	// A command that panics skips the post-run hook that closes the
	// experiment session; close it here so its handle doesn't leak.
	defer closeExperimentSession()
	resetCLIState()
	var out, errw bytes.Buffer
	code := ExecuteWithIO(req.Args, &out, &errw)
	return serve.Response{Stdout: out.Bytes(), Stderr: errw.Bytes(), Code: code}
}

// closeExperimentSession ends and closes the open experiment session, as the
// post-run hook does.
func closeExperimentSession() {
	if experimentSession != nil {
		_ = experimentSession.DB.EndSession(experimentSession.ID)
		_ = experimentSession.DB.Close()
		experimentSession = nil
	}
}

func failed(err error) serve.Response {
	return serve.Response{Stderr: []byte("Error: " + err.Error() + "\n"), Code: 1}
}

// setEnv replaces the environment with kvs ("KEY=value" pairs).
func setEnv(kvs []string) {
	os.Clearenv()
	for _, kv := range kvs {
		if k, v, ok := strings.Cut(kv, "="); ok {
			_ = os.Setenv(k, v)
		}
	}
}

// serveSocket is where the server for this binary listens. The key holds the
// version and the executable's identity, so a client only ever reaches a
// server running its own code: after an upgrade or a rebuild it starts a new
// one, and the old one exits when idle.
func serveSocket() (string, error) {
	dir, err := xdg.StateDir()
	if err != nil {
		return "", err
	}
	info, ok := debug.ReadBuildInfo()
	v, _, _ := buildVersionInfo(Version, Commit, Date, info, ok)
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	st, err := os.Stat(exe)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%d", exe, st.Size(), st.ModTime().UnixNano())))
	return serve.SocketPath(dir, fmt.Sprintf("%s-%x", v, sum[:4])), nil
}

// tryServer answers an ask from the warm server when one is running, and
// reports whether it did. With none it starts one in the background for the
// next ask and lets this one run here; on any failure the command runs here,
// so the server can make an ask faster but never wrong.
func tryServer(args []string) (int, bool) {
	if len(args) == 0 || args[0] != "ask" || os.Getenv(envNoServe) != "" || !serveSupported {
		return 0, false
	}
	sock, err := serveSocket()
	if err != nil {
		return 0, false
	}
	dir, err := os.Getwd()
	if err != nil {
		return 0, false
	}
	env := map[string]string{}
	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok {
			env[k] = v
		}
	}
	resp, err := serve.Forward(sock, serve.Request{Args: args, Dir: dir, Env: env}, serveConnectTimeout, serveRequestTimeout)
	if errors.Is(err, serve.ErrNoServer) {
		startServer()
		return 0, false
	}
	if err != nil {
		return 0, false
	}
	_, _ = os.Stdout.Write(resp.Stdout)
	_, _ = os.Stderr.Write(resp.Stderr)
	return resp.Code, true
}

// startServer launches `vaultmind serve` detached from this process, so it
// outlives the ask that started it. A second one started at the same moment
// finds the first already serving and exits. A variable, so tests (whose
// executable is the test binary) can stand in for it.
var startServer = func() {
	if exe, err := os.Executable(); err == nil {
		spawnServer(exe)
	}
}

// spawnServer starts exe's server in its own session, without waiting for it.
func spawnServer(exe string) {
	// exe is os.Executable() and the arguments are fixed: nothing a caller
	// supplies reaches the command line.
	// nosemgrep: go-dangerous-exec -- exe is this binary; the arguments are fixed
	c := exec.Command(exe, "serve") //nolint:gosec // exe is this binary; the arguments are fixed
	c.Dir = os.TempDir()
	detach(c)
	if err := c.Start(); err != nil {
		return
	}
	_ = c.Process.Release()
}

// serveUntilIdle answers asks on this binary's socket until idle passes with
// no request or the process is signalled. A server already answering there
// is not an error: this one simply isn't needed.
func serveUntilIdle(ctx context.Context, idle string) error {
	wait, err := time.ParseDuration(idle)
	if err != nil || wait <= 0 {
		return fmt.Errorf("--idle %q: want a positive duration such as 30m", idle)
	}
	sock, err := serveSocket()
	if err != nil {
		return fmt.Errorf("finding the server's socket: %w", err)
	}
	// Each ask records its own session; the server itself is not one.
	closeExperimentSession()
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := serve.Serve(ctx, sock, serveExec, wait); err != nil && !errors.Is(err, serve.ErrAlreadyServing) {
		return err
	}
	return nil
}
