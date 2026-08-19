package experiment

import (
	"fmt"
	"time"
)

// consumptionWindow is how long after an injection an access still counts as
// caused by it. Thirty minutes is a working-turn's length: long enough that a
// pointer read mid-task and opened after a detour still counts, short enough
// that tomorrow's unrelated read does not.
const consumptionWindow = 30 * time.Minute

// MemoryUse answers the question the tool has never asked about itself: when
// memory is surfaced to an agent, does the agent take it?
//
// WHAT THIS MEASURES, precisely — a note counts as consumed when it was
// surfaced by a hook-driven retrieval and then ACCESSED within
// consumptionWindow. It is a floor, not a truth, and it is wrong in both
// directions:
//
//   - It over-counts: an access the agent would have made anyway is credited to
//     the injection that happened to precede it.
//   - It under-counts, and this is the larger error: an injection READ IN
//     CONTEXT, with no note_access event, is invisible — and that is the most
//     likely form of real use. Pointer blocks are consumed by reading, not by
//     opening.
//
// It is worth having anyway. The first measurement was 5,900 injections and 26
// accesses over seven days; whatever the true rate is, a number that moves is
// the only way to tell whether a change to the delivery path helped. Before
// this, the honest answer to "is VaultMind used?" was an anecdote.
type MemoryUse struct {
	// WindowDays is the period measured, for rendering.
	WindowDays int

	// Injections counts notes surfaced by hook-driven retrievals — the rows an
	// agent was shown, not the number of queries.
	Injections int

	// Consumed counts injections followed by an access to that same note
	// within the window.
	Consumed int

	// BodiesDelivered counts injections whose event recorded that a body was
	// actually included. Zero for every event logged before that field existed,
	// which is most of them — so a low number here means "unknown" for old data
	// and "withheld" for new.
	BodiesDelivered int

	// PerCaller breaks the same numbers down by hook, because the recall hook
	// (every prompt) and the reach hook (decision points only) are different
	// bets and deserve separate verdicts.
	PerCaller []CallerUse
}

// CallerUse is one hook's slice of MemoryUse.
type CallerUse struct {
	Caller     string
	Injections int
	Consumed   int
}

// ConsumedRate returns consumed/injections as a fraction, 0 when nothing was
// surfaced. Callers render it; this does not decide how many decimals a number
// this small deserves.
func (m MemoryUse) ConsumedRate() float64 {
	if m.Injections == 0 {
		return 0
	}
	return float64(m.Consumed) / float64(m.Injections)
}

// hookCallerPattern matches the callers that inject memory into an agent's
// context. The CLI is excluded deliberately: a human or agent typing
// `vaultmind ask` has already decided to look, so counting it would measure
// intent that the hooks are supposed to create.
const hookCallerPattern = "vaultmind-%hook"

// The meter counts only AccessSourceRead (see activation_signal.go, where the
// source vocabulary lives).
//
// That distinction IS the meter. note_access is logged with source="ask"
// whenever a note is RETURNED by a query — so a later hook injection of the same
// note logs an access against it, and counting those makes every repeat look
// like consumption. With reach-hook payloads repeating 98% of the time, that is
// not a rounding error: on this machine it turned 47 real reads into 1,657 and
// the rate from 0.8% into 27%.
//
// "neighbors" is excluded for the same reason: a note pulled in as a graph
// neighbour was surfaced, not read.

// MemoryUseSince computes the meter over the last windowDays.
//
// Both sides of the window comparison go through datetime(). Events are stored
// as RFC3339 ("...T20:00:00Z") while SQLite's datetime() returns a
// space-separated form, and comparing the two as strings makes 'T' > ' ' — so
// the upper bound never matches and every access looks like it fell outside the
// window. The first hand-run of this measurement had that bug and under-reported
// consumption; the tests below are what caught it.
func (d *DB) MemoryUseSince(windowDays int) (*MemoryUse, error) {
	if windowDays <= 0 {
		return nil, fmt.Errorf("window must be positive, got %d", windowDays)
	}
	out := &MemoryUse{WindowDays: windowDays}

	since := fmt.Sprintf("-%d days", windowDays)
	window := fmt.Sprintf("+%d minutes", int(consumptionWindow.Minutes()))

	// One statement rather than a scan in Go: the injection set is the
	// cross-product of events and their result rows, which is 5,900 rows over a
	// week here and grows with use. SQLite does the join; we carry the totals.
	row := d.db.QueryRow(`
		WITH inj AS (
			SELECT e.timestamp AS t,
			       json_extract(r.value, '$.note_id') AS note_id,
			       COALESCE(json_extract(e.event_data, '$.body_delivered'), 0) AS body_delivered
			FROM events e
			JOIN sessions s ON s.session_id = e.session_id,
			     json_each(json_extract(e.event_data, '$.variants.hybrid.results')) r
			WHERE s.caller LIKE ?
			  AND e.timestamp > datetime('now', ?)
		),
		acc AS (
			SELECT timestamp AS t, json_extract(event_data, '$.note_id') AS note_id
			FROM events
			WHERE event_type = 'note_access'
			  AND json_extract(event_data, '$.source') = ?
			  AND timestamp > datetime('now', ?)
		)
		SELECT COUNT(*),
		       SUM(CASE WHEN EXISTS (
		             SELECT 1 FROM acc a
		             WHERE a.note_id = inj.note_id
		               AND datetime(a.t) BETWEEN datetime(inj.t) AND datetime(inj.t, ?)
		           ) THEN 1 ELSE 0 END),
		       SUM(CASE WHEN inj.body_delivered THEN 1 ELSE 0 END)
		FROM inj`,
		hookCallerPattern, since, string(AccessSourceRead), since, window)

	// SUM over zero rows is NULL, so these are nullable even though COUNT is not.
	var consumed, delivered *int
	if err := row.Scan(&out.Injections, &consumed, &delivered); err != nil {
		return nil, fmt.Errorf("computing memory use: %w", err)
	}
	if consumed != nil {
		out.Consumed = *consumed
	}
	if delivered != nil {
		out.BodiesDelivered = *delivered
	}

	perCaller, err := d.memoryUsePerCaller(since, window)
	if err != nil {
		return nil, err
	}
	out.PerCaller = perCaller
	return out, nil
}

// memoryUsePerCaller is the same measurement split by hook.
func (d *DB) memoryUsePerCaller(since, window string) ([]CallerUse, error) {
	rows, err := d.db.Query(`
		WITH inj AS (
			SELECT s.caller AS caller, e.timestamp AS t,
			       json_extract(r.value, '$.note_id') AS note_id
			FROM events e
			JOIN sessions s ON s.session_id = e.session_id,
			     json_each(json_extract(e.event_data, '$.variants.hybrid.results')) r
			WHERE s.caller LIKE ?
			  AND e.timestamp > datetime('now', ?)
		),
		acc AS (
			SELECT timestamp AS t, json_extract(event_data, '$.note_id') AS note_id
			FROM events
			WHERE event_type = 'note_access'
			  AND json_extract(event_data, '$.source') = ?
			  AND timestamp > datetime('now', ?)
		)
		SELECT caller, COUNT(*),
		       SUM(CASE WHEN EXISTS (
		             SELECT 1 FROM acc a
		             WHERE a.note_id = inj.note_id
		               AND datetime(a.t) BETWEEN datetime(inj.t) AND datetime(inj.t, ?)
		           ) THEN 1 ELSE 0 END)
		FROM inj GROUP BY caller ORDER BY COUNT(*) DESC`,
		hookCallerPattern, since, string(AccessSourceRead), since, window)
	if err != nil {
		return nil, fmt.Errorf("computing per-caller memory use: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []CallerUse
	for rows.Next() {
		var c CallerUse
		var consumed *int
		if err := rows.Scan(&c.Caller, &c.Injections, &consumed); err != nil {
			return nil, fmt.Errorf("scanning per-caller memory use: %w", err)
		}
		if consumed != nil {
			c.Consumed = *consumed
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
