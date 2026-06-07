// ckeletin:allow-custom-command
//
// This file is not a command — it's a helper that installs a custom
// help-rendering function on the root command. The ultra-thin-command
// validator (ADR-001) flags it because of its location (`cmd/`) and the
// presence of `cobra.Command` references; the whitelist comment above
// opts out of that check. The agent-first help layout this file
// implements is co-designed via inter-agent review and the rationale
// for *not* moving it to internal/help/ is documented inline in
// installAgentRootHelp's docstring.
package cmd

import (
	"fmt"

	"github.com/peiman/vaultmind/internal/commandcatalog"
	"github.com/spf13/cobra"
)

// installAgentRootHelp wires a custom help renderer onto the root command
// only — subcommand help (`<binary> ask --help`, etc.) keeps Cobra's
// default reference-shaped layout because that's the right shape for
// "give me the syntax for THIS command." Root help is decision-shaped,
// for agents asking "what should I do here?", organised by intent.
//
// We use SetHelpFunc rather than SetHelpTemplate so the cheat-sheet
// scope stays surgical: SetHelpTemplate on the root would inherit to
// every subcommand and override its (correct) reference layout.
//
// Co-designed via inter-agent review (docs/reviews/help-redesign-review-*.md).
// The fresh-session evaluator confirmed the intent-organised layout
// matches how an agent reaches for help; specific edits applied:
//   - "long-term" cut from header (across-sessions implies it)
//   - self parenthetical trimmed (over-explaining)
//   - Output-Contracts section dropped (belongs in --json's flag help,
//     not the discoverability surface)
//   - Pairs-Well-Together tightened to one strong pair
//   - Verify-Vault-Integrity gained when-to-run qualifiers
//   - Alphabetical command dump retired entirely (Cobra still indexes
//     subcommands; `<binary> <command> --help` works for any of them)
func installAgentRootHelp(root *cobra.Command, binary string) {
	defaultHelpFunc := root.HelpFunc()
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		// Subcommands fall through to Cobra's default — reference shape
		// is the right shape for per-command syntax help.
		if cmd != root {
			defaultHelpFunc(cmd, args)
			return
		}
		w := cmd.OutOrStdout()
		// All Fprint errors here are help-output write failures — there's
		// no useful recovery path (we can't tell the user we couldn't
		// tell them anything). Discard explicitly so the linter sees we
		// considered the error. Following Cobra's own help template
		// convention (it discards too).
		//
		// Three parts: the curated lead (intent menu + anti-patterns +
		// pairs), the generated grouped catalog of EVERY command (rendered
		// from the assembled tree — see commandcatalog), then the footer.
		// The catalog replaces the old hand-maintained name-only
		// "INFRASTRUCTURE COMMANDS" block: now every command shows up,
		// grouped, with its when-to-use, and nothing drifts on add.
		_, _ = fmt.Fprint(w, agentRootHelpLead(binary))
		_, _ = fmt.Fprint(w, agentRootHelpCatalog())
		_, _ = fmt.Fprint(w, agentRootHelpFooter(binary))
		// Render the global-flags block in the same section-divider
		// style as the rest of the cheat-sheet so it reads as part of
		// the page rather than a Cobra-default tail. Round-2 evaluator
		// flagged the previous flat "Flags:" block as visually
		// regressing from the curated surface above.
		if cmd.HasAvailableLocalFlags() || cmd.HasAvailableInheritedFlags() {
			_, _ = fmt.Fprint(w, "──────────────────────────────────────────────────────────────────────────────\n")
			_, _ = fmt.Fprint(w, "GLOBAL FLAGS (apply to every subcommand)\n")
			_, _ = fmt.Fprint(w, "──────────────────────────────────────────────────────────────────────────────\n\n")
			_, _ = fmt.Fprint(w, cmd.LocalFlags().FlagUsages())
			_, _ = fmt.Fprintln(w)
		}
	})
}

// agentRootHelpLead is the curated top of root help: the intent menu, the
// anti-patterns, and the one strong pairing. It ends just before the grouped
// command catalog (which agentRootHelpCatalog renders from the live tree).
func agentRootHelpLead(binary string) string {
	return fmt.Sprintf(`%[1]s — your associative memory across sessions

──────────────────────────────────────────────────────────────────────────────
WHEN YOU WANT TO ...
──────────────────────────────────────────────────────────────────────────────

  Find what's relevant in the vault
    %[1]s ask "<query>"                    menu + context-pack (default)
    %[1]s ask "<query>" --pointers-only    menu only — cheapest, no bodies
    %[1]s ask "<query>" --preview          menu + 1-line body snippets

  Read a specific note by id
    %[1]s note get <id>                    body inline, fires access tracking

  See your own memory state
    %[1]s self                             recent / hot / stale notes
                                                (auto-injected at session start)

  Verify vault integrity
    task check:citations                       CrossRef + arxiv title-match gate
                                                (run after vault edits)
    task check:retrieval                       Hit@5 / MRR floors per vault
                                                (run after content waves or ranking changes)
    %[1]s doctor [--summary]               vault health overview

──────────────────────────────────────────────────────────────────────────────
ANTI-PATTERNS
──────────────────────────────────────────────────────────────────────────────

  ask "X" --budget N | tail -M
      Don't double-clip. The budget asks for N tokens of context; tail throws
      most away. Pick one shape per intent (pointers-only / preview / default).

  Read tool on a vault note
      Use `+"`"+`note get`+"`"+` instead. The Read tool bypasses access tracking; the
      cleanest read path should also be the tracked one.

  Treating top-1 as the answer when confidence is "no clear winner"
      That label means top results are essentially tied. Treat top-N as
      candidates rather than committing to top-1.

──────────────────────────────────────────────────────────────────────────────
PAIRS WELL TOGETHER
──────────────────────────────────────────────────────────────────────────────

  ask --pointers-only "<topic>"  →  note get <id-from-results>
      Probe → read. Two clean access events on exactly the notes you wanted.

`, binary)
}

// agentRootHelpCatalog renders the grouped catalog of EVERY user-facing command
// (RenderTerminal over the assembled tree) under a single section header. This
// replaced the old hand-maintained name-only "INFRASTRUCTURE COMMANDS" block —
// now adding a command surfaces it here automatically, with its when-to-use,
// and the catalog can never drift from the tree.
func agentRootHelpCatalog() string {
	return "──────────────────────────────────────────────────────────────────────────────\n" +
		"ALL COMMANDS (grouped by intent)\n" +
		"──────────────────────────────────────────────────────────────────────────────\n\n" +
		commandcatalog.RenderTerminal(buildCommandCatalog())
}

// agentRootHelpFooter is the curated tail: where to go for per-command syntax
// and the manifesto.
func agentRootHelpFooter(binary string) string {
	return fmt.Sprintf(`
──────────────────────────────────────────────────────────────────────────────

For more on any command:  %[1]s <command> --help
For the manifesto / philosophy:  see vaultmind-identity/ in the repo

`, binary)
}
