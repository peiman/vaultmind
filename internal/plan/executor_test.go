package plan_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/git"
	"github.com/peiman/vaultmind/internal/plan"
	"github.com/peiman/vaultmind/internal/schema"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeDetector returns a canned RepoState for tests.
type fakeDetector struct {
	state git.RepoState
}

func (f *fakeDetector) Detect(_ string) (git.RepoState, error) {
	return f.state, nil
}

// setupPlanVault creates a temp vault with config and two project notes.
func setupPlanVault(t *testing.T) (string, *schema.Registry, *git.PolicyChecker) {
	t.Helper()
	dir := t.TempDir()

	configDir := filepath.Join(dir, ".vaultmind")
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	configYAML := `types:
  project:
    required: [status, title]
    optional: [owner_id, tags, aliases, related_ids]
    statuses: [active, paused, completed, cancelled]
  decision:
    required: [title, status]
    optional: [tags]
    statuses: [proposed, accepted, rejected]
`
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(configYAML), 0o644))

	projDir := filepath.Join(dir, "projects")
	require.NoError(t, os.MkdirAll(projDir, 0o755))
	note1 := "---\nid: proj-alpha\ntype: project\nstatus: active\ntitle: Alpha Project\ntags:\n  - billing\ncreated: 2026-01-01\nupdated: 2026-01-02\n---\n# Alpha Project\n\nBody content.\n"
	require.NoError(t, os.WriteFile(filepath.Join(projDir, "alpha.md"), []byte(note1), 0o644))

	note2 := "---\nid: proj-beta\ntype: project\nstatus: paused\ntitle: Beta Project\ncreated: 2026-02-01\nupdated: 2026-02-02\n---\n# Beta Project\n\nBeta body.\n"
	require.NoError(t, os.WriteFile(filepath.Join(projDir, "beta.md"), []byte(note2), 0o644))

	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	reg := schema.NewRegistry(cfg.Types)
	checker, err := git.NewPolicyChecker(cfg.Git)
	require.NoError(t, err)

	return dir, reg, checker
}

// newTestExecutor builds an Executor with a clean-repo fakeDetector.
func newTestExecutor(t *testing.T, vaultPath string, reg *schema.Registry, checker *git.PolicyChecker) *plan.Executor {
	t.Helper()
	cfg, err := vault.LoadConfig(vaultPath)
	require.NoError(t, err)

	detector := &fakeDetector{state: git.RepoState{RepoDetected: true, WorkingTreeClean: true}}
	return &plan.Executor{
		VaultPath: vaultPath,
		Detector:  detector,
		Checker:   checker,
		Committer: nil, // no real git for unit tests
		Registry:  reg,
		Config:    cfg,
	}
}

func TestApply_SingleSet(t *testing.T) {
	vaultPath, reg, checker := setupPlanVault(t)
	exec := newTestExecutor(t, vaultPath, reg, checker)

	p := plan.Plan{
		Version:     1,
		Description: "set status on alpha",
		Operations: []plan.Operation{
			{Op: plan.OpFrontmatterSet, Target: "projects/alpha.md", Key: "status", Value: "paused"},
		},
	}

	result, err := exec.Apply(p, false, false, false)
	require.NoError(t, err)
	require.Len(t, result.Operations, 1)
	assert.Equal(t, "ok", result.Operations[0].Status)
	assert.Equal(t, 1, result.OperationsCompleted)
	assert.Equal(t, 1, result.OperationsTotal)
	assert.Equal(t, "set status on alpha", result.PlanDescription)

	content, err := os.ReadFile(filepath.Join(vaultPath, "projects/alpha.md"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "status: paused")
	assert.NotContains(t, string(content), "status: active")
}

func TestApply_MultipleOps(t *testing.T) {
	vaultPath, reg, checker := setupPlanVault(t)
	exec := newTestExecutor(t, vaultPath, reg, checker)

	p := plan.Plan{
		Version:     1,
		Description: "update both projects",
		Operations: []plan.Operation{
			{Op: plan.OpFrontmatterSet, Target: "projects/alpha.md", Key: "status", Value: "completed"},
			{Op: plan.OpFrontmatterSet, Target: "projects/beta.md", Key: "status", Value: "active"},
		},
	}

	result, err := exec.Apply(p, false, false, false)
	require.NoError(t, err)
	assert.Equal(t, 2, result.OperationsTotal)
	assert.Equal(t, 2, result.OperationsCompleted)
	for _, op := range result.Operations {
		assert.Equal(t, "ok", op.Status)
	}

	alphaContent, _ := os.ReadFile(filepath.Join(vaultPath, "projects/alpha.md"))
	assert.Contains(t, string(alphaContent), "status: completed")
	betaContent, _ := os.ReadFile(filepath.Join(vaultPath, "projects/beta.md"))
	assert.Contains(t, string(betaContent), "status: active")
}

func TestApply_NoteCreate(t *testing.T) {
	vaultPath, reg, checker := setupPlanVault(t)
	exec := newTestExecutor(t, vaultPath, reg, checker)

	p := plan.Plan{
		Version:     1,
		Description: "create a decision note",
		Operations: []plan.Operation{
			{
				Op: plan.OpNoteCreate, Path: "decisions/new-decision.md", Type: "decision",
				Frontmatter: map[string]interface{}{"title": "New Decision", "status": "proposed"},
			},
		},
	}

	result, err := exec.Apply(p, false, false, false)
	require.NoError(t, err)
	require.Len(t, result.Operations, 1)
	assert.Equal(t, "ok", result.Operations[0].Status)
	assert.NotEmpty(t, result.Operations[0].Path)

	content, err := os.ReadFile(filepath.Join(vaultPath, "decisions/new-decision.md"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "title: New Decision")
	assert.Contains(t, string(content), "type: decision")
}

func TestApply_Rollback(t *testing.T) {
	vaultPath, reg, checker := setupPlanVault(t)
	exec := newTestExecutor(t, vaultPath, reg, checker)

	// Read original content before the plan
	originalContent, err := os.ReadFile(filepath.Join(vaultPath, "projects/alpha.md"))
	require.NoError(t, err)

	p := plan.Plan{
		Version:     1,
		Description: "first succeeds, second fails",
		Operations: []plan.Operation{
			{Op: plan.OpFrontmatterSet, Target: "projects/alpha.md", Key: "status", Value: "paused"},
			{Op: plan.OpFrontmatterSet, Target: "nonexistent/file.md", Key: "status", Value: "active"},
		},
	}

	result, err := exec.Apply(p, false, false, false)
	require.NoError(t, err) // result returned, not error
	require.Len(t, result.Operations, 2)
	assert.Equal(t, "ok", result.Operations[0].Status)
	assert.Equal(t, "error", result.Operations[1].Status)
	assert.NotNil(t, result.Operations[1].Error)
	assert.Equal(t, 1, result.OperationsCompleted) // only first completed

	// Verify first op was rolled back
	rolledBack, err := os.ReadFile(filepath.Join(vaultPath, "projects/alpha.md"))
	require.NoError(t, err)
	assert.Equal(t, string(originalContent), string(rolledBack))
}

func TestApply_DryRun(t *testing.T) {
	vaultPath, reg, checker := setupPlanVault(t)
	exec := newTestExecutor(t, vaultPath, reg, checker)

	originalContent, err := os.ReadFile(filepath.Join(vaultPath, "projects/alpha.md"))
	require.NoError(t, err)

	p := plan.Plan{
		Version:     1,
		Description: "dry run set",
		Operations: []plan.Operation{
			{Op: plan.OpFrontmatterSet, Target: "projects/alpha.md", Key: "status", Value: "paused"},
		},
	}

	result, err := exec.Apply(p, true, false, false)
	require.NoError(t, err)
	require.Len(t, result.Operations, 1)
	assert.Equal(t, "ok", result.Operations[0].Status)

	// Verify file was NOT changed
	afterContent, err := os.ReadFile(filepath.Join(vaultPath, "projects/alpha.md"))
	require.NoError(t, err)
	assert.Equal(t, string(originalContent), string(afterContent))
}

func TestApply_ValidationFails(t *testing.T) {
	vaultPath, reg, checker := setupPlanVault(t)
	exec := newTestExecutor(t, vaultPath, reg, checker)

	p := plan.Plan{
		Version:     99, // bad version
		Description: "invalid plan",
		Operations: []plan.Operation{
			{Op: plan.OpFrontmatterSet, Target: "projects/alpha.md", Key: "status", Value: "paused"},
		},
	}

	result, err := exec.Apply(p, false, false, false)
	require.NoError(t, err) // returns result, not error
	require.Len(t, result.Operations, 1)
	assert.Equal(t, "error", result.Operations[0].Status)
	assert.NotNil(t, result.Operations[0].Error)
	assert.Equal(t, "unsupported_version", result.Operations[0].Error.Code)
	assert.Equal(t, 0, result.OperationsCompleted)
}

func TestApply_GitPolicyRefuse(t *testing.T) {
	vaultPath, reg, checker := setupPlanVault(t)
	cfg, err := vault.LoadConfig(vaultPath)
	require.NoError(t, err)

	// dirty target triggers Refuse
	detector := &fakeDetector{state: git.RepoState{
		RepoDetected: true, WorkingTreeClean: false,
		UnstagedFiles: []string{"projects/alpha.md"},
	}}
	exec := &plan.Executor{
		VaultPath: vaultPath,
		Detector:  detector,
		Checker:   checker,
		Committer: nil,
		Registry:  reg,
		Config:    cfg,
	}

	p := plan.Plan{
		Version:     1,
		Description: "should be refused",
		Operations: []plan.Operation{
			{Op: plan.OpFrontmatterSet, Target: "projects/alpha.md", Key: "status", Value: "paused"},
		},
	}

	_, err = exec.Apply(p, false, false, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dirty_target")
}

func TestApply_NoteCreateThenSet(t *testing.T) {
	vaultPath, reg, checker := setupPlanVault(t)
	exec := newTestExecutor(t, vaultPath, reg, checker)

	p := plan.Plan{
		Version:     1,
		Description: "create note then set field",
		Operations: []plan.Operation{
			{
				Op: plan.OpNoteCreate, Path: "decisions/chain-test.md", Type: "decision",
				Frontmatter: map[string]interface{}{"title": "Chain Test", "status": "proposed"},
			},
			{
				Op: plan.OpFrontmatterSet, Target: "decisions/chain-test.md",
				Key: "status", Value: "accepted",
			},
		},
	}

	result, err := exec.Apply(p, false, false, false)
	require.NoError(t, err)
	assert.Equal(t, 2, result.OperationsCompleted)
	for _, op := range result.Operations {
		assert.Equal(t, "ok", op.Status)
	}

	content, err := os.ReadFile(filepath.Join(vaultPath, "decisions/chain-test.md"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "status: accepted")
	assert.Contains(t, string(content), "type: decision")
}

func TestApply_RollbackNoteCreate(t *testing.T) {
	vaultPath, reg, checker := setupPlanVault(t)
	exec := newTestExecutor(t, vaultPath, reg, checker)

	p := plan.Plan{
		Version:     1,
		Description: "create note then fail",
		Operations: []plan.Operation{
			{
				Op: plan.OpNoteCreate, Path: "decisions/rollback-test.md", Type: "decision",
				Frontmatter: map[string]interface{}{"title": "Will Rollback", "status": "proposed"},
			},
			{Op: plan.OpFrontmatterSet, Target: "nonexistent/file.md", Key: "status", Value: "active"},
		},
	}

	result, err := exec.Apply(p, false, false, false)
	require.NoError(t, err)
	assert.Equal(t, "ok", result.Operations[0].Status)
	assert.Equal(t, "error", result.Operations[1].Status)
	assert.Equal(t, 1, result.OperationsCompleted)

	// Verify created file was deleted during rollback
	_, err = os.Stat(filepath.Join(vaultPath, "decisions/rollback-test.md"))
	assert.True(t, os.IsNotExist(err), "created file should be deleted during rollback")
}
