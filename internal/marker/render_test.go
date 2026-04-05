package marker_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/git"
	"github.com/peiman/vaultmind/internal/marker"
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

func setupRenderVault(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	configDir := filepath.Join(dir, ".vaultmind")
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.yaml"),
		[]byte("types:\n  project:\n    required: [status, title]\n    statuses: [active]\n"), 0o644))

	sectionDir := filepath.Join(configDir, "sections", "project")
	require.NoError(t, os.MkdirAll(sectionDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sectionDir, "related.md"),
		[]byte("```dataview\nTABLE title FROM \"projects\"\nWHERE contains(related_ids, this.id)\n```\n"), 0o644))

	projDir := filepath.Join(dir, "projects")
	require.NoError(t, os.MkdirAll(projDir, 0o755))
	noteContent := "---\nid: proj-test\ntype: project\nstatus: active\ntitle: Test\n---\n# Test\n\n<!-- VAULTMIND:GENERATED:related:START -->\nplaceholder\n<!-- VAULTMIND:GENERATED:related:END -->\n"
	require.NoError(t, os.WriteFile(filepath.Join(projDir, "test.md"), []byte(noteContent), 0o644))

	return dir
}

func TestRenderRegion_Basic(t *testing.T) {
	vaultPath := setupRenderVault(t)
	cfg, err := vault.LoadConfig(vaultPath)
	require.NoError(t, err)
	detector := &fakeDetector{state: git.RepoState{RepoDetected: true, WorkingTreeClean: true}}
	checker, err := git.NewPolicyChecker(cfg.Git)
	require.NoError(t, err)

	result, err := marker.RenderRegion(marker.RenderConfig{
		VaultPath: vaultPath, Target: "projects/test.md", SectionKey: "related",
		Detector: detector, Checker: checker,
	})
	require.NoError(t, err)
	assert.Equal(t, "dataview_render", result.Operation)
	assert.NotEmpty(t, result.WriteHash)

	content, err := os.ReadFile(filepath.Join(vaultPath, "projects/test.md"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "TABLE title")
	assert.NotContains(t, string(content), "placeholder")
	assert.Contains(t, string(content), "<!-- checksum:")
}

func TestRenderRegion_DryRun(t *testing.T) {
	vaultPath := setupRenderVault(t)
	cfg, err := vault.LoadConfig(vaultPath)
	require.NoError(t, err)
	detector := &fakeDetector{state: git.RepoState{RepoDetected: true, WorkingTreeClean: true}}
	checker, err := git.NewPolicyChecker(cfg.Git)
	require.NoError(t, err)

	result, err := marker.RenderRegion(marker.RenderConfig{
		VaultPath: vaultPath, Target: "projects/test.md", SectionKey: "related",
		DryRun: true, Diff: true, Detector: detector, Checker: checker,
	})
	require.NoError(t, err)
	assert.True(t, result.DryRun)
	assert.NotEmpty(t, result.Diff)
	assert.Empty(t, result.WriteHash)

	content, err := os.ReadFile(filepath.Join(vaultPath, "projects/test.md"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "placeholder")
}

func TestRenderRegion_MissingTemplate(t *testing.T) {
	vaultPath := setupRenderVault(t)
	cfg, err := vault.LoadConfig(vaultPath)
	require.NoError(t, err)
	detector := &fakeDetector{state: git.RepoState{RepoDetected: true, WorkingTreeClean: true}}
	checker, err := git.NewPolicyChecker(cfg.Git)
	require.NoError(t, err)

	_, err = marker.RenderRegion(marker.RenderConfig{
		VaultPath: vaultPath, Target: "projects/test.md", SectionKey: "nonexistent",
		Detector: detector, Checker: checker,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "template_not_found")
}

func TestRenderRegion_NoMarkers(t *testing.T) {
	vaultPath := setupRenderVault(t)
	require.NoError(t, os.WriteFile(filepath.Join(vaultPath, "projects/nomarkers.md"),
		[]byte("---\nid: proj-no\ntype: project\nstatus: active\ntitle: No Markers\n---\n# No markers\n"), 0o644))

	cfg, err := vault.LoadConfig(vaultPath)
	require.NoError(t, err)
	detector := &fakeDetector{state: git.RepoState{RepoDetected: true, WorkingTreeClean: true}}
	checker, err := git.NewPolicyChecker(cfg.Git)
	require.NoError(t, err)

	_, err = marker.RenderRegion(marker.RenderConfig{
		VaultPath: vaultPath, Target: "projects/nomarkers.md", SectionKey: "related",
		Detector: detector, Checker: checker,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestLoadSectionTemplate_Valid(t *testing.T) {
	vaultPath := setupRenderVault(t)
	content, err := marker.LoadSectionTemplate(vaultPath, "project", "related")
	require.NoError(t, err)
	assert.Contains(t, string(content), "TABLE title")
}

func TestLoadSectionTemplate_Missing(t *testing.T) {
	vaultPath := setupRenderVault(t)
	_, err := marker.LoadSectionTemplate(vaultPath, "project", "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "template_not_found")
}
