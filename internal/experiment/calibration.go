package experiment

import (
	"database/sql"
	"errors"
	"fmt"
)

// CalibrationSnapshot is a per-vault retrieval-calibration measurement, stored
// in the experiment DB. Every field is a content-free scalar — no note ids, no
// content, no query text — so the snapshot doubles as the vault_features the
// federated study (Paper #2) aggregates, and can be exported as-is.
//
//   - NoiseFloor (N): the cosine an off-topic probe query gets to any note —
//     the relevance floor. Relevance R = top_cosine - N.
//   - NTNCosineMu/Sigma: the vault's note-to-note cosine dispersion (how tight
//     or loose the embedding space is). NTNSampleCount is the number of pairs
//     sampled.
type CalibrationSnapshot struct {
	CalibrationID    string  `json:"calibration_id"`
	CreatedAt        string  `json:"created_at"` // RFC3339
	EmbedderLabel    string  `json:"embedder_label"`
	EmbeddingDims    int     `json:"embedding_dims"`
	NoteCount        int     `json:"note_count"`
	NoiseFloor       float64 `json:"noise_floor"`
	NoiseFloorProbes int     `json:"noise_floor_probes"`
	ProbeSetVersion  int     `json:"probe_set_version"`
	NTNCosineMu      float64 `json:"ntn_cosine_mu"`
	NTNCosineSigma   float64 `json:"ntn_cosine_sigma"`
	NTNSampleCount   int     `json:"ntn_sample_count"`
	// VaultPath scopes the snapshot to one vault so the ask path reads THIS
	// vault's floor (LatestCalibrationForVault), not the globally-most-recent
	// one. `json:"-"` — it is a storage discriminator, NOT part of the
	// content-free federated-export surface, and must never be serialized.
	VaultPath string `json:"-"`
}

// StoreCalibration inserts a calibration snapshot. Snapshots are append-only
// (history is kept); LatestCalibration reads the most recent.
func (d *DB) StoreCalibration(snap *CalibrationSnapshot) error {
	// SYNC: this column list and its placeholders must stay aligned with
	// calibrationColumns + scanCalibration (the readers). Adding a column means
	// touching all three. (Kept as an inline literal — not the shared const —
	// because INSERT needs matching placeholders, not a bare column list.)
	_, err := d.db.Exec(
		`INSERT INTO calibration_snapshots (
            calibration_id, created_at, embedder_label, embedding_dims, note_count,
            noise_floor, noise_floor_probes, probe_set_version, ntn_cosine_mu, ntn_cosine_sigma, ntn_sample_count,
            vault_path
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		snap.CalibrationID, snap.CreatedAt, snap.EmbedderLabel, snap.EmbeddingDims, snap.NoteCount,
		snap.NoiseFloor, snap.NoiseFloorProbes, snap.ProbeSetVersion, snap.NTNCosineMu, snap.NTNCosineSigma, snap.NTNSampleCount,
		snap.VaultPath,
	)
	if err != nil {
		return fmt.Errorf("storing calibration snapshot: %w", err)
	}
	return nil
}

// calibrationColumns is the shared SELECT list for reading a snapshot, kept in
// one place (Principle 7) so the two readers below can't drift in column order.
// The full query strings are assembled from it as compile-time constants (no
// runtime concatenation), so the QueryRow call sites pass a single named const
// — not a built-up string — keeping them clear of the SQL-injection lint while
// the column list stays single-sourced.
const calibrationColumns = `calibration_id, created_at, embedder_label, embedding_dims, note_count,
        noise_floor, noise_floor_probes, probe_set_version, ntn_cosine_mu, ntn_cosine_sigma, ntn_sample_count,
        vault_path`

const latestCalibrationQuery = `SELECT ` + calibrationColumns + `
        FROM calibration_snapshots
        ORDER BY created_at DESC, rowid DESC
        LIMIT 1`

const latestCalibrationForVaultQuery = `SELECT ` + calibrationColumns + `
        FROM calibration_snapshots
        WHERE vault_path = ?
        ORDER BY created_at DESC, rowid DESC
        LIMIT 1`

// scanCalibration scans one row in calibrationColumns order. Returns (nil, nil)
// on sql.ErrNoRows so callers fall back to the shipped per-embedder default.
func scanCalibration(row *sql.Row) (*CalibrationSnapshot, error) {
	var s CalibrationSnapshot
	var vaultPath sql.NullString // pre-v7 rows have no vault
	switch err := row.Scan(
		&s.CalibrationID, &s.CreatedAt, &s.EmbedderLabel, &s.EmbeddingDims, &s.NoteCount,
		&s.NoiseFloor, &s.NoiseFloorProbes, &s.ProbeSetVersion, &s.NTNCosineMu, &s.NTNCosineSigma, &s.NTNSampleCount,
		&vaultPath,
	); {
	case err == nil:
		s.VaultPath = vaultPath.String
		return &s, nil
	case errors.Is(err, sql.ErrNoRows):
		return nil, nil //nolint:nilnil // no calibration yet → caller uses the shipped default
	default:
		return nil, fmt.Errorf("reading latest calibration: %w", err)
	}
}

// LatestCalibration returns the most recently created snapshot across ALL
// vaults, or (nil, nil) when none exists. Use this only for vault-agnostic
// checks; the ask path must use LatestCalibrationForVault to avoid reading
// another vault's floor.
func (d *DB) LatestCalibration() (*CalibrationSnapshot, error) {
	return scanCalibration(d.db.QueryRow(latestCalibrationQuery))
}

// LatestCalibrationForVault returns the most recent snapshot for one vault, or
// (nil, nil) when that vault has none. This is the lookup the ask path uses:
// the experiment DB is global (many vaults), so scoping by vault_path is what
// keeps a query against vault B from reading vault A's noise floor. Callers
// should pass a filepath.Clean'd path so "/a/b" and "/a/b/" match the stored
// key.
func (d *DB) LatestCalibrationForVault(vaultPath string) (*CalibrationSnapshot, error) {
	return scanCalibration(d.db.QueryRow(latestCalibrationForVaultQuery, vaultPath))
}
