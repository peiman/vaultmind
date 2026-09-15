package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// ArcReciteMetadata defines `arc recite` — load the WHOLE arc layer, as bodies.
//
// The counterpart to `ask`, not a variant of it. `ask` ranks; this enumerates.
// The identity vault's promise is that an agent reconstructs itself from its
// arcs at session start, and the shipped hook implemented that with
// `ask "who am I"`. Measured on a real 27-arc vault: three arcs arrived, the
// same three on every run, selected by similarity to a literal string that has
// nothing to do with the session. 89% of the layer was not occasionally
// missing — it was permanently dark (issue #47).
var ArcReciteMetadata = config.CommandMetadata{
	Use:   "recite",
	Short: "Load every arc as a body — the whole identity layer, unranked and ungated",
	Long: "Emit EVERY arc in the vault as a token-bounded pack. Not top-K, not " +
		"relevance-gated, not a search.\n\n" +
		"WHY THIS IS NOT `ask`\n\n" +
		"  `ask` is retrieval: it ranks notes against a query and returns the best few. That is " +
		"the right shape for \"what do I know about X?\" and the wrong shape for \"who am I?\". " +
		"An identity layer is not a ranking problem — you do not want the arcs most similar to a " +
		"query, you want all of them.\n\n" +
		"  Measured on a 27-arc identity vault, `ask \"who am I\"` delivered 3 arcs, the SAME 3 on " +
		"every run: selection was semantic similarity to that literal string, which is " +
		"uncorrelated with whatever the session is about. The other 24 were not occasionally " +
		"missed, they were permanently absent.\n\n" +
		"ORDER AND TRUNCATION\n\n" +
		"  Arcs come back sorted by id, so two sessions get the same layer in the same order and " +
		"runs are comparable.\n\n" +
		"  If --budget cannot fit them all, the ones left out are REPORTED by count and by id. A " +
		"bulk loader that silently drops its tail is the same defect this command exists to " +
		"fix — you could not tell \"here are your 27 arcs\" from \"here are 9 of them\".\n\n" +
		"  --excerpt caps each arc's contribution, preferring its Principle section (the rule) " +
		"over its opening lines (the story setup). Excerpting reduces DEPTH per arc, never " +
		"COVERAGE of the layer.\n\n" +
		"EXAMPLES\n\n" +
		"  vaultmind arc recite --vault ~/.vaultmind/persona\n" +
		"      Every arc, whole bodies. Use when you want the full layer and have the room.\n\n" +
		"  vaultmind arc recite --budget 3000 --excerpt 120\n" +
		"      Session-start shape: every arc's rule, bounded. 27 arcs fit comfortably.\n\n" +
		"  vaultmind arc recite --type principle\n" +
		"      The same treatment for another always-load layer.",
	ConfigPrefix: "app.arc.recite",
	FlagOverrides: map[string]string{
		"app.arc.recite.vault":   "vault",
		"app.arc.recite.json":    "json",
		"app.arc.recite.budget":  "budget",
		"app.arc.recite.excerpt": "excerpt",
		"app.arc.recite.type":    "type",
	},
}

// ArcReciteOptions returns configuration options for `arc recite`.
func ArcReciteOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.arc.recite.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.arc.recite.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
		{Key: "app.arc.recite.budget", DefaultValue: 0, Description: "Total token ceiling for the pack. 0 = unbounded. Arcs that do not fit are reported by id, never dropped silently", Type: "int"},
		{Key: "app.arc.recite.excerpt", DefaultValue: 0, Description: "Cap each arc at N tokens, preferring its Principle section (the rule it carries) over its opening lines (story setup). 0 = whole bodies", Type: "int"},
		{Key: "app.arc.recite.type", DefaultValue: "arc", Description: "Note type to recite. Defaults to arc; set to another always-load layer (e.g. principle) to give it the same unranked treatment", Type: "string"},
	}
}

func init() {
	config.RegisterOptionsProvider(ArcReciteOptions)
}
