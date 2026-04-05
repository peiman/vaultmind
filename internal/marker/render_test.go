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

func TestRenderRegion_PathTraversal(t *testing.T) {
	vaultPath := setupRenderVault(t)
	cfg, _ := vault.LoadConfig(vaultPath)
	detector := &fakeDetector{state: git.RepoState{RepoDetected: true, WorkingTreeClean: true}}
	checker, _ := git.NewPolicyChecker(cfg.Git)
	_, err := marker.RenderRegion(marker.RenderConfig{
		VaultPath: vaultPath, Target: "../../etc/passwd", SectionKey: "related",
		Detector: detector, Checker: checker,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path_traversal")
}

func TestRenderRegion_AllSections(t *testing.T) {
	vaultPath := setupRenderVault(t)
	// Add a second section template
	sectionDir := filepath.Join(vaultPath, ".vaultmind", "sections", "project")
	require.NoError(t, os.WriteFile(filepath.Join(sectionDir, "backlinks.md"),
		[]byte("```dataview\nLIST FROM [[this]]\n```\n"), 0o644))

	// Create note with two markers
	noteContent := "---\nid: proj-multi\ntype: project\nstatus: active\ntitle: Multi\n---\n# Multi\n\n<!-- VAULTMIND:GENERATED:related:START -->\nold1\n<!-- VAULTMIND:GENERATED:related:END -->\n\n<!-- VAULTMIND:GENERATED:backlinks:START -->\nold2\n<!-- VAULTMIND:GENERATED:backlinks:END -->\n"
	require.NoError(t, os.WriteFile(filepath.Join(vaultPath, "projects/multi.md"), []byte(noteContent), 0o644))

	cfg, _ := vault.LoadConfig(vaultPath)
	detector := &fakeDetector{state: git.RepoState{RepoDetected: true, WorkingTreeClean: true}}
	checker, _ := git.NewPolicyChecker(cfg.Git)

	result, err := marker.RenderRegion(marker.RenderConfig{
		VaultPath: vaultPath, Target: "projects/multi.md", SectionKey: "",
		Detector: detector, Checker: checker,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, result.WriteHash)

	content, _ := os.ReadFile(filepath.Join(vaultPath, "projects/multi.md"))
	s := string(content)
	assert.Contains(t, s, "TABLE title") // related template
	assert.Contains(t, s, "LIST FROM")   // backlinks template
	assert.NotContains(t, s, "old1")
	assert.NotContains(t, s, "old2")
}

func TestRenderRegion_GitPolicyRefuse(t *testing.T) {
	vaultPath := setupRenderVault(t)
	cfg, _ := vault.LoadConfig(vaultPath)
	detector := &fakeDetector{state: git.RepoState{
		RepoDetected: true, WorkingTreeClean: false,
		UnstagedFiles: []string{"projects/test.md"},
	}}
	checker, _ := git.NewPolicyChecker(cfg.Git)

	_, err := marker.RenderRegion(marker.RenderConfig{
		VaultPath: vaultPath, Target: "projects/test.md", SectionKey: "related",
		Detector: detector, Checker: checker,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dirty_target")
}

func TestRenderRegion_NoFrontmatter(t *testing.T) {
	vaultPath := setupRenderVault(t)
	require.NoError(t, os.WriteFile(filepath.Join(vaultPath, "projects/nofm.md"),
		[]byte("# No frontmatter\n\n<!-- VAULTMIND:GENERATED:related:START -->\nold\n<!-- VAULTMIND:GENERATED:related:END -->\n"), 0o644))

	cfg, _ := vault.LoadConfig(vaultPath)
	detector := &fakeDetector{state: git.RepoState{RepoDetected: true, WorkingTreeClean: true}}
	checker, _ := git.NewPolicyChecker(cfg.Git)

	_, err := marker.RenderRegion(marker.RenderConfig{
		VaultPath: vaultPath, Target: "projects/nofm.md", SectionKey: "related",
		Detector: detector, Checker: checker,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse_error")
}

func TestRenderRegion_NonexistentFile(t *testing.T) {
	vaultPath := setupRenderVault(t)
	cfg, _ := vault.LoadConfig(vaultPath)
	detector := &fakeDetector{state: git.RepoState{RepoDetected: true, WorkingTreeClean: true}}
	checker, _ := git.NewPolicyChecker(cfg.Git)

	_, err := marker.RenderRegion(marker.RenderConfig{
		VaultPath: vaultPath, Target: "nonexistent.md", SectionKey: "related",
		Detector: detector, Checker: checker,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unresolved_target")
}

func TestRenderRegion_GitPolicyWarn(t *testing.T) {
	vaultPath := setupRenderVault(t)
	cfg, _ := vault.LoadConfig(vaultPath)
	detector := &fakeDetector{state: git.RepoState{
		RepoDetected: true, WorkingTreeClean: false,
		UnstagedFiles: []string{"other/unrelated.md"},
	}}
	checker, _ := git.NewPolicyChecker(cfg.Git)

	result, err := marker.RenderRegion(marker.RenderConfig{
		VaultPath: vaultPath, Target: "projects/test.md", SectionKey: "related",
		Detector: detector, Checker: checker,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, result.WriteHash)
	assert.Len(t, result.Warnings, 1)
}

func TestRenderRegion_AllSections_DryRun(t *testing.T) {
	vaultPath := setupRenderVault(t)
	cfg, _ := vault.LoadConfig(vaultPath)
	detector := &fakeDetector{state: git.RepoState{RepoDetected: true, WorkingTreeClean: true}}
	checker, _ := git.NewPolicyChecker(cfg.Git)

	result, err := marker.RenderRegion(marker.RenderConfig{
		VaultPath: vaultPath, Target: "projects/test.md", SectionKey: "",
		DryRun: true, Diff: true, Detector: detector, Checker: checker,
	})
	require.NoError(t, err)
	assert.True(t, result.DryRun)
	assert.NotEmpty(t, result.Diff)
	assert.Empty(t, result.WriteHash)
}

func TestRenderRegion_AllSections_NoMarkers(t *testing.T) {
	vaultPath := setupRenderVault(t)
	require.NoError(t, os.WriteFile(filepath.Join(vaultPath, "projects/empty.md"),
		[]byte("---\nid: proj-empty\ntype: project\nstatus: active\ntitle: Empty\n---\n# Empty\n"), 0o644))

	cfg, _ := vault.LoadConfig(vaultPath)
	detector := &fakeDetector{state: git.RepoState{RepoDetected: true, WorkingTreeClean: true}}
	checker, _ := git.NewPolicyChecker(cfg.Git)

	result, err := marker.RenderRegion(marker.RenderConfig{
		VaultPath: vaultPath, Target: "projects/empty.md", SectionKey: "",
		Detector: detector, Checker: checker,
	})
	require.NoError(t, err)
	assert.Equal(t, "all", result.SectionKey)
	assert.NotEmpty(t, result.WriteHash)
}

func TestRenderRegion_StagedTargetSetsGitInfo(t *testing.T) {
	vaultPath := setupRenderVault(t)
	cfg, _ := vault.LoadConfig(vaultPath)
	// Target in StagedFiles — git policy default is Refuse, so use Force to bypass checksum,
	// but the policy will still refuse. Use DryRun to skip policy check.
	detector := &fakeDetector{state: git.RepoState{
		RepoDetected: true, WorkingTreeClean: false,
		StagedFiles: []string{"projects/test.md"},
	}}
	checker, _ := git.NewPolicyChecker(cfg.Git)

	result, err := marker.RenderRegion(marker.RenderConfig{
		VaultPath: vaultPath, Target: "projects/test.md", SectionKey: "related",
		DryRun: true, Detector: detector, Checker: checker,
	})
	require.NoError(t, err)
	assert.False(t, result.Git.TargetFileClean)
}

func TestRenderRegion_NoDetector(t *testing.T) {
	vaultPath := setupRenderVault(t)

	result, err := marker.RenderRegion(marker.RenderConfig{
		VaultPath: vaultPath, Target: "projects/test.md", SectionKey: "related",
		DryRun: true,
	})
	require.NoError(t, err)
	assert.False(t, result.Git.RepoDetected)
}

func TestRenderRegion_FrontmatterNoNewline(t *testing.T) {
	vaultPath := setupRenderVault(t)
	// Write a file that starts with --- but has no newline
	require.NoError(t, os.WriteFile(filepath.Join(vaultPath, "projects/nonl.md"),
		[]byte("---"), 0o644))

	cfg, _ := vault.LoadConfig(vaultPath)
	detector := &fakeDetector{state: git.RepoState{RepoDetected: true, WorkingTreeClean: true}}
	checker, _ := git.NewPolicyChecker(cfg.Git)

	_, err := marker.RenderRegion(marker.RenderConfig{
		VaultPath: vaultPath, Target: "projects/nonl.md", SectionKey: "related",
		Detector: detector, Checker: checker,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse_error")
}

func TestRenderRegion_FrontmatterNoClosingDelimiter(t *testing.T) {
	vaultPath := setupRenderVault(t)
	require.NoError(t, os.WriteFile(filepath.Join(vaultPath, "projects/noclosing.md"),
		[]byte("---\nid: x\ntype: project\n"), 0o644))

	cfg, _ := vault.LoadConfig(vaultPath)
	detector := &fakeDetector{state: git.RepoState{RepoDetected: true, WorkingTreeClean: true}}
	checker, _ := git.NewPolicyChecker(cfg.Git)

	_, err := marker.RenderRegion(marker.RenderConfig{
		VaultPath: vaultPath, Target: "projects/noclosing.md", SectionKey: "related",
		Detector: detector, Checker: checker,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse_error")
}

func TestRenderRegion_FrontmatterMissingType(t *testing.T) {
	vaultPath := setupRenderVault(t)
	require.NoError(t, os.WriteFile(filepath.Join(vaultPath, "projects/notype.md"),
		[]byte("---\nid: proj-notype\n---\n# No type\n"), 0o644))

	cfg, _ := vault.LoadConfig(vaultPath)
	detector := &fakeDetector{state: git.RepoState{RepoDetected: true, WorkingTreeClean: true}}
	checker, _ := git.NewPolicyChecker(cfg.Git)

	_, err := marker.RenderRegion(marker.RenderConfig{
		VaultPath: vaultPath, Target: "projects/notype.md", SectionKey: "related",
		Detector: detector, Checker: checker,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse_error")
}

func TestRenderRegion_AllSections_MissingTemplate(t *testing.T) {
	vaultPath := setupRenderVault(t)
	// Note with a marker for a section that has no template
	noteContent := "---\nid: proj-notmpl\ntype: project\nstatus: active\ntitle: NoTmpl\n---\n# NoTmpl\n\n<!-- VAULTMIND:GENERATED:missing_section:START -->\nold\n<!-- VAULTMIND:GENERATED:missing_section:END -->\n"
	require.NoError(t, os.WriteFile(filepath.Join(vaultPath, "projects/notmpl.md"), []byte(noteContent), 0o644))

	cfg, _ := vault.LoadConfig(vaultPath)
	detector := &fakeDetector{state: git.RepoState{RepoDetected: true, WorkingTreeClean: true}}
	checker, _ := git.NewPolicyChecker(cfg.Git)

	_, err := marker.RenderRegion(marker.RenderConfig{
		VaultPath: vaultPath, Target: "projects/notmpl.md", SectionKey: "",
		Detector: detector, Checker: checker,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "template_not_found")
}

func TestRenderRegion_DiffWithWrite(t *testing.T) {
	vaultPath := setupRenderVault(t)
	cfg, _ := vault.LoadConfig(vaultPath)
	detector := &fakeDetector{state: git.RepoState{RepoDetected: true, WorkingTreeClean: true}}
	checker, _ := git.NewPolicyChecker(cfg.Git)

	result, err := marker.RenderRegion(marker.RenderConfig{
		VaultPath: vaultPath, Target: "projects/test.md", SectionKey: "related",
		Diff: true, Detector: detector, Checker: checker,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, result.Diff)
	assert.NotEmpty(t, result.WriteHash)
}

func TestRenderRegion_AllSections_ChecksumMismatch(t *testing.T) {
	vaultPath := setupRenderVault(t)
	// Write a note with a marker that has an incorrect checksum (simulating hand-edit)
	noteContent := "---\nid: proj-cs\ntype: project\nstatus: active\ntitle: CS\n---\n# CS\n\n<!-- VAULTMIND:GENERATED:related:START -->\n<!-- checksum:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa -->\nsome content\n<!-- VAULTMIND:GENERATED:related:END -->\n"
	require.NoError(t, os.WriteFile(filepath.Join(vaultPath, "projects/cs.md"), []byte(noteContent), 0o644))

	cfg, _ := vault.LoadConfig(vaultPath)
	detector := &fakeDetector{state: git.RepoState{RepoDetected: true, WorkingTreeClean: true}}
	checker, _ := git.NewPolicyChecker(cfg.Git)

	_, err := marker.RenderRegion(marker.RenderConfig{
		VaultPath: vaultPath, Target: "projects/cs.md", SectionKey: "",
		Detector: detector, Checker: checker,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checksum_mismatch")
}

func TestRenderRegion_AllSections_GitPolicyWarn(t *testing.T) {
	vaultPath := setupRenderVault(t)
	sectionDir := filepath.Join(vaultPath, ".vaultmind", "sections", "project")
	require.NoError(t, os.WriteFile(filepath.Join(sectionDir, "backlinks.md"),
		[]byte("```dataview\nLIST FROM [[this]]\n```\n"), 0o644))

	noteContent := "---\nid: proj-warn\ntype: project\nstatus: active\ntitle: Warn\n---\n# Warn\n\n<!-- VAULTMIND:GENERATED:related:START -->\nold\n<!-- VAULTMIND:GENERATED:related:END -->\n"
	require.NoError(t, os.WriteFile(filepath.Join(vaultPath, "projects/warn.md"), []byte(noteContent), 0o644))

	cfg, _ := vault.LoadConfig(vaultPath)
	detector := &fakeDetector{state: git.RepoState{
		RepoDetected: true, WorkingTreeClean: false,
		UnstagedFiles: []string{"other/unrelated.md"},
	}}
	checker, _ := git.NewPolicyChecker(cfg.Git)

	result, err := marker.RenderRegion(marker.RenderConfig{
		VaultPath: vaultPath, Target: "projects/warn.md", SectionKey: "",
		Detector: detector, Checker: checker,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, result.WriteHash)
	assert.Len(t, result.Warnings, 1)
}
