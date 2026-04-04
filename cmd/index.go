package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/spf13/cobra"
)

var indexCmd = MustNewCommand(commands.IndexMetadata, runIndex)

func init() {
	MustAddToRoot(indexCmd)
}

func runIndex(cmd *cobra.Command, _ []string) error {
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppIndexVault)
	jsonOut := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppIndexJson)

	cfg, err := vault.LoadConfig(vaultPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	dbPath := filepath.Join(vaultPath, cfg.Index.DBPath)
	idxr := index.NewIndexer(vaultPath, dbPath, cfg)
	result, err := idxr.Rebuild()
	if err != nil {
		return fmt.Errorf("rebuilding index: %w", err)
	}

	if jsonOut {
		env := envelope.OK("index", result)
		env.Meta.VaultPath = vaultPath
		return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
	}

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Indexed %d notes (%d domain, %d unstructured, %d errors)\n",
		result.Indexed, result.DomainNotes, result.UnstructuredNotes, result.Errors)
	return err
}
