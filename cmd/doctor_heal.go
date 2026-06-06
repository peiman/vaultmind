package cmd

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/mutation"
	"github.com/spf13/cobra"
)

// doctorHealCmd applies every auto-fixable repair doctor's diagnosis can
// resolve. Today that set is exactly the wikilink rewriter, so `doctor heal`
// runs the same fixer as `doctor heal wikilinks`. The `fix` alias makes
// `doctor fix` resolve here. heal APPLIES by default; --dry-run previews.
var doctorHealCmd = func() *cobra.Command {
	c := MustNewCommand(commands.DoctorHealMetadata, runDoctorHeal)
	c.Aliases = []string{"fix"}
	return c
}()

func init() {
	doctorCmd.AddCommand(doctorHealCmd)
	setupCommandConfig(doctorHealCmd)
}

// runDoctorHeal resolves heal's flags and applies every auto-fixable repair.
// Today the auto-fixable set is exactly the wikilink fixer, so it routes
// through the shared runWikilinkFix engine with the "apply"/"dry-run" labels.
func runDoctorHeal(cmd *cobra.Command, _ []string) error {
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppDoctorhealVault)
	jsonOut := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppDoctorhealJson)
	dryRun := getConfigValueWithFlags[bool](cmd, "dry-run", config.KeyAppDoctorhealDryRun)
	return runWikilinkFix(cmd, vaultPath, jsonOut, !dryRun, healModeLabel(!dryRun), "doctor heal")
}

// healModeLabel maps the apply flag to heal's human-output mode label. heal
// speaks "apply" (it applies by default), distinct from the deprecated
// fix-links which speaks "fix" — both share the same engine, only the label
// differs.
func healModeLabel(apply bool) string {
	if apply {
		return "apply"
	}
	return "dry-run"
}

// runWikilinkFix is the single cmd-layer engine behind every wikilink repair:
// `doctor heal`, `doctor heal wikilinks`, and the deprecated `lint fix-links`
// alias. It opens the vault, calls the shared internal/mutation fixer
// (FixWikilinks — the SAME engine the old lint fix-links used), and renders
// either the JSON envelope or the human report. `apply` decides whether the
// fixer writes; `modeLabel` is the human-output verb (apply / fix / dry-run);
// `cmdName` names the operation in error and envelope text.
func runWikilinkFix(cmd *cobra.Command, vaultPath string, jsonOut, apply bool, modeLabel, cmdName string) error {
	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, vaultPath, cmdName)
	if err != nil {
		if errors.Is(err, cmdutil.ErrAlreadyWritten) {
			return nil
		}
		return err
	}
	defer vdb.Close()

	result, err := mutation.FixWikilinks(vdb.DB, vaultPath, apply)
	if err != nil {
		return fmt.Errorf("%s: %w", cmdName, err)
	}

	if jsonOut {
		env := envelope.OK(cmdName, result)
		env.Meta.VaultPath = vaultPath
		env.Meta.IndexHash = vdb.GetIndexHash()
		return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
	}

	w := cmd.OutOrStdout()
	if _, err = fmt.Fprintf(w, "Mode: %s\nFiles scanned: %d\nFiles changed: %d\nLinks fixed: %d\n",
		modeLabel, result.FilesScanned, result.FilesChanged, result.LinksFixed); err != nil {
		return err
	}
	for _, d := range result.Details {
		if _, err = fmt.Fprintf(w, "  %s: %s → %s\n", d.Path, d.OldLink, d.NewLink); err != nil {
			return err
		}
	}
	return nil
}
