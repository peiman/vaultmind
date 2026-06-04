package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// FrontmatterValidateMetadata defines the metadata for the frontmatter validate command.
var FrontmatterValidateMetadata = config.CommandMetadata{
	Use:   "validate",
	Short: "Check vault notes for frontmatter rule violations",
	Long: `Scan all domain notes and report frontmatter violations against the type registry.
Catches four rule classes: missing required fields, invalid status values, unknown types, and
broken frontmatter references (explicit_relation edges pointing to non-existent notes).

MODES

  Default (indexed):
    Reads from the vault's SQLite index built by "vaultmind index". Fast.
    Use for routine health checks and CI pipelines. Reports the index hash
    in JSON output so results are traceable to a specific index snapshot.

  --live:
    Reads raw .md files on disk instead of the indexed database.
    Use when the index may be stale or you want to validate before indexing.
    No index hash is reported in this mode.

FLAGS

  --vault PATH   Path to vault root (default: ".")
  --live         Validate raw .md files on disk instead of the indexed database
  --json         Output structured JSON instead of human-readable text

OUTPUT INCLUDES (human-readable)

  "Checked N files: M valid, K issues"
  Per issue: [severity] path: message (rule)

  Severity values:  error (missing required field), warning (unknown type,
                    invalid status, broken reference)
  Rule values:      missing_required_field, unknown_type, invalid_status,
                    broken_reference

OUTPUT INCLUDES (--json)

  status:          "ok" or "warning" (warning when any issues found)
  result:
    files_checked  total notes scanned
    valid          notes with no violations
    issues[]       array of {path, id, severity, rule, message, field, value}
  meta:
    vault_path     vault root used
    index_hash     index snapshot identifier (empty in --live mode)

EXAMPLES

  vaultmind frontmatter validate                      # check indexed database in current vault
  vaultmind frontmatter validate --vault ./my-vault   # check a specific vault (indexed)
  vaultmind frontmatter validate --live               # check raw .md files before indexing
  vaultmind frontmatter validate --json               # machine-readable output for CI`,
	ConfigPrefix: "app.frontmatter",
	FlagOverrides: map[string]string{
		"app.frontmatter.vault": "vault",
		"app.frontmatter.json":  "json",
		"app.frontmatter.live":  "live",
	},
}

// FrontmatterOptions returns configuration options for frontmatter commands.
func FrontmatterOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.frontmatter.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.frontmatter.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
		{Key: "app.frontmatter.live", DefaultValue: false, Description: "Validate raw .md files on disk instead of the indexed database", Type: "bool"},
	}
}

func init() {
	config.RegisterOptionsProvider(FrontmatterOptions)
}
