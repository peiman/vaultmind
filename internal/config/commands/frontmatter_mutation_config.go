package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// FrontmatterSetMetadata defines metadata for the frontmatter set command.
var FrontmatterSetMetadata = config.CommandMetadata{
	Use:   "set <note-id-or-path> <key> <value>",
	Short: "Set a frontmatter field",
	Long: `Set a single frontmatter key to a value. Validates against the type schema.

Arguments:
  note-id-or-path   Note ID, title, or file path (resolved via entity resolution)
  key               Frontmatter key to set (e.g., "status", "tags")
  value             Value to assign (strings, numbers, JSON arrays like '["a","b"]')

The key is validated against the note's type schema. Use --allow-extra to set
keys not defined in the schema. Use --dry-run --diff to preview the change.`,
	ConfigPrefix: "app.frontmatterset",
	FlagOverrides: map[string]string{
		"app.frontmatterset.vault":       "vault",
		"app.frontmatterset.json":        "json",
		"app.frontmatterset.dry_run":     "dry-run",
		"app.frontmatterset.diff":        "diff",
		"app.frontmatterset.commit":      "commit",
		"app.frontmatterset.allow_extra": "allow-extra",
	},
}

// FrontmatterUnsetMetadata defines metadata for the frontmatter unset command.
var FrontmatterUnsetMetadata = config.CommandMetadata{
	Use:   "unset <note-id-or-path> <key>",
	Short: "Remove a frontmatter field",
	Long: `Remove a frontmatter key from a note's frontmatter.

Arguments:
  note-id-or-path   Note ID, title, or file path (resolved via entity resolution)
  key               Frontmatter key to remove

Returns an error if the key is required by the note's type schema.
Use --dry-run --diff to preview the change.`,
	ConfigPrefix: "app.frontmatterunset",
	FlagOverrides: map[string]string{
		"app.frontmatterunset.vault":   "vault",
		"app.frontmatterunset.json":    "json",
		"app.frontmatterunset.dry_run": "dry-run",
		"app.frontmatterunset.diff":    "diff",
		"app.frontmatterunset.commit":  "commit",
	},
}

// FrontmatterMergeMetadata defines metadata for the frontmatter merge command.
var FrontmatterMergeMetadata = config.CommandMetadata{
	Use:   "merge <note-id-or-path> --file <yaml-file>",
	Short: "Merge multiple frontmatter fields",
	Long: `Merge key-value pairs from a YAML file into the note's frontmatter.

Arguments:
  note-id-or-path   Note ID, title, or file path (resolved via entity resolution)

Required flags:
  --file   Path to a YAML file containing key-value pairs to merge

The YAML file should contain top-level keys matching frontmatter fields.
Existing keys are overwritten; new keys are added. Use --allow-extra to
merge keys not defined in the type schema. Use --dry-run --diff to preview.`,
	ConfigPrefix: "app.frontmattermerge",
	FlagOverrides: map[string]string{
		"app.frontmattermerge.vault":       "vault",
		"app.frontmattermerge.json":        "json",
		"app.frontmattermerge.dry_run":     "dry-run",
		"app.frontmattermerge.diff":        "diff",
		"app.frontmattermerge.commit":      "commit",
		"app.frontmattermerge.allow_extra": "allow-extra",
		"app.frontmattermerge.file":        "file",
	},
}

// FrontmatterNormalizeMetadata defines metadata for the frontmatter normalize command.
var FrontmatterNormalizeMetadata = config.CommandMetadata{
	Use:   "normalize [<file-or-path>] [flags]",
	Short: "Normalize frontmatter on a file (sort keys, fix aliases/tags, dates, snake_case)",
	Long: `Normalize frontmatter formatting on a single vault file.

Applies these transformations to the target file's frontmatter:
  - Sort keys into canonical order
  - Convert scalar aliases and tags fields to lists
  - Normalize all date fields (use --strip-time to truncate to date-only)
  - Convert all keys to snake_case

The optional argument is the path to a note file. When omitted, "." is used as the
target (current directory).

FLAGS

  --vault string      Path to vault root (default ".")
  --dry-run           Preview changes without writing to disk
  --diff              Show a unified diff of the proposed changes
  --commit            Stage and commit the file after applying changes
  --strip-time        Convert all datetime fields to date-only (drop time component)
  --json              Output result in JSON format

EXAMPLES

  vaultmind frontmatter normalize notes/my-note.md --dry-run --diff
      Preview what normalizations would be applied, showing a unified diff.

  vaultmind frontmatter normalize notes/my-note.md
      Apply all normalizations and write the file.

  vaultmind frontmatter normalize notes/my-note.md --strip-time --commit
      Normalize and strip time from dates, then stage and commit the change.

  vaultmind frontmatter normalize notes/my-note.md --json
      Apply normalizations and return structured JSON output.

WHEN TO USE

  Use "frontmatter normalize" to clean up a single file before review or commit.
  For setting or removing individual fields, use "frontmatter set" or "frontmatter unset".
  For bulk key-value updates from a YAML file, use "frontmatter merge".`,
	ConfigPrefix: "app.frontmatternormalize",
	FlagOverrides: map[string]string{
		"app.frontmatternormalize.vault":      "vault",
		"app.frontmatternormalize.json":       "json",
		"app.frontmatternormalize.dry_run":    "dry-run",
		"app.frontmatternormalize.diff":       "diff",
		"app.frontmatternormalize.commit":     "commit",
		"app.frontmatternormalize.strip_time": "strip-time",
	},
}

// FrontmatterMutationOptions returns config options for mutation commands.
func FrontmatterMutationOptions() []config.ConfigOption {
	return []config.ConfigOption{
		// set
		{Key: "app.frontmatterset.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.frontmatterset.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
		{Key: "app.frontmatterset.dry_run", DefaultValue: false, Description: "Preview changes without writing", Type: "bool"},
		{Key: "app.frontmatterset.diff", DefaultValue: false, Description: "Show unified diff", Type: "bool"},
		{Key: "app.frontmatterset.commit", DefaultValue: false, Description: "Stage and commit after mutation", Type: "bool"},
		{Key: "app.frontmatterset.allow_extra", DefaultValue: false, Description: "Allow keys not in type schema", Type: "bool"},
		// unset
		{Key: "app.frontmatterunset.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.frontmatterunset.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
		{Key: "app.frontmatterunset.dry_run", DefaultValue: false, Description: "Preview changes without writing", Type: "bool"},
		{Key: "app.frontmatterunset.diff", DefaultValue: false, Description: "Show unified diff", Type: "bool"},
		{Key: "app.frontmatterunset.commit", DefaultValue: false, Description: "Stage and commit after mutation", Type: "bool"},
		// merge
		{Key: "app.frontmattermerge.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.frontmattermerge.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
		{Key: "app.frontmattermerge.dry_run", DefaultValue: false, Description: "Preview changes without writing", Type: "bool"},
		{Key: "app.frontmattermerge.diff", DefaultValue: false, Description: "Show unified diff", Type: "bool"},
		{Key: "app.frontmattermerge.commit", DefaultValue: false, Description: "Stage and commit after mutation", Type: "bool"},
		{Key: "app.frontmattermerge.allow_extra", DefaultValue: false, Description: "Allow keys not in type schema", Type: "bool"},
		{Key: "app.frontmattermerge.file", DefaultValue: "", Description: "YAML file with fields to merge", Type: "string"},
		// normalize
		{Key: "app.frontmatternormalize.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.frontmatternormalize.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
		{Key: "app.frontmatternormalize.dry_run", DefaultValue: false, Description: "Preview changes without writing", Type: "bool"},
		{Key: "app.frontmatternormalize.diff", DefaultValue: false, Description: "Show unified diff", Type: "bool"},
		{Key: "app.frontmatternormalize.commit", DefaultValue: false, Description: "Stage and commit after mutation", Type: "bool"},
		{Key: "app.frontmatternormalize.strip_time", DefaultValue: false, Description: "Force all datetimes to date-only", Type: "bool"},
	}
}

func init() {
	config.RegisterOptionsProvider(FrontmatterMutationOptions)
}
