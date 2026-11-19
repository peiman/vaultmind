// Package signhealth records and reads the last sign-envelope attempt for each
// sender on this machine.
package signhealth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/peiman/vaultmind/internal/xdg"
)

// State is the on-disk record for one sender's latest signing attempt.
type State struct {
	Agent   string `json:"agent"`
	OK      bool   `json:"ok"`
	TS      string `json:"ts"`
	Error   string `json:"error,omitempty"`
	Version string `json:"version,omitempty"`
}

// Entry is a state record plus the report-only stale/unreadable status.
type Entry struct {
	Record     State
	Stale      bool
	File       string
	Unreadable bool
	Err        string
}

// ParseSender extracts only from_agent, allowing later validation to remain
// strict while making the recording path independent of it.
func ParseSender(raw []byte) (string, bool) {
	var envelope struct {
		FromAgent string `json:"from_agent"`
	}
	if json.Unmarshal(raw, &envelope) != nil || envelope.FromAgent == "" {
		return "", false
	}
	return envelope.FromAgent, true
}

// SanitizeAgent maps a sender to a single safe filename component.
func SanitizeAgent(agent string) string {
	var b strings.Builder
	for _, r := range agent {
		if isSafeASCII(r) {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	name := b.String()
	if name == "." || name == ".." {
		return "_"
	}
	return name
}

// Record writes the outcome atomically and returns only storage errors.
func Record(agent, version string, attemptErr error) error {
	if agent == "" {
		return nil
	}
	state := State{Agent: agent, OK: attemptErr == nil, TS: time.Now().UTC().Format(time.RFC3339), Version: version}
	if attemptErr != nil {
		state.Error = truncateBytes(attemptErr.Error(), 500)
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	path, err := statePath(agent)
	if err != nil {
		return err
	}
	return writeAtomic(path, raw)
}

// Read returns every JSON state file, retaining corrupt files as hard failures.
func Read(now time.Time, maxAge time.Duration) ([]Entry, error) {
	dir, err := xdg.StateFile("sign-health")
	if err != nil {
		return nil, err
	}
	files, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(files))
	for _, file := range files {
		if file.IsDir() || filepath.Ext(file.Name()) != ".json" {
			continue
		}
		entries = append(entries, readEntry(filepath.Join(dir, file.Name()), now, maxAge))
	}
	sort.Slice(entries, func(i, j int) bool { return entrySortKey(entries[i]) < entrySortKey(entries[j]) })
	return entries, nil
}

func readEntry(path string, now time.Time, maxAge time.Duration) Entry {
	raw, err := os.ReadFile(path) // #nosec G304 -- path is inside the XDG state directory // nosemgrep: go-path-traversal -- path comes from ReadDir under the fixed XDG state directory
	if err != nil {
		return Entry{File: path, Unreadable: true, Err: err.Error()}
	}
	var state State
	if err := json.Unmarshal(raw, &state); err != nil {
		return Entry{File: path, Unreadable: true, Err: err.Error()}
	}
	when, err := time.Parse(time.RFC3339, state.TS)
	if state.Agent == "" || state.TS == "" || err != nil {
		if err == nil {
			err = fmt.Errorf("missing agent or timestamp")
		}
		return Entry{File: path, Unreadable: true, Err: err.Error()}
	}
	return Entry{Record: state, Stale: maxAge > 0 && now.Sub(when) > maxAge}
}

func statePath(agent string) (string, error) {
	return xdg.StateFile(filepath.Join("sign-health", SanitizeAgent(agent)+".json"))
}

func writeAtomic(path string, raw []byte) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".sign-health-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()
	if _, err = tmp.Write(raw); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func isSafeASCII(r rune) bool {
	return r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-'
}

// truncateBytes keeps at most limit bytes without splitting a UTF-8 character.
func truncateBytes(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}
	return value[:cut]
}

func entrySortKey(entry Entry) string {
	if entry.Unreadable {
		return "\uffff" + entry.File
	}
	return entry.Record.Agent
}
