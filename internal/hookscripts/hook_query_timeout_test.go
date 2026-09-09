package hookscripts_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/hookscripts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The two query hooks bound the vault query so a slow answer degrades to
// silence instead of stalling the turn. `timeout` is GNU coreutils: it is on
// every Linux box and on NO stock macOS — it arrives only via Homebrew, under
// either name. So the bound has to be resolved, not assumed.
//
// Assuming it cost us both halves. vault-reach.sh hardcoded `timeout 10`, so on
// a stock Mac the command did not exist, the substitution came back empty, and
// the hook logged `"matched":true,"injected":false` — indistinguishable in the
// log from "the vault had nothing to say". A hook that is silent because a
// binary is missing, reporting the same shape as a hook that is silent because
// it decided to be, is the failure class this project keeps finding.
// vault-recall.sh had the opposite bug: no bound at all, so under machine load
// it ran past Claude Code's 30s hook budget and got killed at the harness
// boundary with its output discarded — 30 seconds spent to inject nothing.

// hookEnv builds an environment for a hook script run, with a stub `vaultmind`
// on PATH and control over whether a `timeout` binary is visible.
type hookEnv struct {
	binDir     string
	projectDir string
	home       string
}

func newHookEnv(t *testing.T, stub string) hookEnv {
	t.Helper()
	binDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "vaultmind"), []byte(stub), 0o700))

	projectDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "vaultmind-identity"), 0o750))

	return hookEnv{binDir: binDir, projectDir: projectDir, home: t.TempDir()}
}

// env returns the process environment. PATH deliberately contains ONLY the stub
// directory plus the minimal system dirs, so `timeout` is absent unless the test
// puts it there — reproducing stock macOS on a machine that has Homebrew.
func (h hookEnv) env(withTimeout bool) []string {
	path := h.binDir + ":/usr/bin:/bin"
	if withTimeout {
		if p, err := exec.LookPath("timeout"); err == nil {
			path = filepath.Dir(p) + ":" + path
		}
	}
	return []string{
		"PATH=" + path,
		"CLAUDE_PROJECT_DIR=" + h.projectDir,
		"HOME=" + h.home,
	}
}

func runHookScript(t *testing.T, name string, env []string, stdin string) (string, time.Duration) {
	t.Helper()
	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}
	script, err := filepath.Abs(name)
	require.NoError(t, err)

	cmd := exec.Command(bashPath, script)
	cmd.Env = env
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb

	start := time.Now()
	runErr := cmd.Run()
	elapsed := time.Since(start)
	require.NoErrorf(t, runErr, "%s must always exit 0 (stderr: %s)", name, errb.String())
	return out.String(), elapsed
}

// A stub that answers instantly. Any hook that produces nothing with this stub
// in place failed for a reason other than the vault having nothing to say.
const fastStub = "#!/bin/bash\nif [ \"$1\" = ask ]; then echo '  0.42  some-note   A Note'; fi\nexit 0\n"

// A stub that outlives any sane bound. `exec` matters: timeout signals its
// direct child, so a plain `sleep` would be orphaned by the kill and keep the
// inherited stdout pipe open, hanging the command substitution long after the
// bound fired — and leaking a process into every test that runs after this one.
// The real binary does not fork, so this is a stub artifact, not the hook's.
const slowStub = "#!/bin/bash\nif [ \"$1\" = ask ]; then exec sleep 30; fi\nexit 0\n"

func TestReachHook_InjectsWhenTimeoutBinaryIsAbsent(t *testing.T) {
	h := newHookEnv(t, fastStub)
	payload := `{"tool_name":"Bash","tool_input":{"command":"git commit -m x"}}`

	out, _ := runHookScript(t, "vault-reach.sh", h.env(false), payload)

	require.NotEmpty(t, out,
		"reach hook went silent with no `timeout` on PATH — on stock macOS that is every reach, "+
			"logged as if the vault had nothing to say")
	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &parsed))
	assert.Contains(t, out, "some-note")
}

func TestReachHook_StillInjectsWhenTimeoutBinaryIsPresent(t *testing.T) {
	h := newHookEnv(t, fastStub)
	payload := `{"tool_name":"Bash","tool_input":{"command":"git commit -m x"}}`

	out, _ := runHookScript(t, "vault-reach.sh", h.env(true), payload)

	assert.Contains(t, out, "some-note", "bounding the query must not change the fast path")
}

// A timeout must be VISIBLE, not silent.
//
// This test used to assert the output was empty. Its real intent — stated in
// its own message — was "no PARTIAL pointers", and that still holds; but
// silence was the wrong way to get it. Measured 2026-09-09: with three vaults
// the federated query took 15.9s against a 15s bound, so the hook was killed
// and injected nothing while exiting 0. Nothing in the turn distinguished
// "your memory was consulted and had nothing" from "your memory was never
// consulted". One line costs nothing and is the difference between a bounded
// query and an invisible one.
func TestRecallHook_SlowQueryReportsTheTimeoutInsteadOfVanishing(t *testing.T) {
	if _, err := exec.LookPath("timeout"); err != nil {
		t.Skip("no timeout binary; the bound cannot be enforced on this machine")
	}
	h := newHookEnv(t, slowStub)
	payload, err := json.Marshal(map[string]string{"prompt": "what did we decide about retries?", "session_id": "test"})
	require.NoError(t, err)

	env := append(h.env(true), "VAULTMIND_HOOK_QUERY_TIMEOUT=1")
	out, elapsed := runHookScript(t, "vault-recall.sh", env, string(payload))

	assert.NotContains(t, out, "some-note",
		"a query that outran its bound must not inject partial pointers")
	assert.Contains(t, out, "recall timed out",
		"a bounded query that was killed must say so; silence is indistinguishable from an empty vault")
	assert.Less(t, elapsed, 15*time.Second,
		"recall must return on its own bound; unbounded it runs until the harness kills it at 30s "+
			"and discards the output")
}

// A genuine no-match stays QUIET. The visible-timeout rule above must not turn
// every uneventful turn into noise: --quiet-on-no-match exists because most
// prompts have nothing to recall, and that is not a failure.
func TestRecallHook_GenuineNoMatchStaysSilent(t *testing.T) {
	// Exits 0 with no output, exactly as --quiet-on-no-match does.
	h := newHookEnv(t, "#!/bin/bash\nexit 0\n")
	payload, err := json.Marshal(map[string]string{"prompt": "what did we decide about retries?", "session_id": "test"})
	require.NoError(t, err)

	out, _ := runHookScript(t, "vault-recall.sh", h.env(true), string(payload))

	assert.Empty(t, strings.TrimSpace(out),
		"nothing to recall is the common case and must not announce itself")
}

// The reach hook had the same silence, one level worse: it never captured the
// exit status at all, so a killed query and an empty vault produced the same
// log line — `"matched":true,"injected":false` — which is verbatim the failure
// this file's own header describes. That was fixed for the missing-`timeout`-
// binary case and left open for the timed-out case.
func TestReachHook_SlowQueryReportsTheTimeoutInsteadOfVanishing(t *testing.T) {
	if _, err := exec.LookPath("timeout"); err != nil {
		t.Skip("no timeout binary; the bound cannot be enforced on this machine")
	}
	h := newHookEnv(t, slowStub)
	// reach fires on the allowlisted Bash commands, not on reads.
	payload := `{"tool_name":"Bash","tool_input":{"command":"git commit -m x"}}`

	env := append(h.env(true), "VAULTMIND_HOOK_QUERY_TIMEOUT=1")
	out, elapsed := runHookScript(t, "vault-reach.sh", env, payload)

	assert.Contains(t, out, "reach timed out",
		"a bounded reach that was killed must say so; silence reads as an empty vault")
	assert.Less(t, elapsed, 15*time.Second, "reach must return on its own bound")
}

func TestRecallHook_BoundDoesNotClipAFastQuery(t *testing.T) {
	h := newHookEnv(t, fastStub)
	payload, err := json.Marshal(map[string]string{"prompt": "what did we decide about retries?", "session_id": "test"})
	require.NoError(t, err)

	out, _ := runHookScript(t, "vault-recall.sh", h.env(true), string(payload))

	assert.Contains(t, out, "some-note")
}

// MUTATION-REVIEW FINDING (2026-09-06): the two tests that lived here asserted
// only that the string "VAULTMIND_USER_SESSION_ID" appears in a shell script.
// Verified by mutation: they pass if the hook forwards a hardcoded constant,
// and they pass if it reads the wrong JSON key (`sessionId`). One of them
// looked "killed" only because a script-drift test noticed the edit — an
// incidental kill that vanishes if both copies are mutated together.
//
// Behaviour, then. A stub binary echoes the env var it received, so the test
// asserts the id the BINARY sees, not the text of the script that sends it.

// envEchoStub answers `ask` by printing one env var's value, so a test can
// assert what the hook actually forwarded.
func envEchoStub(varName string) string {
	return "#!/bin/bash\nif [ \"$1\" = ask ]; then printf '%s\\n' \"$" + varName + "\"; fi\nexit 0\n"
}

func TestRecallHook_ForwardsTheRealSessionIDFromThePayload(t *testing.T) {
	h := newHookEnv(t, envEchoStub("VAULTMIND_USER_SESSION_ID"))
	stdin, err := json.Marshal(map[string]string{
		"prompt":     "what do I know about spreading activation in memory systems",
		"session_id": "conv-abc-123",
	})
	require.NoError(t, err)

	out, _ := runHookScript(t, "vault-recall.sh", h.env(false), string(stdin))
	require.Contains(t, out, "conv-abc-123",
		"the harness id must reach the binary — a hardcoded constant or the wrong JSON key must not pass")
}

func TestReachHook_ForwardsTheRealSessionIDFromThePayload(t *testing.T) {
	h := newHookEnv(t, envEchoStub("VAULTMIND_USER_SESSION_ID"))
	stdin, err := json.Marshal(map[string]any{
		"session_id": "conv-xyz-789",
		"tool_input": map[string]string{"command": "git commit -m wip"},
	})
	require.NoError(t, err)

	out, _ := runHookScript(t, "vault-reach.sh", h.env(false), string(stdin))
	require.Contains(t, out, "conv-xyz-789")
}

// A payload with no session_id must degrade to the heuristic, not forward a
// literal empty string that groups every session under "".
func TestReachHook_NoSessionIDInPayloadForwardsNothingHarmful(t *testing.T) {
	h := newHookEnv(t, envEchoStub("VAULTMIND_USER_SESSION_ID"))
	stdin, err := json.Marshal(map[string]any{
		"tool_input": map[string]string{"command": "git commit -m wip"},
	})
	require.NoError(t, err)

	out, _ := runHookScript(t, "vault-reach.sh", h.env(false), string(stdin))
	require.NotContains(t, out, "conv-", "no id in the payload means no id forwarded")
}

// Federation only matters if the HOOKS use it. The measured 1-of-4 coverage
// failure was not a CLI limitation — it was that vault-recall.sh and
// vault-reach.sh each searched exactly one vault, so two findings sat in the
// desk, retrievable there, unreachable from where the agent works.
//
// VAULTMIND_VAULTS is additive: unset ⇒ today's single-vault behaviour, so no
// existing adopter changes.

func TestRecallHook_FederatesWhenVaultsListIsSet(t *testing.T) {
	body := hookScriptBody(t, "vault-recall.sh")
	require.Contains(t, body, "VAULTMIND_VAULTS",
		"the recall hook must be able to search more than the one vault it was born with")
	require.Contains(t, body, "--vaults",
		"and must pass the list to the binary, not just read the variable")
}

func TestReachHook_FederatesWhenVaultsListIsSet(t *testing.T) {
	body := hookScriptBody(t, "vault-reach.sh")
	require.Contains(t, body, "VAULTMIND_VAULTS")
	require.Contains(t, body, "--vaults")
}

// Behavioural, not textual: with the variable unset the hook must call the
// binary WITHOUT --vaults, or every existing single-vault adopter silently
// changes behaviour on upgrade.
func TestRecallHook_UnsetVaultsListKeepsTheSingleVaultCall(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "argv.log")
	h := newHookEnv(t, argvRecordingStub(logPath))
	stdin, err := json.Marshal(map[string]string{
		"prompt":     "what do I know about spreading activation in memory systems",
		"session_id": "conv-1",
	})
	require.NoError(t, err)

	runHookScript(t, "vault-recall.sh", h.env(false), string(stdin))

	raw, err := os.ReadFile(logPath) //nolint:gosec // temp path owned by this test
	require.NoError(t, err)
	require.NotContains(t, string(raw), "--vaults",
		"no VAULTMIND_VAULTS ⇒ no federation flag ⇒ unchanged behaviour")
}

// hookScriptBody returns the EMBEDDED script — the copy that actually ships —
// rather than whatever sits in the working directory, so a test cannot pass
// against a local edit that never reaches an adopter.
func hookScriptBody(t *testing.T, name string) string {
	t.Helper()
	body, ok := hookscripts.Get(name)
	require.True(t, ok, "embedded script %s must exist", name)
	return string(body)
}
