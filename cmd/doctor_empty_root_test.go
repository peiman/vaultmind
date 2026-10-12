package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// doctorNotesOutside indexes two notes into a database that is not inside
// the vault, so the vault directory can be emptied without deleting the rows
// doctor is looking at.
func doctorNotesOutside(t *testing.T) (dir string, db *index.DB) {
	t.Helper()
	dir = t.TempDir()
	body := func(id, title string) string {
		return "---\nid: " + id + "\ntype: concept\ntitle: " + title + "\ncreated: 2026-10-12\n---\n" + title + "\n"
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "keep.md"), []byte(body("keep-note", "Keep")), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "drop.md"), []byte(body("drop-note", "Drop")), 0o644))
	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	dbPath := filepath.Join(t.TempDir(), "index.db")
	_, err = index.NewIndexer(dir, dbPath, cfg).Rebuild()
	require.NoError(t, err)
	db, err = index.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return dir, db
}

func doctorText(t *testing.T, db *index.DB, dir string) string {
	t.Helper()
	res, err := query.Doctor(db, dir, nil)
	require.NoError(t, err)
	var buf bytes.Buffer
	require.NoError(t, writeDoctorHuman(&buf, res, false))
	return buf.String()
}

// A root the walk did not enter still has rows. Prescribing `vaultmind index`
// hands the operator a command that refuses to delete them.
func TestDoctor_EmptyRootDoesNotPrescribeIndex(t *testing.T) {
	dir, db := doctorNotesOutside(t)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		require.NoError(t, os.RemoveAll(filepath.Join(dir, e.Name())))
	}

	out := doctorText(t, db, dir)
	assert.Contains(t, out, "empty or unwalkable")
	root, err := filepath.Abs(dir)
	require.NoError(t, err)
	assert.Contains(t, out, root)
	assert.NotContains(t, out, staleIndexRemedy)
}

// A blocked root still has rows, so the embeddings section and the raw-body
// line would both name a command index refuses. Neither text nor JSON may
// prescribe `vaultmind index`. The empty-or-unwalkable root is named once.
func TestDoctor_BlockedRootPrescribesNoIndex(t *testing.T) {
	dir, db := doctorNotesOutside(t)
	_, err := db.Exec(`UPDATE notes SET body_raw = NULL`)
	require.NoError(t, err)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		require.NoError(t, os.RemoveAll(filepath.Join(dir, e.Name())))
	}

	res, err := query.Doctor(db, dir, nil)
	require.NoError(t, err)
	require.True(t, res.Issues.IndexRootBlocked, "SawNothing with rows is the gate")
	require.Greater(t, res.Issues.MissingRawBodies, 0)
	require.NotNil(t, res.Embeddings)
	require.False(t, res.Embeddings.SemanticReady)

	text := doctorText(t, db, dir)
	assert.NotContains(t, text, "vaultmind index")
	assert.Equal(t, 1, strings.Count(text, "empty or unwalkable"), "name the root once")
	root, err := filepath.Abs(dir)
	require.NoError(t, err)
	assert.Contains(t, text, root)

	env := envelope.OK("doctor", res)
	raw, err := json.Marshal(env)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "vaultmind index")
}

// Modality imbalance is a second `vaultmind index --embed` prescription.
// A blocked root must not print it, in the report or in JSON.
func TestDoctor_BlockedRootSuppressesModalityIndex(t *testing.T) {
	res := &query.DoctorResult{
		VaultPath: "/vault",
		Embeddings: &query.DoctorEmbeddings{
			SemanticReady:        true,
			TotalNotes:           2,
			Model:                "bge-m3",
			DenseCount:           2,
			SparseCount:          0,
			ColBERTCount:         0,
			HasModalityImbalance: true,
		},
		Issues: query.DoctorIssues{
			IndexRootBlocked: true,
			ResolvedRoot:     "/vault",
			MissingRawBodies: 1,
			OrphanedEntries:  1,
		},
	}
	var buf bytes.Buffer
	require.NoError(t, writeDoctorHuman(&buf, res, false))
	text := buf.String()
	assert.NotContains(t, text, "vaultmind index")
	assert.Equal(t, 1, strings.Count(text, "empty or unwalkable"))

	raw, err := json.Marshal(envelope.OK("doctor", res))
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "vaultmind index")
}

// MiniLM on an ORT binary prescribes `vaultmind index --embed --model bge-m3`.
// A blocked root drops that command and keeps the pure-Go download hint,
// which does not name index. The root is still named once.
func TestDoctor_BlockedMiniLMDoesNotPrescribeIndex(t *testing.T) {
	orig := doctorBackend
	t.Cleanup(func() { doctorBackend = orig })

	report := func() string {
		t.Helper()
		res := &query.DoctorResult{
			VaultPath: "/vault",
			Embeddings: &query.DoctorEmbeddings{
				SemanticReady: true,
				TotalNotes:    2,
				Model:         embedding.ModelMiniLM,
				DenseCount:    2,
			},
			Issues: query.DoctorIssues{
				IndexRootBlocked: true,
				ResolvedRoot:     "/vault",
				OrphanedEntries:  1,
			},
		}
		var buf bytes.Buffer
		require.NoError(t, writeDoctorHuman(&buf, res, false))
		return buf.String()
	}

	doctorBackend = func() string { return embedding.BackendNameORT }
	ort := report()
	assert.NotContains(t, ort, "vaultmind index")
	assert.Contains(t, ort, "degraded recall")
	assert.Equal(t, 1, strings.Count(ort, "empty or unwalkable"))

	doctorBackend = func() string { return embedding.BackendNameGo }
	pure := report()
	assert.NotContains(t, pure, "vaultmind index")
	assert.Contains(t, pure, "prebuilt ORT binary")
	assert.Equal(t, 1, strings.Count(pure, "empty or unwalkable"))
}

// The ORT MiniLM remedy still names the re-embed when the root is walkable.
func TestWriteEmbeddingStatus_ORTMiniLMStillPrescribesIndex(t *testing.T) {
	orig := doctorBackend
	doctorBackend = func() string { return embedding.BackendNameORT }
	t.Cleanup(func() { doctorBackend = orig })

	var buf bytes.Buffer
	require.NoError(t, writeEmbeddingStatus(&buf, &query.DoctorEmbeddings{
		SemanticReady: true, TotalNotes: 2, Model: embedding.ModelMiniLM, DenseCount: 2,
	}, &query.DoctorIssues{}))
	assert.Contains(t, buf.String(), "vaultmind index --embed --model bge-m3 --full")
}

// A walkable vault with no embeddings, and notes whose raw body was never
// stored, still names both commands. The gate is the blocked root, not the
// missing data.
func TestDoctor_WalkableVaultStillPrescribesIndex(t *testing.T) {
	dir, db := doctorNotesOutside(t)
	_, err := db.Exec(`UPDATE notes SET body_raw = NULL`)
	require.NoError(t, err)

	out := doctorText(t, db, dir)
	assert.Contains(t, out, "vaultmind index --embed --vault <vault>")
	assert.Contains(t, out, "run vaultmind index")
	assert.NotContains(t, out, "empty or unwalkable")
}

// A README is an entry. The notes are gone, and index will purge them.
// Doctor still names that command.
func TestDoctor_ReadmeOnlyStillPrescribesIndex(t *testing.T) {
	dir, db := doctorNotesOutside(t)
	require.NoError(t, os.Remove(filepath.Join(dir, "keep.md")))
	require.NoError(t, os.Remove(filepath.Join(dir, "drop.md")))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Vault\n"), 0o644))

	out := doctorText(t, db, dir)
	assert.Contains(t, out, staleIndexRemedy)
	assert.NotContains(t, out, "empty or unwalkable")
}
