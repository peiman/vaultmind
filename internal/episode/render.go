package episode

import (
	"fmt"
	"sort"
	"strings"
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

func quoteBlock(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = "> " + line
	}
	return strings.Join(lines, "\n") + "\n"
}
