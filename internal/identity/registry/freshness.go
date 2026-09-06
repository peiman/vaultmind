package registry

import (
	"encoding/json"
	"fmt"
	"time"
)

// Freshness reports a registry's validity window WITHOUT verifying the root
// signature.
//
// Age and authenticity are independent questions, and conflating them cost a
// day of mesh silence: doctor could only answer "is this registry stale?" by
// asking a daemon for a copy, its daemon probe is loopback-pinned by design,
// and the fleet's daemon is remote — so the question was structurally
// unanswerable while the answer sat in a file on disk. The signature needs a
// root key; the clock does not.
//
// This grants NO trust. An unverified registry claiming to be fresh proves
// nothing — the consuming daemon still runs the full check. It exists to
// PREDICT the daemon's freshness verdict locally, early enough to act.
func Freshness(env SignedRegistry) (validFrom, validUntil int64, err error) {
	var w wireRegistry
	if uerr := json.Unmarshal(env.Registry, &w); uerr != nil {
		return 0, 0, fmt.Errorf("registry: decode registry body for freshness: %w", uerr)
	}
	return w.ValidFrom, w.ValidUntil, nil
}

// IsStaleAt reports whether a registry is past maxStaleness or past its
// valid_until at now, mirroring VerifyAndLoad's freshness rule (both bounds
// INCLUSIVE) so a local prediction and the daemon's verdict cannot disagree
// about the boundary. A future valid_from is stale too: without that guard a
// clock-skewed registry yields negative age and looks perpetually fresh.
func IsStaleAt(validFrom, validUntil int64, now time.Time, maxStaleness time.Duration) bool {
	if now.Unix() > validUntil {
		return true
	}
	age := now.Sub(time.Unix(validFrom, 0))
	return age < 0 || age > maxStaleness
}
