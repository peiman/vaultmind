package mcpserver_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/peiman/vaultmind/internal/mcpserver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recorder is a Runner that keeps the command each tool would run and
// answers with out, or fails with err.
type recorder struct {
	args [][]string
	out  string
	err  error
}

func (r *recorder) run(_ context.Context, args []string) ([]byte, error) {
	r.args = append(r.args, args)
	return []byte(r.out), r.err
}

// connect serves cfg to an in-process MCP client.
func connect(t *testing.T, cfg mcpserver.Config) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	serverT, clientT := mcp.NewInMemoryTransports()
	server := mcpserver.New(cfg)
	ss, err := server.Connect(ctx, serverT, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, clientT, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func call(t *testing.T, cs *mcp.ClientSession, tool string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
	require.NoError(t, err)
	return res
}

func text(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

// Every tool runs the vaultmind command an agent with a shell would, with
// --json, on the server's vault, and returns its output as it is.
func TestTools_RunTheCommandAShellAgentWould(t *testing.T) {
	cases := []struct {
		tool string
		args map[string]any
		want []string
	}{
		{"ask", map[string]any{"query": "why sqlite"},
			[]string{"ask", "--json", "--vault=/v", "--", "why sqlite"}},
		{"ask", map[string]any{"query": "q", "max_items": 3, "type": "decision", "tag": "db", "path": "decisions/"},
			[]string{"ask", "--json", "--vault=/v", "--max-items=3", "--type=decision", "--tag=db", "--path=decisions/", "--", "q"}},
		{"search", map[string]any{"query": "q", "mode": "hybrid", "limit": 5, "type": "concept", "tag": "t"},
			[]string{"search", "--json", "--vault=/v", "--mode=hybrid", "--limit=5", "--type=concept", "--tag=t", "--", "q"}},
		{"note_get", map[string]any{"id": "decision-sqlite"},
			[]string{"note", "get", "--json", "--vault=/v", "--", "decision-sqlite"}},
		{"tree", map[string]any{},
			[]string{"tree", "--json", "--vault=/v"}},
		{"tree", map[string]any{"path": "decisions/", "type": "decision", "depth": 1},
			[]string{"tree", "--json", "--vault=/v", "--path=decisions/", "--type=decision", "--depth=1"}},
		{"links", map[string]any{"id": "decision-sqlite", "direction": "in"},
			[]string{"memory", "links", "--json", "--vault=/v", "--in", "--", "decision-sqlite"}},
		{"note_create", map[string]any{"path": "decisions/cache.md", "type": "decision", "body": "Why.", "fields": map[string]any{"title": "Cache", "status": "draft"}},
			[]string{"note", "create", "--json", "--vault=/v", "--type=decision", "--body=Why.", "--field=status=draft", "--field=title=Cache", "--", "decisions/cache.md"}},
		{"import", map[string]any{"source": "https://example.com/docs", "crawl": true},
			[]string{"import", "--json", "--vault=/v", "--public-only", "--crawl", "--", "https://example.com/docs"}},
	}
	for _, c := range cases {
		t.Run(c.tool, func(t *testing.T) {
			r := &recorder{out: `{"status":"ok"}`}
			cs := connect(t, mcpserver.Config{Vault: "/v", Run: r.run})
			res := call(t, cs, c.tool, c.args)
			assert.False(t, res.IsError, text(res))
			assert.Equal(t, `{"status":"ok"}`, text(res))
			require.Len(t, r.args, 1)
			assert.Equal(t, c.want, r.args[0])
		})
	}
}

// With several vaults, the read tools look across all of them; writes and
// links go to the first, the server's own vault.
func TestTools_ReadAcrossVaultsWriteToTheFirst(t *testing.T) {
	r := &recorder{out: "{}"}
	cs := connect(t, mcpserver.Config{Vault: "/a", Vaults: []string{"/a", "/b"}, Run: r.run})
	call(t, cs, "ask", map[string]any{"query": "q"})
	call(t, cs, "search", map[string]any{"query": "q"})
	call(t, cs, "note_get", map[string]any{"id": "x"})
	call(t, cs, "tree", map[string]any{})
	call(t, cs, "links", map[string]any{"id": "x"})
	call(t, cs, "note_create", map[string]any{"path": "n.md", "type": "concept"})
	call(t, cs, "import", map[string]any{"source": "https://example.com/"})
	for i, scope := range []string{"--vaults=/a,/b", "--vaults=/a,/b", "--vaults=/a,/b", "--vaults=/a,/b", "--vault=/a", "--vault=/a", "--vault=/a"} {
		assert.Contains(t, r.args[i], scope)
	}
}

// A command that fails is a tool error carrying its message, so the agent
// sees why; the server keeps serving.
func TestTools_AFailedCommandIsAToolError(t *testing.T) {
	r := &recorder{err: errors.New("note \"nope\" not found")}
	cs := connect(t, mcpserver.Config{Vault: "/v", Run: r.run})
	res := call(t, cs, "note_get", map[string]any{"id": "nope"})
	assert.True(t, res.IsError)
	assert.Contains(t, text(res), `note "nope" not found`)

	r.err = nil
	r.out = "{}"
	assert.False(t, call(t, cs, "tree", map[string]any{}).IsError)
}

// A required argument left out never reaches the command.
func TestTools_RejectAMissingQuery(t *testing.T) {
	r := &recorder{out: "{}"}
	cs := connect(t, mcpserver.Config{Vault: "/v", Run: r.run})
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "ask", Arguments: map[string]any{}})
	if err == nil {
		assert.True(t, res.IsError)
	}
	assert.Empty(t, r.args)
}

// The tools an agent is offered, each described.
func TestTools_AreListedWithDescriptions(t *testing.T) {
	cs := connect(t, mcpserver.Config{Vault: "/v", Run: (&recorder{}).run})
	res, err := cs.ListTools(context.Background(), nil)
	require.NoError(t, err)
	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
		assert.NotEmpty(t, tool.Description, tool.Name)
	}
	assert.ElementsMatch(t, []string{"ask", "search", "note_get", "tree", "links", "note_create", "import"}, names)
}

// ExecRunner runs the binary and, when it fails, reports what it printed on
// both streams.
func TestExecRunner_ReportsOutputOfAFailedCommand(t *testing.T) {
	run := mcpserver.ExecRunner("/bin/sh")
	out, err := run(context.Background(), []string{"-c", "echo fine"})
	require.NoError(t, err)
	assert.Equal(t, "fine\n", string(out))

	_, err = run(context.Background(), []string{"-c", "echo '{\"status\":\"error\"}'; echo 'vault not found' >&2; exit 3"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vault not found")
	assert.Contains(t, err.Error(), `{"status":"error"}`)
}

// A links direction other than in, out or both is refused with the choices.
func TestLinks_RefusesAnUnknownDirection(t *testing.T) {
	r := &recorder{out: "{}"}
	cs := connect(t, mcpserver.Config{Vault: "/v", Run: r.run})
	res := call(t, cs, "links", map[string]any{"id": "x", "direction": "sideways"})
	assert.True(t, res.IsError)
	assert.Contains(t, text(res), "use in, out or both")
	assert.Empty(t, r.args)
}

// An argument that looks like a flag stays an argument: a client a page
// prompted can't widen the vaults, prune or overwrite through a value.
func TestTools_AValueThatLooksLikeAFlagStaysAValue(t *testing.T) {
	r := &recorder{out: "{}"}
	cs := connect(t, mcpserver.Config{Vault: "/v", Run: r.run})
	call(t, cs, "ask", map[string]any{"query": "--vaults=/elsewhere", "type": "--config=/tmp/x"})
	call(t, cs, "note_get", map[string]any{"id": "--frontmatter-only"})
	call(t, cs, "import", map[string]any{"source": "https://example.com/--prune"})
	assert.Equal(t, []string{"ask", "--json", "--vault=/v", "--type=--config=/tmp/x", "--", "--vaults=/elsewhere"}, r.args[0])
	assert.Equal(t, []string{"note", "get", "--json", "--vault=/v", "--", "--frontmatter-only"}, r.args[1])
	for _, args := range r.args {
		sep := -1
		for i, a := range args {
			if a == "--" {
				sep = i
			}
		}
		require.GreaterOrEqual(t, sep, 0, args)
		assert.Equal(t, len(args)-2, sep, "the one positional comes last, after --: %v", args)
	}
}

// import takes only http(s) URLs: a local path would give an agent without
// a shell the whole filesystem to read into the vault.
func TestImport_RefusesALocalPath(t *testing.T) {
	r := &recorder{out: "{}"}
	cs := connect(t, mcpserver.Config{Vault: "/v", Run: r.run})
	for _, src := range []string{"/etc", "~/.ssh", "docs", "file:///etc/passwd", "--prune"} {
		res := call(t, cs, "import", map[string]any{"source": src})
		assert.True(t, res.IsError, src)
		assert.Contains(t, text(res), "http(s) URL", src)
	}
	assert.Empty(t, r.args)
}
