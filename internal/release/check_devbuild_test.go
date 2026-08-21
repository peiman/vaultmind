package release

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestIsReleaseVersion_AcceptsGitDescribeBuilds pins who gets told about a release.
//
// Check bailed on anything that was not an exact release tag, on the reasoning
// that someone running their own build knows what they are running. That holds
// for an untagged scratch build; it does not hold for `git clone && task build`,
// which is one of the three install paths the README documents and the one an
// adopter following it lands on. Those builds carry a git-describe version like
// v0.6.0-13-g2965b23 — a real base tag plus distance — and splitVersion already
// truncates at the first '-', so the comparison was always possible. Only the
// gate was wrong.
//
// Concretely: with v0.7.0 released and v0.6.0-13-g2965b23 installed, doctor said
// nothing at all.
func TestIsReleaseVersion_AcceptsGitDescribeBuilds(t *testing.T) {
	told := []string{
		"v0.7.0",
		"v0.6.0-13-g2965b23",
		"v0.6.0-13-g2965b23-dirty",
		"v1.2.3-rc1",
	}
	for _, v := range told {
		require.True(t, isReleaseVersion(v), "%q carries a base tag, so it can be compared", v)
	}

	silent := []string{
		"",
		"dev",
		"vdev",
		"v",
		"unknown",
	}
	for _, v := range silent {
		require.False(t, isReleaseVersion(v), "%q has no base tag to compare against", v)
	}
}

// TestIsNewer_AcrossGitDescribeBuild is the end-to-end of the case that was
// silent: a source build thirteen commits past v0.6.0 must be told that v0.7.0
// exists, and must not be told when it is already past the latest tag.
func TestIsNewer_AcrossGitDescribeBuild(t *testing.T) {
	require.True(t, isNewer("v0.6.0-13-g2965b23", "v0.7.0"))
	require.False(t, isNewer("v0.7.0-2-gabcdef0", "v0.7.0"))
	require.False(t, isNewer("v0.7.0", "v0.7.0"))
}
