// cmd/version_test.go

package cmd

import (
	"bytes"
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildVersionInfo pins the version-resolution rules. ldflags-injected
// values (release binaries) always win; a `go install …@version` build — which
// has no ldflags and would otherwise show the "dev" default — falls back to the
// module version and VCS stamps from the embedded build info. The build info is
// injected so the cases stay deterministic.
func TestBuildVersionInfo(t *testing.T) {
	tests := []struct {
		name                              string
		ldVersion, ldCommit, ldDate       string
		info                              *debug.BuildInfo
		ok                                bool
		wantVersion, wantCommit, wantDate string
	}{
		{
			name:      "ldflags win for a release binary",
			ldVersion: "v0.1.3", ldCommit: "abc1234", ldDate: "2026-01-01",
			info: &debug.BuildInfo{Main: debug.Module{Version: "v9.9.9"}}, ok: true,
			wantVersion: "v0.1.3", wantCommit: "abc1234", wantDate: "2026-01-01",
		},
		{
			name:      "go install falls back to the module version",
			ldVersion: "dev", ldCommit: "", ldDate: "",
			info: &debug.BuildInfo{Main: debug.Module{Version: "v0.1.3"}}, ok: true,
			wantVersion: "v0.1.3", wantCommit: "", wantDate: "",
		},
		{
			name:      "(devel) module version is not a real version",
			ldVersion: "dev", ldCommit: "", ldDate: "",
			info: &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, ok: true,
			wantVersion: "dev", wantCommit: "", wantDate: "",
		},
		{
			name:      "go build from a checkout fills commit and date from VCS settings",
			ldVersion: "dev", ldCommit: "", ldDate: "",
			info: &debug.BuildInfo{
				Main: debug.Module{Version: "(devel)"},
				Settings: []debug.BuildSetting{
					{Key: "vcs.revision", Value: "0123456789abcdef0123"},
					{Key: "vcs.time", Value: "2026-06-04T00:00:00Z"},
				},
			}, ok: true,
			wantVersion: "dev", wantCommit: "0123456789ab", wantDate: "2026-06-04T00:00:00Z",
		},
		{
			name:      "no build info leaves the ldflags defaults untouched",
			ldVersion: "dev", ldCommit: "", ldDate: "",
			info: nil, ok: false,
			wantVersion: "dev", wantCommit: "", wantDate: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, c, d := buildVersionInfo(tt.ldVersion, tt.ldCommit, tt.ldDate, tt.info, tt.ok)
			assert.Equal(t, tt.wantVersion, v, "version")
			assert.Equal(t, tt.wantCommit, c, "commit")
			assert.Equal(t, tt.wantDate, d, "date")
		})
	}
}

// TestVersionCommandRegistered guards the P2 papercut from the focalc field
// report: `vaultmind version` errored with "unknown command" even though
// --help advertised it. The subcommand must be wired to RootCmd.
func TestVersionCommandRegistered(t *testing.T) {
	var found bool
	for _, c := range RootCmd.Commands() {
		if c.Name() == "version" {
			found = true
			break
		}
	}
	assert.True(t, found, "`version` subcommand must be registered (focalc report P2)")
}

// TestVersionCommandOutput pins the output to the same shape as the global
// --version flag: "<name> version <Version>, commit <Commit>, built at <Date>".
func TestVersionCommandOutput(t *testing.T) {
	origV, origC, origD := Version, Commit, Date
	t.Cleanup(func() { Version, Commit, Date = origV, origC, origD })
	Version, Commit, Date = "v9.9.9", "abc1234", "2026-01-01_00:00:00"

	buf := &bytes.Buffer{}
	versionCmd.SetOut(buf)
	require.NoError(t, versionCmd.RunE(versionCmd, []string{}))

	out := buf.String()
	assert.Contains(t, out, "version v9.9.9")
	assert.Contains(t, out, "commit abc1234")
	assert.Contains(t, out, "built at 2026-01-01_00:00:00")
}
