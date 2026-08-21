package cmd

import (
	"fmt"
	"io"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/hooks"
	"github.com/spf13/cobra"
)

var hooksStatusCmd = func() *cobra.Command {
	c := MustNewCommand(commands.HooksStatusMetadata, runHooksStatus)
	c.Args = cobra.MaximumNArgs(1)
	return c
}()

func init() {
	hooksCmd.AddCommand(hooksStatusCmd)
	setupCommandConfig(hooksStatusCmd)
}

func runHooksStatus(cmd *cobra.Command, args []string) error {
	projectDir := resolveProjectDir(args)
	jsonOut := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppHooksstatusJson)

	report, err := hooks.Status(projectDir)
	if err != nil {
		if jsonOut {
			return cmdutil.WriteJSONError(cmd.OutOrStdout(), "hooks status", "status_failed", err.Error())
		}
		return err
	}

	if jsonOut {
		if err := cmdutil.WriteJSON(cmd.OutOrStdout(), "hooks status", report, "", ""); err != nil {
			return err
		}
	} else if err := renderHooksStatus(cmd.OutOrStdout(), report); err != nil {
		return err
	}

	// Non-zero when anything is drifted or missing. A status command that always
	// exits 0 cannot gate anything, and this one exists precisely so drift stops
	// depending on somebody reading a line.
	_, drifted, missing := report.Counts()
	// An unwired canonical event gates too. Contents and wiring are independent
	// failures: a project can hold every script byte-identical and still run
	// none of them, and that absence renders as nothing — the exact shape this
	// command was built to end, one layer up from where it ended it.
	_, unwired := report.EventCounts()
	if drifted+missing+unwired > 0 {
		return cmdutil.ErrAlreadyWritten
	}
	return nil
}

func renderHooksStatus(w io.Writer, report hooks.StatusReport) error {
	inSync, drifted, missing := report.Counts()

	if !report.Installed {
		_, err := fmt.Fprintf(w,
			"No hook scripts installed in %s\n  run: vaultmind hooks install %s --merge\n",
			report.ProjectDir, report.ProjectDir)
		return err
	}

	if _, err := fmt.Fprintf(w, "Hook scripts in %s: %d in sync, %d drifted, %d missing\n",
		report.ProjectDir, inSync, drifted, missing); err != nil {
		return err
	}

	if wired, unwired := report.EventCounts(); len(report.Events) > 0 {
		if _, err := fmt.Fprintf(w, "Hook events: %d wired, %d unwired\n", wired, unwired); err != nil {
			return err
		}
		for _, e := range report.Events {
			if e.State == hooks.EventWired {
				continue
			}
			// Name the event AND the script: "SessionEnd is off" and "the
			// script is missing" have different fixes, and a project can have
			// the script sitting right there unrun.
			if _, err := fmt.Fprintf(w, "  unwired   %s -> %s\n", e.Event, e.Script); err != nil {
				return err
			}
		}
	}

	// Name every script that is not in sync. A count alone cannot be acted on,
	// and the two states have different fixes.
	for _, s := range report.Scripts {
		if s.State == hooks.ScriptInSync {
			continue
		}
		if _, err := fmt.Fprintf(w, "  %-9s %s\n", s.State, s.Name); err != nil {
			return err
		}
	}

	if drifted > 0 {
		if _, err := fmt.Fprintf(w,
			"\nDrifted scripts differ from the canonical copy. If the change is yours and\n"+
				"worth keeping, send it upstream — an update overwrites it otherwise.\n"+
				"  overwrite with canonical: vaultmind hooks install %s --force\n", report.ProjectDir); err != nil {
			return err
		}
	}
	if missing > 0 {
		if _, err := fmt.Fprintf(w,
			"\nMissing scripts ship with this binary but are not installed here.\n"+
				"  install them: vaultmind hooks install %s --merge\n", report.ProjectDir); err != nil {
			return err
		}
	}
	return nil
}
