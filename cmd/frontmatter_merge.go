package cmd

import (
	"fmt"
	"os"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/mutation"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var frontmatterMergeCmd = MustNewCommand(commands.FrontmatterMergeMetadata, runFrontmatterMerge)

func init() {
	frontmatterCmd.AddCommand(frontmatterMergeCmd)
	setupCommandConfig(frontmatterMergeCmd)
}

func runFrontmatterMerge(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: frontmatter merge <target> --file <yaml-file>")
	}
	filePath := getConfigValueWithFlags[string](cmd, "file", config.KeyAppFrontmattermergeFile)
	if filePath == "" {
		return fmt.Errorf("--file flag is required")
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("reading merge file: %w", err)
	}
	var fields map[string]interface{}
	if err := yaml.Unmarshal(data, &fields); err != nil {
		return fmt.Errorf("parsing merge file: %w", err)
	}
	return runMutation(cmd, mutation.MutationRequest{
		Op: mutation.OpMerge, Target: args[0], Fields: fields,
	}, "frontmatter merge", config.KeyAppFrontmattermergeVault, config.KeyAppFrontmattermergeJson,
		config.KeyAppFrontmattermergeDryRun, config.KeyAppFrontmattermergeDiff,
		config.KeyAppFrontmattermergeCommit, config.KeyAppFrontmattermergeAllowExtra)
}
