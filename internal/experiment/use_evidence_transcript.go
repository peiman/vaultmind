package experiment

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type evidenceTurn struct {
	id                string
	start, next, last time.Time
	texts             []string
}
type evidenceTranscript struct {
	session string
	turns   []evidenceTurn
}

type evidenceRecord struct {
	Type             string `json:"type"`
	Session          string `json:"sessionId"`
	UUID             string `json:"uuid"`
	Timestamp        string `json:"timestamp"`
	IsMeta           bool   `json:"isMeta"`
	IsCompactSummary bool   `json:"isCompactSummary"`
	IsSidechain      bool   `json:"isSidechain"`
	AgentID          string `json:"agentId"`
	Message          struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type evidenceBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text"`
	Input json.RawMessage `json:"input"`
}

// readEvidenceTranscript deliberately allowlists assistant text and tool inputs.
// Attachments, hook_success/additionalContext, user/tool-result records and
// thinking are never searched, even when they echo an entire recall block.
func readEvidenceTranscript(path string) (*evidenceTranscript, error) {
	// #nosec G304 -- explicitly supplied local transcript, read-only.
	// nosemgrep: go-path-traversal -- operator/hook supplies the transcript path to a local CLI; same trust boundary as episode.ParseTranscript.
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open evidence transcript: %w", err)
	}
	defer func() { _ = f.Close() }()
	out := &evidenceTranscript{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<24)
	line := 0
	for scanner.Scan() {
		line++
		var r evidenceRecord
		if json.Unmarshal(scanner.Bytes(), &r) != nil {
			continue
		} // partial tail can be retried next capture
		if r.IsSidechain || r.AgentID != "" || r.IsMeta || r.IsCompactSummary || r.Session == "" {
			continue
		}
		if out.session == "" {
			out.session = r.Session
		}
		if r.Session != out.session {
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, r.Timestamp)
		if err != nil {
			continue
		}
		if r.Type == "user" && isEvidencePrompt(r.Message.Content) {
			if len(out.turns) > 0 {
				out.turns[len(out.turns)-1].next = at
			}
			id := r.UUID
			if id == "" {
				id = fmt.Sprintf("line-%d", line)
			}
			out.turns = append(out.turns, evidenceTurn{id: id, start: at})
		}
		if r.Type != "assistant" || len(out.turns) == 0 || (r.Message.Role != "" && r.Message.Role != "assistant") {
			continue
		}
		turn := &out.turns[len(out.turns)-1]
		turn.last = at
		turn.texts = append(turn.texts, assistantEvidenceText(r.Message.Content)...)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read evidence transcript: %w", err)
	}
	if out.session == "" {
		return nil, fmt.Errorf("evidence transcript has no Claude Code session id")
	}
	return out, nil
}

func isEvidencePrompt(raw json.RawMessage) bool {
	var s string
	if json.Unmarshal(raw, &s) != nil {
		var blocks []evidenceBlock
		if json.Unmarshal(raw, &blocks) != nil {
			return false
		}
		for _, b := range blocks {
			if b.Type == "tool_result" {
				return false
			}
			if b.Type == "text" {
				s += b.Text
			}
		}
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	for _, prefix := range []string{"<system-reminder>", "<task-notification", "<command-", "<local-command"} {
		if strings.HasPrefix(s, prefix) {
			return false
		}
	}
	return true
}

func assistantEvidenceText(raw json.RawMessage) []string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return []string{s}
	}
	var blocks []evidenceBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return nil
	}
	var out []string
	for _, b := range blocks {
		switch b.Type {
		case "text":
			out = append(out, b.Text)
		case "tool_use":
			var input any
			if json.Unmarshal(b.Input, &input) == nil {
				out = append(out, evidenceInputStrings(input)...)
			}
		}
	}
	return out
}

// Only values are assistant-produced inputs; a tool name, call id or schema
// key containing a note id is bookkeeping, not citation evidence.
func evidenceInputStrings(v any) []string {
	switch x := v.(type) {
	case string:
		return []string{x}
	case []any:
		var out []string
		for _, v := range x {
			out = append(out, evidenceInputStrings(v)...)
		}
		return out
	case map[string]any:
		var out []string
		for _, v := range x {
			out = append(out, evidenceInputStrings(v)...)
		}
		return out
	}
	return nil
}

// exactEvidenceID accepts punctuation/path delimiters but not a prefix of a
// different id. It is case-sensitive and never substitutes note titles.
func exactEvidenceID(text, id string) bool {
	if id == "" {
		return false
	}
	for offset := 0; offset < len(text); {
		i := strings.Index(text[offset:], id)
		if i < 0 {
			return false
		}
		i += offset
		beforeRune, _ := utf8.DecodeLastRuneInString(text[:i])
		before := i == 0 || !evidenceIDRune(beforeRune)
		end := i + len(id)
		afterRune, _ := utf8.DecodeRuneInString(text[end:])
		after := end == len(text) || !evidenceIDRune(afterRune)
		if before && after {
			return true
		}
		offset = end
	}
	return false
}

func evidenceIDRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == ':'
}
