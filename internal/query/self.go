package query

import (
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/index"
)

// SelfConfig holds parameters for the self-introspection view.
type SelfConfig struct {
	Limit          int
	StaleThreshold time.Duration
	DecayD         float64
	Now            time.Time
}

// SelfDefaults fills zero-value SelfConfig fields with sensible defaults.
func SelfDefaults(cfg SelfConfig) SelfConfig {
	if cfg.Limit <= 0 {
		cfg.Limit = 10
	}
	if cfg.StaleThreshold <= 0 {
		cfg.StaleThreshold = 7 * 24 * time.Hour
	}
	if cfg.DecayD <= 0 {
		cfg.DecayD = 0.5
	}
	if cfg.Now.IsZero() {
		cfg.Now = time.Now().UTC()
	}
	return cfg
}

// RunSelf renders the agent's memory state — recent / hot / stale.
// Reads only existing schema columns (access_count, last_accessed_at,
// title, type). Three sections:
//
//   - Recent: notes touched most recently, regardless of count.
//   - Hot: notes ranked by approximate ACT-R activation
//     (ln(1+count) - d*ln(elapsed_hours)), capturing both frequency
//     and recency in one number.
//   - Stale: accessed notes whose last_accessed_at is older than the
//     stale threshold, sorted by activation desc.
//
// Empty vault prints "no accesses recorded yet" so the caller can tell
// blank-slate from rendering failure.
func RunSelf(db *index.DB, cfg SelfConfig, w io.Writer) error {
	cfg = SelfDefaults(cfg)
	all, err := index.ListAccessedNotes(db)
	if err != nil {
		return fmt.Errorf("self: listing accessed notes: %w", err)
	}
	if len(all) == 0 {
		_, err = fmt.Fprintln(w, "no accesses recorded yet — query the vault and come back")
		return err
	}

	recent := capN(all, cfg.Limit)

	hot := make([]selfRow, 0, len(all))
	for _, s := range all {
		hot = append(hot, toRow(s, cfg))
	}
	sort.SliceStable(hot, func(i, j int) bool { return hot[i].activation > hot[j].activation })
	hot = capRows(hot, cfg.Limit)

	staleCutoff := cfg.Now.Add(-cfg.StaleThreshold)
	var stale []selfRow
	for _, s := range all {
		if s.LastAccessedAt == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339Nano, s.LastAccessedAt)
		if err != nil {
			continue
		}
		if t.Before(staleCutoff) {
			stale = append(stale, toRow(s, cfg))
		}
	}
	sort.SliceStable(stale, func(i, j int) bool { return stale[i].activation > stale[j].activation })
	stale = capRows(stale, cfg.Limit)

	if _, err = fmt.Fprintf(w, "Memory state — %d accessed notes\n\n", len(all)); err != nil {
		return err
	}
	if _, err = fmt.Fprintln(w, "Recent (newest first):"); err != nil {
		return err
	}
	for _, s := range recent {
		if _, err = fmt.Fprintf(w, "  %-7s  %-50s  count %d\n", agoString(s, cfg.Now), selfTruncate(s.NoteID, 50), s.AccessCount); err != nil {
			return err
		}
	}
	// Normalize activation column so the top hit shows 0.00 and others
	// show how-much-below. Raw activations can go negative when the
	// decay term dominates (correct math, confusing to read at a glance).
	// Shifting by the max is order-preserving and turns the column into
	// a relative-distance signal that doesn't snag the eye on minus signs.
	hotMax := topActivation(hot)
	staleMax := topActivation(stale)

	if _, err = fmt.Fprintln(w, "\nHot (top activation):"); err != nil {
		return err
	}
	for _, r := range hot {
		if _, err = fmt.Fprintf(w, "  %+5.2f  %-50s  count %d, %s\n", r.activation-hotMax, selfTruncate(r.NoteID, 50), r.AccessCount, agoString(r.NoteAccessStats, cfg.Now)); err != nil {
			return err
		}
	}
	if len(stale) == 0 {
		_, err = fmt.Fprintf(w, "\nStale (older than %s): none\n", humanDuration(cfg.StaleThreshold))
		return err
	}
	if _, err = fmt.Fprintf(w, "\nStale (older than %s, drifting away):\n", humanDuration(cfg.StaleThreshold)); err != nil {
		return err
	}
	for _, r := range stale {
		if _, err = fmt.Fprintf(w, "  %+5.2f  %-50s  count %d, %s\n", r.activation-staleMax, selfTruncate(r.NoteID, 50), r.AccessCount, agoString(r.NoteAccessStats, cfg.Now)); err != nil {
			return err
		}
	}
	return nil
}

// topActivation returns the highest activation in a sorted-desc rows
// slice, or 0 when empty (so subtraction is a no-op for the empty case).
func topActivation(rows []selfRow) float64 {
	if len(rows) == 0 {
		return 0
	}
	return rows[0].activation
}

// humanDuration formats a duration as "Nd" / "Nh" / "Nm" — the same
// units agoString uses, so the stale-threshold label matches the
// per-row "ago" column instead of mixing "168h0m0s" with "7d".
func humanDuration(d time.Duration) string {
	switch {
	case d >= 24*time.Hour:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day"
		}
		return fmt.Sprintf("%d days", days)
	case d >= time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
}

type selfRow struct {
	index.NoteAccessStats
	activation float64
}

func toRow(s index.NoteAccessStats, cfg SelfConfig) selfRow {
	var lastT time.Time
	if s.LastAccessedAt != "" {
		if t, err := time.Parse(time.RFC3339Nano, s.LastAccessedAt); err == nil {
			lastT = t
		}
	}
	act := experiment.ComputeApproxRetrieval(s.AccessCount, lastT, cfg.Now, nil, 1.0, cfg.DecayD)
	return selfRow{NoteAccessStats: s, activation: act}
}

func capN[T any](s []T, n int) []T {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func capRows(s []selfRow, n int) []selfRow { return capN(s, n) }

func agoString(s index.NoteAccessStats, now time.Time) string {
	if s.LastAccessedAt == "" {
		return "?"
	}
	t, err := time.Parse(time.RFC3339Nano, s.LastAccessedAt)
	if err != nil {
		return "?"
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func selfTruncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
