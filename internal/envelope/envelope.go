// Package envelope provides the standard JSON response wrapper for all --json output.
package envelope

import (
	"encoding/json"
	"errors"
	"io"
	"time"
)

// ErrAlreadyWritten signals that an error envelope has already been written to
// output, so the caller must set a non-zero exit status WITHOUT describing the
// failure a second time.
//
// It lives here rather than beside one of its users because both the command
// layer and the query layer produce envelopes and cannot import each other.
// Before this, only the command layer had a sentinel: the query layer encoded
// an envelope saying status "error" and returned nil, so
// `vaultmind note get missing-id --json || handle_failure` never fired.
var ErrAlreadyWritten = errors.New("error already written to output")

// WriteError encodes env and returns ErrAlreadyWritten, so that "wrote an error
// envelope" and "reported success" stop being a state a caller can express.
// An encoding failure is returned as itself — nothing reached the reader, so
// there is no envelope for an exit code to agree with.
func WriteError(w io.Writer, env *Envelope) error {
	if err := json.NewEncoder(w).Encode(env); err != nil {
		return err
	}
	return ErrAlreadyWritten
}

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
