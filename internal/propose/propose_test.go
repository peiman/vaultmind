package propose

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/parser"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/schema"
	tmpl "github.com/peiman/vaultmind/internal/template"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const noteTemplate = "---\nid: <%=id%>\ntype: <%=type%>\ntitle: <%=title%>\ncreated: <%=created%>\n---\n"

const vaultConfig = `
types:
  concept:
    required: [title]
    optional: [aliases, tags, sources]
    template: .vaultmind/templates/concept.md
  project:
    required: [title]
    statuses: [active, paused]
    template: .vaultmind/templates/concept.md
  principle:
    required: [title]
    template: .vaultmind/templates/concept.md
`

var proposeNow = time.Date(2026, 10, 14, 15, 30, 0, 0, time.UTC)

type stubClosest struct {
	hit   Hit
	ok    bool
	err   error
	calls int
	text  string
}

func (s *stubClosest) Closest(_ context.Context, _ string, _ *index.DB, text string) (Hit, bool, error) {
	s.calls++
	s.text = text
	return s.hit, s.ok, s.err
}

func testVault(t *testing.T) Vault {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind", "templates"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"), []byte(vaultConfig), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "templates", "concept.md"), []byte(noteTemplate), 0o644))
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

func writeTarget(t *testing.T, v Vault, rel, id, noteType, title, status, body string) {
	t.Helper()
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("id: " + id + "\n")
	b.WriteString("type: " + noteType + "\n")
	b.WriteString("title: " + title + "\n")
	if status != "" {
		b.WriteString("status: " + status + "\n")
	}
	b.WriteString("created: 2026-04-03\n---\n\n")
	b.WriteString(body)
	abs := filepath.Join(v.Path, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
	require.NoError(t, os.WriteFile(abs, []byte(b.String()), 0o644))
	dbPath := filepath.Join(v.Path, v.Config.Index.DBPath)
	require.NoError(t, index.NewIndexer(v.Path, dbPath, v.Config).IndexFile(rel))
}

func reopen(t *testing.T, dir string) Vault {
	t.Helper()
	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	dbPath, err := vault.ResolveInside(dir, cfg.Index.DBPath)
	require.NoError(t, err)
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return Vault{Path: dir, DB: db, Config: cfg, Reg: schema.NewRegistryWithAliases(cfg.Types, cfg.Schema.Aliases)}
}

func reqOf(question, finding string) (Request, *bytes.Buffer, *bytes.Buffer, *int) {
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	embeds := 0
	return Request{
		Question: question,
		Finding:  finding,
		Now:      proposeNow,
		Stdout:   out,
		Stderr:   errOut,
		Embed:    func(string) { embeds++ },
	}, out, errOut, &embeds
}

func parseNote(t *testing.T, v Vault, rel string) (map[string]interface{}, string) {
	t.Helper()
	abs, err := vault.ResolveInside(v.Path, rel)
	require.NoError(t, err)
	raw, err := os.ReadFile(abs)
	require.NoError(t, err)
	note, err := parser.Parse(raw)
	require.NoError(t, err)
	return note.Frontmatter, note.Body
}

func fmStrings(v interface{}) []string {
	switch t := v.(type) {
	case nil:
		return nil
	case []string:
		return t
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		if t == "" {
			return nil
		}
		return []string{t}
	default:
		return nil
	}
}

func assertNoSchemaErrors(t *testing.T, v Vault, id string) {
	t.Helper()
	res, err := query.Validate(v.DB, v.Reg)
	require.NoError(t, err)
	var messages []string
	for _, issue := range res.Issues {
		if issue.ID == id && issue.Severity == "error" {
			messages = append(messages, issue.Message)
		}
	}
	assert.Empty(t, messages)
}

func TestPropose_PatchWhenTheTopHitIsClose(t *testing.T) {
	v := testVault(t)
	const (
		targetID  = "concept-memory"
		targetRel = "concepts/memory.md"
		question  = "How does quartz work?"
		finding   = "Quartz keeps one timeline."
		source    = "https://example.com/quartz"
	)
	writeTarget(t, v, targetRel, targetID, "concept", "Memory", "accepted", "The memory note.")
	ret := &stubClosest{hit: Hit{ID: targetID, Path: targetRel, Type: "concept", Title: "Memory", Band: "strong"}, ok: true}
	req, out, _, embeds := reqOf(question, finding)
	req.Sources = []string{source}
	require.NoError(t, Run(context.Background(), v, req, ret))

	assert.Equal(t, question+"\n"+finding, ret.text)
	assert.Equal(t, 1, *embeds)
	rel := proposalPath(targetRel, targetID, proposeNow)
	id := tmpl.GenerateID(rel, "concept")
	assert.Equal(t, "mode: patch\npath: "+rel+"\nid: "+id+"\ntarget: "+targetID+"\n"+nextStep(id)+"\n", out.String())

	fm, body := parseNote(t, v, rel)
	assert.Equal(t, "draft", fm["status"])
	assert.Equal(t, targetID, fm["patches"])
	assert.Equal(t, question, fm["question"])
	assert.Equal(t, "concept", fm["type"])
	assert.Equal(t, question, fm["title"])
	assert.Equal(t, []string{source}, fmStrings(fm["sources"]))
	assert.Contains(t, body, finding)
	assert.Contains(t, body, "## Sources")
	assert.Contains(t, body, source)
	assertNoSchemaErrors(t, v, id)

	row, err := v.DB.QueryNoteByID(id)
	require.NoError(t, err)
	require.NotNil(t, row)
	assert.Equal(t, "draft", row.Status)
}

func TestPropose_NewWhenNothingIsClose(t *testing.T) {
	t.Run("moderate hit", func(t *testing.T) {
		v := testVault(t)
		writeTarget(t, v, "concepts/memory.md", "concept-memory", "concept", "Memory", "accepted", "memory")
		ret := &stubClosest{hit: Hit{ID: "concept-memory", Path: "concepts/memory.md", Type: "concept", Band: "moderate"}, ok: true}
		req, out, _, _ := reqOf("How does quartz work?", "A finding.")
		require.NoError(t, Run(context.Background(), v, req, ret))
		rel := "concepts/how-does-quartz-work.md"
		id := tmpl.GenerateID(rel, "concept")
		assert.Contains(t, out.String(), "mode: new\n")
		assert.Contains(t, out.String(), "path: "+rel+"\n")
		fm, body := parseNote(t, v, rel)
		assert.Equal(t, "draft", fm["status"])
		assert.Empty(t, fm["patches"])
		assert.Equal(t, "concept", fm["type"])
		assert.Equal(t, id, fm["id"])
		assert.Contains(t, body, "A finding.")
		assert.Contains(t, body, "## Sources")
		assertNoSchemaErrors(t, v, id)
	})

	t.Run("no hit uses the default type at the vault root", func(t *testing.T) {
		v := testVault(t)
		ret := &stubClosest{}
		req, out, _, _ := reqOf("Where does the day go?", "It goes to the work.")
		require.NoError(t, Run(context.Background(), v, req, ret))
		rel := "where-does-the-day-go.md"
		assert.Contains(t, out.String(), "path: "+rel+"\n")
		fm, _ := parseNote(t, v, rel)
		assert.Equal(t, "concept", fm["type"])
		assert.Equal(t, "draft", fm["status"])
	})
}

func TestPropose_ExistingDraftOnRepeatedQuestion(t *testing.T) {
	v := testVault(t)
	ret := &stubClosest{hit: Hit{ID: "concept-memory", Path: "concepts/memory.md", Type: "concept", Band: "moderate"}, ok: true}
	writeTarget(t, v, "concepts/memory.md", "concept-memory", "concept", "Memory", "accepted", "memory")
	first, _, _, embeds := reqOf("How does quartz work?", "First finding.")
	require.NoError(t, Run(context.Background(), v, first, ret))
	require.Equal(t, 1, ret.calls)

	second, out, _, embeds2 := reqOf("  HOW   does quartz   work? ", "A different finding.")
	require.NoError(t, Run(context.Background(), v, second, ret))
	assert.Equal(t, 1, ret.calls, "a repeated question must not retrieve or write again")
	assert.Equal(t, 0, *embeds2)
	assert.Contains(t, out.String(), "mode: existing\n")
	rel := "concepts/how-does-quartz-work.md"
	id := tmpl.GenerateID(rel, "concept")
	assert.Contains(t, out.String(), "path: "+rel+"\n")
	assert.Contains(t, out.String(), "id: "+id+"\n")
	assert.Contains(t, out.String(), nextStep(id))
	matches, err := filepath.Glob(filepath.Join(v.Path, "concepts", "*.md"))
	require.NoError(t, err)
	assert.Len(t, matches, 2, "the target plus the one draft")

	third, out3, _, _ := reqOf("How does quartz work", "Punctuation differs.")
	require.NoError(t, Run(context.Background(), v, third, ret))
	assert.Contains(t, out3.String(), "mode: new\n")
	_ = embeds
}

func TestPropose_SourceValidation(t *testing.T) {
	v := testVault(t)
	writeTarget(t, v, "concepts/memory.md", "concept-memory", "concept", "Memory", "", "body")
	inside := filepath.Join(v.Path, "concepts", "memory.md")
	repo := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repo, "outside.md"), []byte("x"), 0o644))
	// The vault's parent is not necessarily a git repo. An absolute path
	// outside both is the rejection case; a file inside the vault is accepted.

	cases := []struct {
		name    string
		sources []string
		want    string
	}{
		{name: "empty entry", sources: []string{""}, want: "empty"},
		{name: "url without a host", sources: []string{"http://"}, want: "URL"},
		{name: "missing path", sources: []string{"no/such.md"}, want: "no/such.md"},
		{name: "outside the vault", sources: []string{filepath.Join(repo, "outside.md")}, want: "outside"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, _, _, embeds := reqOf("How does quartz work?", "A finding.")
			req.Sources = tc.sources
			err := Run(context.Background(), v, req, &stubClosest{ok: true, hit: Hit{Band: "strong", ID: "concept-memory", Path: "concepts/memory.md", Type: "concept"}})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			assert.Equal(t, 0, *embeds)
		})
	}

	t.Run("url and vault file", func(t *testing.T) {
		req, _, _, _ := reqOf("How does quartz work?", "A finding.")
		req.Sources = []string{"https://example.com/a", inside}
		req.Now = proposeNow.Add(time.Minute)
		require.NoError(t, Run(context.Background(), v, req, &stubClosest{}))
	})

	t.Run("windows path is not a url", func(t *testing.T) {
		req, _, _, _ := reqOf("How does quartz work?", "A finding.")
		req.Sources = []string{`C:\notes\a.md`}
		err := Run(context.Background(), v, req, &stubClosest{})
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "URL")
	})

	t.Run("finding required", func(t *testing.T) {
		req, out, _, _ := reqOf("How does quartz work?", "  ")
		req.JSON = true
		err := Run(context.Background(), v, req, &stubClosest{})
		require.ErrorIs(t, err, envelope.ErrAlreadyWritten)
		var env envelope.Envelope
		require.NoError(t, json.Unmarshal(out.Bytes(), &env))
		assert.Equal(t, "error", env.Status)
		assert.Equal(t, "note propose", env.Command)
		require.NotEmpty(t, env.Errors)
		assert.Equal(t, "finding_required", env.Errors[0].Code)
	})

	t.Run("question required", func(t *testing.T) {
		req, _, _, _ := reqOf("  ", "A finding.")
		err := Run(context.Background(), v, req, &stubClosest{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "question")
	})
}

func TestPropose_PatchJSON(t *testing.T) {
	v := testVault(t)
	writeTarget(t, v, "concepts/memory.md", "concept-memory", "concept", "Memory", "accepted", "body")
	ret := &stubClosest{hit: Hit{ID: "concept-memory", Path: "concepts/memory.md", Type: "concept", Band: "strong"}, ok: true}
	req, out, _, _ := reqOf("How does quartz work?", "A finding.")
	req.JSON = true
	require.NoError(t, Run(context.Background(), v, req, ret))
	var env struct {
		SchemaVersion string `json:"schema_version"`
		Command       string `json:"command"`
		Status        string `json:"status"`
		Result        struct {
			Mode     string `json:"mode"`
			Path     string `json:"path"`
			ID       string `json:"id"`
			TargetID string `json:"target_id"`
			NextStep string `json:"next_step"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "v1", env.SchemaVersion)
	assert.Equal(t, "note propose", env.Command)
	assert.Equal(t, "ok", env.Status)
	assert.Equal(t, "patch", env.Result.Mode)
	assert.Equal(t, "concept-memory", env.Result.TargetID)
	assert.NotEmpty(t, env.Result.Path)
	assert.NotEmpty(t, env.Result.ID)
	assert.Contains(t, env.Result.NextStep, "vaultmind note accept "+env.Result.ID)
}
