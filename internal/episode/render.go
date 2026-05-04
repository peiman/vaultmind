package episode

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/peiman/vaultmind/internal/schema"
)

// RenderMarkdown turns a parsed Episode into a vault-ready markdown file.
// Schema is intentionally simple: frontmatter + section-per-signal-type.
// No distillation, no prose — the next layer (arc distillation) will consume
// this structure.
func RenderMarkdown(ep *Episode) string {
	var b strings.Builder

	b.WriteString("---\n")
	fmt.Fprintf(&b, "id: %s\n", ep.ID)
	b.WriteString("type: episode\n")
	fmt.Fprintf(&b, "session_id: %s\n", ep.SessionID)
	fmt.Fprintf(&b, "started_at: %s\n", ep.StartedAt)
	fmt.Fprintf(&b, "ended_at: %s\n", ep.EndedAt)
	if ep.CWD != "" {
		fmt.Fprintf(&b, "cwd: %s\n", ep.CWD)
	}
	if ep.GitBranch != "" {
		fmt.Fprintf(&b, "git_branch: %s\n", ep.GitBranch)
	}
	b.WriteString("tags:\n  - episode\n")
	// Vaultmind-owned fields per the four-tier taxonomy in
	// schema/registry.go. `created` is the started_at date portion —
	// when the session happened, the episode's semantic birthday.
	// vm_updated uses the SSOT format (RFC3339 second-precision UTC)
	// so doctor's drift detector parses it. %q quotes the timestamp
	// because it contains a colon (YAML auto-quotes anyway, but being
	// explicit avoids producer/consumer drift if YAML serializer
	// changes).
	fmt.Fprintf(&b, "created: %s\n", startedAtDate(ep.StartedAt))
	fmt.Fprintf(&b, "vm_updated: %q\n", time.Now().UTC().Format(schema.VMUpdatedFormat))
	b.WriteString("---\n\n")

	fmt.Fprintf(&b, "# Episode — %s\n\n", ep.ID)

	b.WriteString("## Metadata\n\n")
	fmt.Fprintf(&b, "- User messages: %d\n", len(ep.UserMessages))
	fmt.Fprintf(&b, "- Assistant text blocks: %d\n", len(ep.AssistantMessages))
	fmt.Fprintf(&b, "- Tool calls: %s\n", formatToolCounts(ep.ToolCounts))
	fmt.Fprintf(&b, "- Files touched: %d\n", len(ep.FilesTouched))
	b.WriteString("\n")

	writeCommits(&b, ep)
	writePRs(&b, ep)
	writeFilesTouched(&b, ep)
	writeUserMessages(&b, ep)
	writeAssistantMessages(&b, ep)

	return b.String()
}

func formatToolCounts(counts map[string]int) string {
	if len(counts) == 0 {
		return "none"
	}
	names := make([]string, 0, len(counts))
	for n := range counts {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		if counts[names[i]] != counts[names[j]] {
			return counts[names[i]] > counts[names[j]]
		}
		return names[i] < names[j]
	})
	parts := make([]string, 0, len(names))
	for _, n := range names {
		parts = append(parts, fmt.Sprintf("%s=%d", n, counts[n]))
	}
	return strings.Join(parts, ", ")
}

func writeCommits(b *strings.Builder, ep *Episode) {
	b.WriteString("## Commits made\n\n")
	if len(ep.Commits) == 0 {
		b.WriteString("_(none)_\n\n")
		return
	}
	for _, c := range ep.Commits {
		fmt.Fprintf(b, "- `%s`\n", c)
	}
	b.WriteString("\n")
}

func writePRs(b *strings.Builder, ep *Episode) {
	b.WriteString("## PRs opened\n\n")
	if len(ep.PRs) == 0 {
		b.WriteString("_(none)_\n\n")
		return
	}
	for _, p := range ep.PRs {
		fmt.Fprintf(b, "- #%d — %s\n", p.Number, p.URL)
	}
	b.WriteString("\n")
}

func writeFilesTouched(b *strings.Builder, ep *Episode) {
	b.WriteString("## Files touched\n\n")
	if len(ep.FilesTouched) == 0 {
		b.WriteString("_(none)_\n\n")
		return
	}
	for _, p := range ep.FilesTouched {
		fmt.Fprintf(b, "- `%s`\n", p)
	}
	b.WriteString("\n")
}

func writeUserMessages(b *strings.Builder, ep *Episode) {
	b.WriteString("## User messages (verbatim)\n\n")
	if len(ep.UserMessages) == 0 {
		b.WriteString("_(none)_\n\n")
		return
	}
	for i, m := range ep.UserMessages {
		fmt.Fprintf(b, "### %d — %s\n\n", i+1, m.Timestamp)
		b.WriteString(quoteBlock(m.Text))
		b.WriteString("\n")
	}
}

func writeAssistantMessages(b *strings.Builder, ep *Episode) {
	b.WriteString("## Assistant responses (verbatim)\n\n")
	if len(ep.AssistantMessages) == 0 {
		b.WriteString("_(none)_\n\n")
		return
	}
	for i, m := range ep.AssistantMessages {
		fmt.Fprintf(b, "### %d — %s\n\n", i+1, m.Timestamp)
		b.WriteString(m.Text)
		b.WriteString("\n\n")
	}
}

// startedAtDate extracts the YYYY-MM-DD prefix from an RFC3339 (or
// loosely-RFC3339) timestamp like "2026-04-23T20:22:25.914Z". Falls
// back to today's UTC date when the input is empty or shorter than
// the date prefix — the caller can't produce a malformed `created`
// even on degenerate transcripts.
func startedAtDate(startedAt string) string {
	const dateLen = len("2006-01-02")
	if len(startedAt) >= dateLen {
		// Validate it's actually a date prefix (cheap parse) — otherwise
		// fall through to today.
		if _, err := time.Parse(schema.CreatedDateFormat, startedAt[:dateLen]); err == nil {
			return startedAt[:dateLen]
		}
	}
	return time.Now().UTC().Format(schema.CreatedDateFormat)
}

func quoteBlock(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = "> " + line
	}
	return strings.Join(lines, "\n") + "\n"
}
