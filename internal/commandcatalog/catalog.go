// Package commandcatalog turns an assembled cobra command tree into a
// structured, group-ordered catalog and renders it for two surfaces: a
// terminal cheat-sheet (used by `vaultmind help`) and a markdown reference
// (used by `vaultmind docs commands` and embedded into onboarding).
//
// It is a pure transform — one generator, multiple consumers (SSOT). The
// catalog's group order, titles, and the "when"-annotation key are supplied by
// the caller (cmd/zz_catalog.go owns those constants), so this package carries
// no knowledge of VaultMind's specific groups and stays trivially testable
// against a fixture cobra tree.
package commandcatalog

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// Group is one catalog section: a stable ID and its display title.
type Group struct {
	ID    string
	Title string
}

// Options configures Build: the groups in display order and the cobra
// Annotations key under which each command's when-to-use trigger is stored.
type Options struct {
	// Groups lists the catalog groups in the order they should render.
	Groups []Group
	// WhenKey is the Annotations key carrying the when-to-use trigger phrase.
	WhenKey string
}

// Command is one cataloged command: its full invocation path, the Short (the
// WHAT line) and the when-to-use trigger (the WHEN line).
type Command struct {
	Path  string
	Short string
	When  string
}

// CatalogGroup is a rendered group: its title plus its member commands,
// path-sorted for deterministic output.
type CatalogGroup struct {
	ID       string
	Title    string
	Commands []Command
}

// Catalog is the full structured result: groups in declared order, each with
// its path-sorted commands. Groups with no commands are omitted.
type Catalog struct {
	Groups []CatalogGroup
}

// Build walks the assembled command tree and returns a Catalog: every
// non-hidden command that carries a registered GroupID is bucketed into its
// group. Hidden commands and commands with no (or an unregistered) GroupID are
// excluded. Groups render in opts.Groups order; commands within a group are
// path-sorted. Empty groups are dropped.
func Build(root *cobra.Command, opts Options) Catalog {
	// Pre-seed buckets in declared order so iteration is deterministic.
	order := make([]string, 0, len(opts.Groups))
	titles := make(map[string]string, len(opts.Groups))
	buckets := make(map[string][]Command, len(opts.Groups))
	for _, g := range opts.Groups {
		order = append(order, g.ID)
		titles[g.ID] = g.Title
		buckets[g.ID] = nil
	}

	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			if !sub.Hidden && sub.GroupID != "" {
				if _, known := titles[sub.GroupID]; known {
					buckets[sub.GroupID] = append(buckets[sub.GroupID], Command{
						Path:  sub.CommandPath(),
						Short: sub.Short,
						When:  sub.Annotations[opts.WhenKey],
					})
				}
			}
			walk(sub)
		}
	}
	walk(root)

	cat := Catalog{}
	for _, id := range order {
		cmds := buckets[id]
		if len(cmds) == 0 {
			continue
		}
		sort.Slice(cmds, func(i, j int) bool { return cmds[i].Path < cmds[j].Path })
		cat.Groups = append(cat.Groups, CatalogGroup{ID: id, Title: titles[id], Commands: cmds})
	}
	return cat
}

// RenderTerminal renders the catalog as a grouped cheat-sheet for `help`: a
// title line per group, then one block per command — its path, its Short, and
// an indented "when you ..." trigger line.
func RenderTerminal(cat Catalog) string {
	var b strings.Builder
	for gi, g := range cat.Groups {
		if gi > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "  %s\n", g.Title)
		for _, c := range g.Commands {
			fmt.Fprintf(&b, "    %s\n", c.Path)
			if c.Short != "" {
				fmt.Fprintf(&b, "        %s\n", c.Short)
			}
			if c.When != "" {
				fmt.Fprintf(&b, "        when %s\n", c.When)
			}
		}
	}
	return b.String()
}

// RenderMarkdown renders the catalog as a stable markdown reference: an H1
// title, then an H2 + table per group with Command / What / When columns. The
// output is deterministic (groups in declared order, commands path-sorted) so
// it can back a regenerate-and-diff drift gate.
func RenderMarkdown(cat Catalog) string {
	var b strings.Builder
	b.WriteString("# VaultMind Commands\n\n")
	b.WriteString("Every user-facing command, grouped by intent, with its when-to-use trigger.\n")
	b.WriteString("Generated from the command tree — do not edit by hand (run `task generate:docs:commands`).\n")
	for _, g := range cat.Groups {
		fmt.Fprintf(&b, "\n## %s\n\n", g.Title)
		b.WriteString("| Command | What | When to use |\n")
		b.WriteString("|---------|------|-------------|\n")
		for _, c := range g.Commands {
			fmt.Fprintf(&b, "| `%s` | %s | %s |\n",
				escapeCell(c.Path), escapeCell(c.Short), escapeCell(c.When))
		}
	}
	return b.String()
}

// escapeCell makes a value safe inside a markdown table cell: pipes would
// otherwise be read as column separators.
func escapeCell(s string) string {
	return strings.ReplaceAll(s, "|", `\|`)
}
