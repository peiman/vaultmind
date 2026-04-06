package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/git"
	"github.com/peiman/vaultmind/internal/plan"
	"github.com/spf13/cobra"
)

var applyCmd = MustNewCommand(commands.ApplyMetadata, runApply)

func init() {
	MustAddToRoot(applyCmd)
}

func runApply(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: apply <plan-file | ->")
	}
	return executeApply(cmd, args[0])
}

func executeApply(cmd *cobra.Command, planArg string) error {
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppApplyVault)
	jsonOut := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppApplyJson)
	dryRun := getConfigValueWithFlags[bool](cmd, "dry-run", config.KeyAppApplyDryRun)
	diff := getConfigValueWithFlags[bool](cmd, "diff", config.KeyAppApplyDiff)
	commit := getConfigValueWithFlags[bool](cmd, "commit", config.KeyAppApplyCommit)

	var planData []byte
	var err error
	if planArg == "-" {
		planData, err = io.ReadAll(os.Stdin)
	} else {
		planData, err = os.ReadFile(planArg) //nolint:gosec // user-provided plan file
	}
	if err != nil {
		if jsonOut {
			return cmdutil.WriteJSONError(cmd.OutOrStdout(), "apply", "read_error", fmt.Sprintf("reading plan: %v", err))
		}
		return fmt.Errorf("reading plan: %w", err)
	}

	var p plan.Plan
	if err := json.Unmarshal(planData, &p); err != nil {
		if jsonOut {
			return cmdutil.WriteJSONError(cmd.OutOrStdout(), "apply", "parse_error", fmt.Sprintf("invalid plan JSON: %v", err))
		}
		return fmt.Errorf("parsing plan JSON: %w", err)
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

	exe := &plan.Executor{
		VaultPath: vaultPath,
		Detector:  detector,
		Checker:   checker,
		Committer: &git.Committer{},
		Registry:  vdb.Reg,
		Config:    vdb.Config,
	}

	result, err := exe.Apply(p, dryRun, diff, commit)
	if err != nil {
		if jsonOut {
			return cmdutil.WriteJSONError(cmd.OutOrStdout(), "apply", "execution_error", err.Error())
		}
		return fmt.Errorf("apply: %w", err)
	}

	if jsonOut {
		env := envelope.OK("apply", result)
		env.Meta.VaultPath = vaultPath
		return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
	}

	return formatApplyResult(result, cmd.OutOrStdout())
}

func formatApplyResult(result *plan.ApplyResult, w io.Writer) error {
	if _, err := fmt.Fprintf(w, "Plan: %s (%d/%d operations)\n",
		result.PlanDescription, result.OperationsCompleted, result.OperationsTotal); err != nil {
		return err
	}
	for i, op := range result.Operations {
		status := op.Status
		if op.Error != nil {
			status = fmt.Sprintf("ERROR: %s", op.Error.Code)
		}
		target := op.Target
		if target == "" {
			target = op.Path
		}
		if _, err := fmt.Fprintf(w, "  [%d/%d] %s %s ... %s\n",
			i+1, result.OperationsTotal, op.Op, target, status); err != nil {
			return err
		}
	}
	if result.Committed {
		if _, err := fmt.Fprintf(w, "Committed: %s\n", result.CommitSHA); err != nil {
			return err
		}
	}
	return nil
}
