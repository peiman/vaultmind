package cmd

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBodyRawFallback_CLIDeliveryAndDoctor(t *testing.T) {
	for _, nullRaw := range []bool{true, false} {
		t.Run(map[bool]string{true: "migrated NULL", false: "intentionally empty"}[nullRaw], func(t *testing.T) {
			dir := buildNamedTestVault(t, "legacy", map[string]string{"legacy.md": "---\nid: legacy\ntitle: Legacy\ntype: concept\n---\nLegacy stored body.\n"})
			cfg, err := vault.LoadConfig(dir)
			require.NoError(t, err)
			dbPath := filepath.Join(dir, cfg.Index.DBPath)
			db, err := index.Open(dbPath)
			require.NoError(t, err)
			if nullRaw {
				_, err = db.Exec("ALTER TABLE notes DROP COLUMN body_raw")
				require.NoError(t, err)
				_, err = db.Exec("DELETE FROM goose_db_version WHERE version_id=9")
			} else {
				_, err = db.Exec("UPDATE notes SET body_raw=''")
			}
			require.NoError(t, err)
			require.NoError(t, db.Close())
			// The CLI opens and migrates the old index, with no indexing pass.
			out, _, err := runRootCmd(t, "ask", "Legacy", "--vault", dir, "--json", "--budget", "900", "--excerpt", "160")
			require.NoError(t, err)
			var ask struct {
				Result struct {
					Context struct {
						Target struct {
							Body string `json:"body"`
						} `json:"target"`
					} `json:"context"`
				} `json:"result"`
			}
			require.NoError(t, json.Unmarshal(out.Bytes(), &ask))
			want := ""
			if nullRaw {
				want = "Legacy stored body."
			}
			assert.Equal(t, want, ask.Result.Context.Target.Body)
			for _, jsonOut := range []bool{true, false} {
				args := []string{"note", "get", "legacy", "--vault", dir}
				if jsonOut {
					args = append(args, "--json")
				}
				out, _, err = runRootCmd(t, args...)
				require.NoError(t, err)
				if jsonOut {
					var note struct {
						Result struct {
							Body string `json:"body"`
						} `json:"result"`
					}
					require.NoError(t, json.Unmarshal(out.Bytes(), &note))
					assert.Equal(t, want, note.Result.Body)
				} else if nullRaw {
					assert.Contains(t, out.String(), want)
				} else {
					assert.NotContains(t, out.String(), "Legacy stored body.")
				}
			}
			out, _, err = runRootCmd(t, "doctor", "--vault", dir)
			require.NoError(t, err)
			assert.Equal(t, nullRaw, strings.Contains(out.String(), "1 notes have no stored body as written — run vaultmind index"))
			out, _, err = runRootCmd(t, "doctor", "--vault", dir, "--json")
			require.NoError(t, err)
			var doc struct {
				Result struct {
					Issues struct {
						MissingRawBodies int `json:"missing_raw_bodies"`
					} `json:"issues"`
				} `json:"result"`
			}
			require.NoError(t, json.Unmarshal(out.Bytes(), &doc))
			count := 0
			if nullRaw {
				count = 1
			}
			assert.Equal(t, count, doc.Result.Issues.MissingRawBodies)
			if nullRaw {
				_, err = index.NewIndexer(dir, dbPath, cfg).Incremental()
				require.NoError(t, err)
				out, _, err = runRootCmd(t, "doctor", "--vault", dir)
				require.NoError(t, err)
				assert.NotContains(t, out.String(), "no stored body as written")
			}
		})
	}
}

func TestBodyRawFallback_DoctorWriterError(t *testing.T) {
	require.Error(t, writeMissingRawBodies(&failAfterNWriter{ok: 0}, 1))
}
