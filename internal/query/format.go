package query

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/peiman/vaultmind/internal/retrieval"
)

// FormatAsk writes a human-readable text representation of an AskResult.
func FormatAsk(result *AskResult, w io.Writer) error {
	return formatAskWithOptions(result, w, formatOpts{})
}

// FormatAskExplain is like FormatAsk but prints per-hit lane breakdowns
// (which sub-retrievers scored the note, each lane's raw 1/(K+rank), and
// how many lanes went into the mean). Lets you see the fusion math on
// the command line instead of piping --json through jq/python — closes
// the diagnostic gap that had operators rebuilding ad-hoc tooling for
// every ranking investigation.
func FormatAskExplain(result *AskResult, w io.Writer) error {
	return formatAskWithOptions(result, w, formatOpts{explain: true})
}

// FormatAskPointersOnly is like FormatAsk but skips body content for both
// the target note and every context-pack item — output is title + id +
// type only. Used by the SessionStart hook so the body of "what matters
// most right now" is never preloaded; the agent has to query for it.
//
// This converts the dogfood rule (use vaultmind ask before answering) from
// honor-system discipline (manifesto principle 9: discipline does not
// survive time pressure) to design: every body-read becomes an explicit,
// logged activation event the agent had to choose, rather than something
// the preload silently satisfied. Closes the trap documented in
// arc-plasticity-gap-from-inside and the 2026-04-25 design signal under
// step 3 of reference-plasticity-priority-order.
//
// Retrieval is unchanged — search hits, context-pack assembly, and
// scoring all happen normally. Only the rendering omits bodies. The
// hint at the bottom names the next move (an explicit ask) so the agent
// knows the loop closes by querying, not by waiting for more context.
func FormatAskPointersOnly(result *AskResult, w io.Writer) error {
	return formatAskWithOptions(result, w, formatOpts{pointersOnly: true})
}

// FormatAskPreview renders each ranked hit with a one-line snippet from
// the note body underneath the title — bridging the gap between
// pointers-only (titles, no body context) and full-body Ask (3000+
// tokens of context pack). Closes the AX gap named in the felt-
// experience inventory: with pointers-only I see ids and titles but
// often can't tell what a note actually is until I query its body.
// The snippet was already being populated by every retriever; this
// just renders it.
func FormatAskPreview(result *AskResult, w io.Writer) error {
	return formatAskWithOptions(result, w, formatOpts{preview: true})
}

// FormatAskRead renders the ask menu (search header + ranked hits) plus
// the body of one specific note inline — the single-command shortcut
// for the probe→read workflow when the agent already knows which hit
// from the menu they want. Backs `vaultmind ask --read N` and
// `vaultmind ask --read <id>`. The note argument is the resolved
// chosen hit's full body; the caller fetches it (cmd/ask.go) so this
// renderer stays in the format layer without taking a DB dependency.
//
// FormatAskRead always renders without explain. To get per-lane RRF
// math under each hit when --read and --explain are combined, use
// FormatAskReadWithOptions.
func FormatAskRead(result *AskResult, note *index.FullNote, w io.Writer) error {
	return FormatAskReadWithOptions(result, note, w, false)
}

// FormatAskReadWithOptions is the explain-aware form of FormatAskRead.
// When explain is true, each ranked hit in the menu shows its per-lane
// RRF contribution underneath — matching what `vaultmind ask --explain`
// renders in default mode. Round-3 inter-agent review caught that
// `--read N --explain` was silently dropping --explain because runAskRead
// short-circuited before the explain path was read; this is the
// rendering side of the fix.
func FormatAskReadWithOptions(result *AskResult, note *index.FullNote, w io.Writer, explain bool) error {
	if err := writeAskHeader(w, result); err != nil {
		return err
	}
	if err := writeAskHits(w, result.TopHits, formatOpts{explain: explain}); err != nil {
		return err
	}
	if note == nil {
		return nil
	}
	if _, err := fmt.Fprintf(w, "\n%s (%s) — %s\n", note.ID, note.Type, note.Title); err != nil {
		return err
	}
	if note.Body != "" {
		if _, err := fmt.Fprintf(w, "\n%s\n", note.Body); err != nil {
			return err
		}
	}
	return nil
}

type formatOpts struct {
	explain      bool
	pointersOnly bool
	preview      bool
}

func formatAskWithOptions(result *AskResult, w io.Writer, opts formatOpts) error {
	// When confidence is below "moderate" — either "weak" (top-1 barely
	// ahead) or "no clear winner" (top-1 essentially tied with the
	// field) — committing to top-1 is misleading. Auto-degrade to
	// pointers-only so the system doesn't spend the agent's working-
	// context budget rendering bodies (and a context-pack of neighbors)
	// around a top-1 the confidence label has already said we shouldn't
	// trust.
	//
	// Round-2 review caught the no_match case (1762-token pack around
	// an unrelated note); round-3 review caught the binding constraint:
	// the kanye-class FTS false positive lands at "weak", not
	// "no_match", so degrading only no_match leaves the louder problem
	// untouched. Round-3 evaluator's framing: "weak is closer to no
	// clear winner than to moderate in terms of what the agent should
	// do with the result." The escape hatch for "I want the body of a
	// weak top hit anyway" is `vaultmind ask "X" --read 1` (shipped
	// the same round) — explicit override, single command, exactly the
	// shape that says "I read the menu and I want this body."
	//
	// Defense-in-depth philosophy: the confidence label alone is a
	// signal the agent has to read; this makes the rendering itself
	// reflect the same epistemic posture without the agent having to
	// remember to check.
	if result.TopHitConfidence == ConfidenceNoMatch || result.TopHitConfidence == ConfidenceWeak {
		opts.pointersOnly = true
	}
	if err := writeAskHeader(w, result); err != nil {
		return err
	}
	if err := writeAskHits(w, result.TopHits, opts); err != nil {
		return err
	}
	if result.Context == nil {
		return nil
	}
	withBodies := countItemsWithBodies(result.Context.Context, opts)
	if err := writeContextHeader(w, result.Context, withBodies, opts); err != nil {
		return err
	}
	if err := writeContextTarget(w, result.Context.Target, opts); err != nil {
		return err
	}
	if err := writeContextItems(w, result.Context.Context, opts); err != nil {
		return err
	}
	return writeContextFooter(w, result.Context, withBodies, opts)
}

// writeAskHeader emits "Search: ... [top-hit confidence: ...]". Confidence
// surfaces only when computable; ConfidenceNoMatch gets a longer label so
// the agent doesn't read it as another "weak" synonym.
func writeAskHeader(w io.Writer, result *AskResult) error {
	header := fmt.Sprintf("Search: %q (%d hits)", result.Query, len(result.TopHits))
	if result.TopHitConfidence != "" {
		// Tiers that auto-degrade to pointers-only get an explanatory
		// suffix so the agent reading the output knows BOTH what the
		// label means AND why the rendering is what it is. Without the
		// suffix an agent who doesn't know the auto-degrade exists
		// might see menu+pointers and wonder whether their --budget
		// was honored, whether the system is broken, or whether
		// something happened that they should know about. Round-4
		// inter-agent review caught this gap on the weak label
		// specifically — the no_match label already carried its
		// suffix from earlier.
		switch result.TopHitConfidence {
		case ConfidenceNoMatch:
			header += "  [top-hit confidence: no clear winner — top results essentially tied]"
		case ConfidenceWeak:
			header += "  [top-hit confidence: weak — body suppressed; use --read N to override]"
		default:
			header += fmt.Sprintf("  [top-hit confidence: %s]", result.TopHitConfidence)
		}
	}
	_, err := fmt.Fprintln(w, header)
	return err
}

// writeAskHits emits one line per ranked hit, optionally with a snippet
// (--preview) and per-lane RRF math (--explain) underneath.
func writeAskHits(w io.Writer, hits []retrieval.ScoredResult, opts formatOpts) error {
	for _, h := range hits {
		if _, err := fmt.Fprintf(w, "  %.2f  %-40s  %s\n", h.Score, h.ID, h.Title); err != nil {
			return err
		}
		if opts.preview && h.Snippet != "" {
			snippet := previewSnippet(h.Snippet, 110)
			if snippet != "" {
				if _, err := fmt.Fprintf(w, "        ↳ %s\n", snippet); err != nil {
					return err
				}
			}
		}
		if opts.explain && len(h.Components) > 0 {
			if err := writeLaneBreakdown(w, h.Components); err != nil {
				return err
			}
		}
	}
	return nil
}

// countItemsWithBodies returns the number of context items whose Body
// is actually rendered in the current opts. Returns 0 in pointers-only
// mode (no bodies render at all).
func countItemsWithBodies(items []memory.ContextItem, opts formatOpts) int {
	if opts.pointersOnly {
		return 0
	}
	n := 0
	for _, item := range items {
		if item.BodyIncluded && item.Body != "" {
			n++
		}
	}
	return n
}

// writeContextHeader emits "Context from: ... (N items[, M with bodies], used/budget tokens)".
// The M-with-bodies count appears only when not all items got bodies —
// keeps the line terse when nothing was truncated. See the truncation
// footer for the matching remedy hint.
func writeContextHeader(w io.Writer, ctx *memory.ContextPackResult, withBodies int, opts formatOpts) error {
	suffix := ""
	if !opts.pointersOnly && len(ctx.Context) > 0 && withBodies < len(ctx.Context) {
		suffix = fmt.Sprintf(", %d with bodies", withBodies)
	}
	_, err := fmt.Fprintf(w, "\nContext from: %s (%d items%s, %d/%d tokens)\n",
		ctx.TargetID, len(ctx.Context), suffix, ctx.UsedTokens, ctx.BudgetTokens)
	return err
}

// writeContextTarget emits the target note's [type] title plus body
// (unless pointers-only). Tolerates a nil target.
func writeContextTarget(w io.Writer, target *memory.ContextPackTarget, opts formatOpts) error {
	if target == nil {
		return nil
	}
	noteType, _ := target.Frontmatter["type"].(string)
	title, _ := target.Frontmatter["title"].(string)
	if _, err := fmt.Fprintf(w, "  [%s] %s\n", noteType, title); err != nil {
		return err
	}
	if !opts.pointersOnly && target.Body != "" {
		_, err := fmt.Fprintf(w, "    %s\n", Truncate(target.Body, 120))
		return err
	}
	return nil
}

// writeContextItems emits one block per context-pack neighbor —
// [type] title, plus body when included by the budget.
func writeContextItems(w io.Writer, items []memory.ContextItem, opts formatOpts) error {
	for _, item := range items {
		noteType, _ := item.Frontmatter["type"].(string)
		title, _ := item.Frontmatter["title"].(string)
		if _, err := fmt.Fprintf(w, "  [%s] %s\n", noteType, title); err != nil {
			return err
		}
		if !opts.pointersOnly && item.BodyIncluded && item.Body != "" {
			if _, err := fmt.Fprintf(w, "    %s\n", Truncate(item.Body, 120)); err != nil {
				return err
			}
		}
	}
	return nil
}

// writeContextFooter emits the closing hint — either a budget-truncation
// note (when bodies were omitted to fit the budget) or the pointers-only
// menu hint. At most one fires.
func writeContextFooter(w io.Writer, ctx *memory.ContextPackResult, withBodies int, opts formatOpts) error {
	if !opts.pointersOnly && len(ctx.Context) > 0 && withBodies < len(ctx.Context) {
		omitted := len(ctx.Context) - withBodies
		_, err := fmt.Fprintf(w,
			"\n(%d item%s above had body omitted to fit the %d-token budget — increase --budget to see more)\n",
			omitted, pluralS(omitted), ctx.BudgetTokens)
		return err
	}
	if opts.pointersOnly {
		_, err := fmt.Fprintf(w, "\n(pointers only — run `vaultmind ask <query>` against any id above to read the body)\n")
		return err
	}
	return nil
}

// pluralS returns "s" for non-singular counts so messages read naturally
// ("1 item above had body omitted" vs "3 items above had bodies omitted"
// — the verb form differs but the script doesn't, so the suffix carries
// the agreement). Returns "" for n==1; "s" otherwise.
func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// previewSnippet prepares a retriever-supplied snippet for one-line
// rendering under a ranked hit (--preview). Strips leading markdown
// headings and blank lines that waste the first ~10 visible characters
// — common pattern is "# Title\n\n## Overview\n\n<actual content>"
// where the headings repeat what we already rendered above as the hit's
// title. Also normalises internal newlines to single spaces so the
// preview stays one line. Truncates last so the visible content
// dominates the available width.
func previewSnippet(s string, maxLen int) string {
	s = stripLeadingHeadings(s)
	// Collapse internal newlines so the preview stays one line.
	s = strings.ReplaceAll(s, "\n", " ")
	// Collapse runs of whitespace produced by the line collapse.
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	s = strings.TrimSpace(s)
	return Truncate(s, maxLen)
}

// stripLeadingHeadings drops leading markdown heading lines (## Foo,
// # Bar) and the blank lines around them, returning what's left from
// the first non-heading content line onward. Pure string work — no
// markdown parsing — so it's cheap and predictable.
func stripLeadingHeadings(s string) string {
	for {
		s = strings.TrimLeft(s, " \t\n\r")
		if s == "" {
			return ""
		}
		// Heading line starts with one-to-six '#' followed by a space.
		if !strings.HasPrefix(s, "#") {
			return s
		}
		hashEnd := 0
		for hashEnd < len(s) && hashEnd < 6 && s[hashEnd] == '#' {
			hashEnd++
		}
		if hashEnd >= len(s) || s[hashEnd] != ' ' {
			return s
		}
		// Skip past this heading line (to and including the next \n).
		nl := strings.IndexByte(s, '\n')
		if nl < 0 {
			return ""
		}
		s = s[nl+1:]
	}
}

// Truncate shortens a string to maxLen runes, appending "..." if truncated.
func Truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

// writeLaneBreakdown renders a single hit's per-lane RRF contributions in a
// deterministic form: lanes sorted alphabetically so diffs are reviewable,
// "mean of N" so readers can spot coverage imbalance at a glance (a hit
// with "mean of 2" next to one with "mean of 4" is the 2026-04-24 failure
// mode made visible without running SQL).
func writeLaneBreakdown(w io.Writer, components map[string]float64) error {
	lanes := make([]string, 0, len(components))
	for name := range components {
		lanes = append(lanes, name)
	}
	sort.Strings(lanes)

	if _, err := fmt.Fprint(w, "    lanes:"); err != nil {
		return err
	}
	for _, name := range lanes {
		if _, err := fmt.Fprintf(w, " %s=%.5f", name, components[name]); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(w, "  mean of %d\n", len(lanes))
	return err
}
