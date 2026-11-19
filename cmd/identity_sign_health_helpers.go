package cmd

// Helpers for identity sign-health. The *_helpers.go name keeps the
// command-pattern check from treating these renderers as commands.

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/peiman/vaultmind/.ckeletin/pkg/output"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/signhealth"
)

func parseSignHealthMaxAge(raw string) (time.Duration, error) {
	if raw == "" || raw == "0" {
		return 0, nil
	}
	age, err := time.ParseDuration(raw)
	if err != nil || age < 0 {
		if err == nil {
			err = fmt.Errorf("duration must not be negative")
		}
		return 0, fmt.Errorf("invalid --max-age %q: %w", raw, err)
	}
	return age, nil
}

func renderSignHealth(cmd interface {
	OutOrStdout() io.Writer
}, entries []signhealth.Entry) error {
	if output.IsJSONMode() {
		if err := writeSignHealthJSON(cmd.OutOrStdout(), entries); err != nil {
			return err
		}
		if names := signHealthFailures(entries); len(names) > 0 {
			return cmdutil.ErrAlreadyWritten
		}
		return nil
	}
	if err := writeSignHealthText(cmd.OutOrStdout(), entries); err != nil {
		return err
	}
	if names := signHealthFailures(entries); len(names) > 0 {
		return fmt.Errorf("%d agent(s) cannot sign: %s", len(names), strings.Join(names, ", "))
	}
	return nil
}

func writeSignHealthText(w io.Writer, entries []signhealth.Entry) error {
	_, err := io.WriteString(w, signHealthText(entries))
	return err
}

func signHealthText(entries []signhealth.Entry) string {
	if len(entries) == 0 {
		return "no sign attempts recorded\n"
	}
	var b strings.Builder
	for _, entry := range entries {
		if entry.Unreadable {
			_, _ = fmt.Fprintf(&b, "UNREADABLE %s: %s\n", entry.File, entry.Err)
			continue
		}
		_, _ = fmt.Fprintf(&b, "%s  %s  %s", entry.Record.Agent, signHealthStatus(entry), entry.Record.TS)
		if entry.Record.Error != "" {
			_, _ = fmt.Fprintf(&b, "  %s", entry.Record.Error)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func signHealthStatus(entry signhealth.Entry) string {
	switch {
	case entry.Stale:
		return "stale"
	case !entry.Record.OK:
		return "FAIL"
	default:
		return "ok"
	}
}

func writeSignHealthJSON(w io.Writer, entries []signhealth.Entry) error {
	agents := make([]signhealth.State, 0, len(entries))
	for _, entry := range entries {
		if entry.Unreadable {
			agents = append(agents, signhealth.State{Agent: "UNREADABLE " + entry.File, Error: entry.Err})
			continue
		}
		agents = append(agents, entry.Record)
	}
	return json.NewEncoder(w).Encode(struct {
		Agents []signhealth.State `json:"agents"`
	}{Agents: agents})
}

func signHealthFailures(entries []signhealth.Entry) []string {
	var names []string
	for _, entry := range entries {
		if entry.Unreadable {
			names = append(names, entry.File)
		} else if !entry.Stale && !entry.Record.OK {
			names = append(names, entry.Record.Agent)
		}
	}
	return names
}
