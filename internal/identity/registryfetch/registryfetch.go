// Package registryfetch decides whether a registry fetched from the hub may
// replace this machine's local copy.
//
// WHY. Member machines had no local copy of the signed registry (dabir,
// 2026-09-22): nothing there could verify it, and nothing could count down to
// its lapse. The hub already serves it at /.well-known/vaultmind-directory, so
// members fetch it. The fetch channel proves nothing — trust comes only from
// the root signature checked against a PINNED root — and a fetch may never move
// the local copy backwards, so a hub or anything in between cannot roll a
// member back to an older validly signed registry.
package registryfetch

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/peiman/vaultmind/internal/identity/registry"
)

var (
	// ErrNotARegistry: the bytes are not a signed-registry envelope at all.
	ErrNotARegistry = errors.New("registryfetch: not a signed registry")
	// ErrNotVerified: the signature does not verify against the pinned root,
	// or the registry is past its own valid_until.
	ErrNotVerified = errors.New("registryfetch: registry does not verify against the pinned root")
	// ErrRollback: the fetched epoch is older than the local copy's.
	ErrRollback = errors.New("registryfetch: fetched registry is OLDER than the local copy (rollback refused)")
)

// Result reports what Update did.
type Result struct {
	Epoch         int   // the fetched (verified) epoch
	PreviousEpoch int   // the local copy's epoch; 0 when there was none
	Changed       bool  // the local copy was replaced
	ValidFrom     int64 // the fetched registry's valid_from (unix seconds)
	ValidUntil    int64 // the fetched registry's valid_until (unix seconds)
}

// noAgeLimit: age is not this package's judgement. The registry's own
// valid_until still applies; the hub's max_staleness is reported elsewhere
// (doctor, the watcher's countdown) against the operator's declared bound.
const noAgeLimit = time.Duration(math.MaxInt64)

// Update verifies fetched against pinnedRoot and, when it is at least as new as
// the local copy at localPath, stores it byte-exact (atomically). A same-epoch
// fetch writes nothing. Every refusal leaves the local copy untouched.
func Update(pinnedRoot ed25519.PublicKey, fetched []byte, localPath string, now time.Time) (Result, error) {
	env, err := registry.ParseDistribution(fetched)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %w", ErrNotARegistry, err)
	}
	reg, _, err := registry.VerifyAndLoad(pinnedRoot, env, 0, now, noAgeLimit)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %w", ErrNotVerified, err)
	}
	res := Result{Epoch: reg.Epoch, ValidFrom: reg.ValidFrom, ValidUntil: reg.ValidUntil}

	res.PreviousEpoch = localEpoch(pinnedRoot, localPath)
	switch {
	case res.Epoch < res.PreviousEpoch:
		return res, fmt.Errorf("%w: fetched epoch %d, local epoch %d", ErrRollback, res.Epoch, res.PreviousEpoch)
	case res.Epoch == res.PreviousEpoch:
		return res, nil
	}
	if err := writeAtomic(localPath, fetched); err != nil {
		return res, err
	}
	res.Changed = true
	return res, nil
}

// localEpoch is the epoch of the local copy when it is signed by pinnedRoot,
// else 0 (absent, unreadable, or not ours — any of which a verified fetch
// should replace, not defer to).
//
// The signature is checked AT THE LOCAL COPY'S OWN valid_from, so an expired
// local copy still counts: expiry must not reopen the door to an older epoch.
func localEpoch(pinnedRoot ed25519.PublicKey, path string) int {
	// Operator-declared registry_path (agents.yaml or --registry-file), the
	// same trust tier as the sibling resolvers in cmd/doctor_mesh.go.
	// #nosec G304
	// nosemgrep: go-path-traversal
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	env, err := registry.ParseDistribution(raw)
	if err != nil {
		return 0
	}
	validFrom, _, err := registry.Freshness(env)
	if err != nil {
		return 0
	}
	reg, _, err := registry.VerifyAndLoad(pinnedRoot, env, 0, time.Unix(validFrom, 0), noAgeLimit)
	if err != nil {
		return 0
	}
	return reg.Epoch
}

// writeAtomic replaces path by rename, so a reader never sees half a registry.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("registryfetch: creating %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".registry-*.tmp")
	if err != nil {
		return fmt.Errorf("registryfetch: temp file: %w", err)
	}
	name := tmp.Name()
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		_ = os.Remove(name)
		return fmt.Errorf("registryfetch: writing temp file: %w", errors.Join(werr, cerr))
	}
	if err := os.Chmod(name, 0o644); err != nil { //nolint:gosec // G302: the registry is public data (pubkeys + root signature)
		_ = os.Remove(name)
		return fmt.Errorf("registryfetch: chmod: %w", err)
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("registryfetch: replacing %s: %w", path, err)
	}
	return nil
}
