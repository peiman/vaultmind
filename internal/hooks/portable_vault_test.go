package hooks

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Run the generated assignments through a real shell: only the leading
// project reference expands; characters in a directory name stay literal.
func TestPortableVaultAssignments_ExpandOnlyProjectReferences(t *testing.T) {
	project := filepath.Join(t.TempDir(), "project with 'quotes")
	for _, tc := range []struct {
		name   string
		vaults []string
		want   string
	}{
		{"project", []string{"$CLAUDE_PROJECT_DIR/knowledge"}, project + "/knowledge"},
		{"project root", []string{"$CLAUDE_PROJECT_DIR"}, project},
		{"literal suffix", []string{"$CLAUDE_PROJECT_DIR/notes $CLAUDE_PROJECT_DIR \"`echo bad` $(echo bad) \\"}, project + "/notes $CLAUDE_PROJECT_DIR \"`echo bad` $(echo bad) \\"},
		{"absolute", []string{"/outside/$CLAUDE_PROJECT_DIR/knowledge"}, "/outside/$CLAUDE_PROJECT_DIR/knowledge"},
		{"mixed list", []string{"$CLAUDE_PROJECT_DIR/knowledge", "/outside/$CLAUDE_PROJECT_DIR/notes"}, project + "/knowledge,/outside/$CLAUDE_PROJECT_DIR/notes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// canonicalHooksWith emits this assignment on searching hooks.
			h := canonicalHooksWith("", tc.vaults, func(string) string { return "unused" })
			var command string
			for _, hook := range h {
				if hook.Script == hookUserPromptSubmitScript {
					command = hook.Group.Hooks[0].Command
				}
			}
			require.NotEmpty(t, command)
			command = command[:len(command)-len("bash unused")] + `bash -c 'printf "%s" "$VAULTMIND_VAULTS"'`
			shell := exec.Command("bash", "-c", command)
			shell.Env = []string{"CLAUDE_PROJECT_DIR=" + project, "PATH=/usr/bin:/bin"}
			got, err := shell.CombinedOutput()
			require.NoError(t, err, string(got))
			assert.Equal(t, tc.want, string(got))
		})
	}
}

func TestPortableVaultRef_BoundariesAndScopes(t *testing.T) {
	project := t.TempDir()
	for _, tc := range []struct {
		name, vault, want string
		scope             Scope
	}{
		{"empty", "", "", ScopeProject},
		{"already portable", "$CLAUDE_PROJECT_DIR/knowledge", "$CLAUDE_PROJECT_DIR/knowledge", ScopeProject},
		{"root", project, "$CLAUDE_PROJECT_DIR", ScopeProject},
		{"inside", filepath.Join(project, "notes", "..", "knowledge"), "$CLAUDE_PROJECT_DIR/knowledge", ScopeProject},
		{"sibling", project + "-other", project + "-other", ScopeProject},
		{"parent", filepath.Dir(project), filepath.Dir(project), ScopeProject},
		{"user", filepath.Join(project, "knowledge"), filepath.Join(project, "knowledge"), ScopeUser},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := PortableVaultRef(project, tc.vault, tc.scope)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
