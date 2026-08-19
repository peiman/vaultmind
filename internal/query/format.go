package query

import (
	"fmt"
	"io"
	"math"
	"sort"
	"strings"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/peiman/vaultmind/internal/noisefloor"
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
// the plasticity-gap arc and the 2026-04-25 design signal under
// step 3 of the plasticity roadmap.
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
	// --read exists to render a body and always does, so the header is told
	// exactly that rather than left to infer it from the confidence label.
	if err := writeAskHeader(w, result, explain, true); err != nil {
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
	//
	// Exception: a vault below the calibration gate. There the low-confidence
	// verdict is not a finding about the top hit, it is an artifact of judging a
	// handful of notes against a floor calibrated for a large corpus — and a
	// vault that small has no working-context budget worth protecting. Withhold
	// the body there and a new user's first query, on the vault `init` just
	// scaffolded for them, returns the right note and refuses to show it.
	// Same decision the telemetry records, from the same function — see
	// AskResult.BodyDecision. Two copies of this rule would let the log claim a
	// body was delivered on a render that withheld it.
	// Captured before the mutation below: after it, opts.pointersOnly means
	// "no bodies will render" rather than "the caller asked for ids", and the
	// header needs the caller's original request to decide what to promise.
	delivered, _ := result.BodyDecision(opts.pointersOnly)
	if !delivered {
		opts.pointersOnly = true
	}
	if err := writeAskHeader(w, result, opts.explain, delivered); err != nil {
		return err
	}
	if err := writeAskHits(w, result.TopHits, opts); err != nil {
		return err
	}
	if result.Context == nil {
		return nil
	}
	if err := writeContextHeader(w, result.Context, opts); err != nil {
		return err
	}
	if err := writeContextTarget(w, result.Context.Target, opts); err != nil {
		return err
	}
	if err := writeContextItems(w, result.Context.Context, opts); err != nil {
		return err
	}
	return writeContextFooter(w, result.Context, opts)
}

// writeAskHeader emits "Search: ... [relevance: ...]". In noise-floor mode the
// label is band-normalized — z = (top_cosine − N)/σ, glossed as "Nσ above/below
// the off-topic noise floor" so the agent reads both the tier and its magnitude.
// ConfidenceNoMatch gets a longer label so it isn't read as another "weak"
// synonym. With explain set, a reconstruction line shows the z derivation (and
// surfaces a stale/cross-vault N). Without a noise floor (keyword-only), it
// keeps the legacy RRF-gap vocabulary.
func writeAskHeader(w io.Writer, result *AskResult, explain, bodyDelivered bool) error {
	// bodyDelivered is the ANSWER, passed in — not a question this function
	// re-derives. It took `callerAsked` and computed the answer itself, which
	// cannot express "a body is definitely coming": --read always renders one,
	// so the header printed "body suppressed; use --read N to override" directly
	// above the body --read had just delivered. A caller that knows what is
	// about to happen has to be able to say so.
	suppressedNote := ""
	if !bodyDelivered {
		suppressedNote = " — body suppressed; use --read N to override"
	}
	header := fmt.Sprintf("Search: %q (%d hits)", result.Query, len(result.TopHits))
	if result.NoiseFloorApplied {
		// Below the calibration gate, report the absence of a judgement rather
		// than a judgement. Quoting σ against a handful of notes dresses a
		// shipped default up as a measurement of THIS vault, and the tiers that
		// would be reported there ("weak", "nothing relevant") both read as
		// verdicts on the top hit when the real state is "not enough corpus to
		// have an opinion". Confident tiers are left alone: clearing a
		// conservative floor on a small vault is still information.
		if result.tooSmallToJudge() &&
			(result.TopHitConfidence == ConfidenceWeak || result.TopHitConfidence == ConfidenceNoMatch) {
			header += fmt.Sprintf(
				"  [relevance: not yet measurable — %d notes is below the %d needed to calibrate this vault; showing the top hit anyway]",
				result.VaultNoteCount, noisefloor.MinCalibNotes)
			_, err := fmt.Fprintln(w, header)
			return err
		}
		switch result.TopHitConfidence {
		case ConfidenceNoMatch:
			header += fmt.Sprintf("  [relevance: nothing relevant — top hit at/below the off-topic noise floor (z=%+.2f); body suppressed]", result.RelevanceZ)
		case ConfidenceWeak:
			header += fmt.Sprintf("  [relevance: weak (z=%+.2f, %s)%s]", result.RelevanceZ, zGloss(result.RelevanceZ), suppressedNote)
		default:
			header += fmt.Sprintf("  [relevance: %s (z=%+.2f, %s)]", result.TopHitConfidence, result.RelevanceZ, zGloss(result.RelevanceZ))
		}
		if result.LowContrastVault && result.TopHitConfidence == ConfidenceWeak {
			// A tight vault's notes are so self-similar the embedder can't spread a
			// correct hit far above the noise floor, so genuine matches read weak.
			// Say so, once, so the agent doesn't read weak as "nothing here".
			if bodyDelivered {
				header += "\n  [tight vault: a weak top hit here is often the best available correct match, not 'nothing relevant' — body included below]"
			} else {
				header += "\n  [tight vault: a weak top hit here is often the best available correct match, not 'nothing relevant' — use --read 1 for the body]"
			}
		}
		if explain {
			// Reconstruct the derivation so the operator can see the inputs — a
			// stale or cross-vault N (the wrong vault's floor) shows up here.
			header += fmt.Sprintf("\n  relevance math: top_cosine=%.3f  N=%.3f  σ=%.3f  →  z=(%.3f−%.3f)/%.3f=%+.2f",
				result.TopHitCosine, result.NoiseFloor, result.NoiseFloorSigma,
				result.TopHitCosine, result.NoiseFloor, result.NoiseFloorSigma, result.RelevanceZ)
		}
		_, err := fmt.Fprintln(w, header)
		return err
	}
	if result.TopHitConfidence != "" {
		// RRF-gap fallback (keyword-only mode). Tiers that auto-degrade to
		// pointers-only get an explanatory suffix so the agent reading the
		// output knows BOTH what the label means AND why the rendering is
		// what it is. Round-4 inter-agent review caught this gap on the weak
		// label specifically — the no_match label already carried its suffix.
		switch result.TopHitConfidence {
		case ConfidenceNoMatch:
			header += "  [top-hit confidence: no clear winner — top results essentially tied]"
		case ConfidenceWeak:
			header += "  [top-hit confidence: weak" + suppressedNote + "]"
		default:
			header += fmt.Sprintf("  [top-hit confidence: %s]", result.TopHitConfidence)
		}
	}
	_, err := fmt.Fprintln(w, header)
	return err
}

// zGloss renders a signed z as a magnitude + direction relative to the noise
// floor — "1.9σ above the off-topic noise floor" — so the tier word carries its
// quantitative meaning in the same breath. Negative z (a hit below N but above
// the silence floor) reads "below".
func zGloss(z float64) string {
	dir := "above"
	if z < 0 {
		dir = "below"
	}
	return fmt.Sprintf("%.1fσ %s the off-topic noise floor", math.Abs(z), dir)
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

// bodyIndent prefixes note text so it reads as subordinate to its title line.
const bodyIndent = "    "

// indentBody indents EVERY line of a note's text, not just the first.
//
// Only the first line used to be indented, so a multi-line excerpt — a
// Principle section running to two paragraphs, say — emitted ragged
// continuation lines flush against the left margin, where they were
// indistinguishable from the "  [type] Title" lines that separate notes. That
// made the render ambiguous to a reader and unparseable to a test, which is
// the more important half: an invariant nobody can check is a convention.
func indentBody(body string) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		lines[i] = bodyIndent + line
	}
	return strings.Join(lines, "\n")
}

// deliveryCounts is what the header states, tallied the way a reader would
// tally it from the rendered blocks.
//
// It exists as one value because the header and the footer used to count
// independently and drifted apart: the header called an excerpt "not a body"
// while the footer called it "not omitted", so a pack of pure excerpts read as
// "0 with bodies" with no footer to explain it. One count, two renderers.
type deliveryCounts struct {
	// Notes is every block that renders, the target included. len(ctx.Context)
	// excludes the target, which is why the old header could say "2 items"
	// above three blocks — and the target is the one an agent reads first.
	Notes int
	// Delivered is the blocks that carry note text. An excerpt counts: the
	// agent received words it can act on.
	Delivered int
	// Excerpted is the subset of Delivered that was capped. This is the
	// distinction the old header was reaching for and inverted — an excerpt is
	// not the whole note, so saying "delivered in full" of one would be the
	// same lie in the other direction. Both facts are reported; neither is
	// allowed to stand in for the other.
	Excerpted int
}

// TitlesOnly is the blocks that rendered a title and no text.
func (c deliveryCounts) TitlesOnly() int { return c.Notes - c.Delivered }

// countDelivery mirrors writeContextTarget and writeContextItems exactly: a
// block counts as delivered under precisely the conditions that make those
// functions print text. TestContextHeader_EveryNumberIsRecountable is what
// keeps the mirror honest — it recounts the rendered output and compares.
func countDelivery(ctx *memory.ContextPackResult, opts formatOpts) deliveryCounts {
	var c deliveryCounts
	if ctx.Target != nil {
		c.Notes++
		if !opts.pointersOnly && ctx.Target.Body != "" {
			c.Delivered++
			if ctx.Target.BodyExcerpted {
				c.Excerpted++
			}
		}
	}
	for _, item := range ctx.Context {
		c.Notes++
		if !opts.pointersOnly && item.BodyIncluded && item.Body != "" {
			c.Delivered++
			if item.BodyExcerpted {
				c.Excerpted++
			}
		}
	}
	return c
}

// deliveryPhrase renders the counts as the one sentence an agent reads to
// decide whether it has enough to act on, or has to go fetch something.
//
// "as excerpts" is load-bearing: it says the full note exists and is one
// `note get` away. That is the only thing the reader can act on, so it is the
// only qualifier the line carries.
func deliveryPhrase(c deliveryCounts) string {
	s := fmt.Sprintf("%d delivered", c.Delivered)
	switch {
	case c.Delivered == 0:
	case c.Excerpted == c.Delivered:
		s += " as excerpt" + pluralS(c.Excerpted)
	case c.Excerpted > 0:
		s += fmt.Sprintf(", %d as excerpt%s", c.Excerpted, pluralS(c.Excerpted))
	default:
		s += " in full"
	}
	if t := c.TitlesOnly(); t > 0 && c.Delivered > 0 {
		s += fmt.Sprintf(", %d title%s only", t, pluralS(t))
	}
	return s
}

// writeContextHeader emits "Context from: <id> — N notes, M delivered ...".
//
// Every number here is recountable from the blocks below it. That is the whole
// design rule, and it is enforced by a test rather than by care: three separate
// defects this session were a header describing internal state that no longer
// matched the render, and each was individually plausible.
//
// What is deliberately NOT here: the budget denominator. "982/6000" describes
// the caller's knob, not what arrived, and it produced the worst line the tool
// ever printed — "0 items, 900/900 tokens", a full budget with nothing in it,
// on every reach-hook injection. The budget is named by the footer, and only
// when it actually dropped a note; a number with no action attached is noise
// at best and, as it turned out, a lie at worst.
func writeContextHeader(w io.Writer, ctx *memory.ContextPackResult, opts formatOpts) error {
	c := countDelivery(ctx, opts)
	tokens := ""
	if c.Delivered > 0 {
		tokens = fmt.Sprintf(" (%d tok)", ctx.UsedTokens)
	}
	_, err := fmt.Fprintf(w, "\nContext from: %s — %d note%s, %s%s\n",
		ctx.TargetID, c.Notes, pluralS(c.Notes), deliveryPhrase(c), tokens)
	return err
}

// packHoldsText reports whether the pack actually assembled note TEXT — as
// opposed to frontmatter alone, which carries no body to withhold. Claiming
// text was withheld when there was none would be its own small lie.
func packHoldsText(ctx *memory.ContextPackResult) bool {
	if ctx.Target != nil && ctx.Target.Body != "" {
		return true
	}
	for _, item := range ctx.Context {
		if item.Body != "" {
			return true
		}
	}
	return false
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
		// An excerpt already carries its own budget-derived bound. Truncating it
		// again here would cut the target — the slot the agent reads first — at
		// 120 runes, discarding the half that carries the rule.
		body := target.Body
		if !target.BodyExcerpted {
			body = Truncate(body, itemBodyPreviewRunes)
		}
		_, err := fmt.Fprintf(w, "%s\n", indentBody(body))
		return err
	}
	return nil
}

// itemBodyPreviewRunes bounds a full body rendered as a neighbour preview.
// Excerpts skip it: they carry their own budget-derived bound.
const itemBodyPreviewRunes = 120

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
			// An excerpt is already bounded by ExcerptTokens and was chosen to be
			// the passage worth reading. Re-truncating it here would cut it to a
			// third and land mid-word — computing the right text and then
			// declining to show it.
			body := item.Body
			if !item.BodyExcerpted {
				body = Truncate(body, itemBodyPreviewRunes)
			}
			if _, err := fmt.Fprintf(w, "%s\n", indentBody(body)); err != nil {
				return err
			}
		}
	}
	return nil
}

// writeContextFooter emits the closing hint — either a budget-truncation
// note (when bodies were omitted to fit the budget) or the pointers-only
// menu hint. At most one fires.
// writeContextFooter states the cause and the remedy for whatever the header
// reported missing. The header says WHAT arrived; the footer says why the rest
// did not and which flag changes it. A gap with no remedy is just bad news.
//
// The branches are mutually exclusive and ordered by what the reader can do
// about them, most actionable first.
func writeContextFooter(w io.Writer, ctx *memory.ContextPackResult, opts formatOpts) error {
	if opts.pointersOnly {
		// The pack assembled this text and then threw it away. Saying so — with
		// the token count, so the size of what was discarded is visible — is the
		// difference between a flag doing its job and a silent no-op.
		//
		// Both remedies, because they do different things: --read fetches the top
		// hit inline, `note get` fetches whichever of the titles above the agent
		// actually wants. Offering only the first leaves seven of eight notes
		// unreachable without re-running the query.
		if packHoldsText(ctx) {
			_, err := fmt.Fprintf(w,
				"\n(pointers only: %d tokens assembled and withheld by --pointers-only — drop the flag, "+
					"or read one with --read N / `vaultmind note get <id>`)\n",
				ctx.UsedTokens)
			return err
		}
		_, err := fmt.Fprintf(w, "\n(pointers only — run `vaultmind note get <id>` against any id above to read the body)\n")
		return err
	}
	// An excerpted note was not omitted; it carries text. Counting it as omitted
	// would under-report what arrived, the mirror of the header over-reporting.
	if omitted := countDelivery(ctx, opts).TitlesOnly(); omitted > 0 {
		_, err := fmt.Fprintf(w,
			"\n(%d note%s above had no room in the %d-token budget — raise --budget, or lower --excerpt to fit more)\n",
			omitted, pluralS(omitted), ctx.BudgetTokens)
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
