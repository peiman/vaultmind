package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/git"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/spf13/cobra"
)

var gitStatusCmd = MustNewCommand(commands.GitStatusMetadata, runGitStatus)

func init() {
	gitCmd.AddCommand(gitStatusCmd)
	setupCommandConfig(gitStatusCmd)
}

func runGitStatus(cmd *cobra.Command, _ []string) error {
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppGitstatusVault)
	detector := &git.GoGitDetector{}

	result, err := query.GitStatus(detector, vaultPath)
	if err != nil {
		return fmt.Errorf("git status: %w", err)
	}

	if getConfigValueWithFlags[bool](cmd, "json", config.KeyAppGitstatusJson) {
		env := envelope.OK("git status", result)
		env.Meta.VaultPath = vaultPath
		return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
	}

	return query.FormatGitStatus(result, cmd.OutOrStdout())
}
