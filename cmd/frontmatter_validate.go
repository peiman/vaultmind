package cmd

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/spf13/cobra"
)

var frontmatterValidateCmd = MustNewCommand(commands.FrontmatterValidateMetadata, runFrontmatterValidate)

func init() {
	frontmatterCmd.AddCommand(frontmatterValidateCmd)
	setupCommandConfig(frontmatterValidateCmd)
}

func runFrontmatterValidate(cmd *cobra.Command, _ []string) error {
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppFrontmatterVault)
	live := getConfigValueWithFlags[bool](cmd, "live", config.KeyAppFrontmatterLive)
	jsonOut := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppFrontmatterJson)

	result, indexHash, err := runValidation(cmd, vaultPath, live)
	if err != nil {
		if errors.Is(err, cmdutil.ErrAlreadyWritten) {
			return nil
		}
		return err
	}

	if jsonOut {
		env := envelope.OK("frontmatter validate", result)
		if len(result.Issues) > 0 {
			env.Status = "warning"
		}
		env.Meta.VaultPath = vaultPath
		env.Meta.IndexHash = indexHash
		return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Checked %d files: %d valid, %d issues\n",
		result.FilesChecked, result.Valid, len(result.Issues)); err != nil {
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

// runValidation dispatches to either live (raw .md) or indexed validation and
// returns the result plus an index hash (empty string in live mode).
func runValidation(cmd *cobra.Command, vaultPath string, live bool) (*query.ValidateResult, string, error) {
	if live {
		reg, err := cmdutil.LoadRegistry(vaultPath)
		if err != nil {
			return nil, "", fmt.Errorf("loading registry: %w", err)
		}
		res, err := query.ValidateLive(vaultPath, reg)
		if err != nil {
			return nil, "", fmt.Errorf("validating: %w", err)
		}
		return res, "", nil
	}

	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, vaultPath, "frontmatter validate")
	if err != nil {
		return nil, "", err
	}
	defer vdb.Close()
	res, err := query.Validate(vdb.DB, vdb.Reg)
	if err != nil {
		return nil, "", fmt.Errorf("validating: %w", err)
	}
	return res, vdb.GetIndexHash(), nil
}
