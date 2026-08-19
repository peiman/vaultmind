package query

import (
	"bytes"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/memory"
	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The header used to read:
//
//	Context from: reference-probe-before-commit (0 items, 900/900 tokens)
//
// A full budget and nothing in it. Both halves were true of the PACK and false
// of what the caller received: UsedTokens counts the target's body, and
// pointers-only mode then discards that body without saying so. Every reach-hook
// injection carried that line, so the agent was told 900 tokens of context had
// been spent on its behalf while receiving five titles.
func TestContextHeader_SaysWhenTheTextIsWithheld(t *testing.T) {
	body := strings.Repeat("the note text that was assembled and not shown. ", 60)
	result := &AskResult{
		Query:            "q",
		TopHitConfidence: ConfidenceModerate,
		VaultNoteCount:   500,
		TopHits:          []retrieval.ScoredResult{{ID: "note-a", Title: "A Note", Path: "a.md", Score: 0.5}},
		Context: &memory.ContextPackResult{
			TargetID:     "note-a",
			BudgetTokens: 900,
			UsedTokens:   900,
			Target: &memory.ContextPackTarget{
				Frontmatter: map[string]any{"type": "reference", "title": "A Note"},
				Body:        body,
			},
		},
	}

	var buf bytes.Buffer
	require.NoError(t, formatAskWithOptions(result, &buf, formatOpts{pointersOnly: true}))
	out := buf.String()

	assert.NotContains(t, out, "900/900 tokens",
		"reporting a spent budget for text the caller never got is the lie this fixes")
	assert.Contains(t, out, "withheld",
		"say that the text exists and was not shown")
	assert.Contains(t, out, "--read",
		"and how to get it — a withheld body with no remedy is just bad news")
}

// When bodies ARE delivered, the used/budget accounting is honest and stays.
func TestContextHeader_KeepsTheBudgetLineWhenBodiesAreDelivered(t *testing.T) {
	result := &AskResult{
		Query:            "q",
		TopHitConfidence: ConfidenceModerate,
		VaultNoteCount:   500,
		TopHits:          []retrieval.ScoredResult{{ID: "note-a", Title: "A Note", Path: "a.md", Score: 0.5}},
		Context: &memory.ContextPackResult{
			TargetID:     "note-a",
			BudgetTokens: 900,
			UsedTokens:   120,
			Target: &memory.ContextPackTarget{
				Frontmatter: map[string]any{"type": "reference", "title": "A Note"},
				Body:        "short body",
			},
		},
	}

	var buf bytes.Buffer
	require.NoError(t, formatAskWithOptions(result, &buf, formatOpts{}))
	out := buf.String()
	assert.Contains(t, out, "120/900 tokens")
	assert.NotContains(t, out, "withheld")
}

// A pack with no body text at all must not claim anything was withheld.
func TestContextHeader_NoFalseWithheldClaimWhenThereIsNoText(t *testing.T) {
	result := &AskResult{
		Query:            "q",
		TopHitConfidence: ConfidenceModerate,
		VaultNoteCount:   500,
		TopHits:          []retrieval.ScoredResult{{ID: "note-a", Title: "A Note", Path: "a.md", Score: 0.5}},
		Context: &memory.ContextPackResult{
			TargetID:     "note-a",
			BudgetTokens: 900,
			UsedTokens:   12, // frontmatter only
			Target: &memory.ContextPackTarget{
				Frontmatter: map[string]any{"type": "reference", "title": "A Note"},
			},
		},
	}

	var buf bytes.Buffer
	require.NoError(t, formatAskWithOptions(result, &buf, formatOpts{pointersOnly: true}))
	assert.NotContains(t, buf.String(), "withheld")
}
