package mutation_test

import (
	"crypto/sha256"
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

// Silence unused import warnings - these are used in the test helpers above.
var (
	_ = sha256.New
	_ = fmt.Sprintf
)
