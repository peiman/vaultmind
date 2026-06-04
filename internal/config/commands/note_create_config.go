package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// NoteCreateMetadata defines metadata for the note create command.
var NoteCreateMetadata = config.CommandMetadata{
	Use:   "create <path> --type <type>",
	Short: "Create a note from a template with field overrides",
	Long: `Create a new note at <path> by expanding the template registered for --type,
then override individual frontmatter fields and body text before writing.

--type is required. The type must be registered in the vault schema; unknown
types are rejected with a list of registered alternatives.

TEMPLATE WORKFLOW

  1. The vault schema maps each type to a template file.
  2. Placeholders in the template (ID, TITLE, PATH, CREATED_DATE) are
     auto-filled from the note path and creation time.
  3. --field key=value patches a frontmatter field after auto-fill (repeatable).
  4. --body or --body-stdin replaces the template body.
  5. Required fields declared by the type must be present and non-empty after
     all substitutions; missing required fields fail with an actionable error
     naming the field and suggesting --field.
  6. The note is written to <vault-root>/<path>. The path must stay inside
     the vault directory (path traversal is rejected).

FLAGS

  --type <type>       Note type (required). Must match a type in the vault schema.
  --vault <dir>       Path to vault root (default: ".").
  --field key=value   Patch a frontmatter field after template expansion (repeatable).
  --body <text>       Replace the template body with literal text.
  --body-stdin        Read the body replacement from stdin (overrides --body).
  --commit            Stage and git-commit the new file after writing.
  --json              Output a structured envelope instead of the default one-liner.

OUTPUT INCLUDES (--json)

  path        vault-relative path of the created note
  id          auto-assigned note ID
  type        the --type value used
  created     true on success
  write_hash  sha256 of the file as written (integrity check)
  commit_sha  git commit SHA when --commit is used (omitted otherwise)
  warnings    list of non-fatal issues (e.g. re-index failure)

EXAMPLES

  vaultmind note create concepts/memory.md --type concept --vault my-vault
      # create a concept note; auto-fill ID, TITLE, CREATED_DATE from the template

  vaultmind note create principles/focus.md --type principle --field author=alice
      # create and set the "author" frontmatter field in one step

  vaultmind note create episodes/ep-001.md --type episode --body-stdin --vault vaultmind-identity <<EOF
  Session opened. Explored spreading-activation scoring.
  EOF
      # pipe a multi-line body from stdin

  vaultmind note create arcs/growth.md --type arc --commit --vault vaultmind-identity
      # create and immediately git-commit the new file`,
	ConfigPrefix: "app.notecreate",
	FlagOverrides: map[string]string{
		"app.notecreate.vault":  "vault",
		"app.notecreate.json":   "json",
		"app.notecreate.type":   "type",
		"app.notecreate.body":   "body",
		"app.notecreate.commit": "commit",
	},
}

// NoteCreateOptions returns config options for note create.
func NoteCreateOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.notecreate.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.notecreate.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
		{Key: "app.notecreate.type", DefaultValue: "", Description: "Note type (required)", Type: "string"},
		{Key: "app.notecreate.body", DefaultValue: "", Description: "Body text (overrides template body)", Type: "string"},
		{Key: "app.notecreate.commit", DefaultValue: false, Description: "Stage and commit", Type: "bool"},
	}
}

func init() {
	config.RegisterOptionsProvider(NoteCreateOptions)
}
