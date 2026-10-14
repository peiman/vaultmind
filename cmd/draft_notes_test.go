package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/mcpserver"
	"github.com/peiman/vaultmind/internal/testvault"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	draftID      = "concept-quartz-draft"
	publishedID  = "concept-quartz-published"
	draftQuery   = "quartzdrafttoken"
	draftRelPath = "concepts/quartz-draft.md"
)

func draftNoteBody() string {
	return strings.Repeat(draftQuery+" ", 12)
}

func writeVaultNote(t *testing.T, vault, rel, id, title, status, body string) {
	t.Helper()
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "id: %s\ntype: concept\ntitle: %s\n", id, title)
	if status != "" {
		fmt.Fprintf(&b, "status: %s\n", status)
	}
	b.WriteString("created: 2026-04-03\n---\n\n")
	b.WriteString(body)
	path := filepath.Join(vault, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(b.String()), 0o644))
}

func rebuildVault(t *testing.T, dir string) {
	t.Helper()
	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	_, err = index.NewIndexer(dir, filepath.Join(dir, ".vaultmind", "index.db"), cfg).Rebuild()
	require.NoError(t, err)
}

// draftRankVault is a fixture copy plus a draft that outranks a published
// note on draftQuery. The draft is a concept: that type accepts any status.
func isolateXDG(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
}

func draftRankVault(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	require.NoError(t, os.CopyFS(dst, os.DirFS(testvault.FixtureVault())))
	writeVaultNote(t, dst, draftRelPath, draftID, "Quartz Draft", "draft", draftNoteBody())
	writeVaultNote(t, dst, "concepts/quartz-published.md", publishedID, "Quartz Published", "",
		"A published note mentions "+draftQuery+" once.")
	when := time.Now().Add(-74 * time.Hour)
	require.NoError(t, os.Chtimes(filepath.Join(dst, draftRelPath), when, when))
	rebuildVault(t, dst)
	return dst
}

func searchHitIDs(t *testing.T, raw []byte) []string {
	t.Helper()
	var env struct {
		Result struct {
			Hits []struct {
				ID string `json:"id"`
			} `json:"hits"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(raw, &env), string(raw))
	ids := make([]string, len(env.Result.Hits))
	for i, h := range env.Result.Hits {
		ids[i] = h.ID
	}
	return ids
}

func TestDraftNotes_StayOutOfRecallUntilAsked(t *testing.T) {
	isolateXDG(t)
	vault := draftRankVault(t)
	cases := []struct {
		name   string
		args   []string
		ids    func(*testing.T, []byte) []string
		first  string
		absent string
	}{
		{
			name:   "ask",
			args:   []string{"ask", draftQuery, "--vault", vault, "--json", "--search-limit", "1"},
			ids:    askHitIDs,
			first:  publishedID,
			absent: draftID,
		},
		{
			name:  "ask include-drafts",
			args:  []string{"ask", draftQuery, "--vault", vault, "--json", "--search-limit", "1", "--include-drafts"},
			ids:   askHitIDs,
			first: draftID,
		},
		{
			name:   "search",
			args:   []string{"search", draftQuery, "--vault", vault, "--json", "--limit", "1"},
			ids:    searchHitIDs,
			first:  publishedID,
			absent: draftID,
		},
		{
			name:  "search include-drafts",
			args:  []string{"search", draftQuery, "--vault", vault, "--json", "--limit", "1", "--include-drafts"},
			ids:   searchHitIDs,
			first: draftID,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, err := runRootCmd(t, tc.args...)
			require.NoError(t, err, errOut.String())
			ids := tc.ids(t, out.Bytes())
			require.NotEmpty(t, ids, out.String())
			assert.Equal(t, tc.first, ids[0], "limit 1 returns whoever ranks first after the draft policy")
			if tc.absent != "" {
				assert.NotContains(t, out.String(), tc.absent)
			}
		})
	}
}

func TestDraftNotes_ConfigAndEnvOptIn(t *testing.T) {
	isolateXDG(t)
	// --config sticks for the process. Clear it so the next test does not
	// validate a deleted temp file. Same cleanup as the query-cap precedence tests.
	t.Cleanup(func() {
		configFileUsed, configFileStatus = "", ""
		cfgFile = ""
	})
	vault := draftRankVault(t)
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(cfg, []byte("app:\n  retrieval:\n    include_drafts: true\n"), 0o644))
	ask := []string{"ask", draftQuery, "--vault", vault, "--json", "--search-limit", "1"}

	t.Run("config file beats the default", func(t *testing.T) {
		viper.Reset()
		out, errOut, err := runRootCmd(t, append(ask, "--config", cfg)...)
		require.NoError(t, err, errOut.String())
		assert.Equal(t, draftID, askHitIDs(t, out.Bytes())[0])
	})

	t.Run("env beats the config file", func(t *testing.T) {
		viper.Reset()
		t.Setenv("VAULTMIND_APP_RETRIEVAL_INCLUDE_DRAFTS", "false")
		out, errOut, err := runRootCmd(t, append(ask, "--config", cfg)...)
		require.NoError(t, err, errOut.String())
		assert.Equal(t, publishedID, askHitIDs(t, out.Bytes())[0])
		assert.NotContains(t, out.String(), draftID)
	})

	t.Run("flag beats env", func(t *testing.T) {
		viper.Reset()
		t.Setenv("VAULTMIND_APP_RETRIEVAL_INCLUDE_DRAFTS", "false")
		out, errOut, err := runRootCmd(t, append(ask, "--include-drafts")...)
		require.NoError(t, err, errOut.String())
		assert.Equal(t, draftID, askHitIDs(t, out.Bytes())[0])
	})
}

func TestDraftNotes_FederatedAskOmitsTheDraft(t *testing.T) {
	isolateXDG(t)
	a, b := draftRankVault(t), draftRankVault(t)
	out, errOut, err := runRootCmd(t, "ask", draftQuery, "--vaults", a+","+b, "--json")
	require.NoError(t, err, errOut.String())
	assert.NotContains(t, out.String(), draftID)
	assert.Contains(t, out.String(), publishedID)
}

func TestDraftNotes_MCPAskAndSearchOmitTheDraft(t *testing.T) {
	isolateXDG(t)
	vault := draftRankVault(t)
	runner := func(_ context.Context, args []string) ([]byte, error) {
		out, errOut, err := runRootCmd(t, args...)
		if err != nil {
			return nil, fmt.Errorf("%w\n%s", err, errOut.String())
		}
		return out.Bytes(), nil
	}
	cs := connectDraftMCP(t, mcpserver.Config{Vault: vault, Run: runner})
	for _, tool := range []string{"ask", "search"} {
		t.Run(tool, func(t *testing.T) {
			res := callDraftMCP(t, cs, tool, map[string]any{"query": draftQuery})
			text := draftMCPText(res)
			assert.NotContains(t, text, draftID)
			assert.Contains(t, text, publishedID)
		})
	}
}

func TestDraftNotes_NoteGetReadsTheDraftAndTreeMarksIt(t *testing.T) {
	isolateXDG(t)
	vault := draftRankVault(t)

	text, errOut, err := runRootCmd(t, "note", "get", draftID, "--vault", vault)
	require.NoError(t, err, errOut.String())
	assert.Contains(t, text.String(), draftID+" (concept) — Quartz Draft [draft]")

	raw, errOut, err := runRootCmd(t, "note", "get", draftID, "--vault", vault, "--json")
	require.NoError(t, err, errOut.String())
	var env struct {
		Result struct {
			Frontmatter map[string]any `json:"frontmatter"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(raw.Bytes(), &env), raw.String())
	assert.Equal(t, "draft", env.Result.Frontmatter["status"])

	tree, errOut, err := runRootCmd(t, "tree", "--vault", vault)
	require.NoError(t, err, errOut.String())
	var draftLine, publishedLine string
	for _, line := range strings.Split(tree.String(), "\n") {
		switch {
		case strings.Contains(line, draftID):
			draftLine = line
		case strings.Contains(line, publishedID):
			publishedLine = line
		}
	}
	require.NotEmpty(t, draftLine)
	assert.Contains(t, draftLine, "[draft]")
	require.NotEmpty(t, publishedLine)
	assert.NotContains(t, publishedLine, "[draft]")
}

func TestDoctor_ListsDraftsAndSkipsUncommittedOutsideGit(t *testing.T) {
	isolateXDG(t)
	vault := draftRankVault(t)
	out, errOut, err := runRootCmd(t, "doctor", "--vault", vault, "--json")
	require.NoError(t, err, errOut.String())

	var env DoctorEnvelope
	require.NoError(t, json.Unmarshal(out.Bytes(), &env), out.String())
	assert.Equal(t, "v1", env.SchemaVersion)
	assert.Equal(t, "doctor", env.Command)
	require.GreaterOrEqual(t, env.Result.Drafts.Count, 1)
	var found bool
	for _, n := range env.Result.Drafts.Notes {
		if n.ID != draftID {
			continue
		}
		found = true
		assert.Equal(t, draftRelPath, n.Path)
		assert.Equal(t, 3, n.AgeDays)
	}
	assert.True(t, found, "the draft list names id, path and age")
	assert.True(t, env.Result.UncommittedNotes.Skipped)
	assert.Contains(t, env.Result.UncommittedNotes.Reason, "not a git repository")
	assert.Equal(t, 7, env.Result.UncommittedNotes.ThresholdDays)

	text, errOut, err := runRootCmd(t, "doctor", "--vault", vault)
	require.NoError(t, err, errOut.String())
	assert.Contains(t, text.String(), "Drafts awaiting review:")
	assert.Contains(t, text.String(), draftID)
	assert.Contains(t, text.String(), draftRelPath)
	assert.Contains(t, text.String(), "Uncommitted notes: skipped — vault is not a git repository")
}

func TestDoctor_FlagsOldUntrackedMarkdown(t *testing.T) {
	isolateXDG(t)
	dir := t.TempDir()
	cfgPath := filepath.Join(testvault.FixtureVault(), ".vaultmind", "config.yaml")
	cfg, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"), cfg, 0o644))
	writeVaultNote(t, dir, "committed.md", "concept-committed", "Committed", "", "Committed body.")
	repo, err := gogit.PlainInit(dir, false)
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)
	_, err = wt.Add("committed.md")
	require.NoError(t, err)
	_, err = wt.Add(".vaultmind/config.yaml")
	require.NoError(t, err)
	_, err = wt.Commit("init", &gogit.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@example.com", When: time.Now()},
	})
	require.NoError(t, err)

	now := time.Now()
	writeVaultNote(t, dir, "old.md", "concept-old", "Old", "", "Old body.")
	writeVaultNote(t, dir, "fresh.md", "concept-fresh", "Fresh", "", "Fresh body.")
	require.NoError(t, os.Chtimes(filepath.Join(dir, "old.md"), now.Add(-8*24*time.Hour-time.Hour), now.Add(-8*24*time.Hour-time.Hour)))
	require.NoError(t, os.Chtimes(filepath.Join(dir, "fresh.md"), now.Add(-time.Hour), now.Add(-time.Hour)))
	require.NoError(t, os.Chtimes(filepath.Join(dir, "committed.md"), now.Add(-30*24*time.Hour), now.Add(-30*24*time.Hour)))
	rebuildVault(t, dir)

	out, errOut, err := runRootCmd(t, "doctor", "--vault", dir, "--json")
	require.NoError(t, err, errOut.String())
	var env DoctorEnvelope
	require.NoError(t, json.Unmarshal(out.Bytes(), &env), out.String())
	assert.False(t, env.Result.UncommittedNotes.Skipped, env.Result.UncommittedNotes.Reason)
	assert.Equal(t, 7, env.Result.UncommittedNotes.ThresholdDays)
	byPath := map[string]string{}
	for _, n := range env.Result.UncommittedNotes.Notes {
		byPath[n.Path] = n.State
		if n.Path == "old.md" {
			assert.GreaterOrEqual(t, n.AgeDays, 8)
		}
	}
	assert.Equal(t, "untracked", byPath["old.md"])
	assert.NotContains(t, byPath, "fresh.md")
	assert.NotContains(t, byPath, "committed.md")

	text, errOut, err := runRootCmd(t, "doctor", "--vault", dir)
	require.NoError(t, err, errOut.String())
	assert.Contains(t, text.String(), "Uncommitted notes older than 7 days:")
	assert.Contains(t, text.String(), "old.md")
	assert.Contains(t, text.String(), "untracked")
}

func connectDraftMCP(t *testing.T, cfg mcpserver.Config) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	serverT, clientT := mcp.NewInMemoryTransports()
	ss, err := mcpserver.New(cfg).Connect(ctx, serverT, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, clientT, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func callDraftMCP(t *testing.T, cs *mcp.ClientSession, tool string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
	require.NoError(t, err)
	require.False(t, res.IsError, draftMCPText(res))
	return res
}

func draftMCPText(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}
