package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/spf13/cobra"
)

var schemaListTypesCmd = MustNewCommand(commands.SchemaListTypesMetadata, runSchemaListTypes)

func init() {
	schemaCmd.AddCommand(schemaListTypesCmd)
	setupCommandConfig(schemaListTypesCmd)
}

func runSchemaListTypes(cmd *cobra.Command, _ []string) error {
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppSchemaVault)
	jsonOut := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppSchemaJson)

	cfg, err := vault.LoadConfig(vaultPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if jsonOut {
		env := envelope.OK("schema list-types", cfg.Types)
		env.Meta.VaultPath = vaultPath
		return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
	}

	for name, td := range cfg.Types {
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "%-12s required=%v statuses=%v\n", name, td.Required, td.Statuses)
		if err != nil {
			return err
		}
	}
	return nil
}
