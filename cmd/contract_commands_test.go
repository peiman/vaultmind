package cmd

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These are the commands downstream tools (chat app, custom integrations)
// will consume next. Locking their envelope shape now prevents
// 'we shipped, then Workhorse-next-gen broke silently' the week we
// refactor. Each test uses the same indexed baseline fixture so the
// contract doesn't depend on other tests' state.

func TestContextPackJSONContract_Decodes(t *testing.T) {
	vault := indexedBaselineVault(t)
	out, _, err := runRootCmd(t, "memory", "context-pack", "c-spreading",
		"--vault", vault,
		"--budget", "3000", "--max-items", "4",
		"--json")
	require.NoError(t, err)

	var env ContextPackEnvelope
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "v1", env.SchemaVersion)
	assert.Equal(t, "ok", env.Status)
	assert.Equal(t, "c-spreading", env.Result.TargetID,
		"result.target_id is the consumer's primary key — renaming it would break every caller")
	assert.Equal(t, "c-spreading", env.Result.Target.ID,
		"result.target.id is the full-note view; both must reference the same note")
	assert.Greater(t, env.Result.BudgetTokens, 0,
		"budget_tokens echoes the configured budget — consumers verify requests were honored via this")
}

func TestSearchJSONContract_Decodes(t *testing.T) {
	vault := indexedBaselineVault(t)
	out, _, err := runRootCmd(t, "search", "spreading activation",
		"--vault", vault,
		"--mode", "keyword",
		"--json")
	require.NoError(t, err)

	var env SearchEnvelope
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "v1", env.SchemaVersion)
	assert.Equal(t, "ok", env.Status)
	assert.GreaterOrEqual(t, env.Result.Total, 1,
		"search on a known term in the baseline vault must return at least one hit")
	require.NotEmpty(t, env.Result.Hits)
	assert.NotEmpty(t, env.Result.Hits[0].ID,
		"hit ID is the primary consumer field — consumers dereference notes by this")
}

func TestNoteGetJSONContract_Decodes(t *testing.T) {
	vault := indexedBaselineVault(t)
	out, _, err := runRootCmd(t, "note", "get", "c-spreading",
		"--vault", vault,
		"--json")
	require.NoError(t, err)

	var env NoteGetEnvelope
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "v1", env.SchemaVersion)
	assert.Equal(t, "ok", env.Status)
	assert.Equal(t, "c-spreading", env.Result.ID)
	assert.Equal(t, "concept", env.Result.Type)
}
