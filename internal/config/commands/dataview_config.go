package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// DataviewRenderMetadata defines metadata for the dataview render command.
var DataviewRenderMetadata = config.CommandMetadata{
	Use:   "render <note-id-or-path>",
	Short: "Render a generated region",
	Long: `Replace content between VAULTMIND:GENERATED markers with section template content.

Arguments:
  note-id-or-path   Note ID, title, or file path (resolved via entity resolution)

Markers have the format: <!-- VAULTMIND:GENERATED:{key}:START/END -->
Section templates are loaded from .vaultmind/sections/{type}/{key}.md.
Use --section-key to render a specific section (default: all sections).
Use --force to override checksum mismatch (hand-edited content).`,
	ConfigPrefix: "app.dataviewrender",
	FlagOverrides: map[string]string{
		"app.dataviewrender.vault":       "vault",
		"app.dataviewrender.json":        "json",
		"app.dataviewrender.dry_run":     "dry-run",
		"app.dataviewrender.diff":        "diff",
		"app.dataviewrender.commit":      "commit",
		"app.dataviewrender.force":       "force",
		"app.dataviewrender.section_key": "section-key",
	},
}

// DataviewLintMetadata defines metadata for the dataview lint command.
var DataviewLintMetadata = config.CommandMetadata{
	Use:   "lint",
	Short: "Scan the vault for broken or duplicate VAULTMIND:GENERATED markers",
	Long: `Validate VAULTMIND:GENERATED markers across every note in the vault.

The command reads all notes and checks each one for two classes of problems:

  malformed_markers   A START marker has no matching END, or vice versa, for
                      the same section key. Typically caused by a partial paste
                      or a hand-edit that removed one half of the pair.

  duplicate_markers   The same section key appears more than once in a single
                      note. Rendering would be ambiguous; the second occurrence
                      is flagged.

  read_error          The file could not be read (permissions, encoding).

Run this before "dataview render" to catch problems early. Run it after
manual edits to confirm the markers are still well-formed.

FLAGS

  --json    Machine-readable output. Emits a JSON envelope whose "data" field
            contains files_checked (int), valid (int), and issues (array).
            Each issue has: path, section_key, rule, message, line.
  --vault   Path to the vault root (default: current directory).

OUTPUT INCLUDES

  Text:  "Checked N files: N valid, N issues" followed by one line per issue
         in the form "[rule] path: message".

  JSON:  { "status": "ok"|"warning", "data": { "files_checked": N,
            "valid": N, "issues": [ { "path": "...", "section_key": "...",
            "rule": "...", "message": "...", "line": N } ] } }

EXAMPLES

  vaultmind dataview lint
      Scan the vault in the current directory; print summary and any issues.

  vaultmind dataview lint --vault /path/to/vault
      Scan a vault at an explicit path.

  vaultmind dataview lint --json
      Emit full JSON envelope; useful for piping into other tools.

  vaultmind dataview lint --json | jq '.data.issues'
      Extract just the issues array for scripted checks.`,
	ConfigPrefix: "app.dataviewlint",
	FlagOverrides: map[string]string{
		"app.dataviewlint.vault": "vault",
		"app.dataviewlint.json":  "json",
	},
}

// DataviewOptions returns config options for dataview commands.
func DataviewOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.dataviewrender.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.dataviewrender.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
		{Key: "app.dataviewrender.dry_run", DefaultValue: false, Description: "Preview without writing", Type: "bool"},
		{Key: "app.dataviewrender.diff", DefaultValue: false, Description: "Show unified diff", Type: "bool"},
		{Key: "app.dataviewrender.commit", DefaultValue: false, Description: "Stage and commit", Type: "bool"},
		{Key: "app.dataviewrender.force", DefaultValue: false, Description: "Override checksum mismatch", Type: "bool"},
		{Key: "app.dataviewrender.section_key", DefaultValue: "", Description: "Section key to render", Type: "string"},
		{Key: "app.dataviewlint.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.dataviewlint.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
	}
}

func init() {
	config.RegisterOptionsProvider(DataviewOptions)
}
