package cmd

import "github.com/spf13/cobra"

var linksCmd = &cobra.Command{
	Use:   "links",
	Short: "Trace note graph edges for context discovery",
	Long: `Explore the vault's note graph by querying directed link relationships.

Link operations let you follow edges between notes: find what a note
references (out), find what references it (in), or explore its full
local neighborhood (neighbors). Together these support associative
context-packing — surfacing related notes without a keyword search.

SUBCOMMANDS

  out <id-or-path>
      Return all outbound edges from a note: the notes it links to,
      with edge type and confidence. Use when you have a note and want
      to know what concepts it depends on or cites.

  in <id-or-path>
      Return all inbound edges pointing to a note: the notes that
      reference it. Use when you want to find all callers, dependents,
      or consumers of a concept.

  neighbors <id-or-path>
      Breadth-first traversal from a note up to a configurable depth.
      Returns every reachable node with distance and edge metadata.
      Use when you want the full local cluster around a topic, not
      just one direction of edges.

WHEN TO USE

  You know a note id and want its direct dependencies  ->  links out
  You want to know who references a concept            ->  links in
  You want the full local graph neighborhood           ->  links neighbors
  You want to search by content, not by graph position ->  search or ask

EXAMPLES

  vaultmind links out concept-spreading-activation --vault ./vault   # outbound edges from one note
  vaultmind links in  concept-spreading-activation --vault ./vault   # inbound edges to one note
  vaultmind links out concept-spreading-activation --json            # machine-readable output
  vaultmind links neighbors concept-hebbian --depth 2 --max-nodes 50 # 2-hop neighborhood
  vaultmind links neighbors concept-hebbian --min-confidence medium  # filter weak edges`,
}

func init() {
	RootCmd.AddCommand(linksCmd)
}
