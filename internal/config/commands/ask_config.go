package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// AskMetadata defines metadata for the ask command.
var AskMetadata = config.CommandMetadata{
	Use:   "ask <query>",
	Short: "Compound search + context-pack: answer 'what do I know about X?'",
	Long: `Search the vault, pick the top hit, and pack token-budgeted context around it.
One command replaces the manual search → recall → summarize chain.

With --excerpt, the target keeps its first prose block unless a passage matching
the question scores at least twice as highly; a positive match wins when the first
block scores zero. Principle sections and context-note excerpts keep their existing
behavior, within the same token cap.

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

SEARCH SEVERAL VAULTS AT ONCE

  vaultmind ask "X" --vaults ~/identity,~/desk,~/research
      One query, every vault, one ranked answer. Each vault judges relevance
      against ITS OWN noise floor, so a 12-note desk and a 400-note research
      vault are compared fairly; results merge by rank (cross-vault RRF), and
      every hit is tagged with the vault it came from.

      Use when your memory is not one vault — an identity vault plus a working
      desk plus a reference corpus is the normal shape, and the note you need
      is often not in the one you happened to point at.

      Note: --vault takes ONE path. Repeating it (--vault A --vault B) is an
      error, because it used to silently keep B and answer from it alone.

SCOPE THE SEARCH

  vaultmind ask "X" --type decision
  vaultmind ask "X" --tag engineering
  vaultmind ask "X" --path decisions/
      Rank only notes of that type, carrying that tag, or under that vault
      folder (a path prefix, the same match as tree --path). Applies to every
      vault under --vaults. The context pack around the top hit still follows
      its links, so a neighbour of another type or folder can appear there.

DRAFTS

  A note with status: draft is excluded from ranking and from the context
  pack around a hit. --include-drafts puts it back. Naming the draft with
  note get still reads it. Accepting a draft is editing that status (one
  file, no copy).

ANTI-PATTERN — AVOID

  vaultmind ask "X" --budget 3000 | tail -20
      Don't double-clip. The budget asks the system to compute 3000 tokens of
      context-pack; the tail then throws most of it away. Pick one shape that
      fits the intent:
        --pointers-only           if you want a menu, no bodies
        --preview                 if you want a menu with body snippets
        --budget=N (no tail)      if you really want N tokens of context

LONG QUERIES

  One forward pass embeds the first N tokens (--query-forward-tokens, default
  1152). The cut is a token boundary, so the embedded text is a prefix of the
  query. Dense and sparse rank on that text, so a prompt's later content stays
  in the ranking. ColBERT and FTS use the K most salient terms of that pass
  (--query-salient-terms, default 64). A long word with no spaces contributes
  those tokens' own text, not the whole word. 0 turns each bound off. A query
  already within both bounds is ranked unchanged. The echoed query stays the
  full text, and the Search header reports when a bound applied.

OUTPUT INCLUDES

  Search:    [top-hit confidence: strong|moderate|weak]
  Per-hit:   score, id, title, optional snippet (--preview), optional lanes (--explain)
  Context:   target frontmatter + body, then ranked context items (default mode only)`,
	ConfigPrefix: "app.ask",
	FlagOverrides: map[string]string{
		"app.ask.vault":                "vault",
		"app.ask.vaults":               "vaults",
		"app.ask.json":                 "json",
		"app.ask.budget":               "budget",
		"app.ask.max_items":            "max-items",
		"app.ask.search_limit":         "search-limit",
		"app.ask.explain":              "explain",
		"app.ask.pointers_only":        "pointers-only",
		"app.ask.preview":              "preview",
		"app.ask.read":                 "read",
		"app.ask.quiet_on_nomatch":     "quiet-on-no-match",
		"app.ask.excerpt":              "excerpt",
		"app.ask.type":                 "type",
		"app.ask.tag":                  "tag",
		"app.ask.path":                 "path",
		"app.ask.dedup_window":         "dedup-window",
		"app.ask.query_forward_tokens": "query-forward-tokens",
		"app.ask.query_salient_terms":  "query-salient-terms",
	},
}

// AskOptions returns config options for the ask command.
func AskOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.ask.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.ask.vaults", DefaultValue: "", Description: "Comma-separated vault paths to search TOGETHER (federated). Merged by cross-vault RRF — ranks, not scores, because each vault calibrates its own noise floor. Every hit is tagged with the vault it came from.", Type: "string"},
		{Key: "app.ask.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
		{Key: "app.ask.budget", DefaultValue: 4000, Description: "Token budget for context-pack", Type: "int"},
		{Key: "app.ask.max_items", DefaultValue: 8, Description: "Max context items", Type: "int"},
		{Key: "app.ask.search_limit", DefaultValue: 5, Description: "Max search hits", Type: "int"},
		{Key: "app.ask.explain", DefaultValue: false, Description: "Show per-lane RRF contributions for each hit", Type: "bool"},
		{Key: "app.ask.pointers_only", DefaultValue: false, Description: "Skip context-pack bodies; render only id+title+type pointers (forces ask-to-read loop instead of letting the preload satisfy curiosity)", Type: "bool"},
		{Key: "app.ask.preview", DefaultValue: false, Description: "Render a one-line body snippet under each ranked hit; bridges --pointers-only (titles only) and the full context-pack output", Type: "bool"},
		{Key: "app.ask.read", DefaultValue: "", Description: "Read the body of the named hit inline after the menu — accepts a 1-indexed rank (e.g. --read 2) or an exact id (e.g. --read concept-foo). Single-command shortcut for probe→read when you already know which hit from the titles", Type: "string"},
		{Key: "app.ask.quiet_on_nomatch", DefaultValue: false, Description: "Print nothing when the top hit is at/below the noise floor (no_match). For ambient recall: inject silence instead of irrelevant pointers when the prompt is off-domain. Also skips the context-pack and access fan-out so off-domain prompts don't reinforce irrelevant notes.", Type: "bool"},
		{Key: "app.ask.excerpt", DefaultValue: 0, Description: "Cap each note's contribution at N tokens, preferring its decision-bearing passage — the Principle section where a note has one (an arc's rule lives there; its opening is story setup), else the opening prose. Applies to the target note and every context item. 0 = off, which also means an over-budget note contributes no text at all while the pack still counts it, so a tight budget yields items with no content", Type: "int"},
		{Key: "app.ask.type", DefaultValue: "", Description: "Rank only notes of this type (e.g. decision, concept)", Type: "string"},
		{Key: "app.ask.tag", DefaultValue: "", Description: "Rank only notes carrying this tag", Type: "string"},
		{Key: "app.ask.path", DefaultValue: "", Description: "Rank only notes under this vault folder (path prefix, e.g. decisions/) — the same match as tree --path", Type: "string"},
		{Key: "app.ask.dedup_window", DefaultValue: "", Description: "Send a note as its title alone when this conversation (VAULTMIND_USER_SESSION_ID) received its text within this long, e.g. 10m. Empty = off. For hooks that fire in bursts.", Type: "string"},
		// Default 1152 is the measured BGE-M3 cost of a 512-word budget, not a
		// tuned constant. Content tokens are non-special tokens with a non-empty
		// span. The 10 long qlen-probe prompts (L01–L10) tokenize at these
		// content tokens per whitespace word:
		//   L01 2.041190  L02 2.800693  L03 2.003333  L04 2.237660
		//   L05 2.783985  L06 3.443015  L07 2.125341  L08 2.243849
		//   L09 2.281161  L10 2.565153
		// Median of the 10 is the mean of L08 and L09:
		//   (2.243849 + 2.281161) / 2 = 2.262505
		//   2.262505 × 512 = 1158.402534
		//   1158.402534 / 64 = 18.100, which rounds to 18
		//   18 × 64 = 1152
		{Key: "app.ask.query_forward_tokens", DefaultValue: 1152, Description: "Embed the first N tokens of the query in one forward pass. Dense and sparse rank on that text. The cut is on a token boundary, so the embedded text is a prefix of the query. 0 turns the bound off.", Type: "int"},
		{Key: "app.ask.query_salient_terms", DefaultValue: 64, Description: "ColBERT and FTS use the K tokens with the highest sparse weight from that forward pass. A spaced word stays one FTS term. A long word with no spaces contributes the text of its salient tokens, not the whole word. 0 turns the bound off.", Type: "int"},
		{
			Key:          "app.ask.gap_store_query",
			DefaultValue: false,
			Description:  "Store the raw query text in the possible-gap hint log. Off by default: the log keeps a salted hash and a length. The log is per-user state, outside the vault. Config file or environment only, not a flag.",
			Type:         "bool",
		},
		{
			Key:          "app.ask.gap_log_limit",
			DefaultValue: 1000,
			Description:  "Most records kept in the possible-gap hint log. The file may grow to twice this many, then it is rewritten to the newest this many. Config file or environment only, not a flag.",
			Type:         "int",
		},
	}
}

func init() {
	config.RegisterOptionsProvider(AskOptions)
}
