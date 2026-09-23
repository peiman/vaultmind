package registryfetch_test

import (
	"crypto/ed25519"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/identity/registry"
	"github.com/peiman/vaultmind/internal/identity/registryfetch"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Member machines had no local copy of the signed registry (dabir, 2026-09-22),
// so nothing there could verify it or count down to its lapse. The hub serves
// it; this decides whether a fetched copy may REPLACE the local one. Trust
// comes from the pinned root signature, never from the channel, and a fetch
// may never move the local copy BACKWARDS.

var now = time.Unix(1_800_000_000, 0)

func key(t *testing.T, seed byte) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	s := sha256.Sum256([]byte{seed})
	priv := ed25519.NewKeyFromSeed(s[:])
	return priv.Public().(ed25519.PublicKey), priv
}

func signed(t *testing.T, priv ed25519.PrivateKey, epoch int, from, until time.Time) []byte {
	t.Helper()
	member, _ := key(t, 99)
	pk, err := registry.NewPublicKey(member)
	require.NoError(t, err)
	env, err := registry.SignRegistry(priv, registry.Registry{
		Epoch: epoch, ValidFrom: from.Unix(), ValidUntil: until.Unix(),
		Agents: []registry.AgentBinding{{Slug: "mira", DisplayName: "Mira", KeyEpoch: 1, PubKey: pk,
			ValidFrom: from.Unix(), ValidUntil: until.Unix()}},
	})
	require.NoError(t, err)
	b, err := registry.MarshalDistribution(env)
	require.NoError(t, err)
	return b
}

func fresh(t *testing.T, priv ed25519.PrivateKey, epoch int) []byte {
	return signed(t, priv, epoch, now.Add(-24*time.Hour), now.Add(365*24*time.Hour))
}

func TestUpdate_WritesAVerifiedRegistryWhenThereIsNoLocalCopy(t *testing.T) {
	pub, priv := key(t, 1)
	path := filepath.Join(t.TempDir(), "mesh", "registry.json")
	fetched := fresh(t, priv, 5)

	res, err := registryfetch.Update(pub, fetched, path, now)
	require.NoError(t, err)
	assert.True(t, res.Changed)
	assert.Equal(t, 5, res.Epoch)
	assert.Equal(t, 0, res.PreviousEpoch)
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, fetched, got, "stored byte-exact: the signature covers these bytes")
}

func TestUpdate_ReplacesAnOlderEpoch(t *testing.T) {
	pub, priv := key(t, 1)
	path := filepath.Join(t.TempDir(), "registry.json")
	require.NoError(t, os.WriteFile(path, fresh(t, priv, 4), 0o600))

	res, err := registryfetch.Update(pub, fresh(t, priv, 5), path, now)
	require.NoError(t, err)
	assert.True(t, res.Changed)
	assert.Equal(t, 4, res.PreviousEpoch)
	assert.Equal(t, 5, res.Epoch)
}

func TestUpdate_SameEpochTouchesNothing(t *testing.T) {
	pub, priv := key(t, 1)
	path := filepath.Join(t.TempDir(), "registry.json")
	local := fresh(t, priv, 5)
	require.NoError(t, os.WriteFile(path, local, 0o600))
	before, _ := os.Stat(path)

	res, err := registryfetch.Update(pub, fresh(t, priv, 5), path, now)
	require.NoError(t, err)
	assert.False(t, res.Changed)
	after, _ := os.Stat(path)
	assert.Equal(t, before.ModTime(), after.ModTime(), "an unchanged epoch is not a write")
}

// A hub (or anything in between) serving an OLDER validly signed registry is a
// rollback. The local copy is the floor.
func TestUpdate_RefusesARollback(t *testing.T) {
	pub, priv := key(t, 1)
	path := filepath.Join(t.TempDir(), "registry.json")
	local := fresh(t, priv, 5)
	require.NoError(t, os.WriteFile(path, local, 0o600))

	_, err := registryfetch.Update(pub, fresh(t, priv, 4), path, now)
	require.ErrorIs(t, err, registryfetch.ErrRollback)
	got, _ := os.ReadFile(path)
	assert.Equal(t, local, got, "the local copy is untouched")
}

func TestUpdate_RefusesAForeignRoot(t *testing.T) {
	pub, _ := key(t, 1)
	_, otherPriv := key(t, 2)
	path := filepath.Join(t.TempDir(), "registry.json")

	_, err := registryfetch.Update(pub, fresh(t, otherPriv, 9), path, now)
	require.ErrorIs(t, err, registryfetch.ErrNotVerified)
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "an unverified registry is never written")
}

func TestUpdate_RefusesSomethingThatIsNotARegistry(t *testing.T) {
	pub, _ := key(t, 1)
	path := filepath.Join(t.TempDir(), "registry.json")
	_, err := registryfetch.Update(pub, []byte("agents:\n  - slug: mira\n"), path, now)
	require.ErrorIs(t, err, registryfetch.ErrNotARegistry)
}

// Past its own valid_until the registry is dead everywhere; storing it would
// only replace a live copy with a dead one.
func TestUpdate_RefusesAnExpiredRegistry(t *testing.T) {
	pub, priv := key(t, 1)
	path := filepath.Join(t.TempDir(), "registry.json")
	expired := signed(t, priv, 7, now.Add(-400*24*time.Hour), now.Add(-time.Hour))
	_, err := registryfetch.Update(pub, expired, path, now)
	require.ErrorIs(t, err, registryfetch.ErrNotVerified)
}

// Expiry must not reopen the door: an expired local epoch 5 still refuses a
// fresh epoch 4 — otherwise letting a registry lapse invites its own rollback.
func TestUpdate_AnExpiredLocalCopyStillRefusesARollback(t *testing.T) {
	pub, priv := key(t, 1)
	path := filepath.Join(t.TempDir(), "registry.json")
	local := signed(t, priv, 5, now.Add(-400*24*time.Hour), now.Add(-time.Hour))
	require.NoError(t, os.WriteFile(path, local, 0o600))

	_, err := registryfetch.Update(pub, fresh(t, priv, 4), path, now)
	require.ErrorIs(t, err, registryfetch.ErrRollback)
}

// A local copy the pinned root did not sign carries no weight: its epoch is not
// a floor, whatever number it claims.
func TestUpdate_AForeignLocalCopyIsReplacedNotObeyed(t *testing.T) {
	pub, priv := key(t, 1)
	_, otherPriv := key(t, 2)
	path := filepath.Join(t.TempDir(), "registry.json")
	require.NoError(t, os.WriteFile(path, fresh(t, otherPriv, 99), 0o600))

	res, err := registryfetch.Update(pub, fresh(t, priv, 5), path, now)
	require.NoError(t, err)
	assert.True(t, res.Changed)
	assert.Equal(t, 0, res.PreviousEpoch, "an unverified local epoch is no floor")
}
