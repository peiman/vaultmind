package cmd

import "github.com/spf13/cobra"

var schemaCmd = &cobra.Command{
	Use:   "schema",
	Short: "Query the vault's type schema",
	Long: `Query the vault's type registry — available note types, required fields, and valid statuses.

Agents use schema operations to discover what types can be created before attempting to
write notes. The type registry is defined in the vault's config file; schema commands
expose it in a form suited for agent discovery and type-aware tooling.

SUBCOMMANDS

  list-types    Return every registered note type with its required fields and status
                values. Use before creating notes to avoid type or field mismatches.

WHEN TO USE

  Reach for schema when you need to know what types exist, which fields are mandatory
  for a given type, or what status transitions are valid. It is cheaper than attempting
  a note creation and handling a validation error.`,
}

func init() {
	RootCmd.AddCommand(schemaCmd)
}
