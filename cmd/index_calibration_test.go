package cmd

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/noisefloor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func defaultSigma1024() float64 {
	return noisefloor.ClampSigma(noisefloor.DefaultDispersion(1024))
}

// resolveNoiseFloor prefers the vault's measured calibration when it exists and
// is trustworthy — this is the slice that flips the live label from the shipped
// default N to the vault's own measured floor.
func TestResolveNoiseFloor_PrefersMeasuredCalibration(t *testing.T) {
	expDB := openTestExpDB(t)
	vaultDir := t.TempDir()
	require.NoError(t, expDB.StoreCalibration(&experiment.CalibrationSnapshot{
		CalibrationID: "m", CreatedAt: "2026-05-31T10:00:00Z", EmbedderLabel: "bge-m3",
		EmbeddingDims: 1024, NoteCount: 40, NoiseFloor: 0.5059, NoiseFloorProbes: 8, ProbeSetVersion: 1,
		NTNCosineMu: 0.6241, NTNCosineSigma: 0.0733, NTNSampleCount: 780,
		VaultPath: canonicalVaultKey(vaultDir),
	}))
	ctx := experiment.WithSession(context.Background(), &experiment.Session{DB: expDB})

	n, sigma, lowContrast := resolveNoiseFloor(ctx, vaultDir, 1024)
	assert.InDelta(t, 0.5059, n, 1e-9, "uses the vault's measured noise floor")
	assert.InDelta(t, 0.0733, sigma, 1e-9, "uses the vault's measured σ")
	assert.True(t, lowContrast, "μ=0.6241 ≥ TightVaultMu → flagged low-contrast")
}

// A snapshot measured from too few notes is a noisy estimate and must be
// rejected (the provisional gate), falling back to the shipped defaults.
func TestResolveNoiseFloor_RejectsUntrustedSnapshot(t *testing.T) {
	expDB := openTestExpDB(t)
	vaultDir := t.TempDir()
	require.NoError(t, expDB.StoreCalibration(&experiment.CalibrationSnapshot{
		CalibrationID: "tiny", CreatedAt: "2026-05-31T10:00:00Z", EmbedderLabel: "bge-m3",
		EmbeddingDims: 1024, NoteCount: 5, NoiseFloor: 0.9, NoiseFloorProbes: 8, ProbeSetVersion: 1,
		NTNCosineMu: 0.6, NTNCosineSigma: 0.01, NTNSampleCount: 10,
		VaultPath: canonicalVaultKey(vaultDir),
	}))
	ctx := experiment.WithSession(context.Background(), &experiment.Session{DB: expDB})

	n, sigma, _ := resolveNoiseFloor(ctx, vaultDir, 1024)
	assert.InDelta(t, noisefloor.DefaultNoiseFloor(1024), n, 1e-9, "tiny-vault snapshot rejected → default N")
	assert.InDelta(t, defaultSigma1024(), sigma, 1e-9)
}

// A looser vault (μ below the tightness threshold) is not flagged low-contrast,
// so its weak hits don't get the tight-vault hint.
func TestResolveNoiseFloor_LooseVaultNotLowContrast(t *testing.T) {
	expDB := openTestExpDB(t)
	vaultDir := t.TempDir()
	require.NoError(t, expDB.StoreCalibration(&experiment.CalibrationSnapshot{
		CalibrationID: "loose", CreatedAt: "2026-05-31T10:00:00Z", EmbedderLabel: "bge-m3",
		EmbeddingDims: 1024, NoteCount: 407, NoiseFloor: 0.4782, NoiseFloorProbes: 8, ProbeSetVersion: 1,
		NTNCosineMu: 0.506, NTNCosineSigma: 0.0716, NTNSampleCount: 82621, // research-class μ
		VaultPath: canonicalVaultKey(vaultDir),
	}))
	ctx := experiment.WithSession(context.Background(), &experiment.Session{DB: expDB})

	_, _, lowContrast := resolveNoiseFloor(ctx, vaultDir, 1024)
	assert.False(t, lowContrast, "μ=0.506 < TightVaultMu → not low-contrast")
}

// A measured σ outside the clamp range (corrupt/degenerate measurement) is
// clamped before it reaches the relevance formula — measured N is still used.
func TestResolveNoiseFloor_ClampsOutOfRangeSigma(t *testing.T) {
	expDB := openTestExpDB(t)
	vaultDir := t.TempDir()
	require.NoError(t, expDB.StoreCalibration(&experiment.CalibrationSnapshot{
		CalibrationID: "wide", CreatedAt: "2026-05-31T10:00:00Z", EmbedderLabel: "bge-m3",
		EmbeddingDims: 1024, NoteCount: 40, NoiseFloor: 0.5, NoiseFloorProbes: 8, ProbeSetVersion: 1,
		NTNCosineMu: 0.6, NTNCosineSigma: 0.30, NTNSampleCount: 780, // σ above SigmaCeil
		VaultPath: canonicalVaultKey(vaultDir),
	}))
	ctx := experiment.WithSession(context.Background(), &experiment.Session{DB: expDB})

	n, sigma, _ := resolveNoiseFloor(ctx, vaultDir, 1024)
	assert.InDelta(t, 0.5, n, 1e-9, "measured N is still used")
	assert.InDelta(t, noisefloor.SigmaCeil, sigma, 1e-9, "an out-of-range measured σ is clamped to the ceiling")
}

func TestResolveNoiseFloor_FallsBackWhenUncalibrated(t *testing.T) {
	expDB := openTestExpDB(t)
	ctx := experiment.WithSession(context.Background(), &experiment.Session{DB: expDB})
	n, sigma, _ := resolveNoiseFloor(ctx, t.TempDir(), 1024)
	assert.InDelta(t, noisefloor.DefaultNoiseFloor(1024), n, 1e-9, "no snapshot for this vault → default")
	assert.InDelta(t, defaultSigma1024(), sigma, 1e-9)
}

func TestResolveNoiseFloor_NoSessionUsesDefaults(t *testing.T) {
	n, sigma, _ := resolveNoiseFloor(context.Background(), t.TempDir(), 1024)
	assert.InDelta(t, noisefloor.DefaultNoiseFloor(1024), n, 1e-9, "nil session → default (calibration is optional)")
	assert.InDelta(t, defaultSigma1024(), sigma, 1e-9)
}

// canonicalVaultKey must produce a stable, absolute, symlink-resolved key so two
// vaults indexed from different working directories (both `--vault .`) never
// collide on the calibration lookup.
func TestCanonicalVaultKey(t *testing.T) {
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)

	assert.Equal(t, real, canonicalVaultKey(dir), "absolute path resolves to its real path")
	assert.Equal(t, real, canonicalVaultKey(filepath.Join(dir, "x", "..")),
		"redundant ../ segments collapse to the same key")

	assert.True(t, filepath.IsAbs(canonicalVaultKey(".")),
		"a relative path must canonicalize to absolute (the '.' collision fix)")

	// A symlinked alias of a directory maps to the same key as its target.
	target := t.TempDir()
	realTarget, err := filepath.EvalSymlinks(target)
	require.NoError(t, err)
	link := filepath.Join(t.TempDir(), "alias")
	require.NoError(t, os.Symlink(target, link))
	assert.Equal(t, realTarget, canonicalVaultKey(link), "symlinked alias resolves to its target")
}

// calibrateVaultNoiseFloor skips the (model-loading) measurement entirely when
// nothing was newly embedded AND a calibration already exists — so a no-op
// re-embed doesn't pay to reload the model. Verified without an embedder: the
// function returns nil before touching the index DB, and stores nothing new.
func TestCalibrateVaultNoiseFloor_SkipsNoOpWhenCalibrationExists(t *testing.T) {
	expDB := openTestExpDB(t)
	require.NoError(t, expDB.StoreCalibration(&experiment.CalibrationSnapshot{
		CalibrationID: "existing", CreatedAt: "2026-05-30T10:00:00Z", EmbedderLabel: "bge-m3",
		EmbeddingDims: 1024, NoteCount: 40, NoiseFloor: 0.5, NoiseFloorProbes: 8,
		ProbeSetVersion: noisefloor.ProbeSetVersion, // current set → eligible to skip
		NTNCosineMu:     0.62, NTNCosineSigma: 0.07, NTNSampleCount: 780,
		VaultPath: "/vaults/identity",
	}))

	// nothing embedded, nothing deleted, current-probe-set calibration exists for
	// THIS vault → skip (dbPath never opened).
	err := calibrateVaultNoiseFloor(context.Background(), "/vaults/identity", "/nonexistent/index.db", "bge-m3", expDB, 0, 0)
	require.NoError(t, err, "must skip cleanly without opening the index db")

	got, err := expDB.LatestCalibration()
	require.NoError(t, err)
	assert.Equal(t, "existing", got.CalibrationID, "no new snapshot stored on a skipped no-op")
}

// A snapshot measured against an OLDER probe set is stale: its N was computed
// from different probes. A no-op re-index must NOT skip on it — it must
// re-calibrate against the current probe set. Here the stale snapshot exists but
// the function proceeds past the skip-gate and fails opening the (nonexistent)
// index db, proving it did not skip.
func TestCalibrateVaultNoiseFloor_RecalibratesOnStaleProbeVersion(t *testing.T) {
	expDB := openTestExpDB(t)
	require.NoError(t, expDB.StoreCalibration(&experiment.CalibrationSnapshot{
		CalibrationID: "stale", CreatedAt: "2026-05-30T10:00:00Z", EmbedderLabel: "bge-m3",
		EmbeddingDims: 1024, NoteCount: 40, NoiseFloor: 0.5, NoiseFloorProbes: 8,
		ProbeSetVersion: noisefloor.ProbeSetVersion - 1, // any version != current → stale (0 here also stands in for a pre-versioning snapshot)
		NTNCosineMu:     0.62, NTNCosineSigma: 0.07, NTNSampleCount: 780,
		VaultPath: "/vaults/identity",
	}))

	err := calibrateVaultNoiseFloor(context.Background(), "/vaults/identity", "/nonexistent/index.db", "bge-m3", expDB, 0, 0)
	require.Error(t, err, "a stale-probe-version snapshot must NOT skip; re-calibrates and fails opening the index")
}

// The skip-check must be vault-scoped: a no-op re-index of vault B must NOT skip
// just because vault A has a calibration. Before vault-scoping this was the bug
// that left a second vault permanently uncalibrated. Here vault B has no
// snapshot, so the function proceeds past the skip-gate and fails trying to open
// the (nonexistent) index db — proving it did NOT skip.
func TestCalibrateVaultNoiseFloor_DoesNotSkipOtherVault(t *testing.T) {
	expDB := openTestExpDB(t)
	require.NoError(t, expDB.StoreCalibration(&experiment.CalibrationSnapshot{
		CalibrationID: "vault-a", CreatedAt: "2026-05-31T10:00:00Z", EmbedderLabel: "bge-m3",
		EmbeddingDims: 1024, NoteCount: 40, NoiseFloor: 0.5, NoiseFloorProbes: 8, ProbeSetVersion: 1,
		NTNCosineMu: 0.62, NTNCosineSigma: 0.07, NTNSampleCount: 780,
		VaultPath: "/vaults/a",
	}))

	err := calibrateVaultNoiseFloor(context.Background(), "/vaults/b", "/nonexistent/index.db", "bge-m3", expDB, 0, 0)
	require.Error(t, err, "vault B has no snapshot → must NOT skip; proceeds and fails opening the index")
}

func TestRandomHexID_IsHexAndUnique(t *testing.T) {
	a, err := randomHexID()
	require.NoError(t, err)
	b, err := randomHexID()
	require.NoError(t, err)
	assert.Regexp(t, regexp.MustCompile(`^[0-9a-f]{16}$`), a)
	assert.NotEqual(t, a, b)
}
