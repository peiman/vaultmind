package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// FrontmatterFixMetadata defines metadata for the `frontmatter fix` command.
//
// Surfaces domain notes that are missing the `created` field and (with
// --apply) writes it. Migration scenarios — existing-vault notes that
// predate any auto-stamp era — are the primary use case.
//
// Default mode is dry-run: the command prints what would change but does
// not write. Per the extend-don't-overwrite principle, vaultmind never silently
// rewrites user files; --apply is the explicit gate.
//
// Provenance for the `created` value: git log first-commit (most accurate
// signal — actual creation date in version control), file mtime, today's
// date — tried in order, the first successful one wins.
//
// Scope was originally `created` + `vm_updated` (the four-tier
// vaultmind-owned tier). The 2026-05-04 dogfood pass retired vm_updated
// entirely (no read-side consumer survived the false-positive collapse
// of mtime-based drift), so this command now covers only `created`.
var FrontmatterFixMetadata = config.CommandMetadata{
	Use:   "fix",
	Short: "Backfill missing `created` frontmatter on domain notes",
	Long: `Walk the vault and identify domain notes missing the ` + "`created`" + ` field.
Surface what's missing and the proposed value; with --apply, write the
addition atomically.

Provenance for ` + "`created`" + `: git first-commit (when the file is tracked) → file
mtime → today's date. The first successful resolution wins.

User-owned fields (title, status, tags, related_ids, etc.) are NEVER
touched by this command — only the missing ` + "`created`" + ` field gets backfilled.

Default is dry-run. Pass --apply to write changes. Output as JSON with --json.

Examples:
  vaultmind frontmatter fix --vault .              # dry-run audit
  vaultmind frontmatter fix --vault . --apply      # backfill missing created
  vaultmind frontmatter fix --vault . --json       # machine-readable audit`,
	ConfigPrefix: "app.frontmatterfix",
	FlagOverrides: map[string]string{
		"app.frontmatterfix.vault": "vault",
		"app.frontmatterfix.json":  "json",
		"app.frontmatterfix.apply": "apply",
	},
}

// FrontmatterFixOptions returns configuration options for the fix command.
func FrontmatterFixOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.frontmatterfix.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.frontmatterfix.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
		{Key: "app.frontmatterfix.apply", DefaultValue: false, Description: "Apply changes (default: dry-run)", Type: "bool"},
	}
}

func init() {
	config.RegisterOptionsProvider(FrontmatterFixOptions)
}
