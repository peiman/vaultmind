package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/hooks"
	"github.com/spf13/cobra"
)

func runHooksStatusCore(cmd *cobra.Command, projectDir, rawScope string, jsonOut bool) error {
	scope, err := hooks.ParseScope(rawScope)
	if err != nil {
		return err
	}
	home, homeErr := os.UserHomeDir()
	activeProject := projectDir
	if scope == hooks.ScopeUser {
		if homeErr != nil {
			return fmt.Errorf("resolving user home: %w", homeErr)
		}
		projectDir = home
	}
	report, err := hooks.StatusForScope(projectDir, scope)
	if err == nil && homeErr == nil {
		report.Overlaps, err = hooks.ScopeOverlaps(activeProject, home, nil, "")
	}
	if scope == hooks.ScopeProject {
		report.UserScopeInstalled = hooks.UserScopeInstalled(home)
	}
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
	if report.Cursor != nil {
		_, cursorDrifted, cursorMissing := report.Cursor.ScriptCounts()
		unapproved += cursorDrifted + cursorMissing
	}
	if drifted+missing+unwired+unapproved+len(report.Overlaps)+len(report.BrokenCommands) > 0 {
		return cmdutil.ErrAlreadyWritten
	}
	return nil
}

func renderHooksStatus(w io.Writer, report hooks.StatusReport) error {
	if err := renderWiringHealth(w, report); err != nil {
		return err
	}
	return renderHooksStatusScripts(w, report)
}

func renderHooksStatusScripts(w io.Writer, report hooks.StatusReport) error {
	inSync, drifted, missing := report.Counts()
	if report.UserScopeInstalled {
		if _, err := fmt.Fprintln(w, "User-scope hooks are installed too; inspect them with: vaultmind hooks status --scope user"); err != nil {
			return err
		}
	}
	if report.Scope == hooks.ScopeUser {
		if _, err := fmt.Fprintf(w, "User-scope scripts: %s\nSettings: %s\n", report.ScriptsDir, report.SettingsPath); err != nil {
			return err
		}
	}

	if !report.Installed {
		if report.Codex != nil || report.Cursor != nil {
			// A Codex or Cursor project: its scripts live under .vaultmind/scripts.
			if err := renderCodexApprovals(w, report); err != nil {
				return err
			}
			if err := renderCursorScripts(w, report); err != nil {
				return err
			}
			if report.LeftoverClaudeScripts != "" {
				_, err := fmt.Fprintf(w,
					"\nLeftover from an older install, unused by Codex or Cursor (safe to delete): %s\n",
					report.LeftoverClaudeScripts)
				return err
			}
			return nil
		}
		_, err := fmt.Fprintf(w,
			"No hook scripts installed in %s\n  run: vaultmind hooks install %s --merge\n",
			report.ProjectDir, hooksStatusInstallTarget(report))
		return err
	}

	// Name the profile in the header: "healthy" means healthy FOR A PROFILE,
	// and a reader who thinks they installed the full set deserves to see
	// which narrower contract they are actually being graded against.
	if _, err := fmt.Fprintf(w, "Hook scripts in %s [profile: %s]: %d in sync, %d drifted, %d missing\n",
		report.ProjectDir, report.Profile, inSync, drifted, missing); err != nil {
		return err
	}

	if err := renderHooksStatusEvents(w, report); err != nil {
		return err
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
	if err := renderCursorScripts(w, report); err != nil {
		return err
	}

	if drifted > 0 {
		if _, err := fmt.Fprintf(w,
			"\nDrifted scripts differ from the canonical copy. If the change is yours and\n"+
				"worth keeping, send it upstream — an update overwrites it otherwise.\n"+
				"  overwrite with canonical: vaultmind hooks install %s --force\n", hooksStatusInstallTarget(report)); err != nil {
			return err
		}
	}
	if missing > 0 {
		if _, err := fmt.Fprintf(w,
			"\nMissing scripts ship with this binary but are not installed here.\n"+
				"  install them: vaultmind hooks install %s --merge\n", hooksStatusInstallTarget(report)); err != nil {
			return err
		}
	}
	return nil
}

func renderHooksStatusEvents(w io.Writer, report hooks.StatusReport) error {
	if len(report.Events) == 0 {
		return nil
	}
	wired, unwired := report.EventCounts()
	if _, err := fmt.Fprintf(w, "Hook events: %d wired, %d unwired\n", wired, unwired); err != nil {
		return err
	}
	for _, e := range report.Events {
		label, source, repair := string(e.State), "", ""
		if len(e.Files) > 0 {
			source = " (" + strings.Join(e.Files, ", ") + ")"
		}
		switch e.State {
		case hooks.EventStaleMatcher:
			label = "old matcher"
			repair = " (update it: " + hooksStatusEventRepairs(report, e) + ")"
		case hooks.EventOutsideProfile:
			label = "outside profile"
			repair = " (remove it: " + hooksStatusEventRepairs(report, e) +
				"; if that group also runs a hook of your own, remove ours from it by hand)"
		}
		if _, err := fmt.Fprintf(w, "  %s %s -> %s%s%s\n", label, e.Event, e.Script, source, repair); err != nil {
			return err
		}
	}
	return nil
}

func hooksStatusEventRepairs(report hooks.StatusReport, e hooks.EventStatus) string {
	command := "vaultmind hooks install " + hooksStatusInstallTarget(report)
	if e.State == hooks.EventOutsideProfile {
		command += " --profile " + string(report.Profile)
	}
	command += " --merge"
	if e.LocalOnly() {
		return command + " --local"
	}
	if len(e.Files) > 1 {
		return command + "; " + command + " --local"
	}
	return command
}

// renderCursorScripts states Cursor's scripts and, when any is off, the fix.
// Cursor has no approval step: a script in sync and wired is a script that runs.
func renderCursorScripts(w io.Writer, report hooks.StatusReport) error {
	c := report.Cursor
	if c == nil {
		return nil
	}
	inSync, drifted, missing := c.ScriptCounts()
	if _, err := fmt.Fprintf(w, "Cursor: %d in sync, %d drifted, %d missing (%s)\n",
		inSync, drifted, missing, c.ScriptsDir); err != nil {
		return err
	}
	for _, s := range c.Scripts {
		if s.State == hooks.ScriptInSync {
			continue
		}
		if _, err := fmt.Fprintf(w, "  %-9s %s\n", s.State, s.Name); err != nil {
			return err
		}
	}
	if drifted+missing > 0 {
		_, err := fmt.Fprintf(w, "  fix: vaultmind hooks install %s --agent cursor --merge --force\n", hooksStatusInstallTarget(report))
		return err
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
			hooksStatusInstallTarget(report)); err != nil {
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

func hooksStatusInstallTarget(report hooks.StatusReport) string {
	if report.Scope == hooks.ScopeUser {
		return "--scope user"
	}
	return report.ProjectDir
}

func renderScopeOverlaps(w io.Writer, overlaps []hooks.ScopeOverlap) error {
	for _, overlap := range overlaps {
		if _, err := fmt.Fprintf(w, "  overlap %s %s -> %s\n    user: %s\n    project: %s\n", overlap.Agent, overlap.Event, strings.Join(overlap.Scripts, ", "), overlap.UserSettings, overlap.ProjectSettings); err != nil {
			return err
		}
	}
	if len(overlaps) > 0 {
		_, err := fmt.Fprintln(w, "  Warning: hooks can fire twice; remove one scope's wiring, or install only non-overlapping scripts with --only. Neither scope is removed automatically.")
		return err
	}
	return nil
}

func renderWiringHealth(w io.Writer, report hooks.StatusReport) error {
	if err := renderScopeOverlaps(w, report.Overlaps); err != nil {
		return err
	}
	if err := renderEventNotices(w, report.Events); err != nil {
		return err
	}
	for _, b := range report.BrokenCommands {
		if _, err := fmt.Fprintf(w, "  broken %s %s (%s)\n    command: %s\n    %s\n    fix: %s\n", b.Agent, b.Event, b.SettingsPath, b.Command, b.Reason, b.Fix); err != nil {
			return err
		}
	}
	return nil
}

// renderEventNotices prints each advisory once, with its affected events below.
func renderEventNotices(w io.Writer, events []hooks.EventStatus) error {
	byMessage := map[string][]hooks.EventStatus{}
	var order []string
	for _, e := range events {
		for _, messages := range [][]string{e.Notes, e.Warnings} {
			for _, message := range messages {
				if _, exists := byMessage[message]; !exists {
					order = append(order, message)
				}
				byMessage[message] = append(byMessage[message], e)
			}
		}
	}
	for _, message := range order {
		if _, err := fmt.Fprintf(w, "  %s\n", message); err != nil {
			return err
		}
		for _, e := range byMessage[message] {
			if _, err := fmt.Fprintf(w, "    %s -> %s (%s)\n", e.Event, e.Script, strings.Join(e.Files, ", ")); err != nil {
				return err
			}
		}
	}
	return nil
}
