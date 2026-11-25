package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// vaultmind runs from hooks inside every project and vault, some of them
// public repositories. A command must never write an audit log into the
// directory it runs from: ckeletin 0.2.13 turned the file log on by default at
// ./logs/vaultmind.log, at debug level, which would leave query and note text
// in a logs/ folder that a commit could pick up.
func TestCommand_WritesNoLogFolderInTheWorkingDirectory(t *testing.T) {
	work := t.TempDir()
	t.Chdir(work)

	_, _, err := runRootCmd(t, "version")
	require.NoError(t, err)

	_, statErr := os.Stat(filepath.Join(work, "logs"))
	assert.True(t, os.IsNotExist(statErr), "a command must not create ./logs in its working directory")
}
