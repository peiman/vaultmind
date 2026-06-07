package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// DoctorHealMetadata defines the metadata for the `doctor heal` command.
// heal APPLIES every auto-fixable repair doctor found (today that set is the
// wikilink rewriter); --dry-run previews instead. The `fix` alias is wired in
// cmd/ so `doctor fix` resolves here too.
var DoctorHealMetadata = config.CommandMetadata{
	Use:   "heal",
	Short: "heal (fix): apply all auto-fixable repairs doctor found",
	Long: `heal (fix): apply every auto-fixable repair that doctor's diagnosis can resolve.

heal is the repair half of the doctor health hub. Today the auto-fixable set is
exactly the Obsidian-incompatible wikilink rewriter, so 'doctor heal' rewrites
[[Title]] links that won't resolve in Obsidian to the [[filename|Title]] form.
As doctor learns to auto-fix more issues, they join this command.

heal APPLIES changes by default. Pass --dry-run to preview the planned repairs
without touching disk. (This is the inverse of the old 'lint fix-links', which
was dry-run by default.)

The 'fix' alias makes 'doctor fix' an exact synonym.

SUBCOMMANDS

  wikilinks   Surgically rewrite only Obsidian-incompatible wikilinks.

EXAMPLES

  vaultmind doctor heal --vault ./my-vault            # apply all auto-fixes
  vaultmind doctor heal --vault ./my-vault --dry-run  # preview only
  vaultmind doctor heal wikilinks --vault ./my-vault  # just the wikilink repair
  vaultmind doctor fix --vault ./my-vault             # 'fix' alias`,
	ConfigPrefix: "app.doctorheal",
	FlagOverrides: map[string]string{
		"app.doctorheal.vault":   "vault",
		"app.doctorheal.json":    "json",
		"app.doctorheal.dry_run": "dry-run",
	},
}

// DoctorHealWikilinksMetadata defines the metadata for the
// `doctor heal wikilinks` subcommand — the surgical wikilink repair that the
// old `lint fix-links` performed. Applies by default; --dry-run previews.
var DoctorHealWikilinksMetadata = config.CommandMetadata{
	Use:   "wikilinks",
	Short: "Rewrite Obsidian-incompatible wikilinks to [[filename|Title]]",
	Long: `Detect [[Title]] wikilinks that won't resolve in Obsidian and rewrite them to
the [[filename|Title]] form Obsidian requires. Scans every markdown file in the
vault, reports each rewrite as "old -> new", and prints a summary of files
scanned, files changed, and links fixed.

Applies changes by default; pass --dry-run to preview without writing. The
'fix' alias makes 'doctor fix wikilinks' an exact synonym.

EXAMPLES

  vaultmind doctor heal wikilinks --vault ./my-vault            # apply the rewrites
  vaultmind doctor heal wikilinks --vault ./my-vault --dry-run  # preview only
  vaultmind doctor heal wikilinks --json                        # machine-readable`,
	ConfigPrefix: "app.doctorhealwikilinks",
	FlagOverrides: map[string]string{
		"app.doctorhealwikilinks.vault":   "vault",
		"app.doctorhealwikilinks.json":    "json",
		"app.doctorhealwikilinks.dry_run": "dry-run",
	},
}

// DoctorHealOptions returns configuration options for `doctor heal`.
func DoctorHealOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.doctorheal.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.doctorheal.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
		{Key: "app.doctorheal.dry_run", DefaultValue: false, Description: "Preview repairs without writing (default applies)", Type: "bool"},
	}
}

// DoctorHealWikilinksOptions returns configuration options for
// `doctor heal wikilinks`.
func DoctorHealWikilinksOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.doctorhealwikilinks.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.doctorhealwikilinks.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
		{Key: "app.doctorhealwikilinks.dry_run", DefaultValue: false, Description: "Preview repairs without writing (default applies)", Type: "bool"},
	}
}

func init() {
	config.RegisterOptionsProvider(DoctorHealOptions)
	config.RegisterOptionsProvider(DoctorHealWikilinksOptions)
}
