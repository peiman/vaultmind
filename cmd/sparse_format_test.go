package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setStaleSparse(t *testing.T, vault string) []byte {
	t.Helper()
	db, err := index.Open(filepath.Join(vault, ".vaultmind", "index.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	blob := index.EncodeSparseEmbedding(map[int32]float32{0: 1, 10: 0.2})
	_, err = db.Exec(`UPDATE notes SET sparse_embedding = ? WHERE id = 'concept-alpha'`, blob)
	require.NoError(t, err)
	return blob
}

func TestPlainIndex_WarnsAndDoesNotRewriteStaleSparse(t *testing.T) {
	vault := buildIndexedTestVault(t)
	before := setStaleSparse(t, vault)

	out, _, err := runRootCmd(t, "index", "--vault", vault)
	require.NoError(t, err)
	assert.Contains(t, out.String(), index.SparseFormatStaleNotice)
	assert.Contains(t, out.String(), index.SparseFormatStaleRemedy)

	db, err := index.Open(filepath.Join(vault, ".vaultmind", "index.db"))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	var after []byte
	require.NoError(t, db.QueryRow(`SELECT sparse_embedding FROM notes WHERE id = 'concept-alpha'`).Scan(&after))
	assert.Equal(t, before, after, "a plain index does not rewrite sparse vectors")
	format, err := index.ReadSparseFormat(db)
	require.NoError(t, err)
	assert.Equal(t, 0, format)

	jsonOut, _, err := runRootCmd(t, "index", "--vault", vault, "--json")
	require.NoError(t, err)
	var env struct {
		SchemaVersion string `json:"schema_version"`
		Status        string `json:"status"`
		Result        struct {
			Index struct {
				SparseFormatStale bool `json:"sparse_format_stale"`
			} `json:"index"`
		} `json:"result"`
		Warnings []struct {
			Code string `json:"code"`
		} `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal(jsonOut.Bytes(), &env))
	assert.Equal(t, "v1", env.SchemaVersion)
	assert.Equal(t, "warning", env.Status)
	assert.True(t, env.Result.Index.SparseFormatStale)
	require.NotEmpty(t, env.Warnings)
	assert.Equal(t, "sparse_format_stale", env.Warnings[0].Code)
}

func TestPlainIndex_FreshStampDoesNotWarn(t *testing.T) {
	vault := buildIndexedTestVault(t)
	setStaleSparse(t, vault)
	db, err := index.Open(filepath.Join(vault, ".vaultmind", "index.db"))
	require.NoError(t, err)
	require.NoError(t, index.WriteSparseFormat(db, index.SparseFormatCurrent))
	require.NoError(t, db.Close())

	out, _, err := runRootCmd(t, "index", "--vault", vault)
	require.NoError(t, err)
	assert.NotContains(t, out.String(), index.SparseFormatStaleNotice)
}

func TestDoctorAndAsk_ReportStaleSparseFormat(t *testing.T) {
	vault := buildIndexedTestVault(t)
	setStaleSparse(t, vault)

	out, _, err := runRootCmd(t, "doctor", "--vault", vault)
	require.NoError(t, err)
	text := out.String()
	assert.Contains(t, text, index.SparseFormatStaleNotice)
	assert.Contains(t, text, "vaultmind index --embed")
	noticeAt := strings.Index(text, index.SparseFormatStaleNotice)
	require.GreaterOrEqual(t, noticeAt, 0)
	afterNotice := text[noticeAt+len(index.SparseFormatStaleNotice):]
	nl := strings.Index(afterNotice, "\n")
	require.GreaterOrEqual(t, nl, 0)
	runLine := afterNotice[nl+1:]
	if end := strings.Index(runLine, "\n"); end >= 0 {
		runLine = runLine[:end]
	}
	assert.Equal(t, "  run: "+index.SparseFormatStaleRemedy+" --vault "+vault, runLine,
		"the stale-sparse remedy is the shared command with this vault's path")
	assert.NotRegexp(t, `Issues: \d+ errors, 0 warnings`, text,
		"a printed stale-sparse warning counts as a warning")

	jsonOut, _, err := runRootCmd(t, "doctor", "--vault", vault, "--json")
	require.NoError(t, err)
	var doc struct {
		SchemaVersion string `json:"schema_version"`
		Result        struct {
			Embeddings struct {
				SparseFormatStale bool `json:"sparse_format_stale"`
			} `json:"embeddings"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(jsonOut.Bytes(), &doc))
	assert.Equal(t, "v1", doc.SchemaVersion)
	assert.True(t, doc.Result.Embeddings.SparseFormatStale)

	askOut, _, err := runRootCmd(t, "ask", "alpha", "--vault", vault)
	require.NoError(t, err)
	assert.Contains(t, askOut.String(), index.SparseFormatStaleNotice)
	assert.Contains(t, askOut.String(), index.SparseFormatStaleRemedy)
	assert.Contains(t, askOut.String(), "Search:")

	askJSON, _, err := runRootCmd(t, "ask", "alpha", "--vault", vault, "--json")
	require.NoError(t, err)
	var ask struct {
		SchemaVersion string `json:"schema_version"`
		Status        string `json:"status"`
		Result        struct {
			SparseFormatStale bool `json:"sparse_format_stale"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(askJSON.Bytes(), &ask))
	assert.Equal(t, "v1", ask.SchemaVersion)
	assert.Equal(t, "ok", ask.Status, "the stale field is additive; ask status stays ok")
	assert.True(t, ask.Result.SparseFormatStale)
}

func TestAsk_HookCallerOmitsSparseFormatNotice(t *testing.T) {
	vault := buildIndexedTestVault(t)
	setStaleSparse(t, vault)
	t.Setenv("VAULTMIND_CALLER", "vaultmind-userprompt-hook")

	out, _, err := runRootCmd(t, "ask", "alpha", "--vault", vault)
	require.NoError(t, err)
	assert.NotContains(t, out.String(), index.SparseFormatStaleNotice)
	assert.Contains(t, out.String(), "Search:")
}

// markBGEM3Vectors stores a complete BGE-M3 row on every note and leaves the
// sparse-format stamp unset, so the vault reads as bge-m3 and stale.
func markBGEM3Vectors(t *testing.T, vaultPath string) {
	t.Helper()
	db, err := index.Open(filepath.Join(vaultPath, ".vaultmind", "index.db"))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	dense := index.EncodeEmbedding([]float32{1, 0, 0, 0})
	sparse := index.EncodeSparseEmbedding(map[int32]float32{0: 1})
	colbert := index.EncodeColBERTEmbedding([][]float32{{1, 0}})
	_, err = db.Exec(`UPDATE notes SET embedding = ?, sparse_embedding = ?, colbert_embedding = ?`, dense, sparse, colbert)
	require.NoError(t, err)
}

func useORTBackend(t *testing.T) {
	t.Helper()
	saved := embeddingBackendName
	embeddingBackendName = func() string { return embedding.BackendNameORT }
	t.Cleanup(func() { embeddingBackendName = saved })
}

func countEmbedPasses(t *testing.T) *int {
	t.Helper()
	var calls int
	saved := runEmbedPass
	runEmbedPass = func(_ *cobra.Command, _, _ string, _ *vault.Config, model string) (*index.EmbedResult, error) {
		calls++
		assert.Equal(t, embedding.ModelBGEM3, model)
		return &index.EmbedResult{}, nil
	}
	t.Cleanup(func() { runEmbedPass = saved })
	return &calls
}

func TestEmbedOnWrite_StaleSparseUpgradesDespiteTheBacklogLimit(t *testing.T) {
	vaultPath := buildIndexedTestVault(t)
	markBGEM3Vectors(t, vaultPath)
	useORTBackend(t)
	savedLimit := embedOnWriteLimit
	embedOnWriteLimit = 0
	t.Cleanup(func() { embedOnWriteLimit = savedLimit })
	calls := countEmbedPasses(t)

	// The edit clears that note's vectors, so pending is over the limit.
	// A stale stamp is the one case that still runs the pass.
	_, errOut, err := runRootCmd(t, "frontmatter", "set", "projects/beta.md", "status", "paused", "--vault", vaultPath)
	require.NoError(t, err)
	assert.Equal(t, 1, *calls)
	assert.NotContains(t, errOut.String(), "notes have no embeddings")
}

func TestPlainIndex_CorruptStampIsAnError(t *testing.T) {
	vault := buildIndexedTestVault(t)
	setStaleSparse(t, vault)
	db, err := index.Open(filepath.Join(vault, ".vaultmind", "index.db"))
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO meta (key, value) VALUES ('sparse_format', 'nope')`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	_, _, err = runRootCmd(t, "index", "--vault", vault)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sparse format")
}

func TestNoteStaleSparse_SkipsEmbedAndReportsAnUnreadableIndex(t *testing.T) {
	require.NoError(t, noteStaleSparse(nil, "unused", false))
	require.NoError(t, noteStaleSparse(&index.IndexResult{}, "unused", true))

	parent := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(parent, []byte("x"), 0o644))
	err := noteStaleSparse(&index.IndexResult{}, filepath.Join(parent, "index.db"), false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checking sparse format")
}

func TestFormatIndexResult_StaleWarningWriteError(t *testing.T) {
	err := formatIndexResult(index.IndexAndEmbedResult{
		Index: &index.IndexResult{SparseFormatStale: true},
	}, "", &failAfterNWriter{ok: 1})
	require.Error(t, err)
}

func TestWriteEmbeddingStatus_StaleWarningWriteError(t *testing.T) {
	notReady := &query.DoctorEmbeddings{SemanticReady: false, SparseFormatStale: true, TotalNotes: 3}
	require.Error(t, writeEmbeddingStatus(&failAfterNWriter{ok: 1}, notReady, nil, ""))

	ready := &query.DoctorEmbeddings{
		SemanticReady:     true,
		SparseFormatStale: true,
		Model:             embedding.ModelBGEM3,
		TotalNotes:        1,
		DenseCount:        1,
		SparseCount:       1,
		ColBERTCount:      1,
	}
	require.Error(t, writeEmbeddingStatus(&failAfterNWriter{ok: 1}, ready, nil, ""))
}

func TestAsk_UnreadableSparseStampLeavesTheFieldUnset(t *testing.T) {
	vault := buildIndexedTestVault(t)
	db, err := index.Open(filepath.Join(vault, ".vaultmind", "index.db"))
	require.NoError(t, err)
	_, err = db.Exec(`DROP TABLE meta`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	askJSON, _, err := runRootCmd(t, "ask", "alpha", "--vault", vault, "--json")
	require.NoError(t, err)
	assert.NotContains(t, askJSON.String(), "sparse_format_stale")
}

func TestEmbedOnWrite_UnreadableStampDoesNotEmbed(t *testing.T) {
	vaultPath := buildIndexedTestVault(t)
	markBGEM3Vectors(t, vaultPath)
	useORTBackend(t)
	calls := countEmbedPasses(t)
	db, err := index.Open(filepath.Join(vaultPath, ".vaultmind", "index.db"))
	require.NoError(t, err)
	_, err = db.Exec(`DROP TABLE meta`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	_, errOut, err := runRootCmd(t, "frontmatter", "set", "projects/beta.md", "status", "paused", "--vault", vaultPath)
	require.NoError(t, err)
	assert.Zero(t, *calls)
	assert.Contains(t, errOut.String(), "written, but not embedded")
}

func TestEmbedOnWrite_FreshStampKeepsTheBacklogLimit(t *testing.T) {
	vaultPath := buildIndexedTestVault(t)
	markBGEM3Vectors(t, vaultPath)
	db, err := index.Open(filepath.Join(vaultPath, ".vaultmind", "index.db"))
	require.NoError(t, err)
	require.NoError(t, index.WriteSparseFormat(db, index.SparseFormatCurrent))
	require.NoError(t, db.Close())
	useORTBackend(t)
	savedLimit := embedOnWriteLimit
	embedOnWriteLimit = 0
	t.Cleanup(func() { embedOnWriteLimit = savedLimit })
	calls := countEmbedPasses(t)

	_, errOut, err := runRootCmd(t, "frontmatter", "set", "projects/beta.md", "status", "paused", "--vault", vaultPath)
	require.NoError(t, err)
	assert.Zero(t, *calls)
	assert.Contains(t, errOut.String(), "notes have no embeddings")
	assert.Contains(t, errOut.String(), "vaultmind index --embed --vault "+vaultPath)
}
