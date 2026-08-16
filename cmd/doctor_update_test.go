package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/release"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Doctor must never be held up, or made to fail, by a version check. It is a
// vault health command; the network is incidental. Opting out is the one
// deterministic way to assert that from a test without reaching the internet.
func TestWriteUpdateNotice_OptedOutIsSilentAndClean(t *testing.T) {
	t.Setenv(release.DisableEnv, "1")

	var buf bytes.Buffer
	require.NoError(t, writeUpdateNotice(&buf, "v0.1.0"))
	assert.Empty(t, buf.String(), "opted out means no line and no error")
}

// An unversioned build has nothing to compare and must stay silent — its
// operator is the one person who knows exactly what they are running.
func TestWriteUpdateNotice_UnversionedBuildIsSilent(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeUpdateNotice(&buf, "dev"))
	assert.Empty(t, buf.String())
}

// The version the notice reports must be the one `vaultmind version` prints.
// Two different answers on the same screen would be worse than no notice.
func TestResolvedVersion_MatchesTheVersionCommand(t *testing.T) {
	v := resolvedVersion()
	assert.NotEmpty(t, v, "some version always resolves, even for a dev build")

	out, _, err := runRootCmd(t, "version")
	require.NoError(t, err)
	assert.Truef(t, strings.Contains(out.String(), v),
		"`version` printed %q which does not contain the resolved %q", out.String(), v)
}

// The notice a user actually sees: what is available, what they are running,
// how to get it, and how to make it stop. All four, because a notice missing
// the last one is the kind people disable by uninstalling the feature.
func TestRenderUpdateNotice_NamesVersionsUpgradeAndOptOut(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, renderUpdateNotice(&buf, release.Info{
		Current: "v0.4.1", Latest: "v0.5.0", Newer: true,
	}))

	text := buf.String()
	assert.Contains(t, text, "v0.5.0 is available")
	assert.Contains(t, text, "running v0.4.1")
	assert.Contains(t, text, "go install github.com/peiman/vaultmind@v0.5.0")
	assert.Contains(t, text, release.DisableEnv, "the way to silence it is part of the notice")
}

func TestRenderUpdateNotice_CurrentVersionSaysNothing(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, renderUpdateNotice(&buf, release.Info{
		Current: "v0.5.0", Latest: "v0.5.0", Newer: false,
	}))
	assert.Empty(t, buf.String(), "being up to date is not news")
}

// A doctor run must not fail because stdout closed mid-notice.
func TestRenderUpdateNotice_PropagatesWriteError(t *testing.T) {
	err := renderUpdateNotice(&failAfterNWriter{ok: 0}, release.Info{
		Current: "v0.4.1", Latest: "v0.5.0", Newer: true,
	})
	require.Error(t, err)
}
