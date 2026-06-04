package cmd

import "github.com/spf13/cobra"

var frontmatterCmd = &cobra.Command{
	Use:   "frontmatter",
	Short: "Inspect and mutate YAML frontmatter across vault notes",
	Long: `Read, validate, and write the YAML frontmatter that governs how
VaultMind indexes, retrieves, and reasons over vault notes. Use these
commands when you need to audit schema compliance, update fields
programmatically, or normalize formatting after bulk edits.

SUBCOMMANDS

  validate   Check all domain notes for missing required fields, invalid
             statuses, unknown types, and broken references. Use this
             before indexing a vault or after importing external notes.

  fix        Backfill the "created" field on notes that are missing it,
             deriving the value from git history, file mtime, or today's
             date. Default is dry-run; pass --apply to write.

  set        Set a single frontmatter key to a new value on one note,
             validated against the note's type schema.

  unset      Remove a frontmatter key from one note. Errors if the key is
             required by the schema.

  merge      Merge multiple key-value pairs from a YAML file into one
             note's frontmatter in a single operation.

  normalize  Sort keys into canonical order, convert scalar aliases to
             lists, normalize dates, and convert keys to snake_case —
             on one note.

WHEN TO USE

  Auditing compliance?          use "frontmatter validate"
  Migrating legacy notes?       use "frontmatter fix"
  Updating one field at a time? use "frontmatter set" / "frontmatter unset"
  Applying many fields at once? use "frontmatter merge"
  Cleaning up formatting?       use "frontmatter normalize"

EXAMPLES

  vaultmind frontmatter validate --vault .
  vaultmind frontmatter fix --vault . --apply
  vaultmind frontmatter set my-note status published
  vaultmind frontmatter normalize --dry-run --diff`,
}

func init() {
	RootCmd.AddCommand(frontmatterCmd)
}
