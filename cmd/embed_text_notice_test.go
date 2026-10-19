package cmd

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// noticeEmbedder is a dense embedder that does not touch the model cache.
// Ask against a vault with a dense vector builds MiniLM; this stands in.
type noticeEmbedder struct{}

func (noticeEmbedder) Embed(context.Context, string) ([]float32, error) {
	return make([]float32, embedding.DefaultDims), nil
}
func (noticeEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = make([]float32, embedding.DefaultDims)
	}
	return out, nil
}
func (noticeEmbedder) Dims() int    { return embedding.DefaultDims }
func (noticeEmbedder) Close() error { return nil }

func stubMiniLM(t *testing.T) {
	t.Helper()
	saved := embedding.ConstructMiniLM
	embedding.ConstructMiniLM = func(context.Context, embedding.HugotConfig) (embedding.Embedder, error) {
		return noticeEmbedder{}, nil
	}
	t.Cleanup(func() { embedding.ConstructMiniLM = saved })
}

func setStaleEmbedText(t *testing.T, vault string) []byte {
	t.Helper()
	db, err := index.Open(filepath.Join(vault, ".vaultmind", "index.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	blob := index.EncodeEmbedding(make([]float32, 384))
	_, err = db.Exec(`UPDATE notes SET embedding = ? WHERE id = 'concept-alpha'`, blob)
	require.NoError(t, err)
	return blob
}

func TestPlainIndex_WarnsAndDoesNotRewriteStaleEmbedText(t *testing.T) {
	vault := buildIndexedTestVault(t)
	before := setStaleEmbedText(t, vault)

	out, _, err := runRootCmd(t, "index", "--vault", vault)
	require.NoError(t, err)
	assert.Contains(t, out.String(), index.EmbedTextStaleNotice)
	assert.Contains(t, out.String(), index.EmbedTextStaleRemedy)

	db, err := index.Open(filepath.Join(vault, ".vaultmind", "index.db"))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	var after []byte
	require.NoError(t, db.QueryRow(`SELECT embedding FROM notes WHERE id = 'concept-alpha'`).Scan(&after))
	assert.Equal(t, before, after, "a plain index does not rewrite dense vectors")
	format, err := index.ReadEmbedTextFormat(db)
	require.NoError(t, err)
	assert.Equal(t, 0, format)

	jsonOut, _, err := runRootCmd(t, "index", "--vault", vault, "--json")
	require.NoError(t, err)
	var env struct {
		SchemaVersion string `json:"schema_version"`
		Status        string `json:"status"`
		Result        struct {
			Index struct {
				EmbedTextStale bool `json:"embed_text_stale"`
			} `json:"index"`
		} `json:"result"`
		Warnings []struct {
			Code string `json:"code"`
		} `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal(jsonOut.Bytes(), &env))
	assert.Equal(t, "v1", env.SchemaVersion)
	assert.Equal(t, "warning", env.Status)
	assert.True(t, env.Result.Index.EmbedTextStale)
	require.NotEmpty(t, env.Warnings)
	assert.Equal(t, "embed_text_stale", env.Warnings[0].Code)
}

func TestPlainIndex_CurrentEmbedTextStampDoesNotWarn(t *testing.T) {
	vault := buildIndexedTestVault(t)
	setStaleEmbedText(t, vault)
	db, err := index.Open(filepath.Join(vault, ".vaultmind", "index.db"))
	require.NoError(t, err)
	require.NoError(t, index.WriteEmbedTextFormat(db, index.EmbedTextFormatCurrent))
	require.NoError(t, db.Close())

	out, _, err := runRootCmd(t, "index", "--vault", vault)
	require.NoError(t, err)
	assert.NotContains(t, out.String(), index.EmbedTextStaleNotice)
}

func TestDoctorAndAsk_ReportStaleEmbedText(t *testing.T) {
	vault := buildIndexedTestVault(t)
	setStaleEmbedText(t, vault)

	out, _, err := runRootCmd(t, "doctor", "--vault", vault)
	require.NoError(t, err)
	text := out.String()
	assert.Contains(t, text, index.EmbedTextStaleNotice)
	assert.Contains(t, text, "vaultmind index --embed")
	noticeAt := strings.Index(text, index.EmbedTextStaleNotice)
	require.GreaterOrEqual(t, noticeAt, 0)
	afterNotice := text[noticeAt+len(index.EmbedTextStaleNotice):]
	nl := strings.Index(afterNotice, "\n")
	require.GreaterOrEqual(t, nl, 0)
	runLine := afterNotice[nl+1:]
	if end := strings.Index(runLine, "\n"); end >= 0 {
		runLine = runLine[:end]
	}
	assert.Equal(t, "  run: "+index.EmbedTextStaleRemedy+" --vault "+vault, runLine,
		"the stale-title remedy is the shared command with this vault's path")
	assert.NotRegexp(t, `Issues: \d+ errors, 0 warnings`, text,
		"a printed stale-title warning counts as a warning")

	jsonOut, _, err := runRootCmd(t, "doctor", "--vault", vault, "--json")
	require.NoError(t, err)
	var doc struct {
		SchemaVersion string `json:"schema_version"`
		Result        struct {
			Embeddings struct {
				EmbedTextStale bool `json:"embed_text_stale"`
			} `json:"embeddings"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(jsonOut.Bytes(), &doc))
	assert.Equal(t, "v1", doc.SchemaVersion)
	assert.True(t, doc.Result.Embeddings.EmbedTextStale)

	stubMiniLM(t)
	askOut, _, err := runRootCmd(t, "ask", "alpha", "--vault", vault)
	require.NoError(t, err)
	assert.Contains(t, askOut.String(), index.EmbedTextStaleNotice)
	assert.Contains(t, askOut.String(), index.EmbedTextStaleRemedy)
	assert.Contains(t, askOut.String(), "Search:")

	askJSON, _, err := runRootCmd(t, "ask", "alpha", "--vault", vault, "--json")
	require.NoError(t, err)
	var ask struct {
		SchemaVersion string `json:"schema_version"`
		Status        string `json:"status"`
		Result        struct {
			EmbedTextStale bool `json:"embed_text_stale"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(askJSON.Bytes(), &ask))
	assert.Equal(t, "v1", ask.SchemaVersion)
	assert.Equal(t, "ok", ask.Status, "the stale field is additive; ask status stays ok")
	assert.True(t, ask.Result.EmbedTextStale)
}

func TestAsk_HookCallerOmitsEmbedTextNotice(t *testing.T) {
	vault := buildIndexedTestVault(t)
	setStaleEmbedText(t, vault)
	stubMiniLM(t)
	t.Setenv("VAULTMIND_CALLER", "vaultmind-userprompt-hook")

	out, _, err := runRootCmd(t, "ask", "alpha", "--vault", vault)
	require.NoError(t, err)
	assert.NotContains(t, out.String(), index.EmbedTextStaleNotice)
	assert.Contains(t, out.String(), "Search:")
}
