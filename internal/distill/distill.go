// Package distill reads rendered episode markdown (the durable episodic
// substrate captured by internal/episode) and surfaces candidate transformation
// moments for arc distillation — plasticity step 2.
//
// It does the RELIABLE, mechanical half of distillation: parse verbatim turns,
// and flag high-precision candidate moments via keyword rules. It deliberately
// does NOT draft arcs or decide whether a candidate is already covered — the
// 2026-05-31 distillation review found extraction reliable but drafting and
// dedup-judgment unreliable to automate. Those stay with the mind (propose,
// never auto-write identity).
package distill

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// MinEpisodeBytes is the signal-filter floor. Episodes below it are sub-minute
// noise captures (the 2026-04-27 review found 2 of 5 such). NOTE: byte size is a
// proxy for signal, not signal itself — a short-but-dense episode would be
// wrongly dropped; revisit if that case appears.
const MinEpisodeBytes = 6000

// Turn is one verbatim message from an episode, with its 1-based position.
type Turn struct {
	Index     int
	Timestamp string
	Text      string
}

// Episode is the parsed verbatim content of a rendered episode .md.
type Episode struct {
	ID             string
	UserTurns      []Turn
	AssistantTurns []Turn
}

// ScanEpisodes globs episode .md files under dir, signal-filters them, parses
// each, and returns the propose-only candidate Report. A per-episode parse
// failure is recorded in Report.ParseErrors (and that episode skipped) rather
// than aborting the whole scan or being swallowed — the corpus tool stays
// robust while the error stays visible.
func ScanEpisodes(dir string) (Report, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return Report{}, fmt.Errorf("globbing episodes in %q: %w", dir, err)
	}
	kept := SignalFilter(paths, MinEpisodeBytes)
	r := Report{EpisodesScanned: len(paths), EpisodesKept: len(kept)}
	for _, p := range kept {
		ep, perr := ParseEpisodeFile(p)
		if perr != nil {
			r.ParseErrors = append(r.ParseErrors, fmt.Sprintf("%s: %v", filepath.Base(p), perr))
			continue
		}
		r.Candidates = append(r.Candidates, ExtractCandidates(ep)...)
	}
	return r, nil
}

// SignalFilter drops episode paths CONFIRMED below minBytes — the minimum-signal
// threshold that keeps noise captures out of the corpus. A path that can't be
// stat'd is KEPT, not silently dropped: the size is unknown, so the safe move is
// to let it through and let ParseEpisodeFile surface any real error downstream
// (dropping a possibly signal-dense episode on a transient stat error is the
// failure mode to avoid).
func SignalFilter(paths []string, minBytes int64) []string {
	kept := make([]string, 0, len(paths))
	for _, p := range paths {
		if info, err := os.Stat(p); err == nil && info.Size() < minBytes {
			continue // confirmed noise — drop
		}
		kept = append(kept, p)
	}
	return kept
}

var (
	idLineRe   = regexp.MustCompile(`^id:\s*(\S+)`)
	turnHeadRe = regexp.MustCompile(`^###\s+(\d+)\s+[—-]\s+(\S+)`) // "### 1 — <timestamp>"
)

// Section + turn-header markers MUST mirror internal/episode/render.go — they
// are the wire contract between the episode renderer (writer) and this parser
// (reader). They can't be a shared const: both packages are infrastructure, and
// ADR-009 forbids infrastructure→infrastructure imports, so the contract is
// duplicated by necessity. A render→parse round-trip test (in a package allowed
// to import both, e.g. the cmd that wires distillation) guards against drift;
// the fixture tests here pin the format this parser expects.
const (
	userSectionHead = "## User messages (verbatim)"
	asstSectionHead = "## Assistant responses (verbatim)"
)

// stripBlockquote removes the "> " markdown blockquote prefix the renderer adds
// to each user-message line, reconstructing the clean verbatim text.
func stripBlockquote(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l == ">" {
			lines[i] = ""
		} else {
			lines[i] = strings.TrimPrefix(l, "> ")
		}
	}
	return strings.Join(lines, "\n")
}

// ParseEpisodeFile parses a rendered episode .md into its verbatim user and
// assistant turns. The renderer writes user/assistant text un-truncated (only
// commit subjects are clipped), so the turns are faithful for quoting.
func ParseEpisodeFile(path string) (*Episode, error) {
	f, err := os.Open(path) // #nosec G304 -- caller-supplied episode path, read-only CLI tool
	if err != nil {
		return nil, fmt.Errorf("open episode: %w", err)
	}
	defer func() { _ = f.Close() }()

	ep := &Episode{ID: strings.TrimSuffix(filepath.Base(path), ".md")}
	var section string // "user" | "assistant" | ""
	var cur *Turn      // turn currently being accumulated
	var body []string  // accumulated body lines for cur
	inFrontmatter, seen := false, false

	flush := func() {
		if cur != nil {
			text := strings.Join(body, "\n")
			switch section {
			case "user":
				// The renderer blockquotes user messages ("> " per line); strip it
				// back to clean verbatim so quotes don't carry the markup.
				cur.Text = strings.TrimSpace(stripBlockquote(text))
				ep.UserTurns = append(ep.UserTurns, *cur)
			case "assistant":
				cur.Text = strings.TrimSpace(text)
				ep.AssistantTurns = append(ep.AssistantTurns, *cur)
			}
		}
		cur, body = nil, nil
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<24)
	for scanner.Scan() {
		line := scanner.Text()

		// Frontmatter: read the id, skip the rest.
		if line == "---" {
			if !seen {
				inFrontmatter, seen = true, true
				continue
			}
			inFrontmatter = false
			continue
		}
		if inFrontmatter {
			if m := idLineRe.FindStringSubmatch(line); m != nil {
				ep.ID = m[1]
			}
			continue
		}

		// Only the two message-section headings are structural. Crucially, a bare
		// "## …" or "### …" is NOT treated as structure — assistant turns render
		// raw (un-blockquoted) and routinely contain markdown headings ("## Push",
		// "### note"), which must stay in the turn body, not truncate it. A new
		// turn starts ONLY on a real turn header (turnHeadRe: "### N — <ts>").
		switch {
		case line == userSectionHead:
			flush()
			section = "user"
		case line == asstSectionHead:
			flush()
			section = "assistant"
		case section == "":
			// Outside the message sections (Metadata/Commits/etc.) — ignore.
		default:
			if m := turnHeadRe.FindStringSubmatch(line); m != nil {
				flush()
				idx, err := strconv.Atoi(m[1])
				if err != nil {
					continue // not a real turn header (overflow) — skip, don't fabricate
				}
				cur = &Turn{Index: idx, Timestamp: m[2]}
			} else if cur != nil {
				body = append(body, line)
			}
		}
	}
	flush()
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan episode: %w", err)
	}
	return ep, nil
}
