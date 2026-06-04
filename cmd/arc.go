package cmd

import "github.com/spf13/cobra"

// arcCmd groups arc-distillation operations (plasticity step 2). Propose-only:
// nothing under it writes an arc — surfaced candidates are for the mind to draft
// and approve.
var arcCmd = &cobra.Command{
	Use:   "arc",
	Short: "Surface arc-distillation candidates from episode captures (propose-only)",
	Long: `Plasticity step 2: surface candidate transformation moments for arc distillation.

An arc is a durable narrative thread in an AI mind's long-term memory — a record
of how its thinking, values, or working style shifted over time. Arc distillation
is the process of recognising those turning-point moments and shaping them into
explicit arcs. Everything under this command is PROPOSE-ONLY: no subcommand ever
writes an arc. Surfaced candidates are pointers for the mind to judge, draft, and
approve against the bar in principle-how-to-write-arcs.

SUBCOMMANDS

  candidates   Scan episode captures and surface candidate transformation moments
               (authority-grants, manifesto-lens invocations). Output is a
               human-readable or JSON report listing the candidate moments with
               their episode source so you can review and decide what to draft.

WHEN TO USE

  Run "arc candidates" at the end of a session or after a batch of episodes has
  been captured to see whether any moments in those episodes cross the arc bar.
  Do not run it expecting auto-written arcs — use the output as a reading list,
  then draft arcs manually.

EXAMPLES

  vaultmind arc candidates --vault ./vaultmind-identity          # scan episodes
  vaultmind arc candidates --vault ./vaultmind-identity --json   # machine-readable candidate report`,
}

func init() {
	RootCmd.AddCommand(arcCmd)
}
