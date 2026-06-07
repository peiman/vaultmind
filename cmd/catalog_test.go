package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCommandCatalog_EveryUserFacingCommandIsCataloged is the enforcement gate:
// every non-hidden command reachable from RootCmd MUST carry a non-empty Short,
// a non-empty Annotations["when"] (the when-to-use trigger phrase), and a
// registered GroupID. This guarantees no future command ships without its
// catalog entry — the moment someone adds a command and forgets the metadata,
// this test goes red.
//
// Excluded: hidden commands (deprecated aliases: links*, lint*, vault*,
// memory recall/context-pack) and cobra's auto-generated "help" and
// "completion" machinery, which we do not author.
func TestCommandCatalog_EveryUserFacingCommandIsCataloged(t *testing.T) {
	groupIDs := map[string]bool{}
	for _, g := range RootCmd.Groups() {
		groupIDs[g.ID] = true
	}
	require.Equal(t, 4, len(groupIDs), "expected exactly four catalog groups registered on root")

	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			if !catalogExcluded(sub) {
				path := sub.CommandPath()
				assert.NotEmpty(t, sub.Short, "%s: missing Short", path)
				assert.NotEmptyf(t, sub.Annotations[annotationWhen],
					"%s: missing Annotations[%q] (when-to-use trigger)", path, annotationWhen)
				assert.NotEmptyf(t, sub.GroupID, "%s: missing GroupID", path)
				assert.Truef(t, groupIDs[sub.GroupID],
					"%s: GroupID %q is not a registered group", path, sub.GroupID)
			}
			walk(sub)
		}
	}
	walk(RootCmd)
}

// catalogExcluded reports whether a command is exempt from the catalog
// enforcement: hidden commands (deprecated aliases), cobra's auto-generated
// help/completion machinery, and developer-only tooling that exists only under
// the `dev` build tag (never in the production binary users get).
func catalogExcluded(c *cobra.Command) bool {
	if c.Hidden {
		return true
	}
	switch c.Name() {
	case "help", "completion":
		return true
	case "check": // dev build-tag-only developer task command
		return true
	}
	// The shell-specific completion leaves (completion bash/zsh/...) are
	// auto-generated children of the completion command.
	if c.Parent() != nil && c.Parent().Name() == "completion" {
		return true
	}
	// The `dev` subtree (dev, dev config/doctor/progress) is dev build-tag-only
	// tooling — not a user-facing vaultmind command, so it carries no catalog
	// entry. Match the command itself and any descendant of it.
	for cur := c; cur != nil; cur = cur.Parent() {
		if cur.Name() == "dev" && (cur.Parent() == nil || cur.Parent().Name() == "vaultmind") {
			return true
		}
	}
	return false
}

// TestCommandCatalog_WhenComposedIntoLong verifies the when-to-use trigger is
// surfaced in --help output: every cataloged command's Long must contain its
// "when" phrase under a "When to use:" line so an agent reading `<cmd> --help`
// sees the trigger, not just developers reading the annotation.
func TestCommandCatalog_WhenComposedIntoLong(t *testing.T) {
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			if !catalogExcluded(sub) {
				when := sub.Annotations[annotationWhen]
				require.NotEmpty(t, when, "%s: missing when annotation", sub.CommandPath())
				assert.Containsf(t, sub.Long, whenToUsePrefix,
					"%s: Long missing %q header", sub.CommandPath(), whenToUsePrefix)
				assert.Containsf(t, sub.Long, when,
					"%s: Long missing the when phrase composed into help", sub.CommandPath())
			}
			walk(sub)
		}
	}
	walk(RootCmd)
}

// TestCommandCatalog_HiddenAliasesExcluded locks the exclusion: the deprecated
// hidden aliases must NOT carry catalog metadata (group/when), so they stay out
// of the grouped help and nobody accidentally promotes them.
func TestCommandCatalog_HiddenAliasesExcluded(t *testing.T) {
	hidden := map[string]bool{}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			if sub.Hidden {
				hidden[sub.CommandPath()] = true
				assert.Emptyf(t, sub.GroupID, "%s: hidden alias must not carry a GroupID", sub.CommandPath())
				assert.Emptyf(t, sub.Annotations[annotationWhen],
					"%s: hidden alias must not carry a when annotation", sub.CommandPath())
			}
			walk(sub)
		}
	}
	walk(RootCmd)
	// Sanity: the known deprecated aliases are present and hidden.
	for _, p := range []string{
		"vaultmind links", "vaultmind lint", "vaultmind vault",
		"vaultmind memory recall", "vaultmind memory context-pack",
	} {
		assert.Truef(t, hidden[p], "expected %s to be a hidden deprecated alias", p)
	}
}

// TestComposeWhenIntoLong covers the Long-composition helper directly: seeding
// from Short when Long is empty, idempotency, and the empty-body edge.
func TestComposeWhenIntoLong(t *testing.T) {
	const when = "you want X."
	whenLine := whenToUsePrefix + " " + when

	// Non-empty Long gets the When line appended once.
	got := composeWhenIntoLong("Body text.", "Short.", when)
	assert.Equal(t, "Body text.\n\n"+whenLine, got)
	// Idempotent: composing again does not duplicate the When line.
	assert.Equal(t, got, composeWhenIntoLong(got, "Short.", when))

	// Empty Long seeds the body from Short.
	assert.Equal(t, "Short.\n\n"+whenLine, composeWhenIntoLong("", "Short.", when))

	// Empty Long AND empty Short collapses to just the When line.
	assert.Equal(t, whenLine, composeWhenIntoLong("", "", when))

	// Whitespace-only Long with empty Short also collapses to the When line.
	assert.Equal(t, whenLine, composeWhenIntoLong("   \n  ", "", when))
}

// TestApplyCatalogEntry covers stamping onto a command with nil Annotations and
// an empty short (Short preserved).
func TestApplyCatalogEntry(t *testing.T) {
	c := &cobra.Command{Use: "x", Short: "Original short", Long: "Original long."}
	applyCatalogEntry(c, catalogEntry{group: groupSetup, when: "you want X."})
	assert.Equal(t, groupSetup, c.GroupID)
	assert.Equal(t, "you want X.", c.Annotations[annotationWhen])
	assert.Equal(t, "Original short", c.Short, "empty short must preserve existing Short")
	assert.Contains(t, c.Long, whenToUsePrefix)

	// With a non-empty short, Short is tightened.
	c2 := &cobra.Command{Use: "y", Short: "Old", Long: ""}
	applyCatalogEntry(c2, catalogEntry{group: groupRetrieval, when: "trigger.", short: "New short"})
	assert.Equal(t, "New short", c2.Short)
	assert.Contains(t, c2.Long, "New short")
	assert.Contains(t, c2.Long, whenToUsePrefix)
}

// TestEnsureCatalogGroup covers the idempotent group registration helper.
func TestEnsureCatalogGroup(t *testing.T) {
	c := &cobra.Command{Use: "parent"}
	ensureCatalogGroup(c, groupMaintenance)
	ensureCatalogGroup(c, groupMaintenance) // idempotent: no duplicate
	count := 0
	for _, g := range c.Groups() {
		if g.ID == groupMaintenance {
			count++
		}
	}
	assert.Equal(t, 1, count, "group must be registered exactly once")
}

// TestCommandCatalog_RootHelpShowsWhenLine renders a representative command's
// help and asserts the When-to-use line is visible end-to-end.
func TestCommandCatalog_RootHelpShowsWhenLine(t *testing.T) {
	out, _, err := runRootCmd(t, "ask", "--help")
	require.NoError(t, err)
	assert.Contains(t, out.String(), whenToUsePrefix,
		"ask --help should render the When to use line")
	assert.Contains(t, strings.ToLower(out.String()), "when",
		"ask --help should mention when-to-use")
}
