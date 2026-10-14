package gap

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/peiman/vaultmind/internal/noisefloor"
	"github.com/rs/zerolog/log"
)

// possibleFile is local vault state, not a note. A hint is not knowledge.
const possibleFile = "possible-gaps.jsonl"

// RecordPossible appends a weak or no-match ask to the vault's hint log.
// Strong, moderate, and empty bands are not hints. The raw query is stored
// only when storeQuery is set; otherwise the line keeps a hash and a length.
func RecordPossible(vaultPath, query, band string, storeQuery bool, now time.Time) error {
	if !hintBand(band) {
		return nil
	}
	path := filepath.Join(vaultPath, ".vaultmind", possibleFile)
	rec := possibleRecord{
		At:          clock(now).Format(time.RFC3339),
		Band:        band,
		QuerySHA256: hashQuery(query),
		QueryLen:    len(query),
	}
	if storeQuery {
		rec.Query = query
	}
	line, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("encoding possible gap: %w", err)
	}
	line = append(line, '\n')
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640) //nolint:gosec // path is the vault's own state file
	if err != nil {
		log.Debug().Err(err).Str("path", path).Msg("possible-gap log open failed")
		return fmt.Errorf("opening possible-gap log: %w", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(line); err != nil {
		log.Debug().Err(err).Str("path", path).Msg("possible-gap log write failed")
		return fmt.Errorf("writing possible-gap log: %w", err)
	}
	return nil
}

func hintBand(band string) bool {
	return band == noisefloor.ConfidenceWeak || band == noisefloor.ConfidenceNoMatch
}

func hashQuery(query string) string {
	sum := sha256.Sum256([]byte(query))
	return hex.EncodeToString(sum[:])
}

type possibleRecord struct {
	At          string `json:"at"`
	Band        string `json:"band"`
	QuerySHA256 string `json:"query_sha256"`
	QueryLen    int    `json:"query_len"`
	Query       string `json:"query,omitempty"`
}

func readPossible(vaultPath string) (int, string, error) {
	path := filepath.Join(vaultPath, ".vaultmind", possibleFile)
	f, err := os.OpenFile(path, os.O_RDONLY, 0) //nolint:gosec // path is the vault's own state file
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
