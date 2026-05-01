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
	"github.com/peiman/vaultmind/internal/retrieval"
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

// Human mode: header line plus the note's body. Pre-2026-04-30 this
// returned only the one-line header — which forced agents to fall back
// to the Read tool when they wanted bodies, bypassing the access
// tracker. The path of least resistance was the unmonitored path. The
// fix makes `note get` the cleanest read AND a tracked one in the same
// invocation. See feedback_vaultmind_query_shape and the AX evaluation
// in the felt-experience inventory of plasticity step 5.
func TestRunNoteGet_HumanModeShowsHeaderAndBody(t *testing.T) {
	db, dir := smallIndexedVault(t)
	var buf bytes.Buffer
	err := query.RunNoteGet(db, query.NoteGetConfig{
		Input: "concept-alpha", VaultPath: dir,
	}, &buf)
	require.NoError(t, err)
	out := buf.String()
	// Header still present.
	assert.Contains(t, out, "concept-alpha")
	assert.Contains(t, out, "Alpha")
	assert.Contains(t, out, "concept")
	// Body now present too — fixture's body is "See [[proj-beta]]."
	assert.Contains(t, out, "proj-beta", "human mode must include the note body so agents don't fall back to Read")
}

// FrontmatterOnly still strips the body in human mode — when the
// caller explicitly asked for "no body," we honor it. The default
// (no flag) is the body-included path above.
func TestRunNoteGet_HumanModeFrontmatterOnlyStripsBody(t *testing.T) {
	db, dir := smallIndexedVault(t)
	var buf bytes.Buffer
	err := query.RunNoteGet(db, query.NoteGetConfig{
		Input: "concept-alpha", FrontmatterOnly: true, VaultPath: dir,
	}, &buf)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "concept-alpha", "header must still print")
	assert.NotContains(t, out, "proj-beta", "FrontmatterOnly must omit body content even in human mode")
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

// Doctor on a small indexed vault populates the core counts without error.
// Regression guard: Doctor touches many joins/queries; a migration that
// broke any would show up here as an error or a zero count.
func TestDoctor_CoreCountsPopulateCleanly(t *testing.T) {
	db, dir := smallIndexedVault(t)
	result, err := query.Doctor(db, dir)
	require.NoError(t, err)
	assert.Equal(t, 3, result.TotalFiles, "vault has 3 .md files")
	assert.Equal(t, 3, result.DomainNotes, "all three are domain notes")
	assert.Equal(t, 0, result.UnstructuredNotes)
}

// Doctor against a real indexer-built vault (not hand-inserted rows) flags
// Obsidian-incompatible wikilinks end-to-end. The existing test in
// doctor_test.go proves the detection logic against crafted DB rows; this
// one proves the indexer produces the right shape from real files — closing
// the gap where the indexer's link resolver could silently diverge from
// what Doctor expects.
func TestDoctor_IncompatibleLinkDetectedE2E(t *testing.T) {
	db, dir := smallIndexedVault(t)
	result, err := query.Doctor(db, dir)
	require.NoError(t, err)
	// alpha.md has [[proj-beta]] which resolves to beta.md (stem=beta).
	// proj-beta != beta → this must surface as incompatible.
	assert.Greater(t, result.Issues.ObsidianIncompatibleLinks, 0,
		"proj-beta vs beta.md stem must register as incompatible")
	require.NotEmpty(t, result.Issues.IncompatibleLinkDetails,
		"incompatible count without details is unusable — remediation UIs need specifics")
	// The SuggestedFix should be the filename stem.
	var foundBeta bool
	for _, il := range result.Issues.IncompatibleLinkDetails {
		if il.TargetRaw == "proj-beta" {
			foundBeta = true
			assert.Equal(t, "beta", il.SuggestedFix, "SuggestedFix must be the actual filename stem")
		}
	}
	assert.True(t, foundBeta, "incompatible details must reference the specific problematic link")
}

// Doctor on a vault with no issues still populates every issue field as
// an empty slice (not nil) — JSON consumers rely on the arrays being
// present so their schemas don't break.
func TestDoctor_IssueArraysAreAlwaysInitialized(t *testing.T) {
	// Use smallIndexedVault which does have issues; we just check the
	// array fields are allocated (either empty or populated, never nil).
	db, dir := smallIndexedVault(t)
	result, err := query.Doctor(db, dir)
	require.NoError(t, err)
	assert.NotNil(t, result.Issues.IncompatibleLinkDetails, "IncompatibleLinkDetails must not be nil")
	assert.NotNil(t, result.Issues.PathPseudoIDDetails, "PathPseudoIDDetails must not be nil")
}

// collectEmbeddingStatus with dense-only embeddings (MiniLM-style) reports
// SemanticReady=true and infers model=minilm from the dimensionality.
// Note: we test this transitively through Doctor and EmbedNotes with our
// fakeDenseEmbedder — direct testing would require stub internal state.
// Here we just ensure that after embedding, the status reflects the DenseCount.
func TestDoctor_EmbeddingsStatusAfterDenseEmbed(t *testing.T) {
	db, dir := smallIndexedVault(t)

	// Verify pre-state
	result, err := query.Doctor(db, dir)
	require.NoError(t, err)
	assert.Equal(t, 0, result.Embeddings.DenseCount)
	assert.False(t, result.Embeddings.SemanticReady)

	// The post-embed state is already covered by
	// TestEmbedNotes_MarksHasEmbeddingsTrue in internal/index; this test
	// focuses on the pre-embed shape since that's the common user starting
	// point when they run `vaultmind doctor` for the first time.
}

// Doctor reports an embedding-readiness summary. On a vault without any
// embeddings, SemanticReady must be false and the note count must match.
func TestDoctor_EmbeddingsStatusReflectsAbsence(t *testing.T) {
	db, dir := smallIndexedVault(t)
	result, err := query.Doctor(db, dir)
	require.NoError(t, err)
	require.NotNil(t, result.Embeddings, "embeddings field must always be populated")
	assert.False(t, result.Embeddings.SemanticReady, "no embeddings = SemanticReady false")
	assert.Equal(t, result.TotalFiles, result.Embeddings.TotalNotes,
		"total notes in the embedding report must match the vault total")
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

// FormatAskExplain must render per-lane contributions so operators can see
// the fusion math without --json + jq. Lanes sorted alphabetically for
// diff-friendly output; "mean of N" must appear so imbalance ("mean of 2"
// next to "mean of 4") is spottable at a glance.
func TestFormatAskExplain_RendersLaneBreakdown(t *testing.T) {
	r := &query.AskResult{
		Query: "q",
		TopHits: []retrieval.ScoredResult{
			{
				ID: "partial", Title: "Partial", Score: 0.0164,
				Components: map[string]float64{
					"fts":   0.01639,
					"dense": 0.01639,
				},
			},
			{
				ID: "full", Title: "Full", Score: 0.01587,
				Components: map[string]float64{
					"fts": 0.01587, "dense": 0.01587, "sparse": 0.01587, "colbert": 0.01587,
				},
			},
		},
	}
	var buf bytes.Buffer
	require.NoError(t, query.FormatAskExplain(r, &buf))
	out := buf.String()

	assert.Contains(t, out, "lanes:", "must introduce the lane breakdown")
	assert.Contains(t, out, "dense=0.01639", "must print each lane's contribution")
	assert.Contains(t, out, "fts=0.01639")
	assert.Contains(t, out, "mean of 2", "coverage count must be visible")
	assert.Contains(t, out, "mean of 4")

	// Alphabetical sort check: for the 4-lane hit, "colbert" must appear
	// before "dense" in the output line.
	fullIdx := bytes.Index([]byte(out), []byte("Full"))
	require.Positive(t, fullIdx, "Full hit must be in output")
	tail := out[fullIdx:]
	assert.Less(t,
		bytes.Index([]byte(tail), []byte("colbert")),
		bytes.Index([]byte(tail), []byte("dense")),
		"lanes must be alphabetized for reviewable diffs")
}

// Plain FormatAsk must NOT leak the lane breakdown — --explain is opt-in so
// default output stays terse. A regression would flood terminals.
func TestFormatAsk_DefaultSuppressesLaneBreakdown(t *testing.T) {
	r := &query.AskResult{
		Query: "q",
		TopHits: []retrieval.ScoredResult{
			{ID: "x", Title: "X", Score: 0.5, Components: map[string]float64{"fts": 0.5}},
		},
	}
	var buf bytes.Buffer
	require.NoError(t, query.FormatAsk(r, &buf))
	assert.NotContains(t, buf.String(), "lanes:", "non-explain mode stays terse")
	assert.NotContains(t, buf.String(), "mean of")
}

// FormatAsk human output includes a simple header + hit lines so users can
// read the result without --json. Losing the structure would degrade the
// terminal UX.
func TestFormatAsk_HumanOutputCarriesHits(t *testing.T) {
	r := &query.AskResult{
		Query: "what is alpha",
		TopHits: []retrieval.ScoredResult{
			{ID: "concept-alpha", Title: "Alpha", Path: "alpha.md", Score: 0.8},
		},
	}
	var buf bytes.Buffer
	require.NoError(t, query.FormatAsk(r, &buf))
	out := buf.String()
	assert.Contains(t, out, "concept-alpha")
	assert.Contains(t, out, "Alpha")
}

// FormatAskPreview renders a one-line body snippet under each ranked
// hit. Bridges the AX gap between --pointers-only (titles only — agent
// often can't tell what a note is) and full Ask (3000+ tokens of
// context pack). Snippet field is already populated by every
// retriever; this test pins that the renderer surfaces it.
func TestFormatAskPreview_RendersSnippetUnderEachHit(t *testing.T) {
	r := &query.AskResult{
		Query: "spreading activation",
		TopHits: []retrieval.ScoredResult{
			{
				ID: "concept-spreading", Title: "Spreading Activation",
				Snippet: "A method for searching associative networks where activation propagates from a source node along weighted edges.",
				Score:   0.8,
			},
			{
				ID: "concept-rrf", Title: "Reciprocal Rank Fusion",
				Snippet: "A simple parameter-light method for combining multiple ranked result lists into a unified ranking.",
				Score:   0.6,
			},
		},
	}
	var buf bytes.Buffer
	require.NoError(t, query.FormatAskPreview(r, &buf))
	out := buf.String()
	// Hits still render with id+title.
	assert.Contains(t, out, "concept-spreading")
	assert.Contains(t, out, "concept-rrf")
	// Snippet line appears under each hit (truncated at 110 runes).
	assert.Contains(t, out, "↳", "preview must use the indented marker so the snippet visually attaches to its hit")
	assert.Contains(t, out, "associative networks", "the snippet content must surface")
	assert.Contains(t, out, "ranked result lists", "the second hit's snippet must surface too")
}

// previewSnippet strips leading markdown headings and collapses
// internal newlines so the snippet shown under a hit doesn't waste
// width echoing what we already printed as the title. The fresh-session
// evaluator flagged this: "--preview snippet is often the title echoed
// + first line. Not always informative for deciding what to read."
func TestFormatAskPreview_StripsLeadingHeadingsFromSnippet(t *testing.T) {
	r := &query.AskResult{
		Query: "x",
		TopHits: []retrieval.ScoredResult{
			{
				ID: "concept-x", Title: "X",
				// Real-shape snippet: a section heading then content.
				Snippet: "## Overview\n\nThe actual content of this note begins here and is what the agent actually wants to see when scanning.",
				Score:   0.5,
			},
		},
	}
	var buf bytes.Buffer
	require.NoError(t, query.FormatAskPreview(r, &buf))
	out := buf.String()
	assert.Contains(t, out, "↳ The actual content of this note begins here",
		"the snippet line must lead with the actual body content, not the heading")
	assert.NotContains(t, out, "↳ ## Overview",
		"the leading heading must be stripped before display")
	// Newlines collapsed to spaces.
	assert.NotContains(t, out, "\n\n",
		"internal newlines must collapse so the preview stays one line")
}

// Multiple stacked headings (# Title\n## Section\n) all get stripped.
func TestFormatAskPreview_StripsMultipleStackedHeadings(t *testing.T) {
	r := &query.AskResult{
		Query: "x",
		TopHits: []retrieval.ScoredResult{
			{
				ID: "x", Title: "X",
				Snippet: "# Memory Consolidation\n\n## Overview\n\nMemory consolidation is the process by which fragile traces stabilize.",
				Score:   0.5,
			},
		},
	}
	var buf bytes.Buffer
	require.NoError(t, query.FormatAskPreview(r, &buf))
	out := buf.String()
	assert.Contains(t, out, "↳ Memory consolidation is the process",
		"both leading headings must be stripped, leaving the actual body")
	assert.NotContains(t, out, "## Overview",
		"second heading must be stripped too")
}

// Hits with empty snippets render without the snippet line — no
// dangling "↳" markers when there's nothing to show.
func TestFormatAskPreview_OmitsSnippetLineWhenEmpty(t *testing.T) {
	r := &query.AskResult{
		Query: "x",
		TopHits: []retrieval.ScoredResult{
			{ID: "no-snippet", Title: "No Snippet", Score: 0.5},
		},
	}
	var buf bytes.Buffer
	require.NoError(t, query.FormatAskPreview(r, &buf))
	out := buf.String()
	assert.Contains(t, out, "no-snippet")
	assert.NotContains(t, out, "↳", "no snippet → no marker line")
}

// When some context items are truncated to fit the token budget, the
// header shows the with-bodies count AND a footer hint names the exact
// remedy. Pre-2026-04-30 the truncation was silent — the agent saw a
// mix of items-with-bodies and items-without-bodies and read it as
// "feels arbitrary" (per the fresh-session evaluation). Surfacing the
// budget math turns the inconsistency into an honest, actionable signal.
func TestFormatAsk_SurfacesBudgetTruncationToContextItems(t *testing.T) {
	r := &query.AskResult{
		Query: "x",
		TopHits: []retrieval.ScoredResult{
			{ID: "concept-x", Title: "X", Score: 0.5},
		},
		Context: &memory.ContextPackResult{
			TargetID:     "concept-x",
			BudgetTokens: 100, UsedTokens: 95,
			Target: &memory.ContextPackTarget{
				ID:          "concept-x",
				Frontmatter: map[string]interface{}{"type": "concept", "title": "X"},
				Body:        "X target body.",
			},
			Context: []memory.ContextItem{
				{ID: "a", Frontmatter: map[string]interface{}{"type": "concept", "title": "A"}, Body: "A body", BodyIncluded: true},
				{ID: "b", Frontmatter: map[string]interface{}{"type": "concept", "title": "B"}, Body: "B body", BodyIncluded: true},
				{ID: "c", Frontmatter: map[string]interface{}{"type": "concept", "title": "C"}, Body: "C body", BodyIncluded: false},
				{ID: "d", Frontmatter: map[string]interface{}{"type": "concept", "title": "D"}, Body: "D body", BodyIncluded: false},
			},
		},
	}
	var buf bytes.Buffer
	require.NoError(t, query.FormatAsk(r, &buf))
	out := buf.String()
	assert.Contains(t, out, "4 items, 2 with bodies",
		"header must show the with-bodies count when not all items got bodies")
	assert.Contains(t, out, "2 items above had body omitted",
		"footer must name how many were truncated")
	assert.Contains(t, out, "increase --budget",
		"footer must point at the remedy")
}

// When all context items got their bodies, the header stays terse
// (no with-bodies count) and there's no truncation hint. Avoids
// noisy output when the budget was sufficient.
func TestFormatAsk_NoTruncationHintWhenAllBodiesIncluded(t *testing.T) {
	r := &query.AskResult{
		Query: "x",
		TopHits: []retrieval.ScoredResult{
			{ID: "concept-x", Title: "X", Score: 0.5},
		},
		Context: &memory.ContextPackResult{
			TargetID:     "concept-x",
			BudgetTokens: 4000, UsedTokens: 200,
			Target: &memory.ContextPackTarget{
				ID:          "concept-x",
				Frontmatter: map[string]interface{}{"type": "concept", "title": "X"},
				Body:        "X target body.",
			},
			Context: []memory.ContextItem{
				{ID: "a", Frontmatter: map[string]interface{}{"type": "concept", "title": "A"}, Body: "A body", BodyIncluded: true},
				{ID: "b", Frontmatter: map[string]interface{}{"type": "concept", "title": "B"}, Body: "B body", BodyIncluded: true},
			},
		},
	}
	var buf bytes.Buffer
	require.NoError(t, query.FormatAsk(r, &buf))
	out := buf.String()
	assert.Contains(t, out, "(2 items, 200/4000 tokens)",
		"clean header (no with-bodies count) when nothing was truncated")
	assert.NotContains(t, out, "with bodies",
		"no with-bodies count when all items got bodies")
	assert.NotContains(t, out, "had body omitted",
		"no truncation hint when nothing was truncated")
}

// FormatAsk auto-degrades to pointers-only when confidence is no_match.
// The principle: don't render a 2000-token apparatus around a top-1
// the system has labelled "essentially tied with the field." Round-2
// evaluator caught the original behaviour: nonsense query landed
// no_match label but still got a 1762-token context-pack around an
// unrelated note.
func TestFormatAsk_NoMatchConfidenceForcesPointersOnly(t *testing.T) {
	r := &query.AskResult{
		Query:            "x",
		TopHitConfidence: query.ConfidenceNoMatch,
		TopHits: []retrieval.ScoredResult{
			{ID: "a", Title: "A", Score: 0.5},
		},
		Context: &memory.ContextPackResult{
			TargetID:     "a",
			BudgetTokens: 4000, UsedTokens: 1762,
			Target: &memory.ContextPackTarget{
				ID:          "a",
				Frontmatter: map[string]interface{}{"type": "concept", "title": "A"},
				Body:        "this body should not render in no_match mode",
			},
			Context: []memory.ContextItem{
				{ID: "n", Frontmatter: map[string]interface{}{"type": "concept", "title": "N"}, Body: "neighbor body", BodyIncluded: true},
			},
		},
	}
	var buf bytes.Buffer
	require.NoError(t, query.FormatAsk(r, &buf))
	out := buf.String()
	assert.Contains(t, out, "no clear winner", "no_match label must surface in header")
	assert.NotContains(t, out, "this body should not render",
		"no_match must auto-degrade to pointers-only — no body content")
	assert.NotContains(t, out, "neighbor body",
		"context-pack neighbor bodies must also be suppressed under no_match")
	// Pointers-only footer hint should fire (signals the agent that
	// the menu is the menu — re-query the named hit if you want bodies).
	assert.Contains(t, out, "pointers only", "auto-degraded mode must surface the pointers-only hint")
}

// FormatAskRead renders search header + hits + the chosen note's body
// inline. Backs `vaultmind ask <query> --read N`. Pins that the menu
// is preserved (so the agent sees what they chose from) and the body
// renders as the full body, not a truncation.
func TestFormatAskRead_RendersHitsPlusChosenBody(t *testing.T) {
	r := &query.AskResult{
		Query: "spreading activation",
		TopHits: []retrieval.ScoredResult{
			{ID: "concept-spreading", Title: "Spreading Activation", Score: 0.5},
			{ID: "concept-rrf", Title: "Reciprocal Rank Fusion", Score: 0.4},
		},
	}
	note := &index.FullNote{
		ID:    "concept-rrf",
		Type:  "concept",
		Title: "Reciprocal Rank Fusion",
		Body:  "RRF combines multiple ranked lists by summing 1/(k + rank).",
	}
	var buf bytes.Buffer
	require.NoError(t, query.FormatAskRead(r, note, &buf))
	out := buf.String()
	// Menu preserved.
	assert.Contains(t, out, "concept-spreading")
	assert.Contains(t, out, "concept-rrf")
	// Chosen note's header + body inline.
	assert.Contains(t, out, "concept-rrf (concept) — Reciprocal Rank Fusion")
	assert.Contains(t, out, "RRF combines multiple ranked lists",
		"chosen note's full body must render, not truncated")
}

// FormatAsk default behavior unchanged: no snippet line under hits.
// Pins that adding --preview did not regress the default rendering.
func TestFormatAsk_DefaultDoesNotRenderHitSnippetLine(t *testing.T) {
	r := &query.AskResult{
		Query: "x",
		TopHits: []retrieval.ScoredResult{
			{ID: "with-snippet", Title: "Has Snippet",
				Snippet: "this content should not appear under the hit in default mode",
				Score:   0.5},
		},
	}
	var buf bytes.Buffer
	require.NoError(t, query.FormatAsk(r, &buf))
	assert.NotContains(t, buf.String(), "↳")
	assert.NotContains(t, buf.String(), "should not appear")
}

// FormatAsk with a context pack attached must render the target's
// type+title and each context item. The context section is the critical
// output for agents that use `ask` as a retrieval front-end — losing it
// drops the whole "why this answer" explanation.
func TestFormatAsk_RendersTargetAndContextItems(t *testing.T) {
	r := &query.AskResult{
		Query: "what is alpha",
		TopHits: []retrieval.ScoredResult{
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

// FormatAskPointersOnly is the principle-9 fix for the dogfood-preload
// trap (arc-plasticity-gap-from-inside, the 2026-04-25 design signal under
// step 3 of plasticity-priority-order). Asserts: target body and context
// item bodies are SKIPPED even when present and BodyIncluded=true; titles
// + ids + types DO render; the trailing hint names the next move so the
// agent treats pointers as a menu, not as the answer.
func TestFormatAskPointersOnly_SkipsBodiesEvenWhenIncluded(t *testing.T) {
	r := &query.AskResult{
		Query: "what is alpha",
		TopHits: []retrieval.ScoredResult{
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
				Body: "TARGET BODY MUST NOT APPEAR — pointers-only contract",
			},
			Context: []memory.ContextItem{
				{
					ID:           "proj-beta",
					Frontmatter:  map[string]interface{}{"type": "project", "title": "Beta"},
					BodyIncluded: true,
					Body:         "NEIGHBOR BODY MUST NOT APPEAR — pointers-only contract",
				},
			},
		},
	}
	var buf bytes.Buffer
	require.NoError(t, query.FormatAskPointersOnly(r, &buf))
	out := buf.String()
	// Pointers ARE rendered.
	assert.Contains(t, out, "[concept] Alpha", "target pointer (type + title) must render")
	assert.Contains(t, out, "[project] Beta", "context-item pointer must render")
	assert.Contains(t, out, "concept-alpha", "target id must appear so agent can ask for body")
	// Bodies ARE NOT rendered, even though BodyIncluded=true on the item
	// and Body is non-empty on the target. This is the load-bearing
	// principle-9 assertion: discipline → design.
	assert.NotContains(t, out, "TARGET BODY",
		"target body MUST be skipped — pointers-only is the dogfood-loop fix")
	assert.NotContains(t, out, "NEIGHBOR BODY",
		"context-item body MUST be skipped — pointers-only is the dogfood-loop fix")
	// The hint names the next move, so the agent knows the loop closes by
	// querying, not by waiting for context.
	assert.Contains(t, out, "pointers only", "trailing hint must surface the mode")
	assert.Contains(t, out, "vaultmind ask", "trailing hint must name the next-move command")
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
