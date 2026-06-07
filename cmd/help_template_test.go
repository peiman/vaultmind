package cmd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRootHelp_RendersGroupedCatalog verifies `vaultmind help` now renders the
// full grouped command catalog (every user-facing command, with its
// when-to-use) under the ALL COMMANDS section, while keeping the curated lead
// and anti-patterns. This is the replacement for the old name-only
// INFRASTRUCTURE COMMANDS block.
func TestRootHelp_RendersGroupedCatalog(t *testing.T) {
	out, _, err := runRootCmd(t, "help")
	require.NoError(t, err)
	help := out.String()

	// Curated lead + anti-patterns are kept.
	assert.Contains(t, help, "WHEN YOU WANT TO ...")
	assert.Contains(t, help, "ANTI-PATTERNS")
	assert.Contains(t, help, "PAIRS WELL TOGETHER")

	// The new grouped catalog section replaced the old block.
	assert.Contains(t, help, "ALL COMMANDS (grouped by intent)")
	assert.NotContains(t, help, "INFRASTRUCTURE COMMANDS",
		"the name-only infrastructure block must be replaced by the grouped catalog")

	// Every catalog group title is rendered.
	assert.Contains(t, help, groupRetrievalTitle)
	assert.Contains(t, help, groupMaintenanceTitle)
	assert.Contains(t, help, groupLifecycleTitle)
	assert.Contains(t, help, groupSetupTitle)

	// A representative command from a previously-hidden-in-help group now
	// surfaces by full path with its when-to-use trigger.
	assert.Contains(t, help, "vaultmind index")
	assert.Contains(t, help, "when vault notes changed and you need to refresh the SQLite index")
}

// TestRootHelp_CatalogOrdersGroupsByCatalogOrder pins the section order:
// retrieval before maintenance before lifecycle before setup, matching
// catalogGroupOrder (not alphabetical).
func TestRootHelp_CatalogOrdersGroupsByCatalogOrder(t *testing.T) {
	out, _, err := runRootCmd(t, "help")
	require.NoError(t, err)
	help := out.String()

	idxRetrieval := strings.Index(help, groupRetrievalTitle)
	idxMaintenance := strings.Index(help, groupMaintenanceTitle)
	idxLifecycle := strings.Index(help, groupLifecycleTitle)
	idxSetup := strings.Index(help, groupSetupTitle)

	require.NotEqual(t, -1, idxRetrieval)
	assert.Less(t, idxRetrieval, idxMaintenance)
	assert.Less(t, idxMaintenance, idxLifecycle)
	assert.Less(t, idxLifecycle, idxSetup)
}

// TestRootHelp_GlobalFlagsStillRendered the global-flags block survives the
// split into lead/catalog/footer.
func TestRootHelp_GlobalFlagsStillRendered(t *testing.T) {
	out, _, err := runRootCmd(t, "help")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "GLOBAL FLAGS (apply to every subcommand)")
}
