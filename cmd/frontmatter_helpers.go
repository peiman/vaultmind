package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/git"
	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/mutation"
	"github.com/peiman/vaultmind/internal/vault"
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

	result, err := runResolvedMutation(m, vdb.DB, req)
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
			// result.ReindexRequired stays true as the structured signal to
			// callers. Log at Warn so the operator sees it in normal terminal
			// output; Debug alone would leave them thinking the mutation
			// fully succeeded when the index is actually stale.
			log.Warn().Err(idxErr).Str("path", result.Path).Msg("post-mutation re-index failed — index is stale for this file")
		} else {
			result.ReindexRequired = false
			embedAfterWrite(cmd, vaultPath, vdb.Config)
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

// runResolvedMutation finds the target note — a path, or an id, title or alias
// resolved the way `vaultmind resolve` does — and runs the mutation on it. Only
// a path used to work; anything else failed with "entity resolution not yet
// available" (#160).
func runResolvedMutation(m *mutation.Mutator, db *index.DB, req mutation.MutationRequest) (*mutation.MutationResult, error) {
	target, err := resolveMutationTarget(db, req.Target)
	if err != nil {
		return nil, err
	}
	req.Target = target
	return m.Run(req)
}

// resolveMutationTarget returns a path for target. A path (it holds "/" or ends
// in .md) is used as given.
func resolveMutationTarget(db *index.DB, target string) (string, error) {
	if strings.Contains(target, "/") || strings.HasSuffix(target, vault.NoteExtension) {
		return target, nil
	}
	res, err := graph.NewResolver(db).Resolve(target)
	if err != nil {
		return "", fmt.Errorf("resolving %q: %w", target, err)
	}
	switch {
	case !res.Resolved:
		return "", &mutation.MutationError{Code: "unresolved_target",
			Message: fmt.Sprintf("no note has the id, title or alias %q — check it with: vaultmind resolve %q", target, target)}
	case res.Ambiguous || len(res.Matches) != 1:
		paths := make([]string, 0, len(res.Matches))
		for _, match := range res.Matches {
			paths = append(paths, match.Path)
		}
		return "", &mutation.MutationError{Code: "ambiguous_target",
			Message: fmt.Sprintf("%q names %d notes (%s) — pass the path of the one you mean", target, len(res.Matches), strings.Join(paths, ", "))}
	}
	return res.Matches[0].Path, nil
}
