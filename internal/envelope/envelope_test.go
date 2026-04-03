package envelope_test

import (
	"encoding/json"
	"testing"

	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnvelope_OK(t *testing.T) {
	env := envelope.OK("test-command", map[string]string{"key": "value"})

	assert.Equal(t, "test-command", env.Command)
	assert.Equal(t, "ok", env.Status)
	assert.Empty(t, env.Errors)
	assert.Empty(t, env.Warnings)
	assert.NotNil(t, env.Result)
	assert.NotEmpty(t, env.Meta.Timestamp)
}

func TestEnvelope_Error(t *testing.T) {
	env := envelope.Error("test-command", "conflict", "file changed on disk", "path")

	assert.Equal(t, "error", env.Status)
	require.Len(t, env.Errors, 1)
	assert.Equal(t, "conflict", env.Errors[0].Code)
	assert.Equal(t, "file changed on disk", env.Errors[0].Message)
	assert.Equal(t, "path", env.Errors[0].Field)
	assert.Nil(t, env.Result)
}

func TestEnvelope_WithWarning(t *testing.T) {
	env := envelope.OK("test", nil)
	env.AddWarning("stale_index", "index may be stale", "")

	assert.Equal(t, "warning", env.Status)
	require.Len(t, env.Warnings, 1)
	assert.Equal(t, "stale_index", env.Warnings[0].Code)
}

func TestEnvelope_JSON_RoundTrip(t *testing.T) {
	env := envelope.OK("resolve", map[string]bool{"resolved": true})

	data, err := json.Marshal(env)
	require.NoError(t, err)

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &parsed))

	assert.Equal(t, "resolve", parsed["command"])
	assert.Equal(t, "ok", parsed["status"])
	assert.NotNil(t, parsed["result"])
	assert.NotNil(t, parsed["meta"])
	assert.NotNil(t, parsed["warnings"])
	assert.NotNil(t, parsed["errors"])
}

func TestEnvelope_MetaFields(t *testing.T) {
	env := envelope.OK("test", nil)
	env.Meta.VaultPath = "/path/to/vault"
	env.Meta.IndexHash = "abc123"

	data, err := json.Marshal(env)
	require.NoError(t, err)

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &parsed))

	meta := parsed["meta"].(map[string]interface{})
	assert.Equal(t, "/path/to/vault", meta["vault_path"])
	assert.Equal(t, "abc123", meta["index_hash"])
}

func TestEnvelope_ErrorWithCandidates(t *testing.T) {
	env := envelope.Error("resolve", "ambiguous_resolution", "multiple matches", "")
	env.Errors[0].Candidates = []string{"proj-a", "proj-b"}
	env.Result = map[string]interface{}{"ambiguous": true}

	data, err := json.Marshal(env)
	require.NoError(t, err)

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &parsed))

	errors := parsed["errors"].([]interface{})
	firstErr := errors[0].(map[string]interface{})
	candidates := firstErr["candidates"].([]interface{})
	assert.Len(t, candidates, 2)
	assert.NotNil(t, parsed["result"])
}
