package passage

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnits_SpansAndGoverningHeadings(t *testing.T) {
	for _, title := range []string{"Title", " TITLE ", ""} {
		body := "# Title ###\n\nIntro. Next!\n\n## Parent\n\nParent text.\n\n### Child\n\n- first\n- second\n\n```go\nignored.\n```\n\nAfter fence."
		units := Units(body, title)
		assert.Len(t, units, 6)
		var texts []string
		for _, u := range units {
			texts = append(texts, body[u.Start:u.End])
		}
		assert.Equal(t, []string{"Intro.", "Next!", "Parent text.", "- first", "- second", "After fence."}, texts)
		assert.Equal(t, units[0].Section, units[1].Section)
		assert.NotEqual(t, units[4].Section, units[5].Section)
		assert.True(t, strings.HasSuffix(units[3].Heading, "## Parent\n### Child"))
		if title != "" {
			assert.Empty(t, units[0].Heading)
		} else {
			assert.Equal(t, "# Title ###", units[0].Heading)
		}
	}
	assert.Empty(t, Units("", "Title"))
	assert.Empty(t, Units("```\nunfinished", "Title"))
}

func TestUnits_SetextAndTitleNormalization(t *testing.T) {
	for _, body := range []string{"  TITLE \n======\n\nIntro.\n\nWarning\n-------\n\nAnswer.", "# title\n\nIntro.\n\nWarning\n-------\n\nAnswer."} {
		units := Units(body, "Title")
		assert.Len(t, units, 2)
		assert.Empty(t, units[0].Heading)
		assert.Equal(t, "Warning\n-------", units[1].Heading)
	}
	for _, tc := range []struct {
		a, b string
		want bool
	}{{"A title", "a TITLE", true}, {"A title", "different", false}, {"", "", false}, {"Title", "Title extra", false}} {
		assert.Equal(t, tc.want, sameTitle(tc.a, tc.b))
	}
	assert.False(t, h1RepeatsTitle("# Other", ""))
}

func TestPassageSentenceBoundaries(t *testing.T) {
	previous := regexp.MustCompile(`[.!?][ \t]+|\n[ \t]*(?:[-*+] |[0-9]+\. )`)
	for _, text := range []string{"", "One. Two!\t Three?  End.", "A. B. C. D. E. ", "Number 1.23 intact.", "Intro\n  - bullet\n\t* next\n+ last", "Intro\n  123. bullet\n1. next", "No.\nEnd", "Intro\n -\n\t2.\n3", "Intro\n + odd. Next.", "背景検索は重要である。"} {
		want := []int{0}
		for _, loc := range previous.FindAllStringIndex(text, -1) {
			at := loc[1]
			if text[loc[0]] == '\n' {
				at = loc[0] + 1
			}
			want = append(want, at)
		}
		assert.Equal(t, want, SentenceStarts(text), "%q", text)
	}
}
