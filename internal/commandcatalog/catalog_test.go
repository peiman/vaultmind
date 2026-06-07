package commandcatalog_test

import (
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/commandcatalog"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixtureOpts is the catalog shape used across these tests: two groups in a
// deliberate (non-alphabetical) order, keyed on the "when" annotation. The
// renderers must honour this order, not sort.
func fixtureOpts() commandcatalog.Options {
	return commandcatalog.Options{
		Groups: []commandcatalog.Group{
			{ID: "retrieval", Title: "Retrieval & memory:"},
			{ID: "setup", Title: "Setup & introspection:"},
		},
		WhenKey: "when",
	}
}

// fixtureTree builds a small cobra tree mirroring the real catalog's shape:
// a root with grouped leaves, a grouped parent with grouped children, one
// hidden command, and one command with no GroupID (must be skipped).
func fixtureTree() *cobra.Command {
	root := &cobra.Command{Use: "tool"}

	ask := &cobra.Command{
		Use:         "ask",
		Short:       "Answer what do I know about X",
		GroupID:     "retrieval",
		Annotations: map[string]string{"when": "you want an answer."},
	}
	note := &cobra.Command{
		Use:         "note",
		Short:       "Read and create notes",
		GroupID:     "retrieval",
		Annotations: map[string]string{"when": "you want a specific note."},
	}
	noteGet := &cobra.Command{
		Use:         "get",
		Short:       "Get one note by ID",
		GroupID:     "retrieval",
		Annotations: map[string]string{"when": "you know the ID."},
	}
	note.AddCommand(noteGet)

	version := &cobra.Command{
		Use:         "version",
		Short:       "Print version",
		GroupID:     "setup",
		Annotations: map[string]string{"when": "you want the build version."},
	}

	// Hidden command — must be excluded from the catalog entirely.
	hidden := &cobra.Command{
		Use:         "lint",
		Short:       "deprecated alias",
		Hidden:      true,
		GroupID:     "retrieval",
		Annotations: map[string]string{"when": "deprecated."},
	}
	// Ungrouped command (no GroupID) — cobra's auto-generated help/completion
	// look like this; the catalog skips them.
	ungrouped := &cobra.Command{Use: "help", Short: "Help about any command"}

	root.AddCommand(ask, note, version, hidden, ungrouped)
	return root
}

// TestBuild_GroupsInDeclaredOrder verifies the catalog preserves the declared
// group order (not alphabetical) and drops empty groups is NOT required — every
// declared group with at least one command appears.
func TestBuild_GroupsInDeclaredOrder(t *testing.T) {
	cat := commandcatalog.Build(fixtureTree(), fixtureOpts())

	require.Len(t, cat.Groups, 2, "both declared groups have commands")
	assert.Equal(t, "retrieval", cat.Groups[0].ID)
	assert.Equal(t, "Retrieval & memory:", cat.Groups[0].Title)
	assert.Equal(t, "setup", cat.Groups[1].ID)
}

// TestBuild_IncludesNestedAndExcludesHiddenAndUngrouped pins membership: nested
// children appear with their full path; hidden and ungrouped commands do not.
func TestBuild_IncludesNestedAndExcludesHiddenAndUngrouped(t *testing.T) {
	cat := commandcatalog.Build(fixtureTree(), fixtureOpts())

	var paths []string
	for _, g := range cat.Groups {
		for _, c := range g.Commands {
			paths = append(paths, c.Path)
		}
	}
	assert.Contains(t, paths, "tool ask")
	assert.Contains(t, paths, "tool note")
	assert.Contains(t, paths, "tool note get", "nested children are cataloged")
	assert.Contains(t, paths, "tool version")
	assert.NotContains(t, paths, "tool lint", "hidden command excluded")
	assert.NotContains(t, paths, "tool help", "ungrouped command excluded")
}

// TestBuild_CommandsSortedByPathWithinGroup keeps the markdown/terminal output
// deterministic regardless of cobra's registration order.
func TestBuild_CommandsSortedByPathWithinGroup(t *testing.T) {
	cat := commandcatalog.Build(fixtureTree(), fixtureOpts())

	for _, g := range cat.Groups {
		for i := 1; i < len(g.Commands); i++ {
			assert.LessOrEqual(t, g.Commands[i-1].Path, g.Commands[i].Path,
				"commands within group %q must be path-sorted", g.ID)
		}
	}
}

// TestBuild_CarriesShortAndWhen each catalog command carries the Short and the
// when-to-use trigger pulled from the annotation.
func TestBuild_CarriesShortAndWhen(t *testing.T) {
	cat := commandcatalog.Build(fixtureTree(), fixtureOpts())

	var get commandcatalog.Command
	for _, g := range cat.Groups {
		for _, c := range g.Commands {
			if c.Path == "tool note get" {
				get = c
			}
		}
	}
	require.Equal(t, "tool note get", get.Path)
	assert.Equal(t, "Get one note by ID", get.Short)
	assert.Equal(t, "you know the ID.", get.When)
}

// TestBuild_OmitsGroupWithNoCommands a declared group that no command belongs to
// is dropped (no empty headers).
func TestBuild_OmitsGroupWithNoCommands(t *testing.T) {
	opts := fixtureOpts()
	opts.Groups = append(opts.Groups, commandcatalog.Group{ID: "ghost", Title: "Ghost:"})
	cat := commandcatalog.Build(fixtureTree(), opts)

	for _, g := range cat.Groups {
		assert.NotEqual(t, "ghost", g.ID, "group with no commands must be omitted")
	}
}

// TestRenderTerminal_GroupedWithWhen the terminal rendering shows each group
// title, every command path, its Short, and its when-to-use trigger.
func TestRenderTerminal_GroupedWithWhen(t *testing.T) {
	cat := commandcatalog.Build(fixtureTree(), fixtureOpts())
	out := commandcatalog.RenderTerminal(cat)

	assert.Contains(t, out, "Retrieval & memory:")
	assert.Contains(t, out, "Setup & introspection:")
	assert.Contains(t, out, "tool ask")
	assert.Contains(t, out, "Answer what do I know about X")
	assert.Contains(t, out, "you want an answer.")
	assert.Contains(t, out, "tool note get")

	// Group order preserved: retrieval header precedes setup header.
	assert.Less(t, strings.Index(out, "Retrieval & memory:"),
		strings.Index(out, "Setup & introspection:"))
}

// TestRenderMarkdown_GroupedTableWithWhen the markdown rendering produces a
// stable doc with an H2 per group and a row per command carrying path, short,
// and when. The drift gate diffs this output against the committed file.
func TestRenderMarkdown_GroupedTableWithWhen(t *testing.T) {
	cat := commandcatalog.Build(fixtureTree(), fixtureOpts())
	md := commandcatalog.RenderMarkdown(cat)

	assert.True(t, strings.HasPrefix(md, "# "), "markdown starts with an H1 title")
	assert.Contains(t, md, "## Retrieval & memory:")
	assert.Contains(t, md, "## Setup & introspection:")
	assert.Contains(t, md, "`tool note get`")
	assert.Contains(t, md, "Get one note by ID")
	assert.Contains(t, md, "you know the ID.")

	// Deterministic: rendering the same catalog twice is identical.
	assert.Equal(t, md, commandcatalog.RenderMarkdown(cat))

	// Group order preserved.
	assert.Less(t, strings.Index(md, "## Retrieval & memory:"),
		strings.Index(md, "## Setup & introspection:"))
}

// TestRenderMarkdown_EscapesPipes a Short or when containing a pipe must not
// break the markdown table.
func TestRenderMarkdown_EscapesPipes(t *testing.T) {
	root := &cobra.Command{Use: "tool"}
	root.AddCommand(&cobra.Command{
		Use:         "x",
		Short:       "a | b",
		GroupID:     "retrieval",
		Annotations: map[string]string{"when": "c | d."},
	})
	cat := commandcatalog.Build(root, fixtureOpts())
	md := commandcatalog.RenderMarkdown(cat)
	assert.Contains(t, md, `a \| b`)
	assert.Contains(t, md, `c \| d.`)
	assert.NotContains(t, md, "a | b", "raw pipe would break the table cell")
}
