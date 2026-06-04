package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// SchemaListTypesMetadata defines the metadata for the schema list-types command.
var SchemaListTypesMetadata = config.CommandMetadata{
	Use:   "list-types",
	Short: "Show the vault's type registry — available note types, required fields, and valid statuses",
	Long: `Return every registered note type together with its required-fields flag and allowed status values.

Agents run this during startup or before creating notes to discover what types the vault
supports and what constraints apply to each. Humans rarely need it directly; its primary
use is feeding agent initialization logic and type-aware tooling.

OUTPUT INCLUDES

  Text (default):  one line per type — name (12-char padded), required=true|false, statuses=[...]
  JSON (--json):   envelope with status "ok", command "schema list-types", and a "data"
                   object keyed by type name; each value carries required and statuses fields.

FLAGS

  --json    Output the registry as a JSON envelope (default: human-readable text).
  --vault   Path to vault root (default: current directory).

EXAMPLES

  vaultmind schema list-types
      Print the type registry as a text table — use to see at a glance what types exist.

  vaultmind schema list-types --vault ./my-vault
      Same, but targeting a vault at a specific path.

  vaultmind schema list-types --json
      Return the registry as a JSON envelope; use in agent init scripts or tooling
      that needs to validate a type before calling "note create".

WHEN TO USE

  Run list-types before creating a note when you are unsure whether a type name is valid
  or which status values are accepted. It is cheaper than attempting a write and handling
  a validation error.`,
	ConfigPrefix: "app.schema",
	FlagOverrides: map[string]string{
		"app.schema.vault": "vault",
		"app.schema.json":  "json",
	},
}

// SchemaOptions returns configuration options for schema commands.
func SchemaOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.schema.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.schema.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
	}
}

func init() {
	config.RegisterOptionsProvider(SchemaOptions)
}
