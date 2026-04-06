// cmd/apply_test.go

package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/plan"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatApplyResult_NoOps(t *testing.T) {
	result := &plan.ApplyResult{
		PlanDescription:     "empty plan",
		OperationsTotal:     0,
		OperationsCompleted: 0,
		Operations:          []plan.OpResult{},
	}

	var buf bytes.Buffer
	err := formatApplyResult(result, &buf)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "empty plan")
	assert.Contains(t, buf.String(), "0/0")
}

func TestFormatApplyResult_WithOps(t *testing.T) {
	result := &plan.ApplyResult{
		PlanDescription:     "set status",
		OperationsTotal:     2,
		OperationsCompleted: 1,
		Operations: []plan.OpResult{
			{Op: "frontmatter_set", Target: "projects/alpha.md", Status: "ok"},
			{Op: "frontmatter_set", Target: "projects/beta.md", Status: "error",
				Error: &plan.OpError{Code: "not_found", Message: "file missing"}},
		},
	}

	var buf bytes.Buffer
	err := formatApplyResult(result, &buf)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "set status")
	assert.Contains(t, out, "1/2")
	assert.Contains(t, out, "ok")
	assert.Contains(t, out, "ERROR: not_found")
	assert.Contains(t, out, "projects/alpha.md")
	assert.Contains(t, out, "projects/beta.md")
}

func TestFormatApplyResult_WithPath(t *testing.T) {
	// OpResult with Path set (note_create case) rather than Target
	result := &plan.ApplyResult{
		PlanDescription:     "note create",
		OperationsTotal:     1,
		OperationsCompleted: 1,
		Operations: []plan.OpResult{
			{Op: "note_create", Path: "decisions/new.md", Status: "ok", ID: "dec-1"},
		},
	}

	var buf bytes.Buffer
	err := formatApplyResult(result, &buf)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "decisions/new.md")
}

func TestFormatApplyResult_WithCommit(t *testing.T) {
	result := &plan.ApplyResult{
		PlanDescription:     "committed",
		OperationsTotal:     1,
		OperationsCompleted: 1,
		Operations: []plan.OpResult{
			{Op: "frontmatter_set", Target: "projects/alpha.md", Status: "ok"},
		},
		Committed: true,
		CommitSHA: "abc123def456",
	}

	var buf bytes.Buffer
	err := formatApplyResult(result, &buf)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Committed: abc123def456")
}

func TestRunApply_MissingArg(t *testing.T) {
	cmd := &cobra.Command{Use: "apply"}
	err := runApply(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "usage")
}

func TestExecuteApply_InvalidJSON(t *testing.T) {
	// Create a temp plan file with invalid JSON
	dir := t.TempDir()
	planFile := filepath.Join(dir, "plan.json")
	require.NoError(t, os.WriteFile(planFile, []byte("not valid json"), 0o644))

	cmd := &cobra.Command{Use: "apply"}
	err := executeApply(cmd, planFile)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing plan JSON")
}

func TestExecuteApply_FileNotFound(t *testing.T) {
	cmd := &cobra.Command{Use: "apply"}
	err := executeApply(cmd, "/nonexistent/path/plan.json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading plan")
}

func TestExecuteApply_JSONOutput(t *testing.T) {
	// Setup a temp vault
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".vaultmind")
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	configYAML := `types:
  project:
    required: [status, title]
    optional: [owner_id, tags]
    statuses: [active, paused, completed, cancelled]
`
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(configYAML), 0o644))

	projDir := filepath.Join(dir, "projects")
	require.NoError(t, os.MkdirAll(projDir, 0o755))
	noteContent := "---\nid: proj-alpha\ntype: project\nstatus: active\ntitle: Alpha\ncreated: 2026-01-01\nupdated: 2026-01-01\n---\n# Alpha\n"
	require.NoError(t, os.WriteFile(filepath.Join(projDir, "alpha.md"), []byte(noteContent), 0o644))

	// Create a plan file
	p := plan.Plan{
		Version:     1,
		Description: "test json output",
		Operations: []plan.Operation{
			{Op: plan.OpFrontmatterSet, Target: "projects/alpha.md", Key: "status", Value: "paused"},
		},
	}
	planBytes, err := json.Marshal(p)
	require.NoError(t, err)
	planFile := filepath.Join(dir, "plan.json")
	require.NoError(t, os.WriteFile(planFile, planBytes, 0o644))

	// Use viper to set config values (the function reads from viper, not from flag defaults)
	viper.Reset()
	defer viper.Reset()
	viper.Set("app.apply.vault", dir)
	viper.Set("app.apply.json", true)
	viper.Set("app.apply.dry_run", false)
	viper.Set("app.apply.diff", false)
	viper.Set("app.apply.commit", false)

	cmd := &cobra.Command{Use: "apply"}
	outBuf := &bytes.Buffer{}
	cmd.SetOut(outBuf)
	// Register flags so getConfigValueWithFlags doesn't error on GetBool/GetString
	cmd.Flags().String("vault", "", "vault path")
	cmd.Flags().Bool("json", false, "json output")
	cmd.Flags().Bool("dry-run", false, "dry run")
	cmd.Flags().Bool("diff", false, "show diff")
	cmd.Flags().Bool("commit", false, "commit")

	err = executeApply(cmd, planFile)
	require.NoError(t, err)

	// Should produce JSON output (contains "status" key from result)
	assert.Contains(t, outBuf.String(), `"status"`)
}

func TestExecuteApply_TextOutput(t *testing.T) {
	// Setup a temp vault
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".vaultmind")
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	configYAML := `types:
  project:
    required: [status, title]
    optional: [owner_id, tags]
    statuses: [active, paused, completed, cancelled]
`
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(configYAML), 0o644))

	projDir := filepath.Join(dir, "projects")
	require.NoError(t, os.MkdirAll(projDir, 0o755))
	noteContent := "---\nid: proj-alpha\ntype: project\nstatus: active\ntitle: Alpha\ncreated: 2026-01-01\nupdated: 2026-01-01\n---\n# Alpha\n"
	require.NoError(t, os.WriteFile(filepath.Join(projDir, "alpha.md"), []byte(noteContent), 0o644))

	// Create a plan file
	p := plan.Plan{
		Version:     1,
		Description: "test text output",
		Operations: []plan.Operation{
			{Op: plan.OpFrontmatterSet, Target: "projects/alpha.md", Key: "status", Value: "paused"},
		},
	}
	planBytes, err := json.Marshal(p)
	require.NoError(t, err)
	planFile := filepath.Join(dir, "plan.json")
	require.NoError(t, os.WriteFile(planFile, planBytes, 0o644))

	// Use viper to set config values
	viper.Reset()
	defer viper.Reset()
	viper.Set("app.apply.vault", dir)
	viper.Set("app.apply.json", false)
	viper.Set("app.apply.dry_run", false)
	viper.Set("app.apply.diff", false)
	viper.Set("app.apply.commit", false)

	cmd := &cobra.Command{Use: "apply"}
	outBuf := &bytes.Buffer{}
	cmd.SetOut(outBuf)
	// Register flags so getConfigValueWithFlags doesn't error on GetBool/GetString
	cmd.Flags().String("vault", "", "vault path")
	cmd.Flags().Bool("json", false, "json output")
	cmd.Flags().Bool("dry-run", false, "dry run")
	cmd.Flags().Bool("diff", false, "show diff")
	cmd.Flags().Bool("commit", false, "commit")

	err = executeApply(cmd, planFile)
	require.NoError(t, err)

	// Should produce text output
	out := outBuf.String()
	assert.Contains(t, out, "test text output")
	assert.Contains(t, out, "1/1")
}
