package cmd

import (
	"encoding/json"
	"errors"
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

// The success path must stay quiet: no error, so exit 0.
func TestJSONSuccessStillReportsSuccess(t *testing.T) {
	out, _, err := runRootCmd(t, "ask", "spreading activation", "--vault", indexedBaselineVault(t), "--json")
	require.NoError(t, err, "a successful --json query must not return an error")

	var env envelope.Envelope
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "ok", env.Status)
}
