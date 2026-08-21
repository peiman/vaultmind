package release

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestIsReleaseVersion_AcceptsGitDescribeBuilds pins who gets told about a
// release — and records a hypothesis that was WRONG, so nobody re-derives it.
//
// When doctor stayed silent on a v0.6.0-13-g2965b23 build while v0.7.0 was
// already tagged, I concluded that isReleaseVersion rejected git-describe
// versions and that source-built adopters were therefore never notified. I wrote
// this test to prove it. It passed unchanged: isReleaseVersion only requires a
// leading "v" followed by a digit, so it accepts these builds already, and
// splitVersion truncates at the first '-' so the comparison always worked.
//
// The real cause was the cache — see TestReadCache_NegativeAnswersExpireSooner.
//
// These assertions stay as regression pins for behaviour that is correct today
// and easy to break while "tightening" version parsing. They are not evidence of
// a fix, and writing the test first is the only reason a fix for a bug that did
// not exist was never shipped.
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
