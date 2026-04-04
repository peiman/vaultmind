package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// SchemaListTypesMetadata defines the metadata for the schema list-types command.
var SchemaListTypesMetadata = config.CommandMetadata{
	Use:          "list-types",
	Short:        "List registered note types",
	Long:         "Return the type registry in machine-readable form for agent discovery.",
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
