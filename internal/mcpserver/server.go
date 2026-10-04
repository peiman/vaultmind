// Package mcpserver serves vaults to MCP clients, such as Claude Desktop and
// other agents without a shell. Each tool runs the vaultmind command an
// agent with a shell would run, with --json, and returns its output, so a
// tool answers exactly what the CLI answers: same flags, config and access
// tracking.
package mcpserver

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Runner runs vaultmind with args and returns what it printed. A failed
// command returns an error carrying its output.
type Runner func(ctx context.Context, args []string) ([]byte, error)

// Config is what the server serves. The vaults are fixed when it starts, so
// a client can't point a tool at another directory.
type Config struct {
	// Vault is where writes, imports and links go.
	Vault string
	// Vaults, when set, is the set the read tools look across; Vault is
	// expected among them.
	Vaults []string
	// Version is reported to clients.
	Version string
	// Run runs a vaultmind command; ExecRunner in production.
	Run Runner
}

// ExecRunner runs the binary at bin.
func ExecRunner(bin string) Runner {
	return func(ctx context.Context, args []string) ([]byte, error) {
		var stdout, stderr bytes.Buffer
		// bin is os.Executable() (see cmd/mcp_helpers.go), and args go to it as
		// argv, never through a shell, so a tool's input can't run a command.
		// nosemgrep: go-dangerous-exec -- bin is this binary; args are argv, no shell
		cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // bin is this binary; args are argv, no shell
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			msg := strings.TrimSpace(stdout.String() + "\n" + stderr.String())
			return nil, fmt.Errorf("vaultmind %s: %w: %s", args[0], err, msg)
		}
		return stdout.Bytes(), nil
	}
}

// Serve runs the server on stdin and stdout until the client disconnects.
func Serve(ctx context.Context, cfg Config) error {
	return New(cfg).Run(ctx, &mcp.StdioTransport{})
}

// New builds the server and its tools.
func New(cfg Config) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "vaultmind", Version: cfg.Version}, &mcp.ServerOptions{
		Instructions: "VaultMind is this project's curated knowledge vault: decisions and why, how the system " +
			"works, gotchas someone paid for once. Before answering from memory, ask it. `ask` answers " +
			"'what do we know about X'; `note_get` reads a note by id; `tree` shows what the vault covers.",
	})
	t := tools{cfg: cfg}
	add(s, cfg.Run, "ask", "Ask the vault what it knows about a question: ranked notes plus their most relevant text. "+
		"Start here. Then read a note in full with note_get.", t.ask)
	add(s, cfg.Run, "search", "Search the vault's notes; returns ranked ids and titles without bodies. "+
		"mode: keyword (default), semantic or hybrid.", t.search)
	add(s, cfg.Run, "note_get", "Read one note in full by its id (or vault path), with its frontmatter.", t.noteGet)
	add(s, cfg.Run, "tree", "Show what the vault holds: folders with what they cover, and one line per note. "+
		"Use depth 1 for a large vault, then narrow with path or type.", t.tree)
	add(s, cfg.Run, "links", "List a note's links: what it links to (out), what links to it (in), or both.", t.links)
	add(s, cfg.Run, "note_create", "Write a new note at a vault path (e.g. decisions/cache.md) with a type, a body and "+
		"frontmatter fields such as title. Fails if the note exists.", t.noteCreate)
	add(s, cfg.Run, "import", "Import a folder, file or URL into the vault as notes (markdown, PDF, Office, HTML, CSV, "+
		"EPUB, archives, images). crawl follows a URL's site.", t.importSource)
	return s
}

// add registers a tool: build turns its input into a vaultmind command
// line, run runs it, and the output is the tool's text. A build or run
// error is a tool error, which the agent sees.
func add[In any](s *mcp.Server, run Runner, name, desc string, build func(In) ([]string, error)) {
	mcp.AddTool(s, &mcp.Tool{Name: name, Description: desc}, func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		args, err := build(in)
		if err != nil {
			return nil, nil, err
		}
		out, err := run(ctx, args)
		if err != nil {
			return nil, nil, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(out)}}}, nil, nil
	})
}

type tools struct{ cfg Config }

func (t tools) readScope() []string {
	if len(t.cfg.Vaults) > 1 {
		return []string{"--vaults", strings.Join(t.cfg.Vaults, ",")}
	}
	return t.writeScope()
}

func (t tools) writeScope() []string { return []string{"--vault", t.cfg.Vault} }

type askIn struct {
	Query    string `json:"query" jsonschema:"the question, in plain words"`
	MaxItems int    `json:"max_items,omitempty" jsonschema:"most notes to return text from (default 8)"`
	Type     string `json:"type,omitempty" jsonschema:"only notes of this type, e.g. decision or concept"`
	Tag      string `json:"tag,omitempty" jsonschema:"only notes with this tag"`
	Path     string `json:"path,omitempty" jsonschema:"only notes under this vault folder, e.g. decisions/"`
}

type searchIn struct {
	Query string `json:"query" jsonschema:"what to search for"`
	Mode  string `json:"mode,omitempty" jsonschema:"keyword (default), semantic or hybrid"`
	Limit int    `json:"limit,omitempty" jsonschema:"most results (default 20)"`
	Type  string `json:"type,omitempty" jsonschema:"only notes of this type"`
	Tag   string `json:"tag,omitempty" jsonschema:"only notes with this tag"`
}

type noteGetIn struct {
	ID string `json:"id" jsonschema:"the note's id, or its path in the vault"`
}

type treeIn struct {
	Path  string `json:"path,omitempty" jsonschema:"only notes under this folder, e.g. decisions/"`
	Type  string `json:"type,omitempty" jsonschema:"only notes of this type"`
	Depth int    `json:"depth,omitempty" jsonschema:"folder levels to list; deeper folders fold into counts (0 = all)"`
}

type linksIn struct {
	ID        string `json:"id" jsonschema:"the note's id or path"`
	Direction string `json:"direction,omitempty" jsonschema:"in, out or both (default)"`
}

type noteCreateIn struct {
	Path   string            `json:"path" jsonschema:"where the note goes in the vault, e.g. decisions/cache.md"`
	Type   string            `json:"type" jsonschema:"the note type, e.g. decision or concept"`
	Body   string            `json:"body,omitempty" jsonschema:"the note's markdown body"`
	Fields map[string]string `json:"fields,omitempty" jsonschema:"frontmatter fields, e.g. {\"title\": \"Cache\"}"`
}

type importIn struct {
	Source string `json:"source" jsonschema:"a folder, a file, or an http(s) URL"`
	Crawl  bool   `json:"crawl,omitempty" jsonschema:"import the site a URL leads to, one note per page"`
}

// cmdline is a command, its argument, --json and a scope.
func cmdline(scope []string, words ...string) []string {
	return append(append(words, "--json"), scope...)
}

// flag appends --name value when value is set.
func flag(args []string, name, value string) []string {
	if value == "" {
		return args
	}
	return append(args, "--"+name, value)
}

func intFlag(args []string, name string, value int) []string {
	if value == 0 {
		return args
	}
	return append(args, "--"+name, strconv.Itoa(value))
}

func (t tools) ask(in askIn) ([]string, error) {
	args := cmdline(t.readScope(), "ask", in.Query)
	args = intFlag(args, "max-items", in.MaxItems)
	return flag(flag(flag(args, "type", in.Type), "tag", in.Tag), "path", in.Path), nil
}

func (t tools) search(in searchIn) ([]string, error) {
	args := flag(cmdline(t.readScope(), "search", in.Query), "mode", in.Mode)
	args = intFlag(args, "limit", in.Limit)
	return flag(flag(args, "type", in.Type), "tag", in.Tag), nil
}

func (t tools) noteGet(in noteGetIn) ([]string, error) {
	return cmdline(t.readScope(), "note", "get", in.ID), nil
}

func (t tools) tree(in treeIn) ([]string, error) {
	args := flag(flag(cmdline(t.readScope(), "tree"), "path", in.Path), "type", in.Type)
	return intFlag(args, "depth", in.Depth), nil
}

func (t tools) links(in linksIn) ([]string, error) {
	args := cmdline(t.writeScope(), "memory", "links", in.ID)
	switch in.Direction {
	case "", "both":
		return args, nil
	case "in", "out":
		return append(args, "--"+in.Direction), nil
	}
	return nil, fmt.Errorf("direction %q: use in, out or both", in.Direction)
}

func (t tools) noteCreate(in noteCreateIn) ([]string, error) {
	args := flag(flag(cmdline(t.writeScope(), "note", "create", in.Path), "type", in.Type), "body", in.Body)
	keys := make([]string, 0, len(in.Fields))
	for k := range in.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "--field", k+"="+in.Fields[k])
	}
	return args, nil
}

func (t tools) importSource(in importIn) ([]string, error) {
	args := cmdline(t.writeScope(), "import", in.Source)
	if in.Crawl {
		args = append(args, "--crawl")
	}
	return args, nil
}
