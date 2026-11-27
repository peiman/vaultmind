package hookscripts_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/hookscripts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runArcGateScript(t *testing.T, event, stub, path string, withBinary bool) (stdout, stderr string, exit int, args, forwarded string) {
	t.Helper()
	bash, err := exec.LookPath("bash")
	require.NoError(t, err)
	dir := t.TempDir()
	scriptBody, ok := hookscripts.Get("arc-gate.sh")
	require.True(t, ok)
	script := filepath.Join(dir, "arc-gate.sh")
	require.NoError(t, os.WriteFile(script, scriptBody, 0o600))
	binDir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	inputFile := filepath.Join(dir, "input")
	if withBinary {
		hookscripts.InstallStub(t, binDir, "vaultmind", stub+"\nprintf '%s' \"$*\" > \""+argsFile+"\"\ncat > \""+inputFile+"\"\n")
	}
	env := []string{"PATH=" + binDir + ":/usr/bin:/bin", "ARC_GATE_VAULT=/tmp/arc-gate-vault", "HOME=" + t.TempDir()}
	if !withBinary {
		env[0] = "PATH=/usr/bin:/bin"
	}
	cmd := exec.Command(bash, script, event)
	cmd.Env = env
	cmd.Stdin = strings.NewReader(`{"session_id":"s"}`)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err = cmd.Run()
	if err == nil {
		exit = 0
	} else if exitErr, ok := err.(*exec.ExitError); ok {
		exit = exitErr.ExitCode()
	} else {
		require.NoError(t, err)
	}
	argsBody, _ := os.ReadFile(argsFile)
	inputBody, _ := os.ReadFile(inputFile)
	return out.String(), errOut.String(), exit, string(argsBody), string(inputBody)
}

func TestArcGateScript_ForwardsBothEventsAndInput(t *testing.T) {
	for _, event := range []string{"pretooluse", "stop"} {
		t.Run(event, func(t *testing.T) {
			out, errOut, exit, args, forwarded := runArcGateScript(t, event, "printf 'canned-reply\\n'", "", true)
			assert.Equal(t, 0, exit)
			assert.Empty(t, errOut)
			assert.Equal(t, "canned-reply\n", out)
			assert.Equal(t, "arc gate --vault /tmp/arc-gate-vault --event "+event, args)
			assert.Equal(t, `{"session_id":"s"}`, forwarded)
		})
	}
}

func TestArcGateScript_MissingOrFailingBinaryAlwaysAllows(t *testing.T) {
	out, _, exit, _, _ := runArcGateScript(t, "pretooluse", "", "", false)
	assert.Equal(t, 0, exit)
	assert.Empty(t, out)
	out, _, exit, _, _ = runArcGateScript(t, "stop", "exit 1", "", true)
	assert.Equal(t, 0, exit)
	assert.Empty(t, out)
}
