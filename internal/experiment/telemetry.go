package experiment

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// Telemetry tier constants.
const (
	TelemetryOff       = "off"
	TelemetryAnonymous = "anonymous"
	TelemetryFull      = "full"
)

// WriteTelemetryPrompt writes the opt-in prompt to w.
func WriteTelemetryPrompt(w io.Writer) {
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Help improve VaultMind?")
	_, _ = fmt.Fprintln(w, "  [1] Anonymous usage statistics (recommended)")
	_, _ = fmt.Fprintln(w, "  [2] Full data sharing including queries and vault content (for early adopters)")
	_, _ = fmt.Fprintln(w, "  [3] No data collection")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprint(w, "Choice [1]: ")
}

// ParseTelemetryChoice maps user input to a telemetry tier.
// Empty or invalid input defaults to anonymous.
func ParseTelemetryChoice(input string) string {
	switch input {
	case "1", "":
		return TelemetryAnonymous
	case "2":
		return TelemetryFull
	case "3":
		return TelemetryOff
	default:
		return TelemetryAnonymous
	}
}

// PromptTelemetry shows the telemetry prompt and reads the user's choice.
// Returns the selected tier string.
func PromptTelemetry(r io.Reader, w io.Writer) string {
	WriteTelemetryPrompt(w)
	scanner := bufio.NewScanner(r)
	if scanner.Scan() {
		return ParseTelemetryChoice(strings.TrimSpace(scanner.Text()))
	}
	return TelemetryAnonymous
}

// IsFirstRun returns true if the experiment DB has no completed sessions.
func (d *DB) IsFirstRun() (bool, error) {
	var count int
	err := d.db.QueryRow("SELECT COUNT(*) FROM sessions WHERE ended_at IS NOT NULL").Scan(&count)
	if err != nil {
		return false, err
	}
	return count == 0, nil
}
