// Package envelope provides the standard JSON response wrapper for all --json output.
package envelope

import "time"

// SchemaVersion is the public contract version. Consumers decoding this
// envelope should branch on major-version changes (v1 -> v2) and expect
// additive changes within a version. Adding a field is backward-compatible
// (unknown fields are ignored by Go's json decoder). Renaming or removing
// a field is a breaking change and requires a major-version bump.
const SchemaVersion = "v1"

// Envelope is the standard JSON response wrapper for all --json output.
type Envelope struct {
	SchemaVersion string      `json:"schema_version"`
	Command       string      `json:"command"`
	Status        string      `json:"status"`
	Warnings      []Issue     `json:"warnings"`
	Errors        []Issue     `json:"errors"`
	Result        interface{} `json:"result"`
	Meta          Meta        `json:"meta"`
}

// Issue represents a structured warning or error.
type Issue struct {
	Code       string   `json:"code"`
	Message    string   `json:"message"`
	Field      string   `json:"field,omitempty"`
	Candidates []string `json:"candidates,omitempty"`
}

// Meta contains envelope metadata.
type Meta struct {
	VaultPath  string `json:"vault_path"`
	IndexHash  string `json:"index_hash"`
	Timestamp  string `json:"timestamp"`
	IndexStale bool   `json:"index_stale,omitempty"`
}

// OK creates a successful envelope.
func OK(command string, result interface{}) *Envelope {
	return &Envelope{
		SchemaVersion: SchemaVersion,
		Command:       command,
		Status:        "ok",
		Warnings:      []Issue{},
		Errors:        []Issue{},
		Result:        result,
		Meta: Meta{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	}
}

// Error creates an error envelope.
func Error(command, code, message, field string) *Envelope {
	return &Envelope{
		SchemaVersion: SchemaVersion,
		Command:       command,
		Status:        "error",
		Warnings:      []Issue{},
		Errors:        []Issue{{Code: code, Message: message, Field: field}},
		Result:        nil,
		Meta: Meta{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	}
}

// AddWarning adds a structured warning and updates status.
func (e *Envelope) AddWarning(code, message, field string) {
	e.Warnings = append(e.Warnings, Issue{Code: code, Message: message, Field: field})
	if e.Status == "ok" {
		e.Status = "warning"
	}
}
