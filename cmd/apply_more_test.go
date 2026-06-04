package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// apply without an argument is a usage error — silent no-op would let a
// script think a plan was "applied" when nothing ran.
func TestApply_MissingArgIsUsageError(t *testing.T) {
	_, _, err := runRootCmd(t, "apply")
	require.Error(t, err)
}

// apply on a non-existent plan file in JSON mode produces a read_error
// envelope. The code makes the failure mode machine-readable.
func TestApply_MissingPlanFileReturnsReadErrorJSON(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "apply", "/does/not/exist/plan.json",
		"--vault", vault, "--json")
	require.NoError(t, err, "JSON-mode error is returned via envelope, not Go error")

	var env struct {
		Status string `json:"status"`
		Errors []struct {
			Code string `json:"code"`
		} `json:"errors"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "error", env.Status)
	require.NotEmpty(t, env.Errors)
	assert.Equal(t, "read_error", env.Errors[0].Code)
}

// apply on a plan file with invalid JSON returns parse_error — distinct
// from read_error so callers can differentiate filesystem vs content issues.
func TestApply_InvalidPlanJSONReturnsParseErrorJSON(t *testing.T) {
	vault := buildIndexedTestVault(t)
	planPath := filepath.Join(t.TempDir(), "bad-plan.json")
	require.NoError(t, os.WriteFile(planPath, []byte("{this is not json"), 0o644))

	out, _, err := runRootCmd(t, "apply", planPath, "--vault", vault, "--json")
	require.NoError(t, err)

	var env struct {
		Status string `json:"status"`
		Errors []struct {
			Code string `json:"code"`
		} `json:"errors"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "error", env.Status)
	require.NotEmpty(t, env.Errors)
	assert.Equal(t, "parse_error", env.Errors[0].Code)
}

// apply on a valid plan that sets a frontmatter field must execute the op
// and report it as completed. Regression: a happy path that silently drops
// ops would pass as long as no error fires — the OperationsCompleted count
// is the active success signal.
func TestApply_ValidSetPlanCompletesOperation(t *testing.T) {
	vault := buildIndexedTestVault(t)
	planPath := filepath.Join(t.TempDir(), "plan.json")
	planBody := `{
		"version": 1,
		"description": "test set status",
		"operations": [
			{
				"op": "frontmatter_set",
				"target": "projects/beta.md",
				"key": "status",
				"value": "paused"
			}
		]
	}`
	require.NoError(t, os.WriteFile(planPath, []byte(planBody), 0o644))

	out, _, err := runRootCmd(t, "apply", planPath, "--vault", vault, "--json")
	require.NoError(t, err)

	var env struct {
		Status string `json:"status"`
		Result struct {
			OperationsTotal     int `json:"operations_total"`
			OperationsCompleted int `json:"operations_completed"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "ok", env.Status)
	assert.Equal(t, 1, env.Result.OperationsTotal)
	assert.Equal(t, 1, env.Result.OperationsCompleted)

	// And the mutation actually hit disk
	content, err := os.ReadFile(filepath.Join(vault, "projects/beta.md"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "status: paused")
}

// --dry-run on a plan must not touch disk — the exec count can still be
// reported, but the file content is preserved.
func TestApply_DryRunLeavesFilesIntact(t *testing.T) {
	vault := buildIndexedTestVault(t)
	target := filepath.Join(vault, "projects/beta.md")
	before, err := os.ReadFile(target)
	require.NoError(t, err)

	planPath := filepath.Join(t.TempDir(), "plan.json")
	require.NoError(t, os.WriteFile(planPath, []byte(`{
		"version": 1,
		"description": "dry-run safety test",
		"operations": [
			{"op": "frontmatter_set", "target": "projects/beta.md", "key": "status", "value": "completed"}
		]
	}`), 0o644))

	_, _, err = runRootCmd(t, "apply", planPath, "--vault", vault, "--dry-run")
	require.NoError(t, err)

	after, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "dry-run must not mutate the file")
}
