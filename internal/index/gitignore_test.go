package index_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIndex_OldVaultGetsGitignore(t *testing.T) {
	for _, full := range []bool{false, true} {
		for _, existing := range []bool{false, true} {
			t.Run(fmtIndexCase(full, existing), func(t *testing.T) {
				dir := t.TempDir()
				state := filepath.Join(dir, ".vaultmind")
				require.NoError(t, os.Mkdir(state, 0o750))
				require.NoError(t, os.WriteFile(filepath.Join(state, "config.yaml"), []byte("types: {}\n"), 0o600))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "note.md"), []byte("# Existing note\n"), 0o600))
				ignore := filepath.Join(state, ".gitignore")
				const custom = "# user's policy\n*.db\n"
				if existing {
					require.NoError(t, os.WriteFile(ignore, []byte(custom), 0o600))
				}
				cfg, err := vault.LoadConfig(dir)
				require.NoError(t, err)
				idx := index.NewIndexer(dir, filepath.Join(state, "index.db"), cfg)
				if full {
					_, err = idx.Rebuild()
				} else {
					_, err = idx.Incremental()
				}
				require.NoError(t, err)
				body, err := os.ReadFile(ignore)
				require.NoError(t, err)
				if existing {
					assert.Equal(t, custom, string(body))
				} else {
					assert.Contains(t, string(body), "*\n!.gitignore\n!config.yaml\n")
				}
			})
		}
	}
}

func fmtIndexCase(full, existing bool) string {
	name := "incremental"
	if full {
		name = "full"
	}
	if existing {
		name += "/user-gitignore"
	}
	return name
}

func TestIndex_CustomDatabaseStillCreatesGitignore(t *testing.T) {
	for _, full := range []bool{false, true} {
		t.Run(fmtIndexCase(full, false), func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "note.md"), []byte("# Existing note\n"), 0o600))
			cfg, err := vault.LoadConfig(dir)
			require.NoError(t, err)
			idx := index.NewIndexer(dir, filepath.Join(dir, "custom", "index.db"), cfg)
			if full {
				_, err = idx.Rebuild()
			} else {
				_, err = idx.Incremental()
			}
			require.NoError(t, err)
			assert.FileExists(t, filepath.Join(dir, ".vaultmind", ".gitignore"))
		})
	}
}
