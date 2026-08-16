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
