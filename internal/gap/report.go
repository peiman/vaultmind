package gap

import (
	"encoding/json"
	"sort"
	"time"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/rs/zerolog/log"
)

// Measure lists open gaps and the meter. A missing possible-gap log counts
// as zero. A log that cannot be read is logged and counted as zero, so a
// hint-log problem does not hide the notes.
func Measure(db *index.DB, vaultPath string, now time.Time) (Report, error) {
	if db == nil {
		return Report{Notes: []OpenNote{}}, nil
	}
	now = clock(now)
	rows, err := db.Query(`
		SELECT n.id, IFNULL(n.status, ''),
			COALESCE(q.value_json, ''),
			COALESCE(o.value_json, ''),
			COALESCE(c.value_json, '')
		FROM notes n
		LEFT JOIN frontmatter_kv q ON q.note_id = n.id AND q.key = 'question'
		LEFT JOIN frontmatter_kv o ON o.note_id = n.id AND o.key = 'opened_at'
		LEFT JOIN frontmatter_kv c ON c.note_id = n.id AND c.key = 'closed_at'
		WHERE n.type = ?
		ORDER BY n.id`, index.TypeGap)
	if err != nil {
		return Report{}, err
	}
	defer func() { _ = rows.Close() }()

	out := Report{Notes: []OpenNote{}}
	var spans []float64
	for rows.Next() {
		var id, status, questionRaw, openedRaw, closedRaw string
		if err := rows.Scan(&id, &status, &questionRaw, &openedRaw, &closedRaw); err != nil {
			return Report{}, err
		}
		openedAt := parseTime(openedRaw)
		switch status {
		case statusOpen:
			out.Opened++
			out.Notes = append(out.Notes, OpenNote{
				ID: id, Question: jsonString(questionRaw), AgeDays: ageDays(openedAt, now),
			})
		case statusClosed:
			out.Opened++
			out.Closed++
			if span, ok := closeSpan(openedAt, parseTime(closedRaw)); ok {
				spans = append(spans, span)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return Report{}, err
	}
	out.Median = median(spans)
	possible, since, logErr := readPossible(vaultPath)
	if logErr != nil {
		log.Debug().Err(logErr).Msg("possible-gap log unreadable")
	} else {
		out.Possible = possible
		out.PossibleSince = since
	}
	return out, nil
}

func jsonString(raw string) string {
	if raw == "" {
		return ""
	}
	var stored string
	if json.Unmarshal([]byte(raw), &stored) != nil {
		return ""
	}
	return stored
}

func parseTime(raw string) time.Time {
	stored := jsonString(raw)
	if stored == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, stored)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func ageDays(opened, now time.Time) int {
	if opened.IsZero() {
		return 0
	}
	days := int(now.Sub(opened) / (24 * time.Hour))
	if days < 0 {
		return 0
	}
	return days
}

func closeSpan(opened, closed time.Time) (float64, bool) {
	if opened.IsZero() || closed.IsZero() {
		return 0, false
	}
	return closed.Sub(opened).Hours() / 24, true
}

func median(samples []float64) *float64 {
	if len(samples) == 0 {
		return nil
	}
	sort.Float64s(samples)
	n := len(samples)
	var m float64
	if n%2 == 1 {
		m = samples[n/2]
	} else {
		m = (samples[n/2-1] + samples[n/2]) / 2
	}
	return &m
}
