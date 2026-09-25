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
	// An unapproved Codex hook gates as well: Codex skips it without a word, so
	// from inside the agent it looks exactly like having no memory at all.
	unapproved := 0
	if report.Codex != nil {
		_, codexDrifted, codexMissing := report.Codex.ScriptCounts()
		unapproved = report.Codex.Unapproved() + codexDrifted + codexMissing
	}
	if drifted+missing+unwired+unapproved > 0 {
		return cmdutil.ErrAlreadyWritten
	}
	return nil
}

func renderHooksStatus(w io.Writer, report hooks.StatusReport) error {
	inSync, drifted, missing := report.Counts()

	if !report.Installed {
		if report.Codex != nil {
			// A Codex-only project: its scripts live under .vaultmind/scripts.
			if err := renderCodexApprovals(w, report); err != nil {
				return err
			}
			if report.LeftoverClaudeScripts != "" {
				_, err := fmt.Fprintf(w,
					"\nLeftover from an older install, unused by Codex (safe to delete): %s\n",
					report.LeftoverClaudeScripts)
				return err
			}
			return nil
		}
		_, err := fmt.Fprintf(w,
			"No hook scripts installed in %s\n  run: vaultmind hooks install %s --merge\n",
			report.ProjectDir, report.ProjectDir)
		return err
	}

	// Name the profile in the header: "healthy" means healthy FOR A PROFILE,
	// and a reader who thinks they installed the full set deserves to see
	// which narrower contract they are actually being graded against.
	if _, err := fmt.Fprintf(w, "Hook scripts in %s [profile: %s]: %d in sync, %d drifted, %d missing\n",
		report.ProjectDir, report.Profile, inSync, drifted, missing); err != nil {
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
			if e.State == hooks.EventStaleMatcher {
				// Wired, but on a matcher an earlier release installed: it runs
				// and misses tools it now needs. Not "unwired" — that would send
				// the reader looking for a hook that is there.
				if _, err := fmt.Fprintf(w,
					"  old matcher %s -> %s (update it: vaultmind hooks install %s --merge)\n",
					e.Event, e.Script, report.ProjectDir); err != nil {
					return err
				}
				continue
			}
			if e.State == hooks.EventOutsideProfile {
				// Wired, but the declared profile excludes it: a persona loader
				// in a knowledge project runs a script it chose not to have.
				// The merge removes a group that is ours alone; one shared with
				// the project's own hook it leaves, so say both.
				if _, err := fmt.Fprintf(w,
					"  outside profile %s -> %s (remove it: vaultmind hooks install %s --profile %s --merge; "+
						"if that group also runs a hook of your own, remove ours from it by hand)\n",
					e.Event, e.Script, report.ProjectDir, report.Profile); err != nil {
					return err
				}
				continue
			}
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

	if err := renderCodexApprovals(w, report); err != nil {
		return err
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

// renderCodexApprovals names every VaultMind hook Codex will skip, and the one
// fix. Approval is checked the way Codex checks it — the recorded hash against
// the hook as it is now — so a hook changed after approval shows as changed.
func renderCodexApprovals(w io.Writer, report hooks.StatusReport) error {
	c := report.Codex
	if c == nil {
		return nil
	}
	if _, err := fmt.Fprintf(w, "Codex: %d of %d VaultMind hooks approved (%s)\n",
		c.Approved(), len(c.Hooks), c.HooksFile); err != nil {
		return err
	}
	inSync, drifted, missing := c.ScriptCounts()
	if _, err := fmt.Fprintf(w, "  scripts in %s: %d in sync, %d drifted, %d missing\n",
		c.ScriptsDir, inSync, drifted, missing); err != nil {
		return err
	}
	for _, sc := range c.Scripts {
		if sc.State != hooks.ScriptInSync {
			if _, err := fmt.Fprintf(w, "  %-9s %s\n", sc.State, sc.Name); err != nil {
				return err
			}
		}
	}
	if drifted+missing > 0 {
		if _, err := fmt.Fprintf(w,
			"  fix: vaultmind hooks install %s --agent codex --merge --force, then approve the changed hooks in /hooks\n",
			report.ProjectDir); err != nil {
			return err
		}
	}
	for _, h := range c.Hooks {
		if h.State == hooks.CodexApproved {
			continue
		}
		label := "not approved"
		switch h.State {
		case hooks.CodexDisabled:
			label = "disabled"
		case hooks.CodexModified:
			label = "changed since you approved it"
		}
		if _, err := fmt.Fprintf(w, "  %s  %s -> %s\n", label, h.Event, h.Script); err != nil {
			return err
		}
	}
	if c.Unapproved() == 0 {
		return nil
	}
	_, err := fmt.Fprintf(w,
		"\nCodex skips these without saying so: the agent starts with no memory.\n"+
			"  fix: open Codex in %s, run /hooks, and trust the VaultMind hooks.\n"+
			"  Re-check after every upgrade: a new or changed hook needs approving again.\n",
		report.ProjectDir)
	return err
}
