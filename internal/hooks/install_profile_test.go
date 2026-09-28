package hooks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// A bare `hooks install` in a fresh project installs the knowledge set —
// the same default `init` uses. Most adopters bring a knowledge vault; a
// persona loader wired into a project with no persona is noise on every turn.
func TestInstallProfile_FreshProjectIsKnowledge(t *testing.T) {
	p, err := InstallProfile(t.TempDir())
	require.NoError(t, err)
	require.Equal(t, ProfileKnowledge, p)
}

// A declaration always wins: the project already chose.
func TestInstallProfile_DeclaredWins(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, WriteDeclaredProfile(dir, ProfilePersona))
	p, err := InstallProfile(dir)
	require.NoError(t, err)
	require.Equal(t, ProfilePersona, p)
}

// Installs made before profiles existed declared nothing, and every one of
// them installed the full set. Re-running the upgrade command there must not
// strip their persona and episode hooks — undeclared-but-installed stays full.
// Checked for both agents' install locations.
func TestInstallProfile_UndeclaredExistingInstallStaysFull(t *testing.T) {
	for _, a := range []Agent{AgentClaude, AgentCodex} {
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(ScriptsDir(dir, a), 0o750))
		p, err := InstallProfile(dir)
		require.NoError(t, err)
		require.Equal(t, ProfileFull, p, "agent %v", a)
	}
}

// A garbled declaration stays loud here too — never a silent default.
func TestInstallProfile_UnknownDeclarationIsLoud(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude", profileFilename), []byte("fulll\n"), 0o600))
	_, err := InstallProfile(dir)
	require.Error(t, err)
}
