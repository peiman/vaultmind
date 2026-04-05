package mutation_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/git"
	"github.com/peiman/vaultmind/internal/mutation"
	"github.com/peiman/vaultmind/internal/schema"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeDetector struct {
	state git.RepoState
}

func (f *fakeDetector) Detect(_ string) (git.RepoState, error) {
	return f.state, nil
}

type fakeErrorDetector struct{}

func (f *fakeErrorDetector) Detect(_ string) (git.RepoState, error) {
	return git.RepoState{}, fmt.Errorf("detector failure")
}

func setupTestVault(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	configDir := filepath.Join(dir, ".vaultmind")
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	configYAML := `types:
  project:
    required: [status, title]
    optional: [owner_id, tags, aliases, related_ids]
    statuses: [active, paused, completed, cancelled]
  concept:
    required: [title]
    optional: [tags, aliases]
    statuses: []
`
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(configYAML), 0o644))

	projDir := filepath.Join(dir, "projects")
	require.NoError(t, os.MkdirAll(projDir, 0o755))
	noteContent := "---\nid: proj-test\ntype: project\nstatus: active\ntitle: Test Project\ntags:\n  - billing\ncreated: 2026-01-01\nupdated: 2026-01-02\n---\n# Test Project\n\nSome body content.\n"
	require.NoError(t, os.WriteFile(filepath.Join(projDir, "test-project.md"), []byte(noteContent), 0o644))

	conceptDir := filepath.Join(dir, "concepts")
	require.NoError(t, os.MkdirAll(conceptDir, 0o755))
	conceptContent := "---\nid: concept-test\ntype: concept\ntitle: Test Concept\n---\n# Test Concept\n\nBody here.\n"
	require.NoError(t, os.WriteFile(filepath.Join(conceptDir, "test-concept.md"), []byte(conceptContent), 0o644))

	return dir
}

func newTestMutator(t *testing.T, vaultPath string) *mutation.Mutator {
	t.Helper()
	cfg, err := vault.LoadConfig(vaultPath)
	require.NoError(t, err)
	reg := schema.NewRegistry(cfg.Types)

	detector := &fakeDetector{state: git.RepoState{RepoDetected: true, WorkingTreeClean: true}}
	checker, err := git.NewPolicyChecker(cfg.Git)
	require.NoError(t, err)

	return &mutation.Mutator{
		VaultPath: vaultPath,
		Detector:  detector,
		Checker:   checker,
		Registry:  reg,
	}
}

func TestMutator_Set_Basic(t *testing.T) {
	vaultPath := setupTestVault(t)
	m := newTestMutator(t, vaultPath)

	result, err := m.Run(mutation.MutationRequest{
		Op: mutation.OpSet, Target: "projects/test-project.md",
		Key: "status", Value: "paused",
	})
	require.NoError(t, err)
	assert.Equal(t, "set", result.Operation)
	assert.Equal(t, "status", result.Key)
	assert.Equal(t, "active", result.OldValue)
	assert.Equal(t, "paused", result.NewValue)
	assert.NotEmpty(t, result.WriteHash)
	assert.Equal(t, "proj-test", result.ID)

	content, err := os.ReadFile(filepath.Join(vaultPath, "projects/test-project.md"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "status: paused")
	assert.NotContains(t, string(content), "status: active")
	assert.Contains(t, string(content), "# Test Project")
	assert.Contains(t, string(content), "Some body content.")
}

func TestMutator_Set_DryRun(t *testing.T) {
	vaultPath := setupTestVault(t)
	m := newTestMutator(t, vaultPath)

	result, err := m.Run(mutation.MutationRequest{
		Op: mutation.OpSet, Target: "projects/test-project.md",
		Key: "status", Value: "paused", DryRun: true, Diff: true,
	})
	require.NoError(t, err)
	assert.True(t, result.DryRun)
	assert.Contains(t, result.Diff, "-status: active")
	assert.Contains(t, result.Diff, "+status: paused")
	assert.Empty(t, result.WriteHash)

	content, err := os.ReadFile(filepath.Join(vaultPath, "projects/test-project.md"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "status: active") // unchanged
}

func TestMutator_Unset(t *testing.T) {
	vaultPath := setupTestVault(t)
	m := newTestMutator(t, vaultPath)

	result, err := m.Run(mutation.MutationRequest{
		Op: mutation.OpUnset, Target: "projects/test-project.md", Key: "tags",
	})
	require.NoError(t, err)
	assert.Equal(t, "unset", result.Operation)
	assert.Equal(t, "tags", result.Key)

	content, err := os.ReadFile(filepath.Join(vaultPath, "projects/test-project.md"))
	require.NoError(t, err)
	assert.NotContains(t, string(content), "tags:")
	assert.NotContains(t, string(content), "billing")
}

func TestMutator_Merge(t *testing.T) {
	vaultPath := setupTestVault(t)
	m := newTestMutator(t, vaultPath)

	result, err := m.Run(mutation.MutationRequest{
		Op: mutation.OpMerge, Target: "projects/test-project.md",
		Fields: map[string]interface{}{"status": "paused", "owner_id": "person-alice"},
	})
	require.NoError(t, err)
	assert.Equal(t, "merge", result.Operation)

	content, err := os.ReadFile(filepath.Join(vaultPath, "projects/test-project.md"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "status: paused")
	assert.Contains(t, string(content), "owner_id: person-alice")
}

func TestMutator_Normalize(t *testing.T) {
	vaultPath := setupTestVault(t)
	noteContent := "---\ntags:\n  - test\nstatus: active\ntitle: Messy Note\nid: proj-test\ntype: project\ncreated: 2026-01-01T00:00:00\nupdated: 2026-01-02\n---\n# Body\n"
	require.NoError(t, os.WriteFile(filepath.Join(vaultPath, "projects/test-project.md"), []byte(noteContent), 0o644))

	m := newTestMutator(t, vaultPath)
	result, err := m.Run(mutation.MutationRequest{
		Op: mutation.OpNormalize, Target: "projects/test-project.md",
	})
	require.NoError(t, err)
	assert.Equal(t, "normalize", result.Operation)

	content, err := os.ReadFile(filepath.Join(vaultPath, "projects/test-project.md"))
	require.NoError(t, err)
	s := string(content)

	// Verify canonical order: id before type before status
	idIdx := strings.Index(s, "id:")
	typeIdx := strings.Index(s, "type:")
	statusIdx := strings.Index(s, "status:")
	assert.Less(t, idIdx, typeIdx)
	assert.Less(t, typeIdx, statusIdx)
}

func TestMutator_Set_ImmutableField(t *testing.T) {
	vaultPath := setupTestVault(t)
	m := newTestMutator(t, vaultPath)

	_, err := m.Run(mutation.MutationRequest{
		Op: mutation.OpSet, Target: "projects/test-project.md",
		Key: "id", Value: "new-id",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "immutable_field")
}

func TestMutator_Set_UnknownKey(t *testing.T) {
	vaultPath := setupTestVault(t)
	m := newTestMutator(t, vaultPath)

	_, err := m.Run(mutation.MutationRequest{
		Op: mutation.OpSet, Target: "projects/test-project.md",
		Key: "totally_unknown", Value: "val",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown_key")
}

func TestMutator_GitPolicyRefuse(t *testing.T) {
	vaultPath := setupTestVault(t)
	cfg, err := vault.LoadConfig(vaultPath)
	require.NoError(t, err)
	reg := schema.NewRegistry(cfg.Types)

	detector := &fakeDetector{state: git.RepoState{
		RepoDetected: true, WorkingTreeClean: false,
		UnstagedFiles: []string{"projects/test-project.md"},
	}}
	checker, err := git.NewPolicyChecker(cfg.Git)
	require.NoError(t, err)

	m := &mutation.Mutator{
		VaultPath: vaultPath, Detector: detector, Checker: checker, Registry: reg,
	}

	_, err = m.Run(mutation.MutationRequest{
		Op: mutation.OpSet, Target: "projects/test-project.md",
		Key: "status", Value: "paused",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dirty_target")
}

func TestMutator_UnresolvedTarget(t *testing.T) {
	vaultPath := setupTestVault(t)
	m := newTestMutator(t, vaultPath)

	_, err := m.Run(mutation.MutationRequest{
		Op: mutation.OpSet, Target: "nonexistent/file.md",
		Key: "status", Value: "active",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unresolved_target")
}

func TestCommitMessage_Set(t *testing.T) {
	req := mutation.MutationRequest{Op: mutation.OpSet, Key: "status", Value: "paused"}
	msg := mutation.CommitMessage(req, "proj-test")
	assert.Contains(t, msg, "frontmatter set")
	assert.Contains(t, msg, "proj-test")
	assert.Contains(t, msg, "status")
}

func TestCommitMessage_Unset(t *testing.T) {
	req := mutation.MutationRequest{Op: mutation.OpUnset, Key: "tags"}
	msg := mutation.CommitMessage(req, "proj-test")
	assert.Contains(t, msg, "frontmatter unset")
	assert.Contains(t, msg, "tags")
}

func TestCommitMessage_Merge(t *testing.T) {
	req := mutation.MutationRequest{Op: mutation.OpMerge, Fields: map[string]interface{}{"status": "paused"}}
	msg := mutation.CommitMessage(req, "proj-test")
	assert.Contains(t, msg, "frontmatter merge")
}

func TestCommitMessage_Normalize(t *testing.T) {
	req := mutation.MutationRequest{Op: mutation.OpNormalize}
	msg := mutation.CommitMessage(req, "proj-test")
	assert.Contains(t, msg, "frontmatter normalize")
}

func TestMutator_Set_Diff(t *testing.T) {
	vaultPath := setupTestVault(t)
	m := newTestMutator(t, vaultPath)

	result, err := m.Run(mutation.MutationRequest{
		Op: mutation.OpSet, Target: "projects/test-project.md",
		Key: "status", Value: "paused", Diff: true,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, result.Diff)
	assert.Contains(t, result.Diff, "-status: active")
	assert.Contains(t, result.Diff, "+status: paused")
	assert.NotEmpty(t, result.WriteHash)
}

func TestMutator_Set_NewKey(t *testing.T) {
	vaultPath := setupTestVault(t)
	m := newTestMutator(t, vaultPath)

	result, err := m.Run(mutation.MutationRequest{
		Op: mutation.OpSet, Target: "projects/test-project.md",
		Key: "owner_id", Value: "person-alice",
	})
	require.NoError(t, err)
	assert.Nil(t, result.OldValue)
	assert.Equal(t, "person-alice", result.NewValue)

	content, err := os.ReadFile(filepath.Join(vaultPath, "projects/test-project.md"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "owner_id: person-alice")
}

func TestMutator_Unset_RequiredField(t *testing.T) {
	vaultPath := setupTestVault(t)
	m := newTestMutator(t, vaultPath)

	_, err := m.Run(mutation.MutationRequest{
		Op: mutation.OpUnset, Target: "projects/test-project.md", Key: "status",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing_required_field")
}

func TestMutator_Set_AllowExtra(t *testing.T) {
	vaultPath := setupTestVault(t)
	m := newTestMutator(t, vaultPath)

	result, err := m.Run(mutation.MutationRequest{
		Op: mutation.OpSet, Target: "projects/test-project.md",
		Key: "custom_field", Value: "custom_value", AllowExtra: true,
	})
	require.NoError(t, err)
	assert.Equal(t, "set", result.Operation)

	content, err := os.ReadFile(filepath.Join(vaultPath, "projects/test-project.md"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "custom_field: custom_value")
}

func TestMutator_GitPolicyWarn(t *testing.T) {
	vaultPath := setupTestVault(t)
	cfg, err := vault.LoadConfig(vaultPath)
	require.NoError(t, err)
	reg := schema.NewRegistry(cfg.Types)

	// Dirty but unrelated files → Warn, not Refuse
	detector := &fakeDetector{state: git.RepoState{
		RepoDetected: true, WorkingTreeClean: false,
		UnstagedFiles: []string{"other/unrelated.md"},
	}}
	checker, err := git.NewPolicyChecker(cfg.Git)
	require.NoError(t, err)

	m := &mutation.Mutator{
		VaultPath: vaultPath, Detector: detector, Checker: checker, Registry: reg,
	}

	result, err := m.Run(mutation.MutationRequest{
		Op: mutation.OpSet, Target: "projects/test-project.md",
		Key: "status", Value: "paused",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, result.WriteHash)
	assert.Len(t, result.Warnings, 1)
	assert.Equal(t, "dirty_unrelated", result.Warnings[0].Rule)
}

func TestMutator_UnresolvedTarget_BareID(t *testing.T) {
	vaultPath := setupTestVault(t)
	m := newTestMutator(t, vaultPath)

	// A bare ID (no "/" and no ".md") triggers the unresolved_target path
	_, err := m.Run(mutation.MutationRequest{
		Op: mutation.OpSet, Target: "proj-test",
		Key: "status", Value: "paused",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unresolved_target")
}

func TestMutator_GitInfo_StagedFile(t *testing.T) {
	vaultPath := setupTestVault(t)
	cfg, err := vault.LoadConfig(vaultPath)
	require.NoError(t, err)
	reg := schema.NewRegistry(cfg.Types)

	// Staged target file → gitInfo reports TargetFileClean = false.
	// Use DryRun to skip the write policy check (dirty_target Refuse).
	detector := &fakeDetector{state: git.RepoState{
		RepoDetected:     true,
		WorkingTreeClean: false,
		StagedFiles:      []string{"projects/test-project.md"},
	}}
	checker, err := git.NewPolicyChecker(cfg.Git)
	require.NoError(t, err)

	m := &mutation.Mutator{
		VaultPath: vaultPath, Detector: detector, Checker: checker, Registry: reg,
	}

	result, err := m.Run(mutation.MutationRequest{
		Op: mutation.OpSet, Target: "projects/test-project.md",
		Key: "status", Value: "paused", DryRun: true,
	})
	require.NoError(t, err)
	assert.True(t, result.DryRun)
	assert.False(t, result.Git.TargetFileClean)
}

func TestMutator_GitInfo_UnstagedTargetFile(t *testing.T) {
	vaultPath := setupTestVault(t)
	cfg, err := vault.LoadConfig(vaultPath)
	require.NoError(t, err)
	reg := schema.NewRegistry(cfg.Types)

	// Note: unstaged target file → Refuse by default git policy (dirty_target).
	// To test the gitInfo unstaged loop independently, we run a DryRun (skips policy check).
	detector := &fakeDetector{state: git.RepoState{
		RepoDetected:     true,
		WorkingTreeClean: false,
		UnstagedFiles:    []string{"projects/test-project.md"},
	}}
	checker, err := git.NewPolicyChecker(cfg.Git)
	require.NoError(t, err)

	m := &mutation.Mutator{
		VaultPath: vaultPath, Detector: detector, Checker: checker, Registry: reg,
	}

	result, err := m.Run(mutation.MutationRequest{
		Op: mutation.OpSet, Target: "projects/test-project.md",
		Key: "status", Value: "paused", DryRun: true,
	})
	require.NoError(t, err)
	assert.True(t, result.DryRun)
	// gitInfo is called for DryRun
	assert.False(t, result.Git.TargetFileClean)
}

func TestCommitMessage_Unknown(t *testing.T) {
	req := mutation.MutationRequest{Op: mutation.OpType(99)}
	msg := mutation.CommitMessage(req, "proj-test")
	assert.Contains(t, msg, "proj-test")
}

func TestMutator_Merge_NoSchema(t *testing.T) {
	// Test merge with a concept type that has minimal schema (no required fields that block)
	vaultPath := setupTestVault(t)
	m := newTestMutator(t, vaultPath)

	result, err := m.Run(mutation.MutationRequest{
		Op:     mutation.OpMerge,
		Target: "concepts/test-concept.md",
		Fields: map[string]interface{}{"title": "Updated Concept"},
	})
	require.NoError(t, err)
	assert.Equal(t, "merge", result.Operation)

	content, err := os.ReadFile(filepath.Join(vaultPath, "concepts/test-concept.md"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "title: Updated Concept")
}

func TestMutator_Unset_ImmutableField(t *testing.T) {
	vaultPath := setupTestVault(t)
	m := newTestMutator(t, vaultPath)

	_, err := m.Run(mutation.MutationRequest{
		Op: mutation.OpUnset, Target: "projects/test-project.md", Key: "id",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "immutable_field")
}

func TestOpType_String_Unknown(t *testing.T) {
	op := mutation.OpType(99)
	assert.Contains(t, op.String(), "unknown")
}

func TestMutationError_WithField(t *testing.T) {
	err := &mutation.MutationError{Code: "test_code", Message: "test message", Field: "test_field"}
	assert.Contains(t, err.Error(), "test_code")
	assert.Contains(t, err.Error(), "test_field")
}

func TestMutationError_NoField(t *testing.T) {
	err := &mutation.MutationError{Code: "test_code", Message: "test message"}
	assert.Contains(t, err.Error(), "test_code")
	assert.NotContains(t, err.Error(), "field:")
}

func TestMutator_Normalize_NonDomainNote(t *testing.T) {
	// Normalize on a non-domain note should succeed (normalize skips schema checks)
	vaultPath := setupTestVault(t)
	// Write a note without type/id
	dir := filepath.Join(vaultPath, "notes")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	noteContent := "---\ntitle: Plain Note\ntags:\n  - test\n---\n# Body\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "plain.md"), []byte(noteContent), 0o644))

	m := newTestMutator(t, vaultPath)
	result, err := m.Run(mutation.MutationRequest{
		Op: mutation.OpNormalize, Target: "notes/plain.md",
	})
	require.NoError(t, err)
	assert.Equal(t, "normalize", result.Operation)
}

func TestMutator_Set_InvalidStatus(t *testing.T) {
	vaultPath := setupTestVault(t)
	m := newTestMutator(t, vaultPath)

	_, err := m.Run(mutation.MutationRequest{
		Op: mutation.OpSet, Target: "projects/test-project.md",
		Key: "status", Value: "not_a_valid_status",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid_status")
}

func TestMutator_Set_NonDomainNote(t *testing.T) {
	vaultPath := setupTestVault(t)
	// Write a note without type
	dir := filepath.Join(vaultPath, "notes")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	noteContent := "---\ntitle: Plain Note\n---\n# Body\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "plain.md"), []byte(noteContent), 0o644))

	m := newTestMutator(t, vaultPath)
	_, err := m.Run(mutation.MutationRequest{
		Op: mutation.OpSet, Target: "notes/plain.md",
		Key: "title", Value: "New Title",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not_domain_note")
}

func TestMutator_GitDetectError(t *testing.T) {
	vaultPath := setupTestVault(t)
	cfg, err := vault.LoadConfig(vaultPath)
	require.NoError(t, err)
	reg := schema.NewRegistry(cfg.Types)

	checker, err := git.NewPolicyChecker(cfg.Git)
	require.NoError(t, err)

	m := &mutation.Mutator{
		VaultPath: vaultPath,
		Detector:  &fakeErrorDetector{},
		Checker:   checker,
		Registry:  reg,
	}

	_, err = m.Run(mutation.MutationRequest{
		Op: mutation.OpSet, Target: "projects/test-project.md",
		Key: "status", Value: "paused",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git_detect_error")
}

func TestMutator_Set_CommitFlag_NilCommitter(t *testing.T) {
	// Commit=true with nil Committer: covers op=OpWriteCommit branch but skips actual commit
	vaultPath := setupTestVault(t)
	cfg, err := vault.LoadConfig(vaultPath)
	require.NoError(t, err)
	reg := schema.NewRegistry(cfg.Types)

	detector := &fakeDetector{state: git.RepoState{RepoDetected: true, WorkingTreeClean: true}}
	checker, err := git.NewPolicyChecker(cfg.Git)
	require.NoError(t, err)

	m := &mutation.Mutator{
		VaultPath: vaultPath,
		Detector:  detector,
		Checker:   checker,
		Committer: nil, // nil committer: commit step is skipped
		Registry:  reg,
	}

	result, err := m.Run(mutation.MutationRequest{
		Op: mutation.OpSet, Target: "projects/test-project.md",
		Key: "status", Value: "paused", Commit: true,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, result.WriteHash)
	assert.Empty(t, result.Git.CommitSHA)
}
