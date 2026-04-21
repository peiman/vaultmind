package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubEmbedder is the minimum viable Embedder for label-only tests —
// retrievalModeLabel only reads nil-ness, so the methods don't need
// meaningful implementations.
type stubEmbedder struct{}

func (stubEmbedder) Embed(context.Context, string) ([]float32, error)       { return nil, nil }
func (stubEmbedder) EmbedBatch(context.Context, []string) ([][]float32, error) { return nil, nil }
func (stubEmbedder) Dims() int                                                 { return 0 }
func (stubEmbedder) Close() error                                              { return nil }

var _ embedding.Embedder = stubEmbedder{}

// doctor must surface the core vault health metrics. A doctor that silently
// drops one of these leaves the user debugging blind.
func TestDoctor_ReportsCoreMetrics(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "doctor", "--vault", vault, "--json")
	require.NoError(t, err)

	var env struct {
		Result struct {
			VaultPath         string `json:"vault_path"`
			TotalFiles        int    `json:"total_files"`
			DomainNotes       int    `json:"domain_notes"`
			UnstructuredNotes int    `json:"unstructured_notes"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, vault, env.Result.VaultPath)
	assert.Equal(t, 4, env.Result.TotalFiles)
	assert.Equal(t, 3, env.Result.DomainNotes)
	assert.Equal(t, 1, env.Result.UnstructuredNotes)
}

// doctor human output must include the embedding-readiness line when
// embeddings are absent. That line is the single actionable suggestion
// a user gets from `doctor` for enabling semantic retrieval.
func TestDoctor_HumanOutputNamesEmbedRemedy(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "doctor", "--vault", vault)
	require.NoError(t, err)
	text := out.String()
	assert.Contains(t, text, "Vault: ")
	assert.Contains(t, text, "Embeddings: none",
		"doctor must name 'none' when no embeddings exist — that's the remedy trigger")
	assert.Contains(t, text, "vaultmind index --embed",
		"doctor must print the exact remedy command users should run")
}

// writeEmbeddingStatus unit-level contract: nil input is a no-op, semantic
// ready renders counts, semantic-not-ready renders the remedy line.
func TestWriteEmbeddingStatus_NilInputIsNoop(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeEmbeddingStatus(&buf, nil))
	assert.Empty(t, buf.String())
}

func TestWriteEmbeddingStatus_NotReadyPrintsRemedy(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeEmbeddingStatus(&buf, &query.DoctorEmbeddings{
		SemanticReady: false, TotalNotes: 12,
	}))
	out := buf.String()
	assert.Contains(t, out, "Embeddings: none (12 notes)")
	assert.Contains(t, out, "vaultmind index --embed")
}

func TestWriteEmbeddingStatus_ReadyPrintsCounts(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeEmbeddingStatus(&buf, &query.DoctorEmbeddings{
		SemanticReady: true, TotalNotes: 10, DenseCount: 10, Model: "bge-m3",
		SparseCount: 8, ColBERTCount: 9,
	}))
	out := buf.String()
	assert.Contains(t, out, "dense 10/10")
	assert.Contains(t, out, "bge-m3")
	assert.Contains(t, out, "sparse 8/10")
	assert.Contains(t, out, "colbert 9/10")
}

// git status on a non-git vault must not error — users routinely initialise
// a vault before `git init`. Silent success with repo_detected=false is
// the contract.
func TestGitStatus_NonGitDirectoryReportsRepoNotDetected(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "git", "status", "--vault", vault, "--json")
	require.NoError(t, err)

	var env struct {
		Result struct {
			RepoDetected bool `json:"repo_detected"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.False(t, env.Result.RepoDetected)
}

// index --json on a missing vault path must return a structured error
// envelope (not crash, not a bare Go error with no code).
func TestIndex_MissingVaultReturnsStructuredError(t *testing.T) {
	out, _, err := runRootCmd(t, "index", "--vault", "/nonexistent/path", "--json")
	// WriteJSONError writes and returns nil; tolerate either outcome.
	_ = err

	if out.Len() > 0 {
		var env struct {
			Status string `json:"status"`
			Errors []struct {
				Code string `json:"code"`
			} `json:"errors"`
		}
		require.NoError(t, json.Unmarshal(out.Bytes(), &env))
		assert.Equal(t, "error", env.Status)
		require.NotEmpty(t, env.Errors)
		assert.Equal(t, "vault_not_found", env.Errors[0].Code)
	} else {
		require.Error(t, err)
	}
}

// Incremental index over an unchanged vault must report 0 added/updated/deleted
// — if it thought every note had changed, every re-index would nuke embeddings.
func TestIndex_IncrementalReturnsSkipsForUnchangedVault(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "index", "--vault", vault, "--json")
	require.NoError(t, err)

	var env struct {
		Result struct {
			Index struct {
				Added   int `json:"Added"`
				Updated int `json:"Updated"`
				Deleted int `json:"Deleted"`
				Skipped int `json:"Skipped"`
			} `json:"Index"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, 0, env.Result.Index.Added, "unchanged vault must add nothing")
	assert.Equal(t, 0, env.Result.Index.Updated, "unchanged vault must update nothing")
	assert.Greater(t, env.Result.Index.Skipped, 0, "unchanged files must be skipped (mtime/hash path)")
}

// dataview lint on a vault without dataview markers finishes clean.
// Silent failure here would mask real syntax issues from users who rely
// on the clean-vault signal.
func TestDataviewLint_CleanVaultReportsValid(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "dataview", "lint", "--vault", vault, "--json")
	require.NoError(t, err)

	var env struct {
		Status string `json:"status"`
		Result struct {
			FilesChecked int `json:"files_checked"`
			Valid        int `json:"valid"`
			Issues       []any `json:"issues"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "ok", env.Status)
	assert.Greater(t, env.Result.FilesChecked, 0)
	assert.Empty(t, env.Result.Issues)
}

// dataview lint flags an unterminated START marker with no matching END.
// The "warning" envelope status is the script-visible signal that something
// needs fixing; losing it would let CI pipelines swallow real breakage.
func TestDataviewLint_FlagsUnterminatedStartMarker(t *testing.T) {
	vault := buildIndexedTestVault(t)
	// Real marker format: <!-- VAULTMIND:GENERATED:<key>:START --> / :END -->
	// Here we open but never close, which ValidateMarkers reports.
	bad := filepath.Join(vault, "malformed.md")
	require.NoError(t, os.WriteFile(bad, []byte(`---
id: bad-1
type: concept
title: Broken
---

<!-- VAULTMIND:GENERATED:list:START -->
content with no matching end marker
`), 0o644))

	out, _, err := runRootCmd(t, "dataview", "lint", "--vault", vault, "--json")
	require.NoError(t, err)

	var env struct {
		Status string `json:"status"`
		Result struct {
			Issues []struct {
				Rule string `json:"rule"`
			} `json:"issues"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "warning", env.Status)
	assert.NotEmpty(t, env.Result.Issues, "unterminated START marker must produce at least one issue")
}

// dataview render on a target with no marker produces a structured
// "not found" style error rather than a panic.
func TestDataviewRender_MissingMarkerReturnsError(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "dataview", "render", "concepts/alpha.md",
		"--section-key", "nonexistent", "--vault", vault, "--json")
	// Either error Go or structured envelope; at least one signal must fire.
	if err == nil {
		assert.Contains(t, strings.ToLower(out.String()), "error")
	}
}

// retrievalModeLabel returns "hybrid" when Embedder is non-nil, "keyword"
// otherwise — this label feeds experiment telemetry and scripts branch on it.
func TestRetrievalModeLabel_HybridWhenEmbedderPresent(t *testing.T) {
	assert.Equal(t, "hybrid", retrievalModeLabel(query.AutoRetrieverResult{Embedder: stubEmbedder{}}))
	assert.Equal(t, "keyword", retrievalModeLabel(query.AutoRetrieverResult{}))
}

// ask without an argument is a usage error.
func TestAsk_MissingArgUsageError(t *testing.T) {
	vault := buildIndexedTestVault(t)
	_, _, err := runRootCmd(t, "ask", "--vault", vault)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "usage")
}

// ask on a small indexed vault must return a non-nil result with the query
// recorded — this is the happy-path contract the Workhorse hook depends on.
func TestAsk_ReturnsResultWithQuery(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "ask", "Alpha body", "--vault", vault, "--json",
		"--budget", "2000", "--max-items", "4", "--search-limit", "5")
	require.NoError(t, err)
	// Envelope must parse and status must be "ok".
	var env struct {
		Status string          `json:"status"`
		Result json.RawMessage `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "ok", env.Status)
	assert.NotEmpty(t, env.Result)
}
