package anchor_test

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/identity/anchor"
	"github.com/peiman/vaultmind/internal/identity/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixedRoot mints a deterministic LOW-ENTROPY root keypair from a one-byte seed
// fill (so the gitleaks entropy scanner stays quiet) and returns its base64-std
// pubkey plus the network id it derives. Reusing ed25519.NewKeyFromSeed keeps the
// keys reproducible across runs.
func fixedRoot(t *testing.T, fill byte) (rootB64, networkID string) {
	t.Helper()
	seed := bytes.Repeat([]byte{fill}, ed25519.SeedSize)
	priv := ed25519.NewKeyFromSeed(seed)
	pub := priv.Public().(ed25519.PublicKey)
	return base64.StdEncoding.EncodeToString(pub), registry.NetworkID(pub)
}

// anchorFor builds an internally consistent NetworkAnchor for the given seed.
func anchorFor(t *testing.T, fill byte) anchor.NetworkAnchor {
	t.Helper()
	rootB64, networkID := fixedRoot(t, fill)
	return anchor.NetworkAnchor{
		NetworkID:   networkID,
		RootPubKey:  rootB64,
		ConfirmedAt: 2_000_000,
		Relay:       "https://relay.example",
	}
}

// TestLoadMissingFileReturnsNilNil proves a not-yet-enrolled member (no anchor
// file) gets (nil, nil) — a missing file is NOT an error.
func TestLoadMissingFileReturnsNilNil(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist", "network-roots.json")
	got, err := anchor.Load(path)
	require.NoError(t, err)
	assert.Nil(t, got)
}

// TestUpsertThenLoadRoundTrips proves an upserted anchor round-trips byte-for-byte
// through Load.
func TestUpsertThenLoadRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "network-roots.json")
	a := anchorFor(t, 0xA1)
	require.NoError(t, anchor.Upsert(path, a))

	got, err := anchor.Load(path)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, a, got[0])
}

// TestUpsertCreatesParentDir proves Upsert creates an absent parent dir (0700).
func TestUpsertCreatesParentDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "identity")
	path := filepath.Join(dir, "network-roots.json")
	require.NoError(t, anchor.Upsert(path, anchorFor(t, 0xA1)))

	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

// TestUpsertWritesFileMode0600 proves the persisted file is chmod 0600.
func TestUpsertWritesFileMode0600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "network-roots.json")
	require.NoError(t, anchor.Upsert(path, anchorFor(t, 0xA1)))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

// TestUpsertNewAppends proves upserting a SECOND distinct network appends rather
// than replacing the first (multi-network forward-compatible).
func TestUpsertNewAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "network-roots.json")
	a1 := anchorFor(t, 0xA1)
	a2 := anchorFor(t, 0xB2)
	require.NoError(t, anchor.Upsert(path, a1))
	require.NoError(t, anchor.Upsert(path, a2))

	got, err := anchor.Load(path)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, a1, got[0])
	assert.Equal(t, a2, got[1])
}

// TestUpsertExistingReplaces proves re-upserting the SAME network replaces the
// entry (no duplicate) and updates the mutable fields.
func TestUpsertExistingReplaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "network-roots.json")
	a := anchorFor(t, 0xA1)
	require.NoError(t, anchor.Upsert(path, a))

	updated := a
	updated.ConfirmedAt = 3_000_000
	updated.Relay = "https://relay2.example"
	require.NoError(t, anchor.Upsert(path, updated))

	got, err := anchor.Load(path)
	require.NoError(t, err)
	require.Len(t, got, 1, "re-upserting the same network must NOT duplicate")
	assert.Equal(t, updated, got[0])
}

// TestUpsertReplacesPreservesOrder proves replacing a middle entry keeps the
// other entries in place.
func TestUpsertReplacesPreservesOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "network-roots.json")
	a1, a2, a3 := anchorFor(t, 0xA1), anchorFor(t, 0xB2), anchorFor(t, 0xC3)
	require.NoError(t, anchor.Upsert(path, a1))
	require.NoError(t, anchor.Upsert(path, a2))
	require.NoError(t, anchor.Upsert(path, a3))

	updated := a2
	updated.ConfirmedAt = 9_000_000
	require.NoError(t, anchor.Upsert(path, updated))

	got, err := anchor.Load(path)
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, a1, got[0])
	assert.Equal(t, updated, got[1])
	assert.Equal(t, a3, got[2])
}

// TestUpsertNoTempLeftBehind proves the atomic write leaves no temp file beside
// the target after a successful Upsert.
func TestUpsertNoTempLeftBehind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "network-roots.json")
	require.NoError(t, anchor.Upsert(path, anchorFor(t, 0xA1)))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "only the final file must remain — no temp leftover")
	assert.Equal(t, "network-roots.json", entries[0].Name())
}

// TestUpsertOnDiskShape pins the on-disk JSON envelope shape: a top-level
// "networks" array of anchors.
func TestUpsertOnDiskShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "network-roots.json")
	a := anchorFor(t, 0xA1)
	require.NoError(t, anchor.Upsert(path, a))

	raw, err := os.ReadFile(path) //nolint:gosec // test reads its own temp file
	require.NoError(t, err)
	var env struct {
		Networks []anchor.NetworkAnchor `json:"networks"`
	}
	require.NoError(t, json.Unmarshal(raw, &env))
	require.Len(t, env.Networks, 1)
	assert.Equal(t, a, env.Networks[0])
}

// TestLoadMalformedJSONErrors proves a malformed / non-object body is an error.
func TestLoadMalformedJSONErrors(t *testing.T) {
	cases := map[string]string{
		"truncated":     "{not json",
		"trailing junk": `{"networks":[]}trailing`,
		"unknown field": `{"networks":[],"evil":1}`,
		"wrong type":    `{"networks":"not-an-array"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "network-roots.json")
			require.NoError(t, os.WriteFile(path, []byte(body), 0600))
			_, err := anchor.Load(path)
			require.Error(t, err)
		})
	}
}

// TestUpsertRejectsBadAnchor proves Upsert validates internal consistency: a bad
// pubkey, a network_id that does not derive from the root, a non-vmnet1 prefix,
// and an empty network_id are all rejected — and the file is NOT created.
func TestUpsertRejectsBadAnchor(t *testing.T) {
	rootB64, networkID := fixedRoot(t, 0xA1)
	otherRootB64, otherNetwork := fixedRoot(t, 0xB2)

	cases := map[string]anchor.NetworkAnchor{
		"empty network_id": {NetworkID: "", RootPubKey: rootB64, ConfirmedAt: 1},
		"non-vmnet1 prefix": {
			NetworkID:  "vmnetX:deadbeefdeadbeefdeadbeefdeadbeef",
			RootPubKey: rootB64,
		},
		"bad base64 pubkey": {NetworkID: networkID, RootPubKey: "!!!not-base64!!!"},
		"wrong-length pubkey": {
			NetworkID:  networkID,
			RootPubKey: base64.StdEncoding.EncodeToString([]byte("too-short")),
		},
		"network_id does not derive from root": {
			NetworkID:  otherNetwork, // derives from otherRoot, not rootB64
			RootPubKey: rootB64,
		},
		"root derives a different network_id": {
			NetworkID:  networkID, // derives from rootB64, not otherRoot
			RootPubKey: otherRootB64,
		},
	}
	for name, a := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "network-roots.json")
			err := anchor.Upsert(path, a)
			require.Error(t, err)
			_, statErr := os.Stat(path)
			assert.True(t, os.IsNotExist(statErr), "a rejected anchor must NOT create the file")
		})
	}
}

// TestUpsertRejectsSmallOrderPubKey proves a small-order (universal-forgery)
// 32-byte pubkey is rejected even when correct-length.
func TestUpsertRejectsSmallOrderPubKey(t *testing.T) {
	// The 32-byte all-zero key is small-order; NewPublicKey rejects it.
	smallOrder := base64.StdEncoding.EncodeToString(make([]byte, ed25519.PublicKeySize))
	path := filepath.Join(t.TempDir(), "network-roots.json")
	err := anchor.Upsert(path, anchor.NetworkAnchor{
		NetworkID:  registry.NetworkID(make([]byte, ed25519.PublicKeySize)),
		RootPubKey: smallOrder,
	})
	require.Error(t, err)
}

// TestLoadReadErrorPropagates proves a read error that is NOT "missing file"
// (e.g. path is a directory) surfaces as an error, not (nil, nil).
func TestLoadReadErrorPropagates(t *testing.T) {
	dir := t.TempDir() // a directory, not a regular file
	_, err := anchor.Load(dir)
	require.Error(t, err, "reading a directory as the store must error, not return (nil,nil)")
}

// TestUpsertPropagatesMalformedExisting proves Upsert fails (does not silently
// overwrite) when the existing on-disk store is malformed — a corrupt store must
// fail loud, not be clobbered.
func TestUpsertPropagatesMalformedExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "network-roots.json")
	require.NoError(t, os.WriteFile(path, []byte("{not json"), 0600))
	err := anchor.Upsert(path, anchorFor(t, 0xA1))
	require.Error(t, err)
}

// TestUpsertRenameFailsWhenTargetIsDir proves the atomic rename fails loud when
// the target path is an existing directory (rename-onto-dir is rejected), so a
// write that cannot complete atomically surfaces an error.
func TestUpsertRenameFailsWhenTargetIsDir(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "network-roots.json")
	require.NoError(t, os.Mkdir(target, 0700)) // make the target a DIRECTORY
	err := anchor.Upsert(target, anchorFor(t, 0xA1))
	require.Error(t, err, "renaming the temp file onto an existing directory must fail")
}

// TestFind proves the lookup helper returns the matching anchor and a found flag,
// and (zero, false) on a miss.
func TestFind(t *testing.T) {
	a1, a2 := anchorFor(t, 0xA1), anchorFor(t, 0xB2)
	anchors := []anchor.NetworkAnchor{a1, a2}

	got, ok := anchor.Find(anchors, a2.NetworkID)
	require.True(t, ok)
	assert.Equal(t, a2, got)

	_, ok = anchor.Find(anchors, "vmnet1:00000000000000000000000000000000")
	assert.False(t, ok)

	_, ok = anchor.Find(nil, a1.NetworkID)
	assert.False(t, ok)
}
