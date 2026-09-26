package query

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// maxLedgerNameLen caps the file name derived from a conversation id.
const maxLedgerNameLen = 64

// ShownLedger records which notes' text one conversation received, so a note
// asked for again within a short window is sent as its title instead of its
// text a second time. Measured before it was built: 82% of the reach hook's
// delivered bodies in a conversation were repeats, 68% of them within five
// minutes.
//
// Keyed on the harness conversation id. A subagent gets a fresh id, so it is
// never deduplicated against its parent; without an id nothing is recorded.
type ShownLedger struct {
	path string
}

// OpenShownLedger names the ledger for sessionID under dir. The id is reduced
// to a safe file name, so it cannot point outside dir.
func OpenShownLedger(dir, sessionID string) ShownLedger {
	if sessionID == "" {
		return ShownLedger{}
	}
	name := strings.Map(func(r rune) rune {
		if r == '-' || r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' {
			return r
		}
		return '_'
	}, sessionID)
	if len(name) > maxLedgerNameLen {
		name = name[:maxLedgerNameLen]
	}
	return ShownLedger{path: filepath.Join(dir, name)}
}

// Recent returns the notes shown within window before now. A missing or
// partly unreadable ledger yields what can be read: deduplication is a saving,
// never a reason to fail an answer.
func (l ShownLedger) Recent(window time.Duration, now time.Time) map[string]bool {
	shown := map[string]bool{}
	if l.path == "" {
		return shown
	}
	f, err := os.Open(l.path) // nosemgrep: go-path-traversal -- the name is reduced to [A-Za-z0-9-_] under a fixed dir (OpenShownLedger); TestShownLedger_AHostileIDStaysInItsDirectory
	if err != nil {
		return shown
	}
	defer func() { _ = f.Close() }()
	cutoff := now.Add(-window).Unix()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		ts, id, ok := strings.Cut(scanner.Text(), "\t")
		if !ok || id == "" {
			continue
		}
		if at, perr := strconv.ParseInt(ts, 10, 64); perr == nil && at >= cutoff {
			shown[id] = true
		}
	}
	return shown
}

// Record appends ids as shown at now.
func (l ShownLedger) Record(ids []string, now time.Time) error {
	if l.path == "" || len(ids) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0o700); err != nil {
		return fmt.Errorf("creating shown ledger dir: %w", err)
	}
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("opening shown ledger: %w", err)
	}
	var b strings.Builder
	for _, id := range ids {
		fmt.Fprintf(&b, "%d\t%s\n", now.Unix(), id)
	}
	if _, err := f.WriteString(b.String()); err != nil {
		_ = f.Close()
		return fmt.Errorf("writing shown ledger: %w", err)
	}
	return f.Close()
}
