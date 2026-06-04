package cmd

import "github.com/spf13/cobra"

var lintCmd = &cobra.Command{
	Use:   "lint",
	Short: "Check and fix vault structure (wikilinks, dataview syntax)",
	Long: `Lint commands detect and fix structural issues in vault content.

Use lint after bulk imports, migrations, or when Obsidian reports broken links.
Lint is structural (syntax, file naming) — not semantic (content accuracy).

AVAILABLE SUBCOMMANDS

  fix-links
      Rewrite [[Title]] wikilinks that will not resolve in Obsidian to the
      [[filename|Title]] format that Obsidian requires. Scans every markdown
      file in the vault, reports each rewrite as "old -> new", and prints a
      summary of files scanned, files changed, and links fixed.
      Default mode is dry-run (preview only); pass --fix to apply changes.

WHEN TO USE

  - Obsidian reports unresolved links after an import or bulk rename
  - You migrated notes from another tool and wikilinks use display titles
    rather than filenames
  - You want to preview what would change before committing rewrites

EXAMPLES

  vaultmind lint fix-links --vault ./my-vault          # preview broken wikilinks (dry-run)
  vaultmind lint fix-links --vault ./my-vault --fix    # apply the rewrites
  vaultmind lint fix-links --json                      # machine-readable output`,
}

func init() {
	RootCmd.AddCommand(lintCmd)
}
