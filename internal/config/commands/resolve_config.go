package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// ResolveMetadata defines the metadata for the resolve command.
var ResolveMetadata = config.CommandMetadata{
	Use:   "resolve <id-or-title-or-alias>",
	Short: "Resolve ambiguous note references to canonical IDs",
	Long: `Turn a fragment — a partial ID, a title, an alias, or a file path — into
one or more matched notes with a resolution tier that explains how the match
was found.

Use resolve when you have a string that might refer to a note but you are not
certain of its exact ID, or when you want to check whether a reference is
ambiguous before acting on it.

RESOLUTION TIERS

  Resolve walks the input through five tiers, returning at the first hit:

    path      Input contains "/" or ends in ".md"; matched against stored paths.
    id        Exact match against the note's canonical ID.
    title     Exact match against the note's title field.
    alias     Exact match against any alias stored for the note.
    normalized  Case-insensitive match on title or alias, including
                treating hyphens and underscores as spaces.

  If no tier produces a hit, resolve reports "No match".
  If a tier produces multiple hits, the result is marked ambiguous and all
  candidates are returned — resolve does not silently pick one.

FLAGS

  --vault: path to the vault root directory (string, default ".")
  --json:  output the full resolution envelope as JSON instead of plain text
           (bool, default false)

OUTPUT INCLUDES

  Plain text (one line per match):
    <id>  <type>  <title>  (<path>)

  JSON envelope:
    resolved          whether any match was found
    ambiguous         true when multiple notes matched at the same tier
    input             the original query string
    resolution_tier   which tier produced the match ("id", "title", etc.)
    matches[]         array of { id, type, title, path, status }

WHEN TO USE

  resolve   You have a fragment or alias and need the canonical ID.
            Handles ambiguity explicitly — tells you when a string is not unique.
  ask       You have a semantic question and want ranked context around the
            best-matching note; works across whole vault topics, not single IDs.
  search    You want a ranked list of notes matching a free-text query without
            needing exact disambiguation.

EXAMPLES

  vaultmind resolve concept-act-r --vault ./my-vault         # exact ID lookup
  vaultmind resolve "ACT-R" --vault ./my-vault               # case-insensitive title match
  vaultmind resolve act-r --vault ./my-vault                 # normalized (hyphen → space)
  vaultmind resolve notes/decision-foo.md --vault .          # path shortcut
  vaultmind resolve spreading-activation --json --vault .    # machine-readable output`,
	ConfigPrefix: "app.resolve",
	FlagOverrides: map[string]string{
		"app.resolve.vault": "vault",
		"app.resolve.json":  "json",
	},
}

// ResolveOptions returns configuration options for the resolve command.
func ResolveOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{
			Key:          "app.resolve.vault",
			DefaultValue: ".",
			Description:  "Path to the vault root directory",
			Type:         "string",
		},
		{
			Key:          "app.resolve.json",
			DefaultValue: false,
			Description:  "Output in JSON format",
			Type:         "bool",
		},
	}
}

func init() {
	config.RegisterOptionsProvider(ResolveOptions)
}
