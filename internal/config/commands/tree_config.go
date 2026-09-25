package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// TreeMetadata defines the `tree` command — the map of a vault.
var TreeMetadata = config.CommandMetadata{
	Use:   "tree",
	Short: "Show what a vault holds — folders, counts, one line per note",
	Long: "Print the map of a vault: every folder with how many notes it holds, and every " +
		"note as `Title (id) — what it is about`. The line is the first sentence of the " +
		"note's Principle section, or of its opening prose. Open a note with " +
		"`vaultmind note get <id>`.\n\n" +
		"Search needs a question already formed; the map shows what is there to ask about. " +
		"Start a large vault with --depth 1 (folders and counts), then narrow with --path " +
		"or --type.\n\n" +
		"A note names the code it is about with `paths:` in its frontmatter — globs relative " +
		"to the repository root (`internal/hooks/**`, `cmd/*.go`), with `repo:` in front for " +
		"code in another repository. --for <file> lists the notes that cover that file.\n\n" +
		"  vaultmind tree --vault ./vaultmind-vault --depth 1\n" +
		"  vaultmind tree --vault ./vaultmind-vault --path decisions/\n" +
		"  vaultmind tree --vault ./vaultmind-vault --type concept --brief\n" +
		"  vaultmind tree --vaults ./vaultmind-vault,./docs-vault --depth 1\n" +
		"  vaultmind tree --vault ./vaultmind-vault --for internal/hooks/merge.go\n" +
		"  vaultmind tree --vault ./vaultmind-vault --json",
	ConfigPrefix: "app.tree",
	FlagOverrides: map[string]string{
		"app.tree.vault":  "vault",
		"app.tree.vaults": "vaults",
		"app.tree.json":   "json",
		"app.tree.path":   "path",
		"app.tree.type":   "type",
		"app.tree.depth":  "depth",
		"app.tree.brief":  "brief",
		"app.tree.for":    "for",
	},
}

// TreeOptions returns configuration options for `tree`.
func TreeOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.tree.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.tree.vaults", DefaultValue: "", Description: "Map several vaults, comma-separated (overrides --vault)", Type: "string"},
		{Key: "app.tree.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
		{Key: "app.tree.path", DefaultValue: "", Description: "Only notes under this folder (path prefix, e.g. decisions/)", Type: "string"},
		{Key: "app.tree.type", DefaultValue: "", Description: "Only notes of this type", Type: "string"},
		{Key: "app.tree.depth", DefaultValue: 0, Description: "Folder levels to list; deeper folders fold into counts (0 = all)", Type: "int"},
		{Key: "app.tree.brief", DefaultValue: false, Description: "Titles and ids only, without the one-line descriptions", Type: "bool"},
		{Key: "app.tree.for", DefaultValue: "", Description: "Only the notes whose paths: frontmatter covers this code file", Type: "string"},
	}
}

func init() {
	config.RegisterOptionsProvider(TreeOptions)
}
