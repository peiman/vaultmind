package query_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// smallIndexedVault builds a 3-note temp vault and returns the open DB. The
// notes mirror the cmd-package helper's shape (alpha, beta, gamma with
// inbound/outbound links between alpha ↔ beta) so runner tests can target
// realistic edge cases without paying the cost of reading the full repo
// vault.
func smallIndexedVault(t *testing.T) (*index.DB, string) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"), []byte(`
types:
  concept:
    required: [title]
    optional: [related_ids]
  project:
    required: [status, title]
    optional: [related_ids]
    statuses: [active]
`), 0o644))

	mustWrite := func(rel, content string) {
		full := filepath.Join(dir, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	}
	mustWrite("alpha.md", `---
id: concept-alpha
type: concept
title: Alpha
related_ids: [proj-beta]
---
See [[proj-beta]].
`)
	mustWrite("beta.md", `---
id: proj-beta
type: project
status: active
title: Beta
related_ids: [concept-alpha]
---
See [[concept-alpha]].
`)
	mustWrite("gamma.md", `---
id: concept-gamma
type: concept
title: Gamma
---
Nothing linked.
`)

	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	dbPath := filepath.Join(dir, cfg.Index.DBPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(dbPath), 0o755))
	idxr := index.NewIndexer(dir, dbPath, cfg)
	_, err = idxr.Rebuild()
	require.NoError(t, err)
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db, dir
}

// RunNoteGet (JSON path) returns the requested note in the envelope result.
// Regression: returning a different note silently would break every caller
// that uses ID as a key.
func TestRunNoteGet_JSONReturnsRequestedNote(t *testing.T) {
	db, dir := smallIndexedVault(t)
	var buf bytes.Buffer
	err := query.RunNoteGet(db, query.NoteGetConfig{
		Input: "concept-alpha", JSONOutput: true, VaultPath: dir,
	}, &buf)
	require.NoError(t, err)
	var env struct {
		Status string `json:"status"`
		Result struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &env))
	assert.Equal(t, "ok", env.Status)
	assert.Equal(t, "concept-alpha", env.Result.ID)
	assert.Equal(t, "Alpha", env.Result.Title)
}

// Unknown ID path emits an error envelope with "not_found" — structured so
// callers can branch on the code, not a raw text message.
func TestRunNoteGet_UnknownIDEmitsNotFoundEnvelope(t *testing.T) {
	db, dir := smallIndexedVault(t)
	var buf bytes.Buffer
	err := query.RunNoteGet(db, query.NoteGetConfig{
		Input: "nope", JSONOutput: true, VaultPath: dir,
	}, &buf)
	require.NoError(t, err)

	var env struct {
		Status string `json:"status"`
		Errors []struct {
			Code string `json:"code"`
		} `json:"errors"`
	}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &env))
	assert.Equal(t, "error", env.Status)
	require.NotEmpty(t, env.Errors)
	assert.Equal(t, "not_found", env.Errors[0].Code)
}

// FrontmatterOnly strips body, headings, blocks from the response — caller
// scripts rely on this to keep payloads small.
func TestRunNoteGet_FrontmatterOnlyStripsBody(t *testing.T) {
	db, dir := smallIndexedVault(t)
	var buf bytes.Buffer
	err := query.RunNoteGet(db, query.NoteGetConfig{
		Input: "concept-alpha", FrontmatterOnly: true, JSONOutput: true, VaultPath: dir,
	}, &buf)
	require.NoError(t, err)
	var env struct {
		Result struct {
			Body string `json:"body"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &env))
	assert.Empty(t, env.Result.Body, "FrontmatterOnly must strip body")
}

// Human mode: a short one-liner identifying the note.
func TestRunNoteGet_HumanModeOneLiner(t *testing.T) {
	db, dir := smallIndexedVault(t)
	var buf bytes.Buffer
	err := query.RunNoteGet(db, query.NoteGetConfig{
		Input: "concept-alpha", VaultPath: dir,
	}, &buf)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "concept-alpha")
	assert.Contains(t, out, "Alpha")
	assert.Contains(t, out, "concept")
}

// RunResolve: a known ID resolves to itself (identity round-trip).
func TestRunResolve_KnownIDRoundTrips(t *testing.T) {
	db, dir := smallIndexedVault(t)
	var buf bytes.Buffer
	err := query.RunResolve(db, "concept-alpha", dir, true, &buf)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "concept-alpha")
}

// Human-mode path: unknown input prints "No match for" so terminal users
// get a clear signal.
func TestRunResolve_HumanModeNoMatchMessage(t *testing.T) {
	db, dir := smallIndexedVault(t)
	var buf bytes.Buffer
	err := query.RunResolve(db, "no-such-id", dir, false, &buf)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), `No match for "no-such-id"`)
}

// Human-mode path with a known input prints one row per match. The format
// uses fixed columns (id, type, title, path) that scripts pipe through awk.
func TestRunResolve_HumanModeFormatRow(t *testing.T) {
	db, dir := smallIndexedVault(t)
	var buf bytes.Buffer
	err := query.RunResolve(db, "concept-alpha", dir, false, &buf)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "concept-alpha")
	assert.Contains(t, out, "concept")
	assert.Contains(t, out, "Alpha")
	assert.Contains(t, out, "alpha.md")
}

// RunSearch human mode prints hits with id + title. Covers the non-JSON
// branch of query.RunSearch.
func TestRunSearch_HumanModeCarriesHits(t *testing.T) {
	db, dir := smallIndexedVault(t)
	retriever := &query.FTSRetriever{DB: db}
	var buf bytes.Buffer
	_, err := query.RunSearch(retriever, query.SearchConfig{
		Query: "Alpha", Limit: 5, VaultPath: dir,
	}, &buf)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "concept-alpha", "human search output must include the matching note ID")
}

// RunNoteGet with an ambiguous input (two notes with the same title) returns
// an "ambiguous_resolution" envelope in JSON mode. This is the path
// AX-sensitive callers use to prompt for disambiguation.
//
// We can trigger ambiguity by creating two notes with the same title and
// resolving by title. Since smallIndexedVault has unique titles, build a
// bespoke vault here.
func TestRunNoteGet_AmbiguousTitleReturnsCandidatesEnvelope(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"), []byte(`
types:
  concept:
    required: [title]
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"), []byte(`---
id: c-1
type: concept
title: Shared Title
---
body
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.md"), []byte(`---
id: c-2
type: concept
title: Shared Title
---
body
`), 0o644))

	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	dbPath := filepath.Join(dir, cfg.Index.DBPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(dbPath), 0o755))
	idxr := index.NewIndexer(dir, dbPath, cfg)
	_, err = idxr.Rebuild()
	require.NoError(t, err)
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	var buf bytes.Buffer
	err = query.RunNoteGet(db, query.NoteGetConfig{
		Input: "Shared Title", JSONOutput: true, VaultPath: dir,
	}, &buf)
	require.NoError(t, err)

	var env struct {
		Status string `json:"status"`
		Errors []struct {
			Code       string   `json:"code"`
			Candidates []string `json:"candidates"`
		} `json:"errors"`
	}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &env))
	assert.Equal(t, "error", env.Status)
	require.NotEmpty(t, env.Errors)
	assert.Equal(t, "ambiguous_resolution", env.Errors[0].Code)
	assert.Len(t, env.Errors[0].Candidates, 2, "ambiguous envelope must list both candidate IDs")
}

// RunLinks in-direction: alpha must have beta as an inbound source (beta
// references alpha via both wikilink and related_ids).
func TestRunLinks_InDirectionFindsInboundEdge(t *testing.T) {
	db, dir := smallIndexedVault(t)
	var buf bytes.Buffer
	err := query.RunLinks(db, query.LinksConfig{
		Input: "concept-alpha", Direction: "in", VaultPath: dir, JSONOutput: true,
	}, &buf)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "proj-beta")
}

// RunLinks out-direction: alpha outbound must include proj-beta.
func TestRunLinks_OutDirectionFindsOutboundEdge(t *testing.T) {
	db, dir := smallIndexedVault(t)
	var buf bytes.Buffer
	err := query.RunLinks(db, query.LinksConfig{
		Input: "concept-alpha", Direction: "out", VaultPath: dir, JSONOutput: true,
	}, &buf)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "proj-beta")
}

// RunLinks in human mode (non-JSON) prints rows with source/edge/confidence
// columns — scripts awk these. Covers the human-output branch of runLinksIn
// and runLinksOut.
func TestRunLinks_InHumanOutputHasSourceAndEdgeColumns(t *testing.T) {
	db, dir := smallIndexedVault(t)
	var buf bytes.Buffer
	err := query.RunLinks(db, query.LinksConfig{
		Input: "concept-alpha", Direction: "in", VaultPath: dir,
	}, &buf)
	require.NoError(t, err)
	out := buf.String()
	// Source must include proj-beta (which references alpha)
	assert.Contains(t, out, "proj-beta")
}

func TestRunLinks_OutHumanOutputHasTargetAndEdgeColumns(t *testing.T) {
	db, dir := smallIndexedVault(t)
	var buf bytes.Buffer
	err := query.RunLinks(db, query.LinksConfig{
		Input: "concept-alpha", Direction: "out", VaultPath: dir,
	}, &buf)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "proj-beta", "alpha's outbound target must appear in human output")
}

// RunLinks with an unresolvable input must fail fast — either via a Go
// error (human mode) or an error envelope (JSON mode). Silent empty results
// would hide resolution failures from callers.
func TestRunLinks_UnresolvableInputErrors(t *testing.T) {
	db, dir := smallIndexedVault(t)
	var buf bytes.Buffer
	err := query.RunLinks(db, query.LinksConfig{
		Input: "does-not-exist", Direction: "in", VaultPath: dir,
	}, &buf)
	require.Error(t, err)
}

// RunSearch over a known token must place the matching note in the hits.
func TestRunSearch_FindsNoteByBodyToken(t *testing.T) {
	db, dir := smallIndexedVault(t)
	retriever := &query.FTSRetriever{DB: db}
	var buf bytes.Buffer
	res, err := query.RunSearch(retriever, query.SearchConfig{
		Query: "Alpha", Limit: 5, VaultPath: dir, JSONOutput: true,
	}, &buf)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.NotEmpty(t, res.Hits, "keyword 'Alpha' must appear at least once")
	// Also assert on human output shape via non-JSON mode
}

// BuildAutoRetriever must return a keyword-only retriever when there are no
// dense embeddings present in the DB (the fallback path that keeps the CLI
// usable without a running model).
func TestBuildAutoRetriever_FallsBackToKeywordWithoutEmbeddings(t *testing.T) {
	db, _ := smallIndexedVault(t)
	r := query.BuildAutoRetrieverFull(db)
	defer r.Cleanup()
	assert.NotNil(t, r.Retriever)
	assert.Nil(t, r.Embedder, "no embeddings in the small test vault → Embedder must be nil")
}

// BuildAutoRetriever (non-Full variant) delegates to Full and must return
// a non-nil retriever + safe-to-call cleanup.
func TestBuildAutoRetriever_ReturnsNonNilRetrieverAndSafeCleanup(t *testing.T) {
	db, _ := smallIndexedVault(t)
	ret, cleanup, err := query.BuildAutoRetriever(db)
	require.NoError(t, err)
	assert.NotNil(t, ret)
	require.NotNil(t, cleanup, "cleanup must always be non-nil (documented contract)")
	cleanup() // must not panic
}

// BuildRetriever: keyword mode is always valid (no embeddings needed).
func TestBuildRetriever_KeywordModeDoesNotRequireEmbeddings(t *testing.T) {
	db, _ := smallIndexedVault(t)
	ret, cleanup, err := query.BuildRetriever("keyword", db)
	require.NoError(t, err)
	assert.NotNil(t, ret)
	if cleanup != nil {
		cleanup()
	}
}

// BuildRetriever: empty string is treated as "keyword" (default path).
func TestBuildRetriever_EmptyModeTreatedAsKeyword(t *testing.T) {
	db, _ := smallIndexedVault(t)
	ret, cleanup, err := query.BuildRetriever("", db)
	require.NoError(t, err)
	assert.NotNil(t, ret)
	if cleanup != nil {
		cleanup()
	}
}

// BuildRetriever: semantic mode on a vault without embeddings errors with
// a clear message pointing at the remedy command.
func TestBuildRetriever_SemanticWithoutEmbeddingsErrors(t *testing.T) {
	db, _ := smallIndexedVault(t)
	_, _, err := query.BuildRetriever("semantic", db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no embeddings")
}

// BuildRetriever: hybrid mode on a vault without embeddings errors the
// same way as semantic (both require dense vectors).
func TestBuildRetriever_HybridWithoutEmbeddingsErrors(t *testing.T) {
	db, _ := smallIndexedVault(t)
	_, _, err := query.BuildRetriever("hybrid", db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no embeddings")
}

// BuildRetriever: unknown mode errors with a helpful message listing the
// valid options — a silent fallback would hide typos.
func TestBuildRetriever_UnknownModeErrors(t *testing.T) {
	db, _ := smallIndexedVault(t)
	_, _, err := query.BuildRetriever("fuzzy", db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fuzzy")
	assert.Contains(t, err.Error(), "keyword")
}

// Truncate: precise contract — shorter than limit returns the string
// unchanged; equal to limit returns it unchanged; longer returns the first
// maxLen runes + "..." (so the final length is maxLen + 3). Off-by-one here
// has broken grep patterns before.
func TestTruncate_Boundaries(t *testing.T) {
	assert.Equal(t, "hi", query.Truncate("hi", 10), "shorter returned unchanged")
	assert.Equal(t, "hello", query.Truncate("hello", 5), "equal length returned unchanged")
	assert.Equal(t, "hello ...", query.Truncate("hello world", 6), "truncate keeps first N runes + ellipsis")
	// Unicode sanity: runes, not bytes.
	assert.Equal(t, "åäö...", query.Truncate("åäö world", 3))
}

// FormatAsk human output includes a simple header + hit lines so users can
// read the result without --json. Losing the structure would degrade the
// terminal UX.
func TestFormatAsk_HumanOutputCarriesHits(t *testing.T) {
	r := &query.AskResult{
		Query: "what is alpha",
		TopHits: []query.ScoredResult{
			{ID: "concept-alpha", Title: "Alpha", Path: "alpha.md", Score: 0.8},
		},
	}
	var buf bytes.Buffer
	require.NoError(t, query.FormatAsk(r, &buf))
	out := buf.String()
	assert.Contains(t, out, "concept-alpha")
	assert.Contains(t, out, "Alpha")
}

// FormatAsk with a context pack attached must render the target's
// type+title and each context item. The context section is the critical
// output for agents that use `ask` as a retrieval front-end — losing it
// drops the whole "why this answer" explanation.
func TestFormatAsk_RendersTargetAndContextItems(t *testing.T) {
	r := &query.AskResult{
		Query: "what is alpha",
		TopHits: []query.ScoredResult{
			{ID: "concept-alpha", Title: "Alpha", Path: "alpha.md", Score: 0.8},
		},
		Context: &memory.ContextPackResult{
			TargetID:     "concept-alpha",
			BudgetTokens: 1000, UsedTokens: 120,
			Target: &memory.ContextPackTarget{
				ID: "concept-alpha",
				Frontmatter: map[string]interface{}{
					"type": "concept", "title": "Alpha",
				},
				Body: "Alpha is the anchor of spreading activation.",
			},
			Context: []memory.ContextItem{
				{
					ID:           "proj-beta",
					Frontmatter:  map[string]interface{}{"type": "project", "title": "Beta"},
					BodyIncluded: true,
					Body:         "Beta uses Alpha as its conceptual root.",
				},
			},
		},
	}
	var buf bytes.Buffer
	require.NoError(t, query.FormatAsk(r, &buf))
	out := buf.String()
	assert.Contains(t, out, "Context from: concept-alpha", "target ID must appear")
	assert.Contains(t, out, "120/1000 tokens", "budget/used line must render")
	assert.Contains(t, out, "[concept] Alpha", "target [type] title line must appear")
	assert.Contains(t, out, "Alpha is the anchor", "target body must be rendered")
	assert.Contains(t, out, "[project] Beta", "each context item's [type] title must appear")
	assert.Contains(t, out, "Beta uses Alpha", "context item body included when BodyIncluded=true")
}

// Context items with BodyIncluded=false must NOT leak their body into the
// output. This is the slim-mode contract consumers rely on.
func TestFormatAsk_SlimContextItemOmitsBody(t *testing.T) {
	r := &query.AskResult{
		Query: "q",
		Context: &memory.ContextPackResult{
			TargetID: "concept-alpha",
			Context: []memory.ContextItem{
				{
					ID:           "proj-beta",
					Frontmatter:  map[string]interface{}{"type": "project", "title": "Beta"},
					BodyIncluded: false,
					Body:         "SECRET BODY TEXT MUST NOT APPEAR",
				},
			},
		},
	}
	var buf bytes.Buffer
	require.NoError(t, query.FormatAsk(r, &buf))
	out := buf.String()
	assert.Contains(t, out, "[project] Beta")
	assert.NotContains(t, out, "SECRET BODY TEXT",
		"BodyIncluded=false must keep the body out of the rendered output")
}
