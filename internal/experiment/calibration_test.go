package experiment_test

import (
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openCalDB(t *testing.T) *experiment.DB {
	t.Helper()
	db, err := experiment.Open(filepath.Join(t.TempDir(), "exp.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestLatestCalibration_NilWhenEmpty(t *testing.T) {
	db := openCalDB(t)
	snap, err := db.LatestCalibration()
	require.NoError(t, err)
	assert.Nil(t, snap, "no calibration stored yet → nil, nil")
}

func TestCalibration_RoundTrip(t *testing.T) {
	db := openCalDB(t)
	in := &experiment.CalibrationSnapshot{
		CalibrationID:    "cal-1",
		CreatedAt:        "2026-05-30T10:00:00Z",
		EmbedderLabel:    "bge-m3",
		EmbeddingDims:    1024,
		NoteCount:        40,
		NoiseFloor:       0.4473,
		NoiseFloorProbes: 8,
		ProbeSetVersion:  1,
		NTNCosineMu:      0.6240,
		NTNCosineSigma:   0.0738,
		NTNSampleCount:   780,
		VaultPath:        "/vaults/identity",
	}
	require.NoError(t, db.StoreCalibration(in))

	got, err := db.LatestCalibration()
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, in.CalibrationID, got.CalibrationID)
	assert.Equal(t, "bge-m3", got.EmbedderLabel)
	assert.Equal(t, 1024, got.EmbeddingDims)
	assert.Equal(t, 40, got.NoteCount)
	assert.InDelta(t, 0.4473, got.NoiseFloor, 1e-9)
	assert.Equal(t, 8, got.NoiseFloorProbes)
	assert.Equal(t, 1, got.ProbeSetVersion)
	assert.InDelta(t, 0.6240, got.NTNCosineMu, 1e-9)
	assert.InDelta(t, 0.0738, got.NTNCosineSigma, 1e-9)
	assert.Equal(t, 780, got.NTNSampleCount)
	assert.Equal(t, "/vaults/identity", got.VaultPath)
}

// LatestCalibrationForVault scopes to one vault. With snapshots from two vaults
// in the same global DB, the per-vault lookup must NOT return the globally-most-
// recent one — that's the multi-vault correctness bug this column fixes.
func TestLatestCalibrationForVault_ScopesToVault(t *testing.T) {
	db := openCalDB(t)
	require.NoError(t, db.StoreCalibration(&experiment.CalibrationSnapshot{
		CalibrationID: "ident", CreatedAt: "2026-05-31T10:00:00Z", EmbedderLabel: "bge-m3",
		EmbeddingDims: 1024, NoteCount: 40, NoiseFloor: 0.5059, NoiseFloorProbes: 8,
		ProbeSetVersion: 1, NTNCosineMu: 0.6241, NTNCosineSigma: 0.0733, NTNSampleCount: 780,
		VaultPath: "/vaults/identity",
	}))
	// Stored LAST → globally most recent.
	require.NoError(t, db.StoreCalibration(&experiment.CalibrationSnapshot{
		CalibrationID: "resrch", CreatedAt: "2026-05-31T11:00:00Z", EmbedderLabel: "bge-m3",
		EmbeddingDims: 1024, NoteCount: 407, NoiseFloor: 0.4782, NoiseFloorProbes: 8,
		ProbeSetVersion: 1, NTNCosineMu: 0.5060, NTNCosineSigma: 0.0716, NTNSampleCount: 82621,
		VaultPath: "/vaults/research",
	}))

	got, err := db.LatestCalibrationForVault("/vaults/identity")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "ident", got.CalibrationID, "identity lookup must not return the globally-newest (research) snapshot")
	assert.InDelta(t, 0.5059, got.NoiseFloor, 1e-9)
	assert.InDelta(t, 0.0733, got.NTNCosineSigma, 1e-9)
}

func TestLatestCalibrationForVault_NilWhenVaultUnknown(t *testing.T) {
	db := openCalDB(t)
	require.NoError(t, db.StoreCalibration(&experiment.CalibrationSnapshot{
		CalibrationID: "ident", CreatedAt: "2026-05-31T10:00:00Z", EmbedderLabel: "bge-m3",
		EmbeddingDims: 1024, NoteCount: 40, NoiseFloor: 0.5059, NoiseFloorProbes: 8,
		ProbeSetVersion: 1, NTNCosineMu: 0.6241, NTNCosineSigma: 0.0733, NTNSampleCount: 780,
		VaultPath: "/vaults/identity",
	}))
	got, err := db.LatestCalibrationForVault("/vaults/never-calibrated")
	require.NoError(t, err)
	assert.Nil(t, got, "no snapshot for this vault → nil, nil (caller uses the embedder default)")
}

func TestLatestCalibrationForVault_ReturnsMostRecentForVault(t *testing.T) {
	db := openCalDB(t)
	require.NoError(t, db.StoreCalibration(&experiment.CalibrationSnapshot{
		CalibrationID: "old", CreatedAt: "2026-05-31T09:00:00Z", EmbedderLabel: "bge-m3",
		EmbeddingDims: 1024, NoteCount: 30, NoiseFloor: 0.50, NoiseFloorProbes: 8,
		ProbeSetVersion: 1, NTNCosineMu: 0.62, NTNCosineSigma: 0.07, NTNSampleCount: 435,
		VaultPath: "/vaults/identity",
	}))
	require.NoError(t, db.StoreCalibration(&experiment.CalibrationSnapshot{
		CalibrationID: "new", CreatedAt: "2026-05-31T12:00:00Z", EmbedderLabel: "bge-m3",
		EmbeddingDims: 1024, NoteCount: 40, NoiseFloor: 0.5059, NoiseFloorProbes: 8,
		ProbeSetVersion: 1, NTNCosineMu: 0.6241, NTNCosineSigma: 0.0733, NTNSampleCount: 780,
		VaultPath: "/vaults/identity",
	}))
	got, err := db.LatestCalibrationForVault("/vaults/identity")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "new", got.CalibrationID, "re-index appends; freshest for this vault wins")
}

func TestLatestCalibration_ReturnsMostRecent(t *testing.T) {
	db := openCalDB(t)
	require.NoError(t, db.StoreCalibration(&experiment.CalibrationSnapshot{
		CalibrationID: "old", CreatedAt: "2026-05-30T09:00:00Z", EmbedderLabel: "bge-m3",
		EmbeddingDims: 1024, NoteCount: 30, NoiseFloor: 0.45, NoiseFloorProbes: 8,
		ProbeSetVersion: 1,
		NTNCosineMu:     0.62, NTNCosineSigma: 0.07, NTNSampleCount: 435,
	}))
	require.NoError(t, db.StoreCalibration(&experiment.CalibrationSnapshot{
		CalibrationID: "new", CreatedAt: "2026-05-30T11:00:00Z", EmbedderLabel: "bge-m3",
		EmbeddingDims: 1024, NoteCount: 40, NoiseFloor: 0.44, NoiseFloorProbes: 8,
		ProbeSetVersion: 1,
		NTNCosineMu:     0.62, NTNCosineSigma: 0.07, NTNSampleCount: 780,
	}))

	got, err := db.LatestCalibration()
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "new", got.CalibrationID, "LatestCalibration returns the most recent by created_at")
}
