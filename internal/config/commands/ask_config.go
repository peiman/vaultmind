package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// AskMetadata defines metadata for the ask command.
var AskMetadata = config.CommandMetadata{
	Use:   "ask <query>",
	Short: "Compound search + context-pack: answer 'what do I know about X?'",
	Long: `Search the vault, pick the top hit, and pack token-budgeted context around it.
One command replaces the manual search → recall → summarize chain.

CHOOSE A RENDERING MODE BY INTENT

  vaultmind ask "spreading activation" --pointers-only
      Menu of relevant ids + titles. Cheapest. Use when you want to see what's
      relevant without reading bodies — then 'note get <id>' the one you want.

  vaultmind ask "spreading activation" --preview
      Menu + one-line snippet under each hit. Use when titles aren't enough
      to know what each note is about. Bridges --pointers-only and default.

  vaultmind ask "spreading activation"
      Default. Full token-budgeted context-pack around the top hit. Use when
      you want bodies in working context (not just to identify the right id).

  vaultmind ask "spreading activation" --explain
      Each hit shows per-lane RRF math (dense / sparse / colbert / fts).
      For investigating ranking decisions, not for answering content questions.

ANTI-PATTERN — AVOID

  vaultmind ask "X" --budget 3000 | tail -20
      Don't double-clip. The budget asks the system to compute 3000 tokens of
      context-pack; the tail then throws most of it away. Pick one shape that
      fits the intent:
        --pointers-only           if you want a menu, no bodies
        --preview                 if you want a menu with body snippets
        --budget=N (no tail)      if you really want N tokens of context

OUTPUT INCLUDES

  Search:    [top-hit confidence: strong|moderate|weak]
  Per-hit:   score, id, title, optional snippet (--preview), optional lanes (--explain)
  Context:   target frontmatter + body, then ranked context items (default mode only)`,
	ConfigPrefix: "app.ask",
	FlagOverrides: map[string]string{
		"app.ask.vault":            "vault",
		"app.ask.json":             "json",
		"app.ask.budget":           "budget",
		"app.ask.max_items":        "max-items",
		"app.ask.search_limit":     "search-limit",
		"app.ask.explain":          "explain",
		"app.ask.pointers_only":    "pointers-only",
		"app.ask.preview":          "preview",
		"app.ask.read":             "read",
		"app.ask.quiet_on_nomatch": "quiet-on-no-match",
	},
}

// AskOptions returns config options for the ask command.
func AskOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.ask.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.ask.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
		{Key: "app.ask.budget", DefaultValue: 4000, Description: "Token budget for context-pack", Type: "int"},
		{Key: "app.ask.max_items", DefaultValue: 8, Description: "Max context items", Type: "int"},
		{Key: "app.ask.search_limit", DefaultValue: 5, Description: "Max search hits", Type: "int"},
		{Key: "app.ask.explain", DefaultValue: false, Description: "Show per-lane RRF contributions for each hit", Type: "bool"},
		{Key: "app.ask.pointers_only", DefaultValue: false, Description: "Skip context-pack bodies; render only id+title+type pointers (forces ask-to-read loop instead of letting the preload satisfy curiosity)", Type: "bool"},
		{Key: "app.ask.preview", DefaultValue: false, Description: "Render a one-line body snippet under each ranked hit; bridges --pointers-only (titles only) and the full context-pack output", Type: "bool"},
		{Key: "app.ask.read", DefaultValue: "", Description: "Read the body of the named hit inline after the menu — accepts a 1-indexed rank (e.g. --read 2) or an exact id (e.g. --read concept-foo). Single-command shortcut for probe→read when you already know which hit from the titles", Type: "string"},
		{Key: "app.ask.quiet_on_nomatch", DefaultValue: false, Description: "Print nothing when the top hit is at/below the noise floor (no_match). For ambient recall: inject silence instead of irrelevant pointers when the prompt is off-domain. Also skips the context-pack and access fan-out so off-domain prompts don't reinforce irrelevant notes.", Type: "bool"},
	}
}

func init() {
	config.RegisterOptionsProvider(AskOptions)
}
