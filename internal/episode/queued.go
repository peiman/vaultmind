package episode

import (
	"encoding/json"
	"strings"
)

// Messages the person types while the agent is still working. Claude Code
// queues them and records each as an attachment of type queued_command, never
// as a user record — measured 2026-09-24 over 32 real transcripts, 296 such
// records and none duplicated as a user record. Reading only user records lost
// every one of them, including an arc's turning point.
const (
	recordTypeAttachment   = "attachment"
	attachmentQueuedPrompt = "queued_command"
	queuedModePrompt       = "prompt"
	queuedOriginHuman      = "human"
	queuedBlockText        = "text"
)

// queuedAttachment is the part of an attachment record this package reads.
type queuedAttachment struct {
	Type        string          `json:"type"`
	Prompt      json.RawMessage `json:"prompt"`
	CommandMode string          `json:"commandMode"`
	Origin      *struct {
		Kind string `json:"kind"`
	} `json:"origin"`
}

// handleQueued adds a queued message to ep when the person wrote it. Other
// queued records are not the person: commandMode task-notification is the
// harness, origin "peer" another agent, "auto-continuation" a re-sent goal.
// Older Claude Code records no origin; those were the person's in every case
// inspected, so a missing origin counts as the person.
func handleQueued(ep *Episode, rec record) {
	var a queuedAttachment
	if err := json.Unmarshal(rec.Attachment, &a); err != nil {
		return
	}
	if a.Type != attachmentQueuedPrompt || a.CommandMode != queuedModePrompt {
		return
	}
	if a.Origin != nil && a.Origin.Kind != queuedOriginHuman {
		return
	}
	text := queuedPromptText(a.Prompt)
	if strings.TrimSpace(text) == "" {
		return
	}
	ep.UserMessages = append(ep.UserMessages, Message{Timestamp: rec.Timestamp, Text: text})
}

// queuedPromptText is a prompt's text: the string itself, or — when an image
// was pasted with it — its text blocks joined, the image left out.
func queuedPromptText(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == queuedBlockText && b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}
