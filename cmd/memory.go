package cmd

import "github.com/spf13/cobra"

var memoryCmd = &cobra.Command{
	Use:   "memory",
	Short: "Traverse the note graph and assemble context for agent consumption",
	Long: `Query associative relationships in your vault: follow directed links,
traverse graph neighborhoods, list connected notes, assemble token-budgeted
context payloads, or gather note material for agent-driven synthesis.

CHOOSE A SUBCOMMAND BY INTENT

  memory links <id-or-path> [--out|--in|--both]
      Directed wikilink edges of a note. --out is outbound (what the note
      references), --in is inbound (backlinks: what references the note),
      --both (default) shows both directions. Use to follow a single hop
      of explicit edges in a known direction.

  memory neighbors <id-or-path>
      BFS traversal starting from a note. Loads all neighbors within a
      depth limit and returns full frontmatter for every node in the
      expanded neighborhood. Use when you want the enriched graph around
      a note — not just its direct connections.

  memory related <id-or-path>
      Direct connections only. Lists notes explicitly or inferentially
      linked to the target, filtered by edge type (explicit, inferred,
      mixed). Use when you want a simple ranked list without deeper
      traversal.

  memory pack <id-or-path>
      Token-budgeted context assembly. Loads the target note, then fills
      a token budget with ranked context items (graph neighbors). Truncates
      when the budget is exhausted. Use when you want a ready-to-ship
      payload for agent consumption with a known token ceiling.

  memory summarize [id1 id2 ...]
      Not a graph operation. Loads frontmatter and optional body excerpts
      from a specific set of note IDs. Use when you have a known list of
      notes to assemble for agent-driven synthesis (e.g., reflection note
      generation). Accepts IDs as positional arguments or via --ids.

EXAMPLES

  vaultmind memory links concept-foo --out                       # outbound edges only
  vaultmind memory neighbors concept-foo --depth 2 --json        # deeper BFS, machine-readable
  vaultmind memory related concept-foo --mode explicit           # only explicit edges
  vaultmind memory pack concept-foo --budget 3000                # cap at 3000 tokens
  vaultmind memory summarize note-a note-b note-c --include-body # gather bodies for synthesis

Run 'vaultmind memory <subcommand> --help' for per-command flags and defaults.`,
}

func init() {
	RootCmd.AddCommand(memoryCmd)
}
