package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// init --profile knowledge makes a project knowledge base and says what to do
// with one — not "who am I" and identity/.
func TestInit_KnowledgeProfileScaffoldsAKnowledgeBase(t *testing.T) {
	vault := filepath.Join(t.TempDir(), "docs-kb")

	out, _, err := runRootCmd(t, "init", vault, "--profile", "knowledge")
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(vault, "decisions", "decision-example.md"))
	assert.NoDirExists(t, filepath.Join(vault, "identity"))
	assert.Contains(t, out.String(), "decisions/")
	assert.NotContains(t, out.String(), "who am I")
	assert.NotContains(t, out.String(), "identity/")
}

// With --wire-hooks, the profile named on init is the one the hooks install.
func TestInit_KnowledgeProfileWiresTheKnowledgeHooks(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "proj")
	require.NoError(t, os.MkdirAll(project, 0o750))
	vault := filepath.Join(project, "docs-kb")

	_, _, err := runRootCmd(t, "init", vault, "--profile", "knowledge", "--wire-hooks", "--project-dir", project)
	require.NoError(t, err)
	declared, err := os.ReadFile(filepath.Join(project, ".claude", "vaultmind-profile")) //nolint:gosec // test path
	require.NoError(t, err)
	assert.Equal(t, "knowledge", strings.TrimSpace(string(declared)))
	_, err = os.Stat(filepath.Join(project, ".claude", "scripts", "load-persona.sh"))
	assert.True(t, os.IsNotExist(err), "a knowledge project does not load a persona")
}

func TestInit_UnknownProfileIsRefusedBeforeWriting(t *testing.T) {
	vault := filepath.Join(t.TempDir(), "v")
	_, _, err := runRootCmd(t, "init", vault, "--profile", "wiki")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "knowledge")
	assert.NoDirExists(t, vault)
}

// v0.10.0: the knowledge vault is the product, so init's default scaffold is
// the knowledge base. The persona vault is one flag away.
func TestInit_DefaultsToTheKnowledgeScaffold(t *testing.T) {
	vault := filepath.Join(t.TempDir(), "kb")

	out, _, err := runRootCmd(t, "init", vault)
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(vault, "decisions", "decision-example.md"))
	assert.NoDirExists(t, filepath.Join(vault, "identity"))
	assert.NotContains(t, out.String(), "who am I")
}

func TestInit_PersonaAndFullProfilesStillScaffoldTheIdentityVault(t *testing.T) {
	for _, profile := range []string{"persona", "full"} {
		vault := filepath.Join(t.TempDir(), "me")
		_, _, err := runRootCmd(t, "init", vault, "--profile", profile)
		require.NoError(t, err, profile)
		assert.FileExists(t, filepath.Join(vault, "identity", "who-am-i.md"), profile)
	}
}

// A fresh project wired without --profile gets hooks for the vault init just
// made — knowledge — not the persona set a project with nothing declared
// used to default to.
func TestInit_WireHooksInAFreshProjectDefaultsToKnowledgeHooks(t *testing.T) {
	project := filepath.Join(t.TempDir(), "proj")
	require.NoError(t, os.MkdirAll(project, 0o750))

	_, _, err := runRootCmd(t, "init", filepath.Join(project, "kb"), "--wire-hooks", "--project-dir", project)
	require.NoError(t, err)
	declared, err := os.ReadFile(filepath.Join(project, ".claude", "vaultmind-profile")) //nolint:gosec // test path
	require.NoError(t, err)
	assert.Equal(t, "knowledge", strings.TrimSpace(string(declared)))
	assert.NoFileExists(t, filepath.Join(project, ".claude", "scripts", "load-persona.sh"))
}

// A project that declared its hooks keeps them: adding a knowledge vault to a
// persona project does not rewire the project.
func TestInit_WireHooksKeepsAProjectsDeclaredProfile(t *testing.T) {
	project := filepath.Join(t.TempDir(), "proj")
	require.NoError(t, os.MkdirAll(filepath.Join(project, ".claude"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(project, ".claude", "vaultmind-profile"), []byte("full\n"), 0o600))

	_, _, err := runRootCmd(t, "init", filepath.Join(project, "kb"), "--wire-hooks", "--project-dir", project)
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(project, "kb", "decisions", "decision-example.md"), "the vault is still a knowledge base")
	declared, err := os.ReadFile(filepath.Join(project, ".claude", "vaultmind-profile")) //nolint:gosec // test path
	require.NoError(t, err)
	assert.Equal(t, "full", strings.TrimSpace(string(declared)), "the project's hooks are unchanged")
}
