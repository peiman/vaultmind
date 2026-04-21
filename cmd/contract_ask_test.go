package cmd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AskTextContract pins the human-text output of `vaultmind ask` that the
// Workhorse persona hook consumes today. The hook invocation is:
//
//	vaultmind ask "who am I" --vault <path> --max-items 8 --budget 6000
//
// The hook captures stdout as raw text, checks exit code, and injects
// stdout into the session verbatim. Its preconditions:
//   1. Exit 0 (bash captures non-zero as failure).
//   2. Stdout is non-empty (hook checks `-n "$IDENTITY"`).
//   3. Stdout contains the recalled content (hook relies on it being
//      substantive — an empty "Search:" header with no hits would produce
//      an anemic persona injection).
//
// The fixture is the committed baseline vault, copied to a tempdir and
// indexed fresh on each test run (see indexedBaselineVault) so tests
// don't mutate committed files and concurrent runs don't race.

func TestAskTextContract_ExitZeroNonEmpty(t *testing.T) {
	vault := indexedBaselineVault(t)
	out, _, err := runRootCmd(t, "ask", "spreading activation",
		"--vault", vault,
		"--max-items", "4", "--budget", "1500")
	require.NoError(t, err, "ask must exit 0 on a well-formed query + vault")
	text := out.String()
	assert.NotEmpty(t, strings.TrimSpace(text),
		"stdout must be non-empty — the persona hook tests -n on it before injecting")
}

// The human-text output must include the 'Search:' header so a user
// reading the terminal can identify what was queried. Hook behavior is
// unchanged, but dropping the header would surprise any human debugging
// a session.
func TestAskTextContract_StdoutHasSearchHeader(t *testing.T) {
	vault := indexedBaselineVault(t)
	out, _, err := runRootCmd(t, "ask", "spreading activation",
		"--vault", vault,
		"--max-items", "4", "--budget", "1500")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "Search:",
		"human output must carry the 'Search:' header — terminal users identify queries by this line")
}

// Stdout must reference at least one hit note ID for a query that clearly
// matches. This is the non-empty-result substance guarantee: if ask ever
// started writing a header without any hits, the hook-injected persona
// would be an empty frame and the agent would get no context.
func TestAskTextContract_StdoutReferencesHitNote(t *testing.T) {
	vault := indexedBaselineVault(t)
	out, _, err := runRootCmd(t, "ask", "spreading activation",
		"--vault", vault,
		"--max-items", "4", "--budget", "1500")
	require.NoError(t, err)
	// The baseline fixture has c-spreading as the clear top match for this query.
	assert.Contains(t, out.String(), "c-spreading",
		"at-least-one hit ID must appear in stdout so injected persona is substantive")
}

// Stderr is reserved for diagnostics only; stdout must stay clean of
// structured-log noise. The hook interleaves stderr into its own error
// channel — it must never mix with the persona text.
func TestAskTextContract_StderrIsNotInjected(t *testing.T) {
	vault := indexedBaselineVault(t)
	out, _, err := runRootCmd(t, "ask", "spreading activation",
		"--vault", vault,
		"--max-items", "4", "--budget", "1500")
	require.NoError(t, err)
	// If stderr ever bleeds into stdout, the hook injects log lines as
	// "persona" which corrupts every session it fires in.
	assert.NotContains(t, out.String(), `"level":`,
		"stdout must not contain zerolog JSON log lines — those belong on stderr only")
}
