package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/spf13/cobra"
)

var vaultStatusCmd = MustNewCommand(commands.VaultStatusMetadata, runVaultStatus)

func init() {
	vaultCmd.AddCommand(vaultStatusCmd)
	setupCommandConfig(vaultStatusCmd)
}

func runVaultStatus(cmd *cobra.Command, _ []string) error {
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppVaultstatusVault)
	vdb, err := cmdutil.OpenVaultDB(vaultPath)
	if err != nil {
		return err
	}
	defer vdb.Close()

	result, err := query.VaultStatus(vdb.DB, vaultPath, vdb.Config, vdb.Reg)
	if err != nil {
		return fmt.Errorf("vault status: %w", err)
	}

	if getConfigValueWithFlags[bool](cmd, "json", config.KeyAppVaultstatusJson) {
		env := envelope.OK("vault status", result)
		env.Meta.VaultPath = vaultPath
		env.Meta.IndexHash = vdb.GetIndexHash()
		return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(),
		"Vault: %s\nNotes: %d (%d domain, %d unstructured)\nTypes: %d\nIssues: %d errors, %d warnings\n",
		result.VaultPath, result.TotalFiles, result.DomainNotes, result.UnstructuredNotes,
		len(result.Types), result.IssuesSummary.Errors, result.IssuesSummary.Warnings)
	return err
}
