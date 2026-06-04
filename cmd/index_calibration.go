package cmd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"time"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/noisefloor"
	"github.com/peiman/vaultmind/internal/query"
)

// calibrateVaultNoiseFloor measures the vault's noise floor N + note-to-note
// dispersion from its embeddings and stores a content-free snapshot in the
// experiment DB, so `ask` uses the vault's real floor instead of the shipped
// per-embedder default. Best-effort: any failure is the caller's to log
// non-fatally — calibration must never fail an index run.
//
// To avoid reloading the embedding model on a no-op re-embed, the measurement
// is skipped when the index did not change (nothing newly embedded AND nothing
// deleted) AND a CURRENT-probe-set calibration already exists. Deletions matter
// because removing notes shifts the noise floor and dispersion even when no new
// embeddings were written. A ProbeSetVersion bump also forces a re-measure: an
// older snapshot's N was measured against a different probe set and is stale.
func calibrateVaultNoiseFloor(ctx context.Context, vaultPath, dbPath, model string, expDB *experiment.DB, newlyEmbedded, deleted int) error {
	vaultKey := canonicalVaultKey(vaultPath)
	if newlyEmbedded == 0 && deleted == 0 {
		// Scope the skip-check to THIS vault. A global LatestCalibration() would
		// let a no-op re-index of vault B skip calibration because vault A has a
		// snapshot, leaving B permanently uncalibrated. The probe-set-version
		// match ensures a probe-set change re-calibrates instead of skipping.
		if existing, err := expDB.LatestCalibrationForVault(vaultKey); err == nil &&
			existing != nil && existing.ProbeSetVersion == noisefloor.ProbeSetVersion {
			return nil
		}
	}

	// index.Open runs goose migrations under their own context by design; the
	// ctx here is for the embedding measurement below, not the DB open.
	db, err := index.Open(dbPath) //nolint:contextcheck // Open owns its migration context; ctx is for Embed
	if err != nil {
		return fmt.Errorf("opening index for calibration: %w", err)
	}
	defer func() { _ = db.Close() }()

	ret := query.BuildAutoRetrieverFull(db)
	defer ret.Cleanup()
	if ret.Embedder == nil {
		return fmt.Errorf("no embedder available for calibration (keyword-only vault)")
	}

	cal, err := query.MeasureNoiseFloor(ctx, ret.Embedder, db)
	if err != nil {
		return err
	}

	id, err := randomHexID()
	if err != nil {
		return err
	}
	return expDB.StoreCalibration(&experiment.CalibrationSnapshot{
		CalibrationID:    id,
		VaultPath:        vaultKey,
		CreatedAt:        time.Now().UTC().Format(time.RFC3339),
		EmbedderLabel:    model,
		EmbeddingDims:    cal.EmbeddingDims,
		NoteCount:        cal.NoteCount,
		NoiseFloor:       cal.NoiseFloor,
		NoiseFloorProbes: cal.NoiseFloorProbes,
		ProbeSetVersion:  cal.ProbeSetVersion,
		NTNCosineMu:      cal.NTNCosineMu,
		NTNCosineSigma:   cal.NTNCosineSigma,
		NTNSampleCount:   cal.NTNSampleCount,
	})
}

// canonicalVaultKey turns a vault path into a stable key for calibration
// scoping, used identically on the write side (here) and the read side (ask) so
// they agree. The default vault path is "." (relative), so without
// canonicalization two vaults indexed from different working directories would
// both store "." and collide — one reading the other's noise floor. filepath.Abs
// resolves to an absolute path; EvalSymlinks collapses symlinked aliases of the
// same directory. Both fall back gracefully so a path that doesn't exist yet
// still yields a deterministic key.
func canonicalVaultKey(vaultPath string) string {
	abs, err := filepath.Abs(vaultPath)
	if err != nil {
		return filepath.Clean(vaultPath)
	}
	if resolved, evalErr := filepath.EvalSymlinks(abs); evalErr == nil {
		return resolved
	}
	return abs
}

func randomHexID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating calibration id: %w", err)
	}
	return hex.EncodeToString(b), nil
}
