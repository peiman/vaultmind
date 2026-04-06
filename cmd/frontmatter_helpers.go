package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/git"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/mutation"
	"github.com/rs/zerolog/log"
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

	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, vaultPath, cmdName)
	if err != nil {
		if errors.Is(err, cmdutil.ErrAlreadyWritten) {
			return nil
		}
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

	// Post-mutation re-index: update the index for the affected file
	if result.ReindexRequired && !req.DryRun {
		dbPath := filepath.Join(vaultPath, vdb.Config.Index.DBPath)
		idxr := index.NewIndexer(vaultPath, dbPath, vdb.Config)
		if idxErr := idxr.IndexFile(result.Path); idxErr != nil {
			log.Debug().Err(idxErr).Str("path", result.Path).Msg("post-mutation re-index failed")
		} else {
			result.ReindexRequired = false
		}
	}

	if getConfigValueWithFlags[bool](cmd, "json", jsonKey) {
		env := envelope.OK(cmdName, result)
		env.Meta.VaultPath = vaultPath
		env.Meta.IndexHash = vdb.GetIndexHash()
		env.Meta.IndexStale = result.ReindexRequired // false if re-index succeeded
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
