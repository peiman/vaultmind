package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/mcpserver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	anchorID       = "concept-foo-anchor"
	graphDraft     = "concept-foo-draft"
	draftBody      = "DRAFTBODYSECRET"
	draftArcID     = "arc-proposed-self"
	draftArcBody   = "DRAFTARCSECRET"
	coverDraft     = "concept-covers-draft"
	coverSecret    = "DRAFTCOVERSECRET"
	graphQuery     = "fooanchorquery"
	gapNeighbor    = "gap-foo-neighbor"
	gapBody        = "GAPBODYSECRET"
	gapCover       = "gap-covers-file"
	gapCoverSecret = "GAPCOVERSECRET"
)

func osWrite(t *testing.T, path string, data []byte) error {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func osMkdir(t *testing.T, path string) error {
	t.Helper()
	return os.MkdirAll(path, 0o755)
}

func writeRawNote(t *testing.T, vault, rel, body string) {
	t.Helper()
	require.NoError(t, osWrite(t, filepath.Join(vault, rel), []byte(body)))
}

// graphNeighborVault is note A plus draft B, linked by a wikilink, a shared
// tag and an alias mention. B must be a real graph neighbor, or a test that
// only checks B's absence would pass with no edge at all.
func graphNeighborVault(t *testing.T) (vault, codeFile string) {
	t.Helper()
	vault = t.TempDir()
	require.NoError(t, osWrite(t, filepath.Join(vault, ".vaultmind", "config.yaml"), []byte(`
memory:
  alias_min_length: 3
  tag_overlap_threshold: 0
`)))
	writeRawNote(t, vault, "concepts/foo-anchor.md", `---
id: concept-foo-anchor
type: concept
title: Foo Anchor
aliases:
  - FooAnchor
tags:
  - quartzlink
created: 2026-04-03
---

`+repeatQuery(graphQuery))
	writeRawNote(t, vault, "concepts/foo-draft.md", `---
id: concept-foo-draft
type: concept
title: Foo Draft
status: draft
tags:
  - quartzlink
created: 2026-04-03
---

[[concept-foo-anchor]] mentions FooAnchor in prose.

`+draftBody+`
`)
	writeRawNote(t, vault, "concepts/covers-file.md", `---
id: concept-covers-file
type: concept
title: Covers The File
paths:
  - internal/a.go
created: 2026-04-03
---

Published line about the file.
`)
	writeRawNote(t, vault, "concepts/covers-draft.md", `---
id: concept-covers-draft
type: concept
title: Draft Cover
status: draft
paths:
  - internal/a.go
created: 2026-04-03
---

`+coverSecret+`
`)
	writeRawNote(t, vault, "arcs/kept.md", `---
id: arc-kept-self
type: arc
title: Kept Arc
created: 2026-04-03
---

I am the accepted arc.
`)
	writeRawNote(t, vault, "arcs/proposed.md", `---
id: arc-proposed-self
type: arc
title: Proposed Arc
status: draft
created: 2026-04-03
---

`+draftArcBody+`
`)
	writeRawNote(t, vault, "gaps/foo-neighbor.md", `---
id: gap-foo-neighbor
type: gap
title: Foo Gap
status: open
question: Where is the anchor?
created: 2026-04-03
---

[[concept-foo-anchor]]

`+gapBody+`
`+repeatQuery(graphQuery))
	writeRawNote(t, vault, "gaps/covers-file.md", `---
id: gap-covers-file
type: gap
title: Gap Cover
status: open
question: What covers the file?
paths:
  - internal/a.go
created: 2026-04-03
---

`+gapCoverSecret+`
`)
	repo := filepath.Join(t.TempDir(), "code-repo")
	require.NoError(t, osMkdir(t, filepath.Join(repo, ".git")))
	require.NoError(t, osMkdir(t, filepath.Join(repo, "internal")))
	codeFile = filepath.Join(repo, "internal", "a.go")
	require.NoError(t, osWrite(t, codeFile, []byte("package internal\n")))
	rebuildVault(t, vault)
	requireGraphEdges(t, vault)
	return vault, codeFile
}

func repeatQuery(q string) string {
	s := ""
	for i := 0; i < 12; i++ {
		s += q + " "
	}
	return s
}

func requireGraphEdges(t *testing.T, vault string) {
	t.Helper()
	db, err := index.Open(filepath.Join(vault, ".vaultmind", "index.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	rows, err := db.Query(`SELECT src_note_id, dst_note_id, edge_type FROM links
		WHERE (src_note_id = ? AND dst_note_id = ?) OR (src_note_id = ? AND dst_note_id = ?)`,
		graphDraft, anchorID, anchorID, graphDraft)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	got := map[string]bool{}
	for rows.Next() {
		var src, dst, edge string
		require.NoError(t, rows.Scan(&src, &dst, &edge))
		got[src+"->"+dst+" "+edge] = true
	}
	require.NoError(t, rows.Err())
	require.True(t, got[graphDraft+"->"+anchorID+" explicit_link"], "wikilink edge missing: %v", got)
	require.True(t, got[graphDraft+"->"+anchorID+" alias_mention"], "alias mention missing: %v", got)
	require.True(t, got[graphDraft+"->"+anchorID+" tag_overlap"] || got[anchorID+"->"+graphDraft+" tag_overlap"],
		"shared-tag edge missing: %v", got)
}

func TestDraftGraph_ContextOmitsTheNeighborUnlessAsked(t *testing.T) {
	isolateXDG(t)
	vault, _ := graphNeighborVault(t)
	cases := []struct {
		name string
		args []string
	}{
		{"ask", []string{"ask", graphQuery, "--vault", vault, "--json", "--budget", "8000", "--max-items", "10"}},
		{"memory pack", []string{"memory", "pack", anchorID, "--vault", vault, "--json", "--budget", "8000"}},
		{"memory context-pack", []string{"memory", "context-pack", anchorID, "--vault", vault, "--json", "--budget", "8000"}},
		{"memory related", []string{"memory", "related", anchorID, "--vault", vault, "--json"}},
		{"memory neighbors", []string{"memory", "neighbors", anchorID, "--vault", vault, "--json", "--min-confidence", "low"}},
		{"links neighbors", []string{"links", "neighbors", anchorID, "--vault", vault, "--json", "--min-confidence", "low"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, err := runRootCmd(t, tc.args...)
			require.NoError(t, err, errOut.String())
			assert.NotContains(t, out.String(), graphDraft)
			assert.NotContains(t, out.String(), draftBody)
			assert.Contains(t, out.String(), anchorID)
		})
		t.Run(tc.name+" include-drafts", func(t *testing.T) {
			out, errOut, err := runRootCmd(t, append(tc.args, "--include-drafts")...)
			require.NoError(t, err, errOut.String())
			assert.Contains(t, out.String(), graphDraft)
			if tc.name == "ask" || tc.name == "memory pack" || tc.name == "memory context-pack" {
				assert.Contains(t, out.String(), draftBody, "opting in delivers the draft body, not only its id")
			}
		})
	}
}

func TestDraftGraph_ReciteSkipsADraftArc(t *testing.T) {
	isolateXDG(t)
	vault, _ := graphNeighborVault(t)

	out, errOut, err := runRootCmd(t, "arc", "recite", "--vault", vault, "--json")
	require.NoError(t, err, errOut.String())
	assert.Contains(t, out.String(), "arc-kept-self")
	assert.NotContains(t, out.String(), draftArcID)
	assert.NotContains(t, out.String(), draftArcBody)

	out, errOut, err = runRootCmd(t, "arc", "recite", "--vault", vault, "--json", "--include-drafts")
	require.NoError(t, err, errOut.String())
	assert.Contains(t, out.String(), draftArcID)
	assert.Contains(t, out.String(), draftArcBody)
}

func TestDraftGraph_NamedDraftStaysReadable(t *testing.T) {
	isolateXDG(t)
	vault, _ := graphNeighborVault(t)

	// memory pack of an id the caller named is a direct read, like note get.
	// The draft is the target. It is not discovered by ranking or by a neighbor walk.
	out, errOut, err := runRootCmd(t, "memory", "pack", graphDraft, "--vault", vault, "--json", "--budget", "8000")
	require.NoError(t, err, errOut.String())
	assert.Contains(t, out.String(), graphDraft)
	assert.Contains(t, out.String(), draftBody)

	// ask --read only reads a hit from the menu. A draft that did not rank is
	// not on that menu; note get is the direct read.
	_, errOut, err = runRootCmd(t, "ask", graphQuery, "--vault", vault, "--read", graphDraft)
	require.Error(t, err)
	assert.NotContains(t, errOut.String(), draftBody)
}

func TestDraftGraph_TreeForSkipsADraftLine(t *testing.T) {
	isolateXDG(t)
	vault, file := graphNeighborVault(t)

	out, errOut, err := runRootCmd(t, "tree", "--vault", vault, "--for", file)
	require.NoError(t, err, errOut.String())
	assert.Contains(t, out.String(), "concept-covers-file")
	assert.NotContains(t, out.String(), coverDraft)
	assert.NotContains(t, out.String(), coverSecret)

	out, errOut, err = runRootCmd(t, "tree", "--vault", vault, "--for", file, "--include-drafts")
	require.NoError(t, err, errOut.String())
	assert.Contains(t, out.String(), coverDraft)
	assert.Contains(t, out.String(), coverSecret)

	// The vault map still lists a draft and marks it. It does not inject the line
	// into a file the agent is reading.
	listed, errOut, err := runRootCmd(t, "tree", "--vault", vault)
	require.NoError(t, err, errOut.String())
	assert.Contains(t, listed.String(), coverDraft)
	assert.Contains(t, listed.String(), "[draft]")
}

func TestDraftGraph_MCPAskOmitsTheNeighbor(t *testing.T) {
	isolateXDG(t)
	vault, _ := graphNeighborVault(t)
	runner := func(_ context.Context, args []string) ([]byte, error) {
		out, errOut, err := runRootCmd(t, args...)
		if err != nil {
			return nil, fmt.Errorf("%w\n%s", err, errOut.String())
		}
		return out.Bytes(), nil
	}
	cs := connectDraftMCP(t, mcpserver.Config{Vault: vault, Run: runner})
	res := callDraftMCP(t, cs, "ask", map[string]any{"query": graphQuery})
	text := draftMCPText(res)
	assert.NotContains(t, text, graphDraft)
	assert.NotContains(t, text, draftBody)
	assert.Contains(t, text, anchorID)
}

// A gap that is a real neighbor, and that would rank on the same query, stays
// out of recall even when drafts are opted in. The full tree still lists it.
func TestGapGraph_StaysOutOfRecallEvenWhenDraftsAreIncluded(t *testing.T) {
	isolateXDG(t)
	vault, file := graphNeighborVault(t)
	requireGapEdge(t, vault)

	cases := []struct {
		name string
		args []string
	}{
		{"ask", []string{"ask", graphQuery, "--vault", vault, "--json", "--budget", "8000", "--max-items", "10"}},
		{"search", []string{"search", graphQuery, "--vault", vault, "--json", "--limit", "10"}},
		{"memory pack", []string{"memory", "pack", anchorID, "--vault", vault, "--json", "--budget", "8000"}},
		{"memory context-pack", []string{"memory", "context-pack", anchorID, "--vault", vault, "--json", "--budget", "8000"}},
		{"memory related", []string{"memory", "related", anchorID, "--vault", vault, "--json"}},
		{"memory neighbors", []string{"memory", "neighbors", anchorID, "--vault", vault, "--json", "--min-confidence", "low"}},
		{"links neighbors", []string{"links", "neighbors", anchorID, "--vault", vault, "--json", "--min-confidence", "low"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, extra := range [][]string{nil, {"--include-drafts"}} {
				out, errOut, err := runRootCmd(t, append(tc.args, extra...)...)
				require.NoError(t, err, errOut.String())
				assert.NotContains(t, out.String(), gapNeighbor)
				assert.NotContains(t, out.String(), gapBody)
				assert.Contains(t, out.String(), anchorID)
			}
		})
	}

	recited, errOut, err := runRootCmd(t, "arc", "recite", "--type", "gap", "--vault", vault, "--json", "--include-drafts")
	require.NoError(t, err, errOut.String())
	var env struct {
		Result struct {
			Total int `json:"total"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(recited.Bytes(), &env))
	assert.Equal(t, 0, env.Result.Total)
	assert.NotContains(t, recited.String(), gapNeighbor)
	assert.NotContains(t, recited.String(), gapBody)

	listed, errOut, err := runRootCmd(t, "tree", "--vault", vault)
	require.NoError(t, err, errOut.String())
	assert.Contains(t, listed.String(), gapNeighbor)
	assert.Contains(t, listed.String(), "[gap]")

	for _, extra := range [][]string{nil, {"--include-drafts"}} {
		covered, errOut, err := runRootCmd(t, append([]string{"tree", "--vault", vault, "--for", file}, extra...)...)
		require.NoError(t, err, errOut.String())
		assert.NotContains(t, covered.String(), gapCover)
		assert.NotContains(t, covered.String(), gapCoverSecret)
		assert.Contains(t, covered.String(), "concept-covers-file")
	}

	got, errOut, err := runRootCmd(t, "note", "get", gapNeighbor, "--vault", vault)
	require.NoError(t, err, errOut.String())
	assert.Contains(t, got.String(), gapNeighbor)
	assert.Contains(t, got.String(), gapBody)
	assert.Contains(t, got.String(), "[gap]")

	runner := func(_ context.Context, args []string) ([]byte, error) {
		out, errOut, err := runRootCmd(t, args...)
		if err != nil {
			return nil, fmt.Errorf("%w\n%s", err, errOut.String())
		}
		return out.Bytes(), nil
	}
	cs := connectDraftMCP(t, mcpserver.Config{Vault: vault, Run: runner})
	res := callDraftMCP(t, cs, "ask", map[string]any{"query": graphQuery})
	text := draftMCPText(res)
	assert.NotContains(t, text, gapNeighbor)
	assert.NotContains(t, text, gapBody)
	searchRes := callDraftMCP(t, cs, "search", map[string]any{"query": graphQuery})
	searchText := draftMCPText(searchRes)
	assert.NotContains(t, searchText, gapNeighbor)
	assert.NotContains(t, searchText, gapBody)
}

func requireGapEdge(t *testing.T, vault string) {
	t.Helper()
	db, err := index.Open(filepath.Join(vault, ".vaultmind", "index.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	var n int
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM links WHERE src_note_id = ? AND dst_note_id = ? AND edge_type = 'explicit_link'`,
		gapNeighbor, anchorID).Scan(&n))
	require.Equal(t, 1, n, "the gap must be a real wikilink neighbor, or absence proves nothing")
	var paths string
	require.NoError(t, db.QueryRow(
		`SELECT value_json FROM frontmatter_kv WHERE note_id = ? AND key = 'paths'`, gapCover).Scan(&paths))
	assert.Contains(t, paths, "internal/a.go")
}
