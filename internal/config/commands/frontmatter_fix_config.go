package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// FrontmatterFixMetadata defines metadata for the `frontmatter fix` command.
//
// Per the four-tier frontmatter taxonomy in schema/registry.go, vaultmind-
// owned fields (created, vm_updated) are the system's responsibility, not
// the user's. The mutator auto-maintains them on every operation, but
// existing notes that predate that auto-write contract may be missing
// them. This command surfaces the gap and (with --apply) closes it,
// without ever touching user-owned fields.
//
// Default mode is dry-run: the command prints what would change but does
// not write. Per arc-extending-not-overwriting, vaultmind never silently
// rewrites user files; --apply is the explicit gate.
//
// Provenance for the `created` value: git log first-commit (most accurate
// signal — actual creation date in version control), file mtime, today.
// vm_updated always becomes today's RFC3339 (schema.VMUpdatedFormat).
var FrontmatterFixMetadata = config.CommandMetadata{
	Use:   "fix",
	Short: "Backfill missing vaultmind-owned frontmatter fields",
	Long: `Walk the vault and identify domain notes missing vaultmind-owned
frontmatter fields (created, vm_updated). Surface what's missing and the
proposed values; with --apply, write the additions atomically.

Vaultmind owns these fields per the four-tier schema taxonomy — the user
should never have to maintain them manually. This command exists for
existing-vault audits and migration scenarios where the auto-maintenance
contract didn't yet apply.

User-owned fields (title, status, tags, related_ids, etc.) are NEVER
touched by this command — only vaultmind-owned fields get backfilled.

Default is dry-run. Pass --apply to write changes. Output as JSON with --json.

Examples:
  vaultmind frontmatter fix --vault .              # dry-run audit
  vaultmind frontmatter fix --vault . --apply      # backfill all missing fields
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
