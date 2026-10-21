package initvault_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/initvault"
	"github.com/peiman/vaultmind/internal/testgit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInit_IgnoresDerivedFiles(t *testing.T) {
	for _, scaffold := range []struct {
		name string
		init func(string) (*initvault.Result, error)
	}{
		{"persona", initvault.Init},
		{"knowledge", initvault.InitKnowledge},
	} {
		t.Run(scaffold.name, func(t *testing.T) {
			root := t.TempDir()
			testgit.Run(t, root, "init")
			dst := filepath.Join(root, "knowledge")
			res, err := scaffold.init(dst)
			require.NoError(t, err)
			body, err := os.ReadFile(filepath.Join(dst, ".vaultmind", ".gitignore"))
			require.NoError(t, err)
			assert.Equal(t, "*\n!.gitignore\n!config.yaml\n!fingerprint.txt\n!sections/\n!sections/**\n!scripts/\n!scripts/**\n!vaultmind-profile\n", string(body))
			files := 0
			require.NoError(t, filepath.WalkDir(dst, func(_ string, d os.DirEntry, err error) error {
				if err == nil && !d.IsDir() {
					files++
				}
				return err
			}))
			assert.Equal(t, files, res.FilesAdded)

			for _, tc := range []struct {
				path    string
				ignored bool
			}{
				{"index.db", true}, {"index.db-wal", true}, {"index.db-shm", true},
				{"cache/embedding.bin", true}, {"index.lock", true},
				// The vault carries its own telemetry identity across machines (telemetry.EnsureFingerprint).
				{"fingerprint.txt", false},
				{".gitignore", false}, {"config.yaml", false},
				{"sections/concept/summary.md", false}, {"scripts/vault-recall.sh", false},
				{"vaultmind-profile", false},
			} {
				cmd := testgit.Command(t, root, "check-ignore", "--quiet", "--no-index", "knowledge/.vaultmind/"+tc.path)
				err := cmd.Run()
				if tc.ignored {
					assert.NoError(t, err, "%s must be ignored", tc.path)
				} else {
					var exitErr *exec.ExitError
					require.ErrorAs(t, err, &exitErr, "%s must remain committable", tc.path)
					assert.Equal(t, 1, exitErr.ExitCode(), tc.path)
				}
			}
		})
	}
}

func TestWriteConfigOnly_PreservesUserGitignore(t *testing.T) {
	for _, content := range []string{"", "# my policy\nindex.db*\n"} {
		t.Run(content, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.Mkdir(filepath.Join(dir, ".vaultmind"), 0o750))
			path := filepath.Join(dir, ".vaultmind", ".gitignore")
			require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
			_, err := initvault.WriteConfigOnly(dir)
			require.NoError(t, err)
			body, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, content, string(body))
		})
	}
}

func TestWriteConfigOnly_AddsGitignoreToExistingVault(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, ".vaultmind"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"), []byte("types: {}\n"), 0o600))
	written, err := initvault.WriteConfigOnly(dir)
	require.NoError(t, err)
	assert.False(t, written, "the config was already present")
	assert.FileExists(t, filepath.Join(dir, ".vaultmind", ".gitignore"))
}
