package cmd

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/git"
	"github.com/peiman/vaultmind/internal/mutation"
	"github.com/spf13/cobra"
)

// runMutation is a shared helper for all frontmatter mutation commands.
// It wires vault setup, policy checking, mutation execution, and output formatting.
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
			var me *mutation.MutationError
			if errors.As(err, &me) {
				return cmdutil.WriteJSONError(cmd.OutOrStdout(), cmdName, me.Code, me.Message)
			}
			// AX2: Fallback JSON error for non-MutationError.
			return cmdutil.WriteJSONError(cmd.OutOrStdout(), cmdName, "internal_error", err.Error())
		}
		return fmt.Errorf("%s: %w", cmdName, err)
	}

	if getConfigValueWithFlags[bool](cmd, "json", jsonKey) {
		env := envelope.OK(cmdName, result)
		env.Meta.VaultPath = vaultPath
		env.Meta.IndexStale = true // AX1: Mutations always stale the index.
		for _, w := range result.Warnings {
			env.AddWarning(w.Rule, w.Message, "")
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
	}

	if result.DryRun && result.Diff != "" {
		_, err = fmt.Fprint(cmd.OutOrStdout(), result.Diff)
	} else if result.DryRun {
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Dry run: %s %s (no changes written)\n", result.Operation, result.Path)
	} else {
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s %s: %s\n", result.Operation, result.Path, result.ID)
	}
	return err
}
