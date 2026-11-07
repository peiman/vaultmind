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

// lf strips carriage returns so line endings never count as a change.
func lf(b []byte) string {
	return strings.ReplaceAll(string(b), "\r\n", "\n")
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
	repoGoMod, err := os.ReadFile("../../go.mod")
	require.NoError(t, err)
	_, ok := modulePath(string(repoGoMod))
	require.True(t, ok, "../../go.mod has no module line")

	// CI fetches tags with --depth=1, so a tag commit is not an ancestor of
	// the shallow HEAD. `git tag --merged HEAD` would report no tags and
	// the guard would check nothing. Match the module path instead.
	var candidates []string
	for _, tag := range strings.Fields(string(out)) {
		m := releaseTag.FindStringSubmatch(tag)
		if m == nil || m[1] != "0" {
			continue
		}
		if minor, _ := strconv.Atoi(m[2]); minor >= firstImmutableMinor {
			candidates = append(candidates, tag)
		}
	}
	kept := tagsForModule(candidates, string(repoGoMod), func(tag string) (string, error) {
		body, showErr := testgit.Command(t, ".", "show", tag+":go.mod").CombinedOutput()
		if showErr != nil {
			msg := string(body)
			if strings.Contains(msg, "does not exist") || strings.Contains(msg, "exists on disk, but not in") {
				return "", showErr
			}
			t.Fatalf("git show %s:go.mod: %v\n%s", tag, showErr, body)
			return "", showErr
		}
		return string(body), nil
	})
	var versions []string
	for _, tag := range kept {
		versions = append(versions, tag[1:])
	}
	if len(versions) == 0 {
		// CI checkouts are shallow with no tags; the workflow fetches them.
		// Tags from another module also leave this list empty. A silent skip
		// in CI would turn the guard off without anyone noticing.
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
		// A Windows checkout (text=auto) has CRLF on disk; git show gives LF.
		want, found := changelogSection(lf(tagged), version)
		require.True(t, found, "v%s CHANGELOG.md has no [%s] section", version, version)
		got, _ := changelogSection(lf(changelog), version)
		if got != want {
			failures = append(failures, fmt.Sprintf("[%s] differs from v%s: %s", version, version, firstDifference(want, got)))
		}
	}
	assert.Empty(t, failures, "released CHANGELOG sections must not change; move the entry back under [Unreleased]:\n%s", strings.Join(failures, "\n"))
}

// Another repository's tags can sit in this clone. ckeletin-go's v0.11.0
// arrives when the framework remote is fetched. They are v0.* with a minor
// of at least 10, so the release guard would diff a changelog that was never
// this repo's. Ownership is the module path in that tag's go.mod. A tag with
// no go.mod is not this repo either.
func TestTagsForModuleKeepsOnlyThisRepository(t *testing.T) {
	repoGoMod := "module github.com/peiman/vaultmind\n\ngo 1.27.0\n"
	tags := []string{"v0.11.0", "v0.12.0", "v0.13.0"}
	goMod := func(tag string) (string, error) {
		switch tag {
		case "v0.11.0":
			// The vaultmind path appears only in a comment. The module line
			// is another repository, which is what the filter has to read.
			return "module github.com/peiman/ckeletin-go\n\ngo 1.22.0\n\n// see github.com/peiman/vaultmind\n", nil
		case "v0.12.0":
			return "module github.com/peiman/vaultmind\n\ngo 1.22.0\n", nil
		default:
			return "", fmt.Errorf("path 'go.mod' does not exist in '%s'", tag)
		}
	}

	got := tagsForModule(tags, repoGoMod, goMod)

	assert.Equal(t, []string{"v0.12.0"}, got)
}

// modulePath reads the path from the first module line of a go.mod file.
// The path is the second field, so a trailing comment does not count.
func modulePath(goMod string) (string, bool) {
	for _, line := range strings.Split(goMod, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "module" {
			return fields[1], true
		}
	}
	return "", false
}

// tagsForModule keeps tags whose go.mod module path equals the module path
// in repoGoMod. goMod returns that tag's go.mod text. An error, or a file
// with no module line, means the tag is not this repository. Tag order is
// preserved.
func tagsForModule(tags []string, repoGoMod string, goMod func(tag string) (string, error)) []string {
	want, ok := modulePath(repoGoMod)
	if !ok {
		return nil
	}
	var kept []string
	for _, tag := range tags {
		body, err := goMod(tag)
		if err != nil {
			continue
		}
		got, ok := modulePath(body)
		if !ok || got != want {
			continue
		}
		kept = append(kept, tag)
	}
	return kept
}
