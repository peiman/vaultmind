package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// envelopeShape is a minimal decoder for the common JSON envelope —
// many commands share it. Each test asserts on the fields that matter
// for its behavior rather than re-decoding into per-command structs.
type envelopeShape struct {
	Status string          `json:"status"`
	Result json.RawMessage `json:"result"`
	Meta   struct {
		VaultPath string `json:"vault_path"`
		IndexHash string `json:"index_hash"`
	} `json:"meta"`
}

// note get must return the specific note the user asked for — a regression
// that returned a different note would silently corrupt any caller using
// note IDs as keys (including Workhorse's persona hook).
func TestNoteGet_ReturnsRequestedNote(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "note", "get", "concept-alpha", "--vault", vault, "--json")
	require.NoError(t, err)

	var env envelopeShape
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "ok", env.Status)
	assert.Contains(t, string(env.Result), "concept-alpha")
	assert.Contains(t, string(env.Result), "Alpha Concept")
	assert.NotContains(t, string(env.Result), "proj-beta", "note get must return the requested note, not a related one")
}

// note get with an unknown id must surface a "not_found" error envelope —
// a silent success or missing error code would mask typos and bad ID
// pipelines. The process exit is left non-zero by convention elsewhere; here
// we assert the structured signal the envelope carries.
func TestNoteGet_UnknownIDProducesNotFoundEnvelope(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "note", "get", "does-not-exist", "--vault", vault, "--json")
	require.NoError(t, err) // RunNoteGet writes the error envelope; it does not return a Go error

	var env struct {
		Status string `json:"status"`
		Errors []struct {
			Code string `json:"code"`
		} `json:"errors"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "error", env.Status)
	require.NotEmpty(t, env.Errors)
	assert.Equal(t, "not_found", env.Errors[0].Code)
}

// note mget returns found notes AND a separate not_found list — losing
// that split would hide missing IDs from callers that batch thousands of
// lookups.
func TestNoteMget_PartitionsFoundAndNotFound(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "note", "mget",
		"--ids", "concept-alpha,does-not-exist,proj-beta",
		"--vault", vault, "--json")
	require.NoError(t, err)

	var env envelopeShape
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))

	var res struct {
		Notes []struct {
			ID string `json:"id"`
		} `json:"notes"`
		NotFound []string `json:"not_found"`
	}
	require.NoError(t, json.Unmarshal(env.Result, &res))

	assert.Len(t, res.Notes, 2)
	assert.Len(t, res.NotFound, 1)
	assert.Contains(t, res.NotFound, "does-not-exist")
}

// note mget without --ids or --stdin must fail with a usage error — this
// is the user-facing signal that input was missing.
func TestNoteMget_RequiresIDSource(t *testing.T) {
	vault := buildIndexedTestVault(t)
	_, _, err := runRootCmd(t, "note", "mget", "--vault", vault)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--ids")
}

// collectMgetIDs splits comma inputs and trims whitespace — hidden commas
// or whitespace in IDs have caused real bugs in batching pipelines.
func TestCollectMgetIDs_SplitsAndTrims(t *testing.T) {
	cmd := noteMgetCmd
	require.NoError(t, cmd.Flags().Set("ids", " a , b,  c ,"))
	ids, err := collectMgetIDs(cmd)
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, ids)
	// reset for other tests
	require.NoError(t, cmd.Flags().Set("ids", ""))
}

// note mget human mode emits one line per found note + one "NOT FOUND:"
// line per missing ID. Scripts read both separately.
func TestNoteMget_HumanOutputLinesForFoundAndMissing(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "note", "mget",
		"--ids", "concept-alpha,does-not-exist",
		"--vault", vault)
	require.NoError(t, err)
	text := out.String()
	assert.Contains(t, text, "concept-alpha",
		"found note must appear as a line (ID + type + title format)")
	assert.Contains(t, text, "NOT FOUND: does-not-exist",
		"missing IDs must use the 'NOT FOUND:' prefix (machine-parseable)")
}

// vault status must report the correct partition between domain notes and
// unstructured notes. Miscounting would make every doctor/status report
// fiction.
func TestVaultStatus_CountsDomainVsUnstructured(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "vault", "status", "--vault", vault, "--json")
	require.NoError(t, err)

	var env envelopeShape
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))

	var res struct {
		TotalFiles        int `json:"total_files"`
		DomainNotes       int `json:"domain_notes"`
		UnstructuredNotes int `json:"unstructured_notes"`
	}
	require.NoError(t, json.Unmarshal(env.Result, &res))

	// Vault has 4 .md files; 3 domain (alpha, gamma, beta), 1 unstructured.
	assert.Equal(t, 4, res.TotalFiles)
	assert.Equal(t, 3, res.DomainNotes)
	assert.Equal(t, 1, res.UnstructuredNotes)
}

// schema list-types must surface every type registered in config.yaml —
// missing a type would mean users can't discover what they can create.
func TestSchemaListTypes_EnumeratesAllRegisteredTypes(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "schema", "list-types", "--vault", vault, "--json")
	require.NoError(t, err)

	var env envelopeShape
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))

	var types map[string]any
	require.NoError(t, json.Unmarshal(env.Result, &types))
	assert.Contains(t, types, "concept")
	assert.Contains(t, types, "project")
}

// schema list-types human output shows required fields and statuses —
// if the flag is dropped the human can't tell what a type expects.
func TestSchemaListTypes_HumanOutputShowsConstraints(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "schema", "list-types", "--vault", vault)
	require.NoError(t, err)

	text := out.String()
	assert.Contains(t, text, "concept")
	assert.Contains(t, text, "project")
	assert.Contains(t, text, "required=")
	assert.Contains(t, text, "active", "project's statuses must surface in human output")
}

// search over indexed content must find a term that appears in note bodies.
// If search returns zero hits on a known term the retriever is broken.
func TestSearch_FindsTermInBody(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "search", "Alpha", "--vault", vault, "--json", "--mode", "keyword")
	require.NoError(t, err)

	// Envelope shape: {"result": {"hits": [...], "total": N}}
	body := out.String()
	assert.Contains(t, body, "concept-alpha", "keyword search for 'Alpha' must surface the alpha concept")
}

// search with no args must fail with usage error — silent no-op would
// mask scripting bugs that forgot to interpolate the query.
func TestSearch_MissingQueryErrors(t *testing.T) {
	vault := buildIndexedTestVault(t)
	_, _, err := runRootCmd(t, "search", "--vault", vault)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "usage")
}

// resolve must map a known ID to itself (identity case). If the mapping
// ever stopped being idempotent, every caller that round-trips IDs breaks.
func TestResolve_IdentityForKnownID(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "resolve", "concept-alpha", "--vault", vault, "--json")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "concept-alpha")
}

// links in: alpha has an incoming link from beta (beta's body + related_ids).
// Losing incoming-link detection would break backlink navigation.
func TestLinksIn_FindsIncomingReferences(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "links", "in", "concept-alpha", "--vault", vault, "--json")
	require.NoError(t, err)
	// beta references alpha; the incoming set should name beta.
	assert.Contains(t, out.String(), "proj-beta")
}

// links out: alpha wikilinks to proj-beta and has it in related_ids.
// Outbound traversal must surface that edge.
func TestLinksOut_FindsOutboundReferences(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "links", "out", "concept-alpha", "--vault", vault, "--json")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "proj-beta")
}
