package episode

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Codex CLI rollout support.
//
// A Codex session is a JSONL "rollout" (~/.codex/sessions/YYYY/MM/DD/rollout-
// <ts>-<session-id>.jsonl) whose records are {timestamp, type, payload}. Shapes
// below were measured on 200 real rollouts from Codex 0.153–0.156 (2026-09-24),
// not taken from documentation:
//
//   - session_meta (line 1): session_id, cwd, git.branch, and source. A main
//     session's source is a string ("cli", "vscode", "exec"); a sub-agent's is
//     an object with a "subagent" key, and it carries parent_thread_id.
//   - response_item/message: the conversation. The "user" role ALSO carries text
//     Codex injects — see codexPersonText. "developer" is never the person.
//   - response_item/custom_tool_call: JavaScript calling tools.<name>(…).
//   - response_item/function_call: a named tool call.
//
// Claude Code records have no payload, so the two formats never collide in the
// shared parse loop.

// AgentCodex marks an episode captured from a Codex rollout; it is also the
// episode's tag.
const AgentCodex = "codex"

// codexNotRecorded replaces "(none)" for what Codex capture does not extract.
const codexNotRecorded = "_(not recorded for Codex sessions)_"

// Codex record types and payload types read here.
const (
	codexSessionMeta    = "session_meta"
	codexResponseItem   = "response_item"
	codexItemMessage    = "message"
	codexItemCustomTool = "custom_tool_call"
	codexItemFunction   = "function_call"
	codexRoleUser       = "user"
	codexRoleAssistant  = "assistant"
	codexInputText      = "input_text"
	codexOutputText     = "output_text"
	codexSubagentSource = "subagent"
	codexQuestionReply  = "<send_user_message_question_reply>"
)

// codexInjectedHeadings open user-role text that Codex or the IDE adds, not the
// person: AGENTS.md, and the file lists an editor attaches to a prompt.
var codexInjectedHeadings = []string{
	"# AGENTS.md instructions",
	"# Files mentioned by the user:",
	"# Files pasted by the user:",
}

var (
	// codexToolCall finds the tools a custom_tool_call's JavaScript invokes.
	codexToolCall = regexp.MustCompile(`tools\.([A-Za-z_][A-Za-z0-9_]*)\(`)
	// codexPatchFile finds apply_patch's file headers. The patch sits inside a
	// JavaScript string, so its newlines appear as a literal backslash-n.
	codexPatchFile = regexp.MustCompile(`\*\*\* (?:Update|Add|Delete) File: ([^\\\n"'` + "`" + `]+)`)
)

type codexSessionMetaPayload struct {
	SessionID      string          `json:"session_id"`
	ParentThreadID string          `json:"parent_thread_id"`
	CWD            string          `json:"cwd"`
	Source         json.RawMessage `json:"source"`
	Git            struct {
		Branch string `json:"branch"`
	} `json:"git"`
}

type codexItem struct {
	Type    string `json:"type"`
	Role    string `json:"role"`
	Name    string `json:"name"`
	Input   string `json:"input"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

// handleCodex folds one Codex record into ep. It reports whether rec was a
// Codex record at all, so the caller can fall through to Claude Code handling.
func handleCodex(ep *Episode, rec record, filesSeen map[string]struct{}) bool {
	if len(rec.Payload) == 0 {
		return false
	}
	ep.Agent = AgentCodex
	switch rec.Type {
	case codexSessionMeta:
		var m codexSessionMetaPayload
		if json.Unmarshal(rec.Payload, &m) == nil {
			if ep.SessionID == "" {
				ep.SessionID = m.SessionID
			}
			if ep.CWD == "" {
				ep.CWD = m.CWD
			}
			if ep.GitBranch == "" {
				ep.GitBranch = m.Git.Branch
			}
		}
	case codexResponseItem:
		var it codexItem
		if json.Unmarshal(rec.Payload, &it) == nil {
			handleCodexItem(ep, it, rec.Timestamp, filesSeen)
		}
	}
	return true
}

func handleCodexItem(ep *Episode, it codexItem, ts string, filesSeen map[string]struct{}) {
	switch it.Type {
	case codexItemMessage:
		for _, c := range it.Content {
			switch {
			case it.Role == codexRoleUser && c.Type == codexInputText:
				if text, ok := codexPersonText(c.Text); ok {
					ep.UserMessages = append(ep.UserMessages, Message{Timestamp: ts, Text: text})
				}
			case it.Role == codexRoleAssistant && c.Type == codexOutputText && strings.TrimSpace(c.Text) != "":
				ep.AssistantMessages = append(ep.AssistantMessages, Message{Timestamp: ts, Text: c.Text})
			}
		}
	case codexItemCustomTool:
		for _, m := range codexToolCall.FindAllStringSubmatch(it.Input, -1) {
			ep.ToolCounts[m[1]]++
		}
		for _, m := range codexPatchFile.FindAllStringSubmatch(it.Input, -1) {
			filesSeen[strings.TrimSpace(m[1])] = struct{}{}
		}
	case codexItemFunction:
		if it.Name != "" {
			ep.ToolCounts[it.Name]++
		}
	}
}

// codexPersonText returns the part of a user-role text the person actually
// wrote, or false when Codex injected it. Measured on 407 real user-role
// records: 340 plain text; the rest opened with a tag (<environment_context>,
// <recommended_plugins>, <in-app-browser-context>, <image>,
// <user_shell_command>) or an injected heading. The one tagged form that IS the
// person is an answer to the agent's question, kept with its question.
func codexPersonText(raw string) (string, bool) {
	t := strings.TrimSpace(raw)
	if t == "" {
		return "", false
	}
	if strings.HasPrefix(t, codexQuestionReply) {
		return codexAnswers(t)
	}
	if strings.HasPrefix(t, "<") {
		return "", false
	}
	for _, h := range codexInjectedHeadings {
		if strings.HasPrefix(t, h) {
			return "", false
		}
	}
	return raw, true
}

// codexAnswers renders a question-reply record ({question, answer} pairs) as
// the person's answers, each with the question it answers.
func codexAnswers(t string) (string, bool) {
	start, end := strings.Index(t, "["), strings.LastIndex(t, "]")
	if start < 0 || end <= start {
		return "", false
	}
	var qa []struct {
		Question string `json:"question"`
		Answer   string `json:"answer"`
	}
	if json.Unmarshal([]byte(t[start:end+1]), &qa) != nil {
		return "", false
	}
	var out []string
	for _, x := range qa {
		if a := strings.TrimSpace(x.Answer); a != "" {
			out = append(out, fmt.Sprintf("Answer to %q: %s", x.Question, a))
		}
	}
	return strings.Join(out, "\n"), len(out) > 0
}

// codexSessionID returns the session id of a session_meta record, if rec is one.
func codexSessionID(rec record) string {
	if rec.Type != codexSessionMeta || len(rec.Payload) == 0 {
		return ""
	}
	var m codexSessionMetaPayload
	if json.Unmarshal(rec.Payload, &m) != nil {
		return ""
	}
	return m.SessionID
}

// isCodexSubagent reports whether rec is the session_meta of a sub-agent
// thread. On 200 real rollouts the two markers agreed on every file: 191
// sub-agents had both an object source with "subagent" and a parent_thread_id;
// 9 main sessions had neither. Either one is sufficient.
func isCodexSubagent(rec record) bool {
	if rec.Type != codexSessionMeta || len(rec.Payload) == 0 {
		return false
	}
	var m codexSessionMetaPayload
	if json.Unmarshal(rec.Payload, &m) != nil {
		return false
	}
	if m.ParentThreadID != "" {
		return true
	}
	var src map[string]json.RawMessage
	if json.Unmarshal(m.Source, &src) == nil {
		_, ok := src[codexSubagentSource]
		return ok
	}
	return false
}
