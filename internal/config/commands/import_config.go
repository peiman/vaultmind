package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// ImportMetadata defines the `import` command — a folder of docs as notes.
var ImportMetadata = config.CommandMetadata{
	Use:   "import <dir>",
	Short: "Import a folder of markdown docs as notes, and keep them in step on re-runs",
	Long: "Copy every *.md under <dir> into the vault as a note under imported/<repo>/, so " +
		"a project's existing docs are found by search, ask and the code hooks like any " +
		"other note.\n\n" +
		"Each note records its doc in `paths:` — reading or editing the doc brings the " +
		"note — and in `source:` and `source_hash:`, which is how a re-run knows what " +
		"changed. The doc stays the source of truth: edit the doc, then re-run.\n\n" +
		"A re-run adds new docs, rewrites changed ones and leaves the rest alone. Nothing " +
		"is deleted without asking: a note whose doc is gone is reported as orphaned and " +
		"removed only with --prune. A note edited by hand whose doc also changed is " +
		"reported as a conflict and kept; --force overwrites it. Frontmatter you add to " +
		"an imported note (tags, links) survives a re-run.\n\n" +
		"Left out: hidden folders, dependency folders (node_modules, vendor), symlinks, " +
		"and anything the vault's exclude list would hide from the index. A README is " +
		"imported as readme.md, since vaults exclude README.md as their own meta file. " +
		"A repository is known by its name (its origin remote's, else its folder's; " +
		"outside git, the folder's), so two repositories or folders of the same name " +
		"share imported/<name>/. Names that differ only in case are one note.\n\n" +
		"  vaultmind import docs --vault ./knowledge --dry-run\n" +
		"  vaultmind import docs --vault ./knowledge\n" +
		"  vaultmind import docs --vault ./knowledge --prune --json",
	ConfigPrefix: "app.import",
	FlagOverrides: map[string]string{
		"app.import.vault":   "vault",
		"app.import.dry_run": "dry-run",
		"app.import.prune":   "prune",
		"app.import.force":   "force",
		"app.import.json":    "json",
	},
}

// ImportOptions returns configuration options for `import`.
func ImportOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.import.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.import.dry_run", DefaultValue: false, Description: "Report what the import would do and write nothing", Type: "bool"},
		{Key: "app.import.prune", DefaultValue: false, Description: "Remove imported notes whose doc is gone", Type: "bool"},
		{Key: "app.import.force", DefaultValue: false, Description: "Overwrite imported notes edited by hand when their doc changed", Type: "bool"},
		{Key: "app.import.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
	}
}

func init() {
	config.RegisterOptionsProvider(ImportOptions)
}
