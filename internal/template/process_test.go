package template

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeTempTemplate writes content to a temp file and returns its path.
func writeTempTemplate(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "template.md")
	require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	return p
}

// ─── SubstituteVars ───────────────────────────────────────────────────────────

func TestSubstituteVars_AllKnownVars(t *testing.T) {
	vars := map[string]string{
		"id":         "project-foo",
		"type":       "project",
		"title":      "Foo Project",
		"created":    "2026-04-04",
		"updated":    "2026-04-04T12:00:00Z",
		"vm_updated": "2026-04-04T12:00:00Z",
		"date":       "2026-04-04",
		"datetime":   "2026-04-04T12:00:00Z",
		"path":       "projects/foo.md",
	}
	content := "id: <%=id%>\ntype: <%=type%>\ntitle: <%=title%>\ncreated: <%=created%>\nupdated: <%=updated%>\nvm_updated: <%=vm_updated%>\ndate: <%=date%>\ndatetime: <%=datetime%>\npath: <%=path%>"
	result, warnings := SubstituteVars(content, vars)
	assert.Empty(t, warnings)
	assert.Contains(t, result, "id: project-foo")
	assert.Contains(t, result, "type: project")
	assert.Contains(t, result, "title: Foo Project")
	assert.Contains(t, result, "created: 2026-04-04")
	assert.Contains(t, result, "vm_updated: 2026-04-04T12:00:00Z")
	assert.Contains(t, result, "path: projects/foo.md")
}

func TestSubstituteVars_UnrecognizedVar(t *testing.T) {
	vars := map[string]string{
		"id": "project-foo",
	}
	content := "id: <%=id%>\nunknown: <%=foobar%>"
	result, warnings := SubstituteVars(content, vars)
	assert.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "foobar")
	// Unrecognized var is left as-is.
	assert.Contains(t, result, "<%=foobar%>")
	assert.Contains(t, result, "id: project-foo")
}

func TestSubstituteVars_NoVars(t *testing.T) {
	content := "# Plain content\nNo variables here."
	result, warnings := SubstituteVars(content, map[string]string{})
	assert.Empty(t, warnings)
	assert.Equal(t, content, result)
}

// ─── Process ─────────────────────────────────────────────────────────────────

func TestProcess_WithTemplate(t *testing.T) {
	tmplContent := `---
id: <%=id%>
type: <%=type%>
title: <%=title%>
created: <%=created%>
vm_updated: <%=vm_updated%>
---
# <%=title%>

Body text here.
`
	tmplPath := writeTempTemplate(t, tmplContent)
	cfg := ProcessConfig{
		VaultPath:    "/vault",
		Path:         "projects/my-project.md",
		Type:         "project",
		Fields:       map[string]string{"title": "My Project"},
		Body:         "",
		TemplatePath: tmplPath,
	}
	result, err := Process(cfg)
	require.NoError(t, err)
	content := string(result.Content)
	assert.Contains(t, content, "id: project-my-project")
	assert.Contains(t, content, "type: project")
	assert.Contains(t, content, "title: My Project")
	assert.NotEmpty(t, result.ID)
	assert.Equal(t, "projects/my-project.md", result.Path)
}

func TestProcess_WithFieldOverride(t *testing.T) {
	tmplContent := `---
id: <%=id%>
type: <%=type%>
title: <%=title%>
created: <%=created%>
vm_updated: <%=vm_updated%>
status: draft
---
Body.
`
	tmplPath := writeTempTemplate(t, tmplContent)
	cfg := ProcessConfig{
		VaultPath:    "/vault",
		Path:         "projects/my-project.md",
		Type:         "project",
		Fields:       map[string]string{"title": "My Project", "status": "active"},
		Body:         "",
		TemplatePath: tmplPath,
	}
	result, err := Process(cfg)
	require.NoError(t, err)
	content := string(result.Content)
	assert.Contains(t, content, "status: active")
}

func TestProcess_WithExplicitID(t *testing.T) {
	tmplContent := `---
id: <%=id%>
type: <%=type%>
title: <%=title%>
created: <%=created%>
vm_updated: <%=vm_updated%>
---
Body.
`
	tmplPath := writeTempTemplate(t, tmplContent)
	cfg := ProcessConfig{
		VaultPath:    "/vault",
		Path:         "projects/my-project.md",
		Type:         "project",
		Fields:       map[string]string{"title": "My Project", "id": "custom-id-override"},
		Body:         "",
		TemplatePath: tmplPath,
	}
	result, err := Process(cfg)
	require.NoError(t, err)
	content := string(result.Content)
	assert.Contains(t, content, "id: custom-id-override")
	assert.Equal(t, "custom-id-override", result.ID)
}

func TestProcess_WithBodyOverride(t *testing.T) {
	tmplContent := `---
id: <%=id%>
type: <%=type%>
title: <%=title%>
created: <%=created%>
vm_updated: <%=vm_updated%>
---
Original template body.
`
	tmplPath := writeTempTemplate(t, tmplContent)
	cfg := ProcessConfig{
		VaultPath:    "/vault",
		Path:         "projects/my-project.md",
		Type:         "project",
		Fields:       map[string]string{"title": "My Project"},
		Body:         "# Custom Body\n\nThis is the override body.",
		TemplatePath: tmplPath,
	}
	result, err := Process(cfg)
	require.NoError(t, err)
	content := string(result.Content)
	assert.Contains(t, content, "# Custom Body")
	assert.NotContains(t, content, "Original template body")
}

func TestProcess_MissingTemplate(t *testing.T) {
	cfg := ProcessConfig{
		VaultPath:    "/vault",
		Path:         "projects/my-project.md",
		Type:         "project",
		Fields:       map[string]string{"title": "My Project"},
		Body:         "",
		TemplatePath: "/nonexistent/path/template.md",
	}
	result, err := Process(cfg)
	require.NoError(t, err)
	// Should produce a minimal note with a warning.
	assert.NotEmpty(t, result.Warnings)
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "template") || strings.Contains(w, "not found") || strings.Contains(w, "missing") {
			found = true
			break
		}
	}
	assert.True(t, found, "expected a warning about missing template, got: %v", result.Warnings)
	content := string(result.Content)
	assert.Contains(t, content, "id:")
	assert.Contains(t, content, "type:")
}

func TestProcess_CoreFieldsAlwaysSet(t *testing.T) {
	// Template has no core fields.
	tmplContent := `---
title: <%=title%>
---
Body text.
`
	tmplPath := writeTempTemplate(t, tmplContent)
	cfg := ProcessConfig{
		VaultPath:    "/vault",
		Path:         "projects/core-test.md",
		Type:         "project",
		Fields:       map[string]string{"title": "Core Test"},
		Body:         "",
		TemplatePath: tmplPath,
	}
	result, err := Process(cfg)
	require.NoError(t, err)
	content := string(result.Content)
	// Core fields must be present even if template omitted them.
	assert.Contains(t, content, "id:")
	assert.Contains(t, content, "type:")
	assert.Contains(t, content, "created:")
	assert.Contains(t, content, "vm_updated:")
}

// ─── FinalFrontmatter ─────────────────────────────────────────────────────────

func TestProcess_FinalFrontmatterPopulated(t *testing.T) {
	tmplContent := `---
id: <%=id%>
type: <%=type%>
title: <%=title%>
created: <%=created%>
vm_updated: <%=vm_updated%>
status: draft
---
Body.
`
	tmplPath := writeTempTemplate(t, tmplContent)
	cfg := ProcessConfig{
		VaultPath:    "/vault",
		Path:         "projects/my-project.md",
		Type:         "project",
		Fields:       map[string]string{"title": "My Project", "status": "active"},
		Body:         "",
		TemplatePath: tmplPath,
	}
	result, err := Process(cfg)
	require.NoError(t, err)
	require.NotNil(t, result.FinalFrontmatter, "FinalFrontmatter must not be nil")
	assert.Equal(t, "active", result.FinalFrontmatter["status"])
	assert.Equal(t, "project", result.FinalFrontmatter["type"])
}

// ─── I4: RequiredFields in minimal template fallback ──────────────────────────

func TestProcess_MinimalFallback_IncludesRequiredFields(t *testing.T) {
	// I4: When template is missing, required fields should appear in output.
	cfg := ProcessConfig{
		VaultPath:      "/vault",
		Path:           "projects/my-project.md",
		Type:           "project",
		Fields:         map[string]string{},
		Body:           "",
		TemplatePath:   "/nonexistent/template.md",
		RequiredFields: []string{"goal", "status"},
	}
	result, err := Process(cfg)
	require.NoError(t, err)
	require.NotNil(t, result.FinalFrontmatter, "FinalFrontmatter must not be nil")
	assert.Contains(t, result.FinalFrontmatter, "goal", "required field 'goal' should be present in minimal fallback")
	assert.Contains(t, result.FinalFrontmatter, "status", "required field 'status' should be present in minimal fallback")
}

func TestProcess_MinimalFallback_RequiredFieldsDoNotOverrideUserValues(t *testing.T) {
	// I4: User-provided values must not be overwritten by empty defaults.
	cfg := ProcessConfig{
		VaultPath:      "/vault",
		Path:           "projects/my-project.md",
		Type:           "project",
		Fields:         map[string]string{"goal": "ship it"},
		Body:           "",
		TemplatePath:   "/nonexistent/template.md",
		RequiredFields: []string{"goal"},
	}
	result, err := Process(cfg)
	require.NoError(t, err)
	require.NotNil(t, result.FinalFrontmatter)
	assert.Equal(t, "ship it", result.FinalFrontmatter["goal"], "user-provided value must not be overwritten")
}

// ─── I5: Title fallback uses filename, not generated ID ───────────────────────

func TestProcess_TitleFallback_UsesFilename(t *testing.T) {
	// I5: When no title override is given, title should derive from filename.
	cfg := ProcessConfig{
		VaultPath:    "/vault",
		Path:         "projects/payment-retries.md",
		Type:         "project",
		Fields:       map[string]string{},
		Body:         "",
		TemplatePath: "/nonexistent/template.md",
	}
	result, err := Process(cfg)
	require.NoError(t, err)
	require.NotNil(t, result.FinalFrontmatter)
	// Title should be derived from filename "payment-retries", not "project-payment-retries"
	title, ok := result.FinalFrontmatter["title"]
	require.True(t, ok, "title must be present in frontmatter")
	titleStr, ok := title.(string)
	require.True(t, ok, "title must be a string")
	assert.Equal(t, "payment-retries", titleStr, "title should be filename without extension, not type-prefixed generated ID")
}

func TestProcess_TitleFallback_ExplicitTitleOverridesFilename(t *testing.T) {
	// I5: Explicit title via Fields must take precedence over filename fallback.
	cfg := ProcessConfig{
		VaultPath:    "/vault",
		Path:         "projects/payment-retries.md",
		Type:         "project",
		Fields:       map[string]string{"title": "Payment Retries"},
		Body:         "",
		TemplatePath: "/nonexistent/template.md",
	}
	result, err := Process(cfg)
	require.NoError(t, err)
	require.NotNil(t, result.FinalFrontmatter)
	assert.Equal(t, "Payment Retries", result.FinalFrontmatter["title"])
}
