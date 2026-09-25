package hooks

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Install reports every way the disk can refuse it, instead of claiming an
// install that did not happen. Where the failure is a write, the same case
// also runs as a dry run, which writes nothing and so must not hit it.

func TestInstall_ScriptsDirCannotBeCreated(t *testing.T) {
	project := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(project, []byte("a file"), 0o600))

	_, err := Install(InstallConfig{ProjectDir: project})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "creating")
}

func TestInstall_DeclaredProfileCannotBeWritten(t *testing.T) {
	project := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(baseDir(project, ""), profileFilename), 0o750))

	_, err := Install(InstallConfig{ProjectDir: project})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "writing declared profile")

	_, err = Install(InstallConfig{ProjectDir: project, DryRun: true})
	assert.NoError(t, err)
}

func TestInstall_ScriptCannotBeWritten(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("directory permissions do not block writes here")
	}
	project := t.TempDir()
	scripts := ScriptsDir(project, "")
	require.NoError(t, os.MkdirAll(scripts, 0o750))
	require.NoError(t, os.Chmod(scripts, 0o500))       //nolint:gosec // read-only on purpose
	t.Cleanup(func() { _ = os.Chmod(scripts, 0o750) }) //nolint:gosec // restore for TempDir cleanup

	_, err := Install(InstallConfig{ProjectDir: project})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "writing")

	res, err := Install(InstallConfig{ProjectDir: project, DryRun: true})
	require.NoError(t, err)
	assert.NotEmpty(t, res.Written, "a dry run lists what it would write")
}
