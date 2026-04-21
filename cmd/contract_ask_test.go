package cmd

import (
	"encoding/json"
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

// JSON-mode contract: consumers that opt into --json get a stable envelope
// they can decode with their own struct definitions (see
// contract_types_test.go). The shape of this envelope is the PUBLIC
// CONTRACT of VaultMind's CLI — changes require a schema_version bump.
func TestAskJSONContract_EnvelopeDecodesIntoConsumerShape(t *testing.T) {
	vault := indexedBaselineVault(t)
	out, _, err := runRootCmd(t, "ask", "spreading activation",
		"--vault", vault,
		"--max-items", "4", "--budget", "1500",
		"--json")
	require.NoError(t, err)

	var env AskEnvelope
	require.NoError(t, json.Unmarshal(out.Bytes(), &env),
		"--json output MUST decode cleanly into the consumer-side AskEnvelope shape")

	assert.Equal(t, "v1", env.SchemaVersion,
		"consumers rely on schema_version to branch on major-version changes")
	assert.Equal(t, "ok", env.Status)
	assert.Equal(t, "spreading activation", env.Result.Query,
		"result.query must echo the user's query — regression would lose provenance")
	assert.NotEmpty(t, env.Result.RetrievalMode,
		"result.retrieval_mode must be set ('hybrid' or 'keyword') — experiment telemetry branches on it")
	require.NotEmpty(t, env.Result.TopHits,
		"non-trivial query on the baseline vault must produce at least one hit")
	assert.NotEmpty(t, env.Result.TopHits[0].ID,
		"hit ID is the primary field consumers use to dereference notes")
}

// A missing-vault failure must still produce a decodable envelope with
// status='error' when one is emitted. Consumers that pipe failures
// through their tooling rely on the envelope shape being consistent.
func TestAskJSONContract_ErrorEnvelopeShape(t *testing.T) {
	out, _, _ := runRootCmd(t, "ask", "q",
		"--vault", "/does/not/exist",
		"--json")
	// Some error paths return a Go error; some write the envelope and
	// return nil (ErrAlreadyWritten). Contract holds when the envelope is present.
	if out.Len() == 0 {
		t.Skip("this path returned a Go error instead of writing the envelope")
	}

	var env AskEnvelope
	require.NoError(t, json.Unmarshal(out.Bytes(), &env),
		"error envelope must decode into the same shape as ok envelope — one shape, two statuses")
	assert.Equal(t, "v1", env.SchemaVersion)
	assert.Equal(t, "error", env.Status)
}
