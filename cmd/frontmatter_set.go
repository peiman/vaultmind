package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/git"
	"github.com/peiman/vaultmind/internal/mutation"
	"github.com/spf13/cobra"
)

var frontmatterSetCmd = MustNewCommand(commands.FrontmatterSetMetadata, runFrontmatterSet)

func init() {
	frontmatterCmd.AddCommand(frontmatterSetCmd)
	setupCommandConfig(frontmatterSetCmd)
}

func runFrontmatterSet(cmd *cobra.Command, args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("usage: frontmatter set <target> <key> <value>")
	}
	return runMutation(cmd, mutation.MutationRequest{
		Op: mutation.OpSet, Target: args[0], Key: args[1], Value: args[2],
	}, "frontmatter set", config.KeyAppFrontmattersetVault, config.KeyAppFrontmattersetJson,
		config.KeyAppFrontmattersetDryRun, config.KeyAppFrontmattersetDiff,
		config.KeyAppFrontmattersetCommit, config.KeyAppFrontmattersetAllowExtra)
}

func runMutation(cmd *cobra.Command, req mutation.MutationRequest,
	cmdName, vaultKey, jsonKey, dryRunKey, diffKey, commitKey, allowExtraKey string,
) error {
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", vaultKey)
	req.DryRun = getConfigValueWithFlags[bool](cmd, "dry-run", dryRunKey)
	req.Diff = getConfigValueWithFlags[bool](cmd, "diff", diffKey)
	req.Commit = getConfigValueWithFlags[bool](cmd, "commit", commitKey)
	if allowExtraKey != "" {
		req.AllowExtra = getConfigValueWithFlags[bool](cmd, "allow-extra", allowExtraKey)
	}

	vdb, err := cmdutil.OpenVaultDB(vaultPath)
	if err != nil {
		return err
	}
	defer vdb.Close()

	detector := &git.GoGitDetector{}
	checker, err := git.NewPolicyChecker(vdb.Config.Git)
	if err != nil {
		return fmt.Errorf("creating policy checker: %w", err)
	}

	m := &mutation.Mutator{
		VaultPath: vaultPath,
		Detector:  detector,
		Checker:   checker,
		Committer: &git.Committer{},
		Registry:  vdb.Reg,
	}

	result, err := m.Run(req)
	if err != nil {
		if getConfigValueWithFlags[bool](cmd, "json", jsonKey) {
			if me, ok := err.(*mutation.MutationError); ok {
				return cmdutil.WriteJSONError(cmd.OutOrStdout(), cmdName, me.Code, me.Message)
			}
		}
		return fmt.Errorf("%s: %w", cmdName, err)
	}

	if getConfigValueWithFlags[bool](cmd, "json", jsonKey) {
		env := envelope.OK(cmdName, result)
		env.Meta.VaultPath = vaultPath
		return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
	}

	if result.DryRun && result.Diff != "" {
		fmt.Fprint(cmd.OutOrStdout(), result.Diff)
	} else if result.DryRun {
		fmt.Fprintf(cmd.OutOrStdout(), "Dry run: %s %s (no changes written)\n", result.Operation, result.Path)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "%s %s: %s\n", result.Operation, result.Path, result.ID)
	}
	return nil
}
