package cmd

import (
	"errors"
	"fmt"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/git"
	marker "github.com/peiman/vaultmind/internal/marker"
	"github.com/peiman/vaultmind/internal/mutation"
	"github.com/spf13/cobra"
)

var dataviewRenderCmd = MustNewCommand(commands.DataviewRenderMetadata, runDataviewRender)

func init() {
	dataviewCmd.AddCommand(dataviewRenderCmd)
	setupCommandConfig(dataviewRenderCmd)
}

func runDataviewRender(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: dataview render <target>")
	}
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppDataviewrenderVault)
	useJSON := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppDataviewrenderJson)

	result, indexHash, err := executeDataviewRender(cmd, vaultPath, args[0])
	if err != nil {
		if errors.Is(err, cmdutil.ErrAlreadyWritten) {
			return nil
		}
		return dataviewRenderError(cmd, useJSON, err)
	}
	if useJSON {
		return cmdutil.WriteJSON(cmd.OutOrStdout(), "dataview render", result, vaultPath, indexHash)
	}
	return dataviewRenderText(cmd, result)
}

func executeDataviewRender(cmd *cobra.Command, vaultPath, target string) (*marker.RenderResult, string, error) {
	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, vaultPath, "dataview render")
	if err != nil {
		return nil, "", err
	}
	defer vdb.Close()

	checker, err := git.NewPolicyChecker(vdb.Config.Git)
	if err != nil {
		return nil, "", fmt.Errorf("creating policy checker: %w", err)
	}

	result, err := marker.RenderRegion(marker.RenderConfig{
		VaultPath:  vaultPath,
		Target:     target,
		SectionKey: getConfigValueWithFlags[string](cmd, "section-key", config.KeyAppDataviewrenderSectionKey),
		DryRun:     getConfigValueWithFlags[bool](cmd, "dry-run", config.KeyAppDataviewrenderDryRun),
		Diff:       getConfigValueWithFlags[bool](cmd, "diff", config.KeyAppDataviewrenderDiff),
		Commit:     getConfigValueWithFlags[bool](cmd, "commit", config.KeyAppDataviewrenderCommit),
		Force:      getConfigValueWithFlags[bool](cmd, "force", config.KeyAppDataviewrenderForce),
		Detector:   &git.GoGitDetector{},
		Checker:    checker,
		Committer:  &git.Committer{},
	})
	return result, vdb.GetIndexHash(), err
}

func dataviewRenderError(cmd *cobra.Command, useJSON bool, err error) error {
	if useJSON {
		var me *mutation.MutationError
		if errors.As(err, &me) {
			return cmdutil.WriteJSONError(cmd.OutOrStdout(), "dataview render", me.Code, me.Message)
		}
		return cmdutil.WriteJSONError(cmd.OutOrStdout(), "dataview render", "internal_error", err.Error())
	}
	return fmt.Errorf("dataview render: %w", err)
}

func dataviewRenderText(cmd *cobra.Command, result *marker.RenderResult) error {
	var err error
	if result.DryRun && result.Diff != "" {
		_, err = fmt.Fprint(cmd.OutOrStdout(), result.Diff)
	} else if result.DryRun {
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Dry run: render %s section %s (no changes written)\n", result.Path, result.SectionKey)
	} else {
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "rendered %s section %s\n", result.Path, result.SectionKey)
	}
	return err
}
