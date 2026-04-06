package index

import (
	"fmt"
	"math"
	"regexp"
	"strings"
)

var (
	codeFenceRe   = regexp.MustCompile("(?s)(```[^`]*```|~~~[^~]*~~~)")
	inlineCodeRe  = regexp.MustCompile("`[^`]+`")
	wikilinkRe    = regexp.MustCompile(`\[\[([^\]|]+)\|([^\]]+)\]\]|\[\[([^\]]+)\]\]`)
	htmlCommentRe = regexp.MustCompile(`<!--[\s\S]*?-->`)
)

// StripForAliasMatch removes markup that should be excluded from alias
// detection: code fences, inline code, wikilinks (keeping aliased display
// text), and HTML comments.
func StripForAliasMatch(body string) string {
	result := codeFenceRe.ReplaceAllString(body, "")
	result = inlineCodeRe.ReplaceAllString(result, "")
	result = wikilinkRe.ReplaceAllStringFunc(result, func(match string) string {
		inner := strings.TrimPrefix(strings.TrimSuffix(match, "]]"), "[[")
		if idx := strings.Index(inner, "|"); idx >= 0 {
			return inner[idx+1:]
		}
		return ""
	})
	result = htmlCommentRe.ReplaceAllString(result, "")
	return result
}

// ComputeAliasMentions scans every note body for occurrences of aliases and
// domain-note titles, then writes alias_mention edges into the links table.
// It returns the number of new edges inserted. Edges shorter than minAliasLen
// characters are skipped. Calling this function clears any previous
// alias_mention edges before computing fresh ones.
func ComputeAliasMentions(db *DB, minAliasLen int) (int, error) {
	if _, err := db.Exec("DELETE FROM links WHERE edge_type = 'alias_mention'"); err != nil {
		return 0, fmt.Errorf("clearing old alias_mention edges: %w", err)
	}

	type aliasEntry struct {
		noteID string
		text   string
	}

	rows, err := db.Query(`
		SELECT note_id, alias FROM aliases
		UNION
		SELECT id, title FROM notes WHERE is_domain = TRUE AND title != ''`)
	if err != nil {
		return 0, fmt.Errorf("querying aliases: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var entries []aliasEntry
	aliasToNoteID := make(map[string]string)
	for rows.Next() {
		var e aliasEntry
		if err := rows.Scan(&e.noteID, &e.text); err != nil {
			return 0, fmt.Errorf("scanning alias: %w", err)
		}
		if len(e.text) < minAliasLen {
			continue
		}
		entries = append(entries, e)
		aliasToNoteID[strings.ToLower(e.text)] = e.noteID
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(entries) == 0 {
		return 0, nil
	}

	patterns := make([]string, 0, len(entries))
	for _, e := range entries {
		patterns = append(patterns, regexp.QuoteMeta(e.text))
	}
	pattern := `(?i)\b(` + strings.Join(patterns, "|") + `)\b`
	re, err := regexp.Compile(pattern)
	if err != nil {
		return 0, fmt.Errorf("compiling alias regex: %w", err)
	}

	noteRows, err := db.Query("SELECT id, body_text FROM notes")
	if err != nil {
		return 0, fmt.Errorf("querying note bodies: %w", err)
	}
	defer func() { _ = noteRows.Close() }()

	type edge struct{ src, dst string }
	edgeSet := make(map[edge]bool)

	for noteRows.Next() {
		var noteID, body string
		if err := noteRows.Scan(&noteID, &body); err != nil {
			continue
		}
		stripped := StripForAliasMatch(body)
		matches := re.FindAllString(stripped, -1)
		for _, match := range matches {
			targetID, ok := aliasToNoteID[strings.ToLower(match)]
			if !ok || targetID == noteID {
				continue
			}
			edgeSet[edge{src: noteID, dst: targetID}] = true
		}
	}

	count := 0
	for e := range edgeSet {
		_, err := db.Exec(`
			INSERT OR IGNORE INTO links
			  (src_note_id, dst_note_id, dst_raw, edge_type, resolved, confidence, origin)
			VALUES (?, ?, ?, 'alias_mention', TRUE, 'medium', 'body:alias_scan')`,
			e.src, e.dst, e.dst)
		if err != nil {
			continue
		}
		count++
	}
	return count, nil
}

// ComputeTagOverlap scans the tags table for notes sharing common tags and
// writes tag_overlap edges into the links table weighted by TF-IDF-style tag
// specificity. Only pairs whose combined score meets threshold are inserted.
// Calling this function clears any previous tag_overlap edges before computing
// fresh ones.
func ComputeTagOverlap(db *DB, threshold float64) (int, error) {
	if _, err := db.Exec("DELETE FROM links WHERE edge_type = 'tag_overlap'"); err != nil {
		return 0, fmt.Errorf("clearing old tag_overlap edges: %w", err)
	}

	var totalNotes int
	if err := db.QueryRow("SELECT COUNT(*) FROM notes WHERE is_domain = TRUE").Scan(&totalNotes); err != nil {
		return 0, fmt.Errorf("counting domain notes: %w", err)
	}
	if totalNotes == 0 {
		return 0, nil
	}

	tagRows, err := db.Query("SELECT tag, COUNT(*) FROM tags GROUP BY tag")
	if err != nil {
		return 0, fmt.Errorf("counting tags: %w", err)
	}
	defer func() { _ = tagRows.Close() }()

	specificity := make(map[string]float64)
	for tagRows.Next() {
		var tag string
		var count int
		if err := tagRows.Scan(&tag, &count); err != nil {
			continue
		}
		if count > 0 {
			specificity[tag] = math.Log(float64(totalNotes) / float64(count))
		}
	}

	pairRows, err := db.Query(`
		SELECT a.note_id, b.note_id, a.tag
		FROM tags a JOIN tags b ON a.tag = b.tag AND a.note_id < b.note_id`)
	if err != nil {
		return 0, fmt.Errorf("finding tag pairs: %w", err)
	}
	defer func() { _ = pairRows.Close() }()

	type pair struct{ a, b string }
	scores := make(map[pair]float64)
	for pairRows.Next() {
		var noteA, noteB, tag string
		if err := pairRows.Scan(&noteA, &noteB, &tag); err != nil {
			continue
		}
		scores[pair{a: noteA, b: noteB}] += specificity[tag]
	}

	count := 0
	for p, score := range scores {
		if score < threshold {
			continue
		}
		_, err := db.Exec(`
			INSERT OR IGNORE INTO links
			  (src_note_id, dst_note_id, dst_raw, edge_type, resolved, confidence, origin, weight)
			VALUES (?, ?, ?, 'tag_overlap', TRUE, 'low', 'tag_overlap_scan', ?)`,
			p.a, p.b, p.b, score)
		if err != nil {
			continue
		}
		count++
	}
	return count, nil
}
