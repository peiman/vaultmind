package gap

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/parser"
	"github.com/peiman/vaultmind/internal/schema"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const conceptTypes = "types:\n  concept:\n    required: [title]\n"

func openVault(t *testing.T, body string) Vault {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"), []byte(body), 0o644))
	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	dbPath, err := vault.ResolveInside(dir, cfg.Index.DBPath)
	require.NoError(t, err)
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return Vault{
		Path:   dir,
		DB:     db,
		Config: cfg,
		Reg:    schema.NewRegistryWithAliases(cfg.Types, cfg.Schema.Aliases),
	}
}

func indexNote(t *testing.T, v Vault, rel, content string) {
	t.Helper()
	abs := filepath.Join(v.Path, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o750))
	require.NoError(t, os.WriteFile(abs, []byte(content), 0o640))
	dbPath := filepath.Join(v.Path, v.Config.Index.DBPath)
	require.NoError(t, index.NewIndexer(v.Path, dbPath, v.Config).IndexFile(rel))
}

type stubHits struct {
	ids   []string
	err   error
	calls int
}

func (s *stubHits) IDs(context.Context, string) ([]string, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.ids, nil
}

func TestOpen_WritesAGapAndDedups(t *testing.T) {
	v := openVault(t, conceptTypes)
	now := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	hits := &stubHits{ids: []string{"concept-memory"}}
	var out bytes.Buffer
	req := Request{
		Question: "How does quartz work?",
		Session:  "sess-1",
		Now:      now,
		Stdout:   &out,
	}
	require.NoError(t, Open(context.Background(), v, req, hits))
	assert.Equal(t, 1, hits.calls)
	assert.Contains(t, out.String(), "mode: created")
	assert.Contains(t, out.String(), "gaps opened: 1")
	assert.Contains(t, out.String(), "median days to close: n/a")

	raw, err := os.ReadFile(filepath.Join(v.Path, "gaps", "how-does-quartz-work.md"))
	require.NoError(t, err)
	note, err := parser.Parse(raw)
	require.NoError(t, err)
	assert.Equal(t, "gap-how-does-quartz-work", note.Frontmatter["id"])
	assert.Equal(t, vault.GapType, note.Frontmatter["type"])
	assert.Equal(t, statusOpen, note.Frontmatter["status"])
	assert.Equal(t, "How does quartz work?", note.Frontmatter["question"])
	assert.Equal(t, "How does quartz work?", note.Frontmatter["title"])
	assert.Equal(t, "2026-10-12T00:00:00Z", note.Frontmatter["opened_at"])
	assert.Equal(t, "sess-1", note.Frontmatter["session"])
	assert.Equal(t, []any{"concept-memory"}, note.Frontmatter["asked_hits"])

	row, err := v.DB.QueryNoteByID("gap-how-does-quartz-work")
	require.NoError(t, err)
	require.NotNil(t, row)
	assert.Equal(t, index.TypeGap, row.Type)
	assert.Equal(t, statusOpen, row.Status)

	out.Reset()
	req.Question = "  how   DOES quartz   work?  "
	req.Session = "other"
	require.NoError(t, Open(context.Background(), v, req, hits))
	assert.Equal(t, 1, hits.calls, "an existing open gap is not ranked again")
	assert.Contains(t, out.String(), "mode: existing")
	assert.Contains(t, out.String(), "id: gap-how-does-quartz-work")
	matches, err := filepath.Glob(filepath.Join(v.Path, "gaps", "*.md"))
	require.NoError(t, err)
	assert.Len(t, matches, 1)
}

func TestOpen_ClosedGapDoesNotDedup(t *testing.T) {
	v := openVault(t, conceptTypes)
	indexNote(t, v, "gaps/how-does-quartz-work.md", `---
id: gap-how-does-quartz-work
type: gap
title: How does quartz work?
status: closed
question: How does quartz work?
opened_at: "2026-10-01T00:00:00Z"
closed_at: "2026-10-03T00:00:00Z"
closed_by: concept-answer
---

Closed.
`)
	hits := &stubHits{ids: []string{"concept-answer"}}
	var out bytes.Buffer
	req := Request{Question: "How does quartz work?", Now: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC), Stdout: &out}
	require.NoError(t, Open(context.Background(), v, req, hits))
	assert.Equal(t, 1, hits.calls)
	assert.Contains(t, out.String(), "mode: created")
	_, err := os.Stat(filepath.Join(v.Path, "gaps", "how-does-quartz-work-2.md"))
	require.NoError(t, err)
}

func TestOpen_NoTypesNamesTheFix(t *testing.T) {
	v := openVault(t, "vault:\n  exclude: [\".git\"]\n")
	var out bytes.Buffer
	err := Open(context.Background(), v, Request{Question: "Where is it?", Stdout: &out}, &stubHits{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "types.gap")
	assert.Contains(t, err.Error(), ".vaultmind/config.yaml")
	assert.Contains(t, err.Error(), "vaultmind init")
	matches, globErr := filepath.Glob(filepath.Join(v.Path, "gaps", "*.md"))
	require.NoError(t, globErr)
	assert.Empty(t, matches)

	out.Reset()
	err = Open(context.Background(), v, Request{Question: "Where is it?", JSON: true, Stdout: &out}, &stubHits{})
	require.Error(t, err)
	var env struct {
		SchemaVersion string `json:"schema_version"`
		Command       string `json:"command"`
		Status        string `json:"status"`
		Errors        []struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "v1", env.SchemaVersion)
	assert.Equal(t, commandGap, env.Command)
	assert.Equal(t, "error", env.Status)
	require.NotEmpty(t, env.Errors)
	assert.Equal(t, "type_missing", env.Errors[0].Code)
	assert.Contains(t, env.Errors[0].Message, "types.gap")
}

func TestOpen_JSONCreatedOmitsMedianUntilOneCloses(t *testing.T) {
	v := openVault(t, conceptTypes)
	var out bytes.Buffer
	req := Request{
		Question: "Where is the index?",
		Now:      time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC),
		JSON:     true,
		Stdout:   &out,
	}
	require.NoError(t, Open(context.Background(), v, req, nil))
	var env map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "v1", env["schema_version"])
	assert.Equal(t, commandGap, env["command"])
	result, ok := env["result"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, modeCreated, result["mode"])
	assert.Equal(t, "gap-where-is-the-index", result["id"])
	assert.Equal(t, statusOpen, result["status"])
	assert.Empty(t, result["asked_hits"])
	assert.Equal(t, float64(1), result["opened"])
	assert.Equal(t, float64(0), result["closed"])
	_, hasMedian := result["median_days_to_close"]
	assert.False(t, hasMedian, "a vault with nothing closed omits the median")
	raw, err := os.ReadFile(filepath.Join(v.Path, "gaps", "where-is-the-index.md"))
	require.NoError(t, err)
	note, err := parser.Parse(raw)
	require.NoError(t, err)
	_, hasSession := note.Frontmatter["session"]
	assert.False(t, hasSession, "an empty session is omitted")
}

func TestOpen_EmptyQuestionAndRetrievalError(t *testing.T) {
	v := openVault(t, conceptTypes)
	err := Open(context.Background(), v, Request{Question: "   "}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "a question is required")

	hits := &stubHits{err: assert.AnError}
	err = Open(context.Background(), v, Request{Question: "Where?"}, hits)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "listing what ask returns")
	assert.Equal(t, 1, hits.calls)

	var out bytes.Buffer
	err = Open(context.Background(), v, Request{Question: "Where?", JSON: true, Stdout: &out}, hits)
	require.Error(t, err)
	assert.Contains(t, out.String(), "retrieval_failed")
}

func TestSlugQuestion_FallsBackAndTruncates(t *testing.T) {
	assert.Equal(t, "note", slugQuestion("???"))
	assert.Equal(t, strings.Repeat("b", 79), slugQuestion(strings.Repeat("b", 79)+" cccc"))
	assert.Equal(t, strings.Repeat("a", 80), slugQuestion(strings.Repeat("a", 90)))
}

func TestOpen_StopsWhenTheContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Open(ctx, openVault(t, conceptTypes), Request{Question: "Where?"}, nil)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestOpen_RequiresAnOpenVault(t *testing.T) {
	err := Open(context.Background(), Vault{}, Request{Question: "Where?"}, nil)
	assert.EqualError(t, err, "vault is not open")
}

func TestOpen_ZeroClockUsesUTCNow(t *testing.T) {
	v := openVault(t, conceptTypes)
	before := time.Now().UTC().Add(-time.Second)
	require.NoError(t, Open(context.Background(), v, Request{Question: "When?"}, nil))
	raw, err := os.ReadFile(filepath.Join(v.Path, "gaps", "when.md"))
	require.NoError(t, err)
	note, err := parser.Parse(raw)
	require.NoError(t, err)
	opened, ok := note.Frontmatter["opened_at"].(string)
	require.True(t, ok)
	parsed, err := time.Parse(time.RFC3339, opened)
	require.NoError(t, err)
	assert.False(t, parsed.Before(before))
}

func TestOpen_JSONCarriesTheIndexHash(t *testing.T) {
	v := openVault(t, conceptTypes)
	v.IndexHash = func() string { return "hash-1" }
	var out bytes.Buffer
	require.NoError(t, Open(context.Background(), v, Request{
		Question: "Hashed?",
		JSON:     true,
		Stdout:   &out,
		Now:      time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC),
	}, nil))
	assert.Contains(t, out.String(), `"index_hash":"hash-1"`)
	require.NoError(t, render(Request{JSON: true, Stdout: &out}, v, outcome{Question: "Q", Status: statusOpen}))
	assert.Contains(t, out.String(), `"asked_hits":[]`)
}

func TestOpen_DedupSkipsUnreadableQuestions(t *testing.T) {
	v := openVault(t, conceptTypes)
	indexNote(t, v, "gaps/aaa.md", gapFile("gap-aaa", statusOpen, "2026-10-01T00:00:00Z", "", "Other question"))
	indexNote(t, v, "gaps/bad.md", gapFile("gap-bad", statusOpen, "2026-10-01T00:00:00Z", "", "Bad question"))
	indexNote(t, v, "gaps/dup-a.md", gapFile("gap-dup-a", statusOpen, "2026-10-01T00:00:00Z", "", "Same question"))
	indexNote(t, v, "gaps/dup-b.md", gapFile("gap-dup-b", statusOpen, "2026-10-01T00:00:00Z", "", "Same question"))
	_, err := v.DB.Exec(`UPDATE frontmatter_kv SET value_json = '123' WHERE note_id = 'gap-bad' AND key = 'question'`)
	require.NoError(t, err)

	var out bytes.Buffer
	hits := &stubHits{}
	require.NoError(t, Open(context.Background(), v, Request{
		Question: "Same question",
		Stdout:   &out,
		Now:      time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC),
	}, hits))
	assert.Contains(t, out.String(), "mode: existing")
	assert.Contains(t, out.String(), "id: gap-dup-a")
	assert.Equal(t, 0, hits.calls)
}

func TestOpen_DedupReportsCorruptAskedHits(t *testing.T) {
	v := openVault(t, conceptTypes)
	indexNote(t, v, "gaps/dup-a.md", gapFile("gap-dup-a", statusOpen, "2026-10-01T00:00:00Z", "", "Same question"))
	_, err := v.DB.Exec(
		`INSERT INTO frontmatter_kv (note_id, key, value_json) VALUES ('gap-dup-a', 'asked_hits', 'nope')`)
	require.NoError(t, err)
	err = Open(context.Background(), v, Request{Question: "Same question"}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decoding asked hits")
}

func TestOpen_StopsAfterFiveNameCollisions(t *testing.T) {
	v := openVault(t, conceptTypes)
	slug := slugQuestion("How does quartz work?")
	require.NoError(t, os.MkdirAll(filepath.Join(v.Path, gapsDir), 0o750))
	for n := 0; n < 5; n++ {
		require.NoError(t, os.WriteFile(filepath.Join(v.Path, filepath.FromSlash(gapRel(slug, n))), []byte("x"), 0o640))
	}
	err := Open(context.Background(), v, Request{Question: "How does quartz work?"}, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errExists)
}

func TestWriteGap_RejectsAnEscapingPathAndAMissingType(t *testing.T) {
	v := openVault(t, conceptTypes)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := writeNew(ctx, v, Request{Question: "Where?"}, nil)
	assert.ErrorIs(t, err, context.Canceled)

	_, err = writeGap(context.Background(), v, Request{Question: "Where?"}, "../outside.md", nil, time.Now().UTC())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "escapes the vault")

	bare := openVault(t, "vault:\n  exclude: [\".git\"]\n")
	_, err = writeGap(context.Background(), bare, Request{Question: "Where?"}, "gaps/where.md", nil, time.Now().UTC())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "types.gap")
}

func TestReindex_StopsOnACancelledContextAndABadDatabase(t *testing.T) {
	v := openVault(t, conceptTypes)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.Contains(t, reindex(ctx, v, "gaps/where.md"), "re-index failed")

	cfg := *v.Config
	cfg.Index.DBPath = gapsDir
	v.Config = &cfg
	assert.Contains(t, reindex(context.Background(), v, "gaps/missing.md"), "re-index failed")
}
