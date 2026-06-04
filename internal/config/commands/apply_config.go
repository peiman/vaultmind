package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// ApplyMetadata defines metadata for the apply command.
var ApplyMetadata = config.CommandMetadata{
	Use:   "apply <plan-file | ->",
	Short: "Execute an AI-generated plan to mutate vault notes",
	Long: `Parse a JSON plan file and execute its operations against the vault.

Plans are typically produced by AI agents to make structured changes to your notes:
setting frontmatter keys, creating new notes, or populating generated regions.
Use preview modes to inspect what will change before writing anything to disk.

THE PLAN FILE

  The argument is a path to a JSON file, or "-" to read from stdin.

  Required shape:
    {"version": 1, "description": "...", "operations": [...]}

  Supported operation types:
    frontmatter_set          Set a frontmatter key in one note
    frontmatter_unset        Remove a frontmatter key from one note
    frontmatter_merge        Merge a map of keys into a note's frontmatter
    generated_region_render  Write content into a delimited generated region
    note_create              Create a new note with frontmatter and body

PREVIEW AND EXECUTION MODES

  --dry-run   Parse and simulate without writing. Shows what would change.
  --diff      With --dry-run, display unified diffs of each note mutation.
  --commit    Auto-stage and commit all changes in one git commit after success.
              Without --commit, writes are applied but not committed.

FLAGS

  --vault <path>  Vault root directory (default: ".")
  --json          Machine-readable output: status envelope, per-operation results,
                  and commit SHA when --commit succeeds.

EXAMPLES

  vaultmind apply ./my-plan.json
      Apply the plan. Changes written to disk but not committed.

  vaultmind apply - < ./my-plan.json
      Read plan from stdin (pipe from an AI agent or another command).

  vaultmind apply ./my-plan.json --dry-run
      Simulate the plan without writing anything.

  vaultmind apply ./my-plan.json --dry-run --diff
      Simulate and show unified diffs so you see exactly what each mutation looks like.

  vaultmind apply ./my-plan.json --commit
      Apply, then auto-stage and commit in one step using the plan description.

  vaultmind apply ./my-plan.json --json
      Apply with machine-readable output including per-operation status.`,
	ConfigPrefix: "app.apply",
	FlagOverrides: map[string]string{
		"app.apply.vault":   "vault",
		"app.apply.json":    "json",
		"app.apply.dry_run": "dry-run",
		"app.apply.diff":    "diff",
		"app.apply.commit":  "commit",
	},
}

// ApplyOptions returns config options for the apply command.
func ApplyOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.apply.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.apply.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
		{Key: "app.apply.dry_run", DefaultValue: false, Description: "Preview without executing", Type: "bool"},
		{Key: "app.apply.diff", DefaultValue: false, Description: "Show unified diffs", Type: "bool"},
		{Key: "app.apply.commit", DefaultValue: false, Description: "Stage and commit all changes", Type: "bool"},
	}
}

func init() {
	config.RegisterOptionsProvider(ApplyOptions)
}
