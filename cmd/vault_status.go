package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/schema"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/spf13/cobra"
)

var vaultStatusCmd = MustNewCommand(commands.VaultStatusMetadata, runVaultStatus)

func init() {
	vaultCmd.AddCommand(vaultStatusCmd)
	setupCommandConfig(vaultStatusCmd)
}

func runVaultStatus(cmd *cobra.Command, _ []string) error {
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppVaultstatusVault)
	jsonOut := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppVaultstatusJson)

	cfg, err := vault.LoadConfig(vaultPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	dbPath := filepath.Join(vaultPath, cfg.Index.DBPath)
	db, err := index.Open(dbPath)
	if err != nil {
		return fmt.Errorf("opening index: %w", err)
	}
	defer func() { _ = db.Close() }()

	reg := schema.NewRegistry(cfg.Types)
	result, err := query.VaultStatus(db, vaultPath, cfg, reg)
	if err != nil {
		return fmt.Errorf("vault status: %w", err)
	}

	if jsonOut {
		env := envelope.OK("vault status", result)
		env.Meta.VaultPath = vaultPath
		return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
	}

	_, err = fmt.Fprintf(cmd.OutOrStdout(),
		"Vault: %s\nNotes: %d (%d domain, %d unstructured)\nTypes: %d\nIssues: %d errors, %d warnings\n",
		result.VaultPath, result.TotalFiles, result.DomainNotes, result.UnstructuredNotes,
		len(result.Types), result.IssuesSummary.Errors, result.IssuesSummary.Warnings)
	return err
}
