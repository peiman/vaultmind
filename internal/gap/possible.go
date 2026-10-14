package gap

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/peiman/vaultmind/internal/noisefloor"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/peiman/vaultmind/internal/xdg"
	"github.com/rs/zerolog/log"
)

const (
	// DefaultLogLimit is how many hint records a compaction keeps. The file
	// may hold twice that many before it is rewritten. app.ask.gap_log_limit
	// defaults to this number. A non-positive limit uses it too, so a missing
	// config value cannot wipe the log.
	DefaultLogLimit = 1000

	// saltLen is the install salt, in bytes. One salt per user install.
	saltLen = 32

	// saltFile sits in the XDG state directory, next to possible-gaps/.
	// One salt for every vault: the log is already one file per vault, and
	// the salt only stops a dictionary of unsalted query hashes. A second
	// salt per vault would make the same question hash differently and add
	// a file that has to stay in lockstep with the log.
	saltFile = "possible-gap-salt"

	possibleDirName = "possible-gaps"
)

// RecordPossible appends a weak or no-match ask to the per-user hint log.
// Strong, moderate, and empty bands are not hints. The raw query is stored
// only when storeQuery is set; otherwise the line keeps a salted hash and a
// length.
//
// The log is not vault content. It lives under xdg.StateDir, the same
// resolver episode cursors and the signer socket use: one 0600 file per
// vault, named by the sha256 of vault.CanonicalKey. The directory is 0700.
// An old .vaultmind/possible-gaps.jsonl is not read. It never shipped.
//
// limit is the retained window. When the file grows past twice that, it is
// rewritten to the newest limit records. The scan keeps at most twice the
// window in memory, so one write stays bounded even if a previous file is large.
func RecordPossible(vaultPath, query, band string, storeQuery bool, limit int, now time.Time) error {
	if !hintBand(band) {
		return nil
	}
	limit = normalizeLimit(limit)
	sum, err := hashQuery(query)
	if err != nil {
		return err
	}
	path, err := possibleLogPath(vaultPath)
	if err != nil {
		log.Debug().Err(err).Msg("possible-gap log path failed")
		return fmt.Errorf("opening possible-gap log: %w", err)
	}
	rec := possibleRecord{
		At:          clock(now).Format(time.RFC3339),
		Band:        band,
		QuerySHA256: sum,
		QueryLen:    len(query),
	}
	if storeQuery {
		rec.Query = query
	}
	line, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("encoding possible gap: %w", err)
	}
	if err := appendPossible(path, line); err != nil {
		log.Debug().Err(err).Str("path", path).Msg("possible-gap log write failed")
		return fmt.Errorf("writing possible-gap log: %w", err)
	}
	if err := compactPossible(path, limit); err != nil {
		log.Debug().Err(err).Str("path", path).Msg("possible-gap log compact failed")
		return fmt.Errorf("bounding possible-gap log: %w", err)
	}
	return nil
}

func hintBand(band string) bool {
	return band == noisefloor.ConfidenceWeak || band == noisefloor.ConfidenceNoMatch
}

func normalizeLimit(n int) int {
	if n < 1 {
		return DefaultLogLimit
	}
	return n
}

// hashQuery is sha256(salt || query). The salt is raw bytes, not hex.
func hashQuery(query string) (string, error) {
	salt, err := installSalt()
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, _ = h.Write(salt)
	_, _ = h.Write([]byte(query))
	return hex.EncodeToString(h.Sum(nil)), nil
}

func installSalt() ([]byte, error) {
	root, err := xdg.StateDir()
	if err != nil {
		return nil, fmt.Errorf("opening possible-gap salt: %w", err)
	}
	path := filepath.Join(root, saltFile)
	salt, err := readSalt(path)
	if err == nil {
		return salt, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	salt = make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("generating possible-gap salt: %w", err)
	}
	// O_EXCL so two first uses cannot overwrite each other. The loser reads
	// the winner. 0600 survives umask: umask only clears bits.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // path is the app's own XDG state file
	if err != nil {
		if os.IsExist(err) {
			return readSalt(path)
		}
		return nil, fmt.Errorf("creating possible-gap salt: %w", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(salt); err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("writing possible-gap salt: %w", err)
	}
	if err := f.Chmod(0o600); err != nil {
		return nil, fmt.Errorf("restricting possible-gap salt: %w", err)
	}
	return salt, nil
}

func readSalt(path string) ([]byte, error) {
	// nosemgrep: go-path-traversal -- path is the app's own XDG state file, not a caller path
	b, err := os.ReadFile(path) //nolint:gosec // path is the app's own XDG state file
	if err != nil {
		return nil, err
	}
	if len(b) != saltLen {
		return nil, fmt.Errorf("possible-gap salt is %d bytes, want %d", len(b), saltLen)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return nil, fmt.Errorf("restricting possible-gap salt: %w", err)
	}
	return b, nil
}

func possibleDir() (string, error) {
	root, err := xdg.StateDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, possibleDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("creating possible-gap directory: %w", err)
	}
	// G302 wants 0600 or less. This is a directory: 0600 would not be traversable, and 0700 is the required mode.
	if err := os.Chmod(dir, 0o700); err != nil { //nolint:gosec // G302: directory mode 0700, not a file
		return "", fmt.Errorf("restricting possible-gap directory: %w", err)
	}
	return dir, nil
}

func possibleLogPath(vaultPath string) (string, error) {
	dir, err := possibleDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(vault.CanonicalKey(vaultPath)))
	return filepath.Join(dir, hex.EncodeToString(sum[:])+".jsonl"), nil
}

func appendPossible(path string, line []byte) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // path is the app's own XDG state file
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if err := f.Chmod(0o600); err != nil {
		return err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}
	return nil
}

// compactPossible rewrites the file to the newest limit records once it
// holds more than twice that many. Until then the append stands. The tail
// never keeps more than 2*limit lines, so a large file does not allocate
// one buffer per record.
func compactPossible(path string, limit int) error {
	// nosemgrep: go-path-traversal -- path is the app's own XDG state file, not a caller path
	f, err := os.Open(path) //nolint:gosec // path is the app's own XDG state file
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	var tail [][]byte
	var count int
	ceiling := 2 * limit
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		dup := append([]byte(nil), line...)
		count++
		switch {
		case count <= ceiling:
			tail = append(tail, dup)
		case count == ceiling+1:
			start := len(tail) - (limit - 1)
			if start < 0 {
				start = 0
			}
			kept := append([][]byte{}, tail[start:]...)
			tail = append(kept, dup)
		default:
			copy(tail, tail[1:])
			tail[len(tail)-1] = dup
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if count <= ceiling {
		return nil
	}
	return rewritePossible(path, tail)
}

func rewritePossible(path string, lines [][]byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".possible-gaps-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	done := false
	defer func() {
		if !done {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	for _, line := range lines {
		if _, err := tmp.Write(line); err != nil {
			_ = tmp.Close()
			return err
		}
		if _, err := tmp.Write([]byte{'\n'}); err != nil {
			_ = tmp.Close()
			return err
		}
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	done = true
	return os.Chmod(path, 0o600)
}

type possibleRecord struct {
	At          string `json:"at"`
	Band        string `json:"band"`
	QuerySHA256 string `json:"query_sha256"`
	QueryLen    int    `json:"query_len"`
	Query       string `json:"query,omitempty"`
}

func readPossible(vaultPath string) (int, string, error) {
	path, err := possibleLogPath(vaultPath)
	if err != nil {
		return 0, "", err
	}
	// nosemgrep: go-path-traversal -- path is the app's own XDG state file, not a caller path
	f, err := os.Open(path) //nolint:gosec // path is the app's own XDG state file
	if err != nil {
		if os.IsNotExist(err) {
			return 0, "", nil
		}
		return 0, "", err
	}
	defer func() { _ = f.Close() }()
	var count int
	var earliest time.Time
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec possibleRecord
		if json.Unmarshal(line, &rec) != nil || rec.At == "" {
			continue
		}
		at, err := time.Parse(time.RFC3339, rec.At)
		if err != nil {
			continue
		}
		count++
		if earliest.IsZero() || at.Before(earliest) {
			earliest = at
		}
	}
	if err := sc.Err(); err != nil {
		return 0, "", err
	}
	if count == 0 {
		return 0, "", nil
	}
	return count, earliest.UTC().Format("2006-01-02"), nil
}
