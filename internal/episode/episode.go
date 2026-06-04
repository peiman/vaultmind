// Package episode parses a Claude Code session transcript (JSONL) into a
// structured Episode and renders it as a markdown "episode" file for the
// identity vault. It is the v0 of the episodic substrate — raw per-session
// capture, no distillation, not indexed into the vault's search layer.
package episode

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// Message is one verbatim text exchange captured from a session transcript.
type Message struct {
	Timestamp string
	Text      string
}

// PRLink is a pull request announced by a pr-link record.
type PRLink struct {
	Number     int
	URL        string
	Repository string
	Timestamp  string
}

// Episode is the structured output of parsing a session transcript.
type Episode struct {
	ID                string
	SessionID         string
	StartedAt         string
	EndedAt           string
	CWD               string
	GitBranch         string
	UserMessages      []Message
	AssistantMessages []Message
	ToolCounts        map[string]int
	Commits           []string
	PRs               []PRLink
	FilesTouched      []string
}

// ParseTranscript reads a Claude Code JSONL transcript and returns the
// distilled Episode. Noise records (system reminders, tool results, thinking
// blocks) are filtered — only real human/assistant exchanges, tool uses, and
// structural events are kept.
func ParseTranscript(path string) (*Episode, error) {
	// #nosec G304 -- caller-supplied path, read-only; this is a CLI tool, not a server.
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open transcript: %w", err)
	}
	defer func() { _ = f.Close() }()

	ep := &Episode{ToolCounts: map[string]int{}}
	filesSeen := map[string]struct{}{}
	prsSeen := map[int]struct{}{}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<24) // up to 16 MiB per line

	for scanner.Scan() {
		var rec record
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			continue
		}
		if ep.SessionID == "" && rec.SessionID != "" {
			ep.SessionID = rec.SessionID
		}

		switch rec.Type {
		case "user":
			handleUser(ep, rec)
		case "assistant":
			handleAssistant(ep, rec, filesSeen)
		case "pr-link":
			if _, dup := prsSeen[rec.PRNumber]; !dup && rec.PRNumber != 0 {
				prsSeen[rec.PRNumber] = struct{}{}
				ep.PRs = append(ep.PRs, PRLink{
					Number: rec.PRNumber, URL: rec.PRURL,
					Repository: rec.PRRepository, Timestamp: rec.Timestamp,
				})
			}
		}

		if rec.Timestamp != "" {
			if ep.StartedAt == "" {
				ep.StartedAt = rec.Timestamp
			}
			ep.EndedAt = rec.Timestamp
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan transcript: %w", err)
	}

	ep.FilesTouched = sortedKeys(filesSeen)
	ep.ID = deriveID(ep.StartedAt, ep.SessionID)
	return ep, nil
}

type record struct {
	Type         string          `json:"type"`
	SessionID    string          `json:"sessionId"`
	Timestamp    string          `json:"timestamp"`
	CWD          string          `json:"cwd"`
	GitBranch    string          `json:"gitBranch"`
	Message      json.RawMessage `json:"message"`
	PRNumber     int             `json:"prNumber"`
	PRURL        string          `json:"prUrl"`
	PRRepository string          `json:"prRepository"`
}

type userMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type assistantMessage struct {
	Role    string           `json:"role"`
	Content []assistantBlock `json:"content"`
}

type assistantBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

func handleUser(ep *Episode, rec record) {
	if ep.CWD == "" && rec.CWD != "" {
		ep.CWD = rec.CWD
	}
	if ep.GitBranch == "" && rec.GitBranch != "" {
		ep.GitBranch = rec.GitBranch
	}
	var msg userMessage
	if err := json.Unmarshal(rec.Message, &msg); err != nil {
		return
	}
	// Only string content is a real user message; lists carry tool_result blocks.
	var asString string
	if err := json.Unmarshal(msg.Content, &asString); err != nil {
		return
	}
	trimmed := strings.TrimSpace(asString)
	if trimmed == "" {
		return
	}
	if strings.HasPrefix(trimmed, "<system-reminder>") || strings.HasPrefix(trimmed, "<task-notification>") {
		return
	}
	ep.UserMessages = append(ep.UserMessages, Message{Timestamp: rec.Timestamp, Text: asString})
}

func handleAssistant(ep *Episode, rec record, filesSeen map[string]struct{}) {
	var msg assistantMessage
	if err := json.Unmarshal(rec.Message, &msg); err != nil {
		return
	}
	for _, b := range msg.Content {
		switch b.Type {
		case "text":
			if strings.TrimSpace(b.Text) != "" {
				ep.AssistantMessages = append(ep.AssistantMessages, Message{Timestamp: rec.Timestamp, Text: b.Text})
			}
		case "tool_use":
			ep.ToolCounts[b.Name]++
			handleToolUse(ep, b, filesSeen)
		}
	}
}

func handleToolUse(ep *Episode, b assistantBlock, filesSeen map[string]struct{}) {
	switch b.Name {
	case "Bash":
		var in struct {
			Command, Description string
		}
		if err := json.Unmarshal(b.Input, &in); err == nil {
			if cmd := strings.TrimSpace(in.Command); strings.Contains(cmd, "git commit") {
				ep.Commits = append(ep.Commits, extractCommitSubject(cmd))
			}
		}
	case "Edit", "Write", "Read":
		var in struct {
			FilePath string `json:"file_path"`
		}
		if err := json.Unmarshal(b.Input, &in); err == nil && in.FilePath != "" {
			filesSeen[in.FilePath] = struct{}{}
		}
	}
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func deriveID(startedAt, sessionID string) string {
	date := "unknown-date"
	if len(startedAt) >= 10 {
		date = startedAt[:10]
	}
	sidShort := sessionID
	if len(sidShort) > 8 {
		sidShort = sidShort[:8]
	}
	return fmt.Sprintf("episode-%s-%s", date, sidShort)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// extractCommitSubject pulls a best-effort commit subject from a shell
// invocation of `git commit`. Handles inline `-m "subject"` and HEREDOC
// (`-m "$(cat <<'EOF' ... EOF)"`) forms. Falls back to a truncation of the
// whole command when no message is parseable.
func extractCommitSubject(shellCmd string) string {
	if _, rest, ok := strings.Cut(shellCmd, "<<'EOF'"); ok {
		for line := range strings.SplitSeq(rest, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || line == "EOF" {
				continue
			}
			return truncate(line, 120)
		}
	}
	for _, q := range []string{`-m "`, `-m '`} {
		if _, rest, ok := strings.Cut(shellCmd, q); ok {
			if end := strings.IndexByte(rest, q[len(q)-1]); end > 0 {
				first, _, _ := strings.Cut(rest[:end], "\n")
				return truncate(strings.TrimSpace(first), 120)
			}
		}
	}
	return truncate(shellCmd, 120)
}
