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

var frontmatterValidateCmd = MustNewCommand(commands.FrontmatterValidateMetadata, runFrontmatterValidate)

func init() {
	frontmatterCmd.AddCommand(frontmatterValidateCmd)
	setupCommandConfig(frontmatterValidateCmd)
}

func runFrontmatterValidate(cmd *cobra.Command, _ []string) error {
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppFrontmatterVault)
	jsonOut := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppFrontmatterJson)

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
	result, err := query.Validate(db, reg)
	if err != nil {
		return fmt.Errorf("validating: %w", err)
	}

	if jsonOut {
		env := envelope.OK("frontmatter validate", result)
		if len(result.Issues) > 0 {
			env.Status = "warning"
		}
		env.Meta.VaultPath = vaultPath
		return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
	}

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Checked %d files: %d valid, %d issues\n",
		result.FilesChecked, result.Valid, len(result.Issues))
	if err != nil {
		return err
	}
	for _, issue := range result.Issues {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "  [%s] %s: %s (%s)\n",
			issue.Severity, issue.Path, issue.Message, issue.Rule); err != nil {
			return err
		}
	}
	return nil
}
