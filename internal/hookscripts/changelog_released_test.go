package hookscripts_test

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/testgit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// firstImmutableMinor is the first release line whose CHANGELOG section is
// held fixed. Releases v0.9.4 and earlier were edited during the repo split,
// so their sections differ from what their tags carry; they are not checked.
const firstImmutableMinor = 10 // v0.10.0

var (
	releaseTag = regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)$`)
	linkRef    = regexp.MustCompile(`^\[[^\]]+\]: `)
)

// changelogSection returns the "## [version]" section: from its heading up to,
// not including, the next "## [" heading, without link-reference lines.
func changelogSection(changelog, version string) (string, bool) {
	var out []string
	in := false
	for _, line := range strings.Split(changelog, "\n") {
		if strings.HasPrefix(line, "## [") {
			if in {
				break
			}
			in = strings.HasPrefix(line, "## ["+version+"]")
		}
		if in && !linkRef.MatchString(line) {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n"), len(out) > 0
}

// firstDifference names the first differing line of two sections.
func firstDifference(tagged, current string) string {
	a, b := strings.Split(tagged, "\n"), strings.Split(current, "\n")
	for i := 0; i < len(a) || i < len(b); i++ {
		var x, y string
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			return fmt.Sprintf("section line %d: tag has %q, working tree has %q", i+1, x, y)
		}
	}
	return "no line differs"
}

// A merge across a release can silently move a branch's new entry from
// [Unreleased] into the just-released section (git merges it into the list
// main just put under the release heading). A released section never changes
// after its tag, so any difference from the tag is such a slip.
func TestReleasedChangelogSectionsMatchTheirTags(t *testing.T) {
	changelog, err := os.ReadFile("../../CHANGELOG.md")
	require.NoError(t, err)
	out, err := testgit.Command(t, ".", "tag", "--list", "v*").Output()
	require.NoError(t, err)

	var versions []string
	for _, tag := range strings.Fields(string(out)) {
		m := releaseTag.FindStringSubmatch(tag)
		if m == nil || m[1] != "0" {
			continue
		}
		if minor, _ := strconv.Atoi(m[2]); minor >= firstImmutableMinor {
			versions = append(versions, tag[1:])
		}
	}
	if len(versions) == 0 {
		// CI checkouts are shallow with no tags; the workflow fetches them. A
		// silent skip there would turn the guard off without anyone noticing.
		if os.Getenv("CI") != "" {
			t.Fatal("no v0.10+ git tags in a CI checkout: add `git fetch --tags --depth=1 origin` before the tests")
		}
		t.Skip("no v0.10+ git tags in this checkout; run `git fetch --tags` to check released CHANGELOG sections")
	}
	sort.Strings(versions)

	var failures []string
	for _, version := range versions {
		tagged, err := testgit.Command(t, ".", "show", "v"+version+":CHANGELOG.md").Output()
		require.NoError(t, err, "v%s has no CHANGELOG.md", version)
		want, found := changelogSection(string(tagged), version)
		require.True(t, found, "v%s CHANGELOG.md has no [%s] section", version, version)
		got, _ := changelogSection(string(changelog), version)
		if got != want {
			failures = append(failures, fmt.Sprintf("[%s] differs from v%s: %s", version, version, firstDifference(want, got)))
		}
	}
	assert.Empty(t, failures, "released CHANGELOG sections must not change; move the entry back under [Unreleased]:\n%s", strings.Join(failures, "\n"))
}
