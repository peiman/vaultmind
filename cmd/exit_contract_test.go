package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The --json exit-code contract.
//
// When a command fails after deciding to speak JSON, it writes an error
// envelope and returns cmdutil.ErrAlreadyWritten so the envelope is not printed
// twice. Every such command then translated that sentinel into `return nil`,
// which made the PROCESS exit 0 while the envelope it had just written said
// status "error".
//
// So `vaultmind ask --json ... || handle_failure` never fired, and any hook or
// script wrapping a --json call read success on every failure. This is the
// silent-failure pattern VaultMind exists to argue against, sitting in its own
// agent-facing contract — and --json is the agent-facing surface, so it is the
// one place the contract most has to hold.
//
// The sentinel must therefore travel all the way out (main.go maps it to exit 1
// without writing a second envelope) rather than being swallowed at the command.
// These cases pin that across a representative spread of commands, not just the
// one where it was found.
func TestJSONErrorMustNotReportSuccess(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"ask", []string{"ask", "q", "--vault", "/does/not/exist", "--json"}},
		{"search", []string{"search", "q", "--vault", "/does/not/exist", "--json"}},
		{"note get", []string{"note", "get", "some-id", "--vault", "/does/not/exist", "--json"}},
		{"memory links", []string{"memory", "links", "some-id", "--vault", "/does/not/exist", "--json"}},
		{"memory pack", []string{"memory", "pack", "some-id", "--vault", "/does/not/exist", "--json"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, _, err := runRootCmd(t, tc.args...)

			// The envelope is the consumer-facing half of the contract...
			require.NotEmpty(t, out.Bytes(), "a --json failure must still emit an envelope")
			var env envelope.Envelope
			require.NoError(t, json.Unmarshal(out.Bytes(), &env))
			require.Equal(t, "error", env.Status)

			// ...and the exit code is the other half. A caller that checks only
			// one of them must not be able to reach the wrong conclusion.
			require.Error(t, err,
				"envelope says error, so the command must return one too — otherwise the process exits 0")
			assert.True(t, errors.Is(err, cmdutil.ErrAlreadyWritten),
				"the sentinel must survive to main, which maps it to exit 1 without printing twice")
		})
	}
}

// The cases above all fail at vault-open, which was the only failure the
// contract originally covered — so the fix reached only that one. Everything
// below fails AFTER a working vault is open: a missing id, an ambiguous title,
// an unreadable plan. Those are the everyday failures an agent actually hits,
// and every one of them wrote status "error" and exited 0.
func TestJSONErrorMustNotReportSuccess_AfterTheVaultOpens(t *testing.T) {
	vault := buildIndexedTestVault(t)
	badPlan := filepath.Join(t.TempDir(), "bad.json")
	require.NoError(t, os.WriteFile(badPlan, []byte("{not json"), 0o600))

	cases := []struct {
		name string
		args []string
		code string
	}{
		{"note get: unknown id", []string{"note", "get", "no-such-id", "--vault", vault, "--json"}, "not_found"},
		{"memory links: unresolvable", []string{"memory", "links", "no-such-id", "--vault", vault, "--json"}, ""},
		{"memory pack: unresolvable", []string{"memory", "pack", "no-such-id", "--vault", vault, "--json"}, ""},
		{"apply: unreadable plan", []string{"apply", "/does/not/exist.json", "--vault", vault, "--json"}, "read_error"},
		{"apply: unparseable plan", []string{"apply", badPlan, "--vault", vault, "--json"}, "parse_error"},
		{"note create: path traversal", []string{"note", "create", "../out.md", "--type", "concept",
			"--field", "title=T", "--vault", vault, "--json"}, "path_traversal"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, _, err := runRootCmd(t, tc.args...)

			var env envelope.Envelope
			require.NoError(t, json.Unmarshal(out.Bytes(), &env))
			require.Equal(t, "error", env.Status)
			if tc.code != "" {
				require.NotEmpty(t, env.Errors)
				assert.Equal(t, tc.code, env.Errors[0].Code)
			}

			require.Error(t, err, "the envelope says error; the exit code must agree")
			assert.True(t, errors.Is(err, cmdutil.ErrAlreadyWritten),
				"and the sentinel must survive to main so the failure is described exactly once")
		})
	}
}

// Text mode is the other half, and the half that looked like a judgement call.
// `vaultmind note get "$id" || fallback` took the found path on every typo. The
// friendly line is still printed — the exit code is what changes.
func TestTextModeNotFoundAlsoFails(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "note", "get", "no-such-id", "--vault", vault)

	assert.Contains(t, out.String(), "No note found",
		"the human-readable line stays — this is not a regression in message quality")
	require.Error(t, err, "and a missing note is a failure in text mode too")
	assert.True(t, errors.Is(err, cmdutil.ErrAlreadyWritten),
		"the sentinel means main sets the exit code without printing the failure a second time")
}

// The success path must stay quiet: no error, so exit 0.
func TestJSONSuccessStillReportsSuccess(t *testing.T) {
	out, _, err := runRootCmd(t, "ask", "spreading activation", "--vault", indexedBaselineVault(t), "--json")
	require.NoError(t, err, "a successful --json query must not return an error")

	var env envelope.Envelope
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "ok", env.Status)
}
