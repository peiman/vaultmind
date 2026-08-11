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
	ep, _, err := ParseTranscriptFrom(path, 0)
	return ep, err
}

// ParseTranscriptFrom parses only the transcript lines strictly after
// startLine (0 = parse from the beginning) and returns the resulting delta
// Episode plus the new total line count, for a caller (CaptureIncremental) to
// persist as the next call's startLine.
//
// This is what makes cursor-based incremental capture possible: a long-lived
// session's SessionEnd hook can re-parse only the tail written since the last
// capture instead of re-rendering the whole transcript every time, which is
// what produces one ever-growing episode file for a session that never
// closes. A startLine at or beyond the current end of file is not an error —
// it returns an empty delta at the same line count, the correct "nothing new
// since last time" result for a SessionEnd fired with no new records.
func ParseTranscriptFrom(path string, startLine int) (*Episode, int, error) {
	// #nosec G304 -- caller-supplied path, read-only; this is a CLI tool, not a server.
	f, err := os.Open(path)
	if err != nil {
		return nil, startLine, fmt.Errorf("open transcript: %w", err)
	}
	defer func() { _ = f.Close() }()

	ep := &Episode{ToolCounts: map[string]int{}}
	filesSeen := map[string]struct{}{}
	prsSeen := map[int]struct{}{}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<24) // up to 16 MiB per line

	lineNum := 0
	tailFailedParse := false
	for scanner.Scan() {
		lineNum++
		if lineNum <= startLine {
			continue
		}

		var rec record
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			// A line that fails to unmarshal is normally tolerated as noise
			// (malformed/legacy records elsewhere in the file). But if THIS
			// is the last line the scanner reaches, it may be a record
			// Claude Code is still mid-flush on, not permanently bad data —
			// tailFailedParse is checked after the loop and, if still true,
			// backs the cursor off by one line so this exact line gets a
			// real second chance next call instead of being silently
			// skipped forever the moment the cursor passes it.
			tailFailedParse = true
			continue
		}
		tailFailedParse = false
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
		return nil, startLine, fmt.Errorf("scan transcript: %w", err)
	}
	if lineNum < startLine {
		// This function can't tell "the file genuinely shrank since a real
		// prior cursor was recorded" apart from "the caller simply asked
		// for a startLine this file never reached" — both look identical
		// from here. Stay conservative and never regress the returned
		// line below what was asked for; CaptureIncremental is where
		// shrinkage actually gets detected and handled, because it alone
		// knows startLine came from a real, previously-persisted cursor.
		lineNum = startLine
	}
	if tailFailedParse && lineNum > startLine {
		// The last line reached failed to unmarshal — it may be a record
		// Claude Code hasn't finished writing yet, not permanently bad
		// data. Don't let the cursor advance past it: back off by one line
		// so it gets a real second chance whole, next call, instead of
		// being silently and permanently skipped the moment the cursor
		// moves past it.
		lineNum--
	}

	// A delta can have zero new records but the transcript still identifies
	// its session (e.g. a re-scan past the end of an already-captured file) —
	// recover the session id from the whole file so a no-op capture still
	// knows whose cursor to leave alone. Cheap: SessionID appears on the
	// first record. sidErr is deliberately not propagated: this is a
	// best-effort recovery on top of an already-empty ep.SessionID, so a
	// failure here just leaves it empty, exactly as if the fallback had never
	// run — callers that require a non-empty SessionID (e.g.
	// CaptureIncremental's own sessionIDOf call) already check for that and
	// fail loudly on their own terms.
	if ep.SessionID == "" {
		if sid, sidErr := sessionIDOf(path); sidErr == nil {
			ep.SessionID = sid
		}
	}

	ep.FilesTouched = sortedKeys(filesSeen)
	ep.ID = deriveID(ep.StartedAt, ep.SessionID)
	return ep, lineNum, nil
}

// sessionIDOf returns the sessionId carried by the first record that has
// one, scanning from the start regardless of any cursor — used only to label
// a zero-record delta with the right session, never to extract content.
func sessionIDOf(path string) (string, error) {
	// #nosec G304 -- caller-supplied path, read-only.
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<24)
	for scanner.Scan() {
		var rec record
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			continue
		}
		if rec.SessionID != "" {
			return rec.SessionID, nil
		}
	}
	return "", scanner.Err()
}

// CountLines returns the total number of lines in path, without parsing any
// of them — used to detect a shrunken transcript cheaply, without paying for
// a full JSON-unmarshaling re-scan on the (common) path where nothing has
// changed. See CaptureIncremental's shrinkage check.
func CountLines(path string) (int, error) {
	// #nosec G304 -- caller-supplied path, read-only.
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<24)
	n := 0
	for scanner.Scan() {
		n++
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return n, nil
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
