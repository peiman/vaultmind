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

	status := "clean"
	if !result.WorkingTreeClean {
		status = fmt.Sprintf("dirty (%d unstaged, %d staged, %d untracked)",
			len(result.UnstagedFiles), len(result.StagedFiles), len(result.UntrackedFiles))
	}
	merge := "none"
	if result.MergeInProgress {
		merge = "merge in progress"
	} else if result.RebaseInProgress {
		merge = "rebase in progress"
	}

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Branch:  %s\nStatus:  %s\nMerge:   %s\n",
		result.Branch, status, merge)
	return err
}
