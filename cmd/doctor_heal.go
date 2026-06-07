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

// healResult is the JSON payload for a wikilink-repair run: the shared
// FixWikilinks result plus the stale-index fields. When an apply rewrote files
// the on-disk content no longer matches the index, so StaleIndexAfterHeal is
// true and StaleIndexRemedy names the re-index command (SSOT: staleIndexRemedy
// in doctor.go, the same line doctor's drift warning prints). Embedding the
// mutation result preserves every existing JSON field (files_scanned, etc.) so
// the envelope shape is a superset, not a breaking change.
type healResult struct {
	*mutation.FixWikilinksResult
	StaleIndexAfterHeal bool   `json:"stale_index_after_heal"`
	StaleIndexRemedy    string `json:"stale_index_remedy,omitempty"`
}

// runWikilinkFix is the single cmd-layer engine behind every wikilink repair:
// `doctor heal`, `doctor heal wikilinks`, and the deprecated `lint fix-links`
// alias. It opens the vault, calls the shared internal/mutation fixer
// (FixWikilinks — the SAME engine the old lint fix-links used), and renders
// either the JSON envelope or the human report. `apply` decides whether the
// fixer writes; `modeLabel` is the human-output verb (apply / fix / dry-run);
// `cmdName` names the operation in error and envelope text.
//
// When an apply actually rewrites files, the index goes stale (the indexer's
// stored hashes no longer match disk), so both outputs surface an actionable
// re-index warning. A dry-run or a zero-change apply leaves the index intact
// and emits no warning.
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

	// The apply rewrote files only when both apply was requested AND files
	// actually changed — that's exactly when the index is now stale.
	indexStale := apply && result.FilesChanged > 0

	if jsonOut {
		payload := healResult{FixWikilinksResult: result, StaleIndexAfterHeal: indexStale}
		if indexStale {
			payload.StaleIndexRemedy = staleIndexRemedy
		}
		env := envelope.OK(cmdName, payload)
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
	if indexStale {
		if _, err = fmt.Fprintf(w,
			"⚠ Index is now stale (%d file(s) rewritten) — run: %s\n",
			result.FilesChanged, staleIndexRemedy); err != nil {
			return err
		}
	}
	return nil
}
