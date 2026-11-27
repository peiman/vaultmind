// Package arcgate evaluates opt-in arc action gates for agent hook payloads.
package arcgate

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/peiman/vaultmind/internal/memory"
	"github.com/peiman/vaultmind/internal/parser"
	"gopkg.in/yaml.v3"
)

const (
	eventPreToolUse = "pretooluse"
	eventStop       = "stop"
)

type Trigger struct {
	Arc   string   `yaml:"arc"`
	On    string   `yaml:"on"`
	Tools []string `yaml:"tools"`
	Match string   `yaml:"match"`
	Check string   `yaml:"check"`
}

type arcNote struct {
	ID    string
	Title string
	Path  string
}

type hookPayload struct {
	SessionID            string          `json:"session_id"`
	ToolName             string          `json:"tool_name"`
	ToolInput            json.RawMessage `json:"tool_input"`
	StopHookActive       bool            `json:"stop_hook_active"`
	LastAssistantMessage string          `json:"last_assistant_message"`
	TranscriptPath       string          `json:"transcript_path"`
}

// Gate evaluates one hook payload. Failures are represented by empty stdout and
// a diagnostic string; callers should write the diagnostic to stderr and still
// exit successfully.
func Gate(input []byte, vault, event, triggersPath, stateDir, logDir string, now time.Time) (string, string) {
	out, err := gate(input, vault, event, triggersPath, stateDir, logDir, now)
	if err != nil {
		return "", "arc gate: " + err.Error()
	}
	return out, ""
}

type gateContext struct {
	payload  hookPayload
	triggers []Trigger
	arcs     map[string]arcNote
	fired    map[string]bool
	session  string
	text     string
	active   bool
}

func gate(input []byte, vault, event, triggersPath, stateDir, logDir string, now time.Time) (string, error) {
	ctx, err := prepareGate(input, vault, event, triggersPath, stateDir)
	if err != nil || ctx.active {
		return "", err
	}
	for _, trigger := range ctx.triggers {
		if trigger.On != event || !triggerMatches(trigger, ctx.payload.ToolName, ctx.text) {
			continue
		}
		flags, fires := triggerFlags(trigger, ctx.text)
		if !fires {
			continue
		}
		if ctx.fired[trigger.Arc] {
			writeSidecar(logDir, now, ctx.session, trigger, flags, "allow-repeat")
			continue
		}
		arcText, title, err := readArc(ctx.arcs[trigger.Arc])
		if err != nil {
			return "", err
		}
		if err := writeState(stateDir, ctx.session, ctx.fired, trigger.Arc); err != nil {
			return "", err
		}
		writeSidecar(logDir, now, ctx.session, trigger, flags, "deny")
		reason := blockReason(title, trigger.Arc, arcText, flags)
		if event == eventPreToolUse {
			return marshalPreToolUse(reason), nil
		}
		return marshalStop(reason), nil
	}
	return "", nil
}

func prepareGate(input []byte, vault, event, triggersPath, stateDir string) (gateContext, error) {
	if event != eventPreToolUse && event != eventStop {
		return gateContext{}, fmt.Errorf("unknown event %q", event)
	}
	var payload hookPayload
	if err := json.Unmarshal(input, &payload); err != nil {
		return gateContext{}, fmt.Errorf("invalid hook JSON: %w", err)
	}
	if event == eventStop && payload.StopHookActive {
		return gateContext{active: true}, nil
	}
	session := sanitizeSession(payload.SessionID)
	if session == "" {
		return gateContext{}, fmt.Errorf("missing session_id")
	}
	info, err := os.Stat(vault)
	if err != nil || !info.IsDir() {
		return gateContext{}, fmt.Errorf("vault %q does not exist or is not a directory", vault)
	}
	arcs, err := scanArcs(vault)
	if err != nil {
		return gateContext{}, err
	}
	if triggersPath == "" {
		triggersPath = os.Getenv("VAULTMIND_ARC_TRIGGERS")
		if triggersPath == "" {
			triggersPath = filepath.Join(vault, ".vaultmind", "arc-triggers.yaml")
		}
	}
	triggers, err := loadTriggers(triggersPath, arcs)
	if err != nil {
		return gateContext{}, err
	}
	text, err := hookText(event, payload)
	if err != nil {
		return gateContext{}, err
	}
	if event == eventPreToolUse {
		text, err = compactJSON(payload.ToolInput)
		if err != nil {
			return gateContext{}, fmt.Errorf("invalid tool_input: %w", err)
		}
	}
	fired, err := readState(stateDir, session)
	if err != nil {
		return gateContext{}, err
	}
	return gateContext{payload: payload, triggers: triggers, arcs: arcs, fired: fired, session: session, text: text}, nil
}

func triggerFlags(trigger Trigger, text string) ([]string, bool) {
	if trigger.Check != "" {
		flags := CheckPlainWords(text)
		return flags, len(flags) > 0
	}
	return nil, true
}

func blockReason(title, arc string, text string, flags []string) string {
	reason := fmt.Sprintf("Before this step, your arc %s (%s):\n", title, arc)
	if len(flags) > 0 {
		reason += "What tripped it: " + strings.Join(flags, "; ") + ".\n"
	}
	return reason + text + "\nIf it does not apply here, say why and continue."
}

func loadTriggers(path string, arcs map[string]arcNote) ([]Trigger, error) {
	body, err := os.ReadFile(path) // #nosec G304,G703 -- the trigger path is explicit configuration
	if err != nil {
		return nil, fmt.Errorf("cannot read triggers file %q: %w", path, err)
	}
	var triggers []Trigger
	if err := yaml.Unmarshal(body, &triggers); err != nil {
		return nil, fmt.Errorf("cannot parse triggers file %q: %w", path, err)
	}
	for i, trigger := range triggers {
		where := fmt.Sprintf("trigger entry %d arc %q", i, trigger.Arc)
		if strings.TrimSpace(trigger.Arc) == "" {
			return nil, fmt.Errorf("%s: missing arc", where)
		}
		if trigger.On != eventPreToolUse && trigger.On != eventStop {
			return nil, fmt.Errorf("%s: on must be pretooluse or stop", where)
		}
		if (trigger.Match == "") == (trigger.Check == "") {
			return nil, fmt.Errorf("%s: exactly one of match or check is required", where)
		}
		if trigger.Check != "" && (trigger.Check != "plain-words" || trigger.On != eventStop) {
			return nil, fmt.Errorf("%s: check must be plain-words and is valid only with on: stop", where)
		}
		if trigger.Match != "" {
			if _, err := regexp.Compile(trigger.Match); err != nil {
				return nil, fmt.Errorf("%s: invalid match regex: %w", where, err)
			}
		}
		note, ok := arcs[trigger.Arc]
		if !ok || note.ID == "" {
			return nil, fmt.Errorf("%s: unknown arc id", where)
		}
	}
	return triggers, nil
}

func scanArcs(vault string) (map[string]arcNote, error) {
	out := make(map[string]arcNote)
	err := filepath.WalkDir(vault, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != vault && strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(entry.Name(), ".") || !strings.HasSuffix(entry.Name(), ".md") {
			return nil
		}
		head, err := readHead(path)
		if err != nil {
			return nil //nolint:nilerr // an unreadable non-arc note is not a gate trigger
		}
		fm := cheapFrontmatter(head)
		id, _ := fm["id"].(string)
		typ, _ := fm["type"].(string)
		status, _ := fm["status"].(string)
		title, _ := fm["title"].(string)
		if id != "" && typ == "arc" && status != "draft" {
			out[id] = arcNote{ID: id, Title: title, Path: path}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("cannot scan vault %q: %w", vault, err)
	}
	return out, nil
}

func readHead(path string) ([]byte, error) {
	f, err := os.Open(path) // #nosec G304 -- the vault walker supplies this path
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(io.LimitReader(f, 2048))
}

func cheapFrontmatter(head []byte) map[string]interface{} {
	lines := strings.Split(string(head), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil
	}
	out := make(map[string]interface{})
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "---" {
			return out
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key != "id" && key != "type" && key != "status" && key != "title" {
			continue
		}
		var decoded interface{}
		if yaml.Unmarshal([]byte(strings.TrimSpace(value)), &decoded) == nil {
			out[key] = decoded
		}
	}
	return nil
}

func hookText(event string, payload hookPayload) (string, error) {
	if event == eventPreToolUse {
		return "", nil
	}
	if payload.LastAssistantMessage != "" {
		return payload.LastAssistantMessage, nil
	}
	if payload.TranscriptPath == "" {
		return "", nil
	}
	return lastAssistantMessage(payload.TranscriptPath)
}

func lastAssistantMessage(path string) (string, error) {
	f, err := os.Open(path) // #nosec G304 -- the transcript path is explicit hook input
	if err != nil {
		return "", fmt.Errorf("cannot read transcript %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	var last string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var entry struct {
			Type    string `json:"type"`
			Role    string `json:"role"`
			Message struct {
				Role    string `json:"role"`
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(scanner.Bytes(), &entry) != nil || (entry.Type != "assistant" && entry.Role != "assistant" && entry.Message.Role != "assistant") {
			continue
		}
		var text strings.Builder
		for _, block := range entry.Message.Content {
			if block.Type == "text" {
				text.WriteString(block.Text)
			}
		}
		if text.Len() > 0 {
			last = text.String()
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("cannot read transcript %q: %w", path, err)
	}
	return last, nil
}

func compactJSON(raw json.RawMessage) (string, error) {
	var value interface{}
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		return "", err
	}
	return strings.TrimSpace(b.String()), nil
}

func triggerMatches(trigger Trigger, toolName, text string) bool {
	if trigger.On == eventPreToolUse && len(trigger.Tools) > 0 {
		found := false
		for _, tool := range trigger.Tools {
			if tool == toolName {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if trigger.Check == "plain-words" {
		return trigger.On == eventStop
	}
	return trigger.Match != "" && regexp.MustCompile(trigger.Match).MatchString(text)
}

func readArc(note arcNote) (text, title string, err error) {
	body, err := os.ReadFile(note.Path)
	if err != nil {
		return "", "", fmt.Errorf("cannot read arc %q: %w", note.ID, err)
	}
	fm, content, err := parser.ExtractFrontmatter(body)
	if err != nil {
		return "", "", fmt.Errorf("cannot parse arc %q: %w", note.ID, err)
	}
	title, _ = fm["title"].(string)
	if strings.TrimSpace(title) == "" {
		title = note.Title
	}
	if strings.TrimSpace(title) == "" {
		title = note.ID
	}
	text = strings.TrimSpace(memory.PrincipleSection(content))
	if text == "" {
		text = strings.TrimSpace(memory.RecitePassage(content))
	}
	return text, title, nil
}

func sanitizeSession(session string) string {
	var b strings.Builder
	for _, r := range session {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
		if b.Len() >= 100 {
			break
		}
	}
	return b.String()
}

func readState(dir, session string) (map[string]bool, error) {
	body, err := os.ReadFile(filepath.Join(dir, session)) // #nosec G304 -- session is sanitized and dir is explicit state storage
	if os.IsNotExist(err) {
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read state: %w", err)
	}
	out := make(map[string]bool)
	for _, id := range strings.Split(string(body), "\n") {
		if strings.TrimSpace(id) != "" {
			out[strings.TrimSpace(id)] = true
		}
	}
	return out, nil
}

func writeState(dir, session string, fired map[string]bool, arc string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("cannot create state directory: %w", err)
	}
	fired[arc] = true
	ids := make([]string, 0, len(fired))
	for id := range fired {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	body := strings.Join(ids, "\n") + "\n"
	tmp, err := os.CreateTemp(dir, "."+session+".tmp-*")
	if err != nil {
		return fmt.Errorf("cannot create state temporary file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("cannot set state permissions: %w", err)
	}
	if _, err := tmp.WriteString(body); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("cannot write state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("cannot close state: %w", err)
	}
	if err := os.Rename(tmpName, filepath.Join(dir, session)); err != nil {
		return fmt.Errorf("cannot install state: %w", err)
	}
	return nil
}

func writeSidecar(dir string, now time.Time, session string, trigger Trigger, flags []string, decision string) {
	if dir == "" {
		return
	}
	_ = os.MkdirAll(dir, 0o700)
	entry := struct {
		Timestamp string   `json:"timestamp"`
		SessionID string   `json:"session_id"`
		Arc       string   `json:"arc"`
		Event     string   `json:"event"`
		Rule      string   `json:"rule"`
		Flags     []string `json:"flags,omitempty"`
		Decision  string   `json:"decision"`
	}{now.Format(time.RFC3339), session, trigger.Arc, trigger.On, triggerRule(trigger), flags, decision}
	body, err := json.Marshal(entry)
	if err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "arc-gate.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) // #nosec G304 -- dir is explicit sidecar storage
	if err != nil {
		return
	}
	_, _ = f.Write(append(body, '\n'))
	_ = f.Close()
}

func triggerRule(trigger Trigger) string {
	if trigger.Check != "" {
		return trigger.Check
	}
	return trigger.Match
}

func marshalPreToolUse(reason string) string {
	type output struct {
		HookEventName            string `json:"hookEventName"`
		PermissionDecision       string `json:"permissionDecision"`
		PermissionDecisionReason string `json:"permissionDecisionReason"`
	}
	b, _ := json.Marshal(struct {
		HookSpecificOutput output `json:"hookSpecificOutput"`
	}{output{"PreToolUse", "deny", reason}})
	return string(b)
}

func marshalStop(reason string) string {
	b, _ := json.Marshal(struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}{"block", reason})
	return string(b)
}
