package keeperctl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileGrantStoreLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "grants.json")
	clock := time.Unix(2000000, 0)
	now := func() time.Time { return clock }
	s, err := OpenFileGrantStore(path, now)
	require.NoError(t, err)
	g := sampleGrant()
	sig := grantSig(t, g)
	require.NoError(t, s.Put(g, sig))
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
	s, err = OpenFileGrantStore(path, now)
	require.NoError(t, err)
	got, err := s.Get(g.GrantID)
	require.NoError(t, err)
	assert.Equal(t, g, got.Grant)
	assert.Equal(t, sig, got.Sig)
	require.NoError(t, s.MarkRevoked(g.GrantID))
	for _, timestamp := range []int64{2000000, g.ExpiresAt - 1} {
		clock = time.Unix(timestamp, 0)
		s, err = OpenFileGrantStore(path, now)
		require.NoError(t, err)
		got, err = s.Get(g.GrantID)
		require.NoError(t, err)
		assert.True(t, got.Revoked)
		require.ErrorIs(t, s.Put(g, sig), ErrGrantRevoked)
		longer := g
		longer.ExpiresAt++
		require.ErrorIs(t, s.Put(longer, grantSig(t, longer)), ErrGrantRevoked)
	}
	clock = time.Unix(g.ExpiresAt+1, 0)
	s, err = OpenFileGrantStore(path, now)
	require.NoError(t, err)
	_, err = s.Get(g.GrantID)
	require.ErrorIs(t, err, ErrGrantMissing)
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(contents), g.GrantID)
}

func TestFileGrantStoreCorruption(t *testing.T) {
	for _, input := range []string{"", "{", "null", `{}`, `{"version":1,"grants":{},"revoked":{},"unknown":1}`,
		`{"version":1,"version":1,"grants":{},"revoked":{}}`,
		`{"version":1,"grants":{},"revoked":{"orphan":2001000}}`,
		`{"version":1,"grants":{},"revoked":{}} {}`} {
		t.Run(input, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "bad.json")
			require.NoError(t, os.WriteFile(path, []byte(input), 0600))
			_, err := OpenFileGrantStore(path, nil)
			require.Error(t, err)
		})
	}
}

func TestStoresDefensiveCopiesAndImmutableIDs(t *testing.T) {
	file, err := OpenFileGrantStore(filepath.Join(t.TempDir(), "grants.json"), func() time.Time { return time.Unix(2000000, 0) })
	require.NoError(t, err)
	for _, store := range []GrantStore{NewMemoryGrantStore(), file} {
		g := sampleGrant()
		sig := grantSig(t, g)
		require.NoError(t, store.Put(g, sig))
		require.NoError(t, store.Put(g, sig))
		g.Keepers[0] = "mutated"
		sig[0] ^= 1
		got, err := store.Get("grant-1")
		require.NoError(t, err)
		assert.Equal(t, "keeper-a", got.Grant.Keepers[0])
		got.Grant.Scopes[0] = "mutated"
		got.Sig[0] ^= 1
		again, err := store.Get("grant-1")
		require.NoError(t, err)
		assert.Equal(t, ScopeStatus, again.Grant.Scopes[0])
		require.ErrorIs(t, store.Put(g, sig), ErrGrantExists)
		require.ErrorIs(t, store.MarkRevoked("absent"), ErrGrantMissing)
	}
}

func TestNonceRetentionAndConcurrency(t *testing.T) {
	clock := time.Unix(2000000, 0)
	s := NewMemoryNonceStore(func() time.Time { return clock }, 0, 0)
	require.NoError(t, s.CheckAndRecord("owner", "nonce", 2000000))
	require.NoError(t, s.CheckAndRecord("reader", "nonce", 2000000))
	clock = clock.Add(330 * time.Second)
	require.ErrorIs(t, s.CheckAndRecord("owner", "nonce", 2000000), ErrNonceSeen)
	clock = clock.Add(time.Second)
	require.NoError(t, s.CheckAndRecord("owner", "nonce", clock.Unix()))
	var accepted atomic.Int64
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			if s.CheckAndRecord("owner", "concurrent", clock.Unix()) == nil {
				accepted.Add(1)
			}
		})
	}
	wg.Wait()
	assert.Equal(t, int64(1), accepted.Load())
}

func TestFileGrantStoreFailedWriteFailsClosed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "grants.json")
	s, err := OpenFileGrantStore(path, nil)
	require.NoError(t, err)
	g := sampleGrant()
	require.NoError(t, s.Put(g, grantSig(t, g)))
	// Replacing the target file with a directory forces rename to fail, including
	// when tests run as a user who can bypass directory permission restrictions.
	require.NoError(t, os.Remove(path))
	require.NoError(t, os.Mkdir(path, 0700))
	require.Error(t, s.MarkRevoked(g.GrantID))
	_, err = s.Get(g.GrantID)
	require.Error(t, err, "a write failure must fail closed even when rename success is uncertain")
	require.Error(t, s.Put(g, grantSig(t, g)))
	require.Error(t, s.MarkRevoked(g.GrantID))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
}

func TestFileGrantStoreRejectsMalformedSignedFields(t *testing.T) {
	for _, field := range []string{FieldIssuedAt, FieldGrantee} {
		for _, null := range []bool{false, true} {
			path := filepath.Join(t.TempDir(), "grants.json")
			g := sampleGrant()
			raw := rawJSON(t, g)
			var fields map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(raw, &fields))
			delete(fields, field)
			if null {
				fields[field] = json.RawMessage("null")
			}
			wire := map[string]any{"version": 1, "grants": map[string]any{g.GrantID: map[string]any{
				"grant": fields, "sig": grantSig(t, g), "revoked": false}}, "revoked": map[string]int64{}}
			require.NoError(t, os.WriteFile(path, rawJSON(t, wire), 0600))
			_, err := OpenFileGrantStore(path, nil)
			require.Error(t, err, "%s null=%v", field, null)
		}
	}
}

func TestPersistedGrantEndToEnd(t *testing.T) {
	p := testPolicy(t)
	path := filepath.Join(t.TempDir(), "grants.json")
	s, err := OpenFileGrantStore(path, p.Now)
	require.NoError(t, err)
	p.Grants = s
	install(t, p, sampleGrant())
	p.Grants, err = OpenFileGrantStore(path, p.Now)
	require.NoError(t, err)
	r := sampleRequest()
	r.From, r.Action, r.GrantID = "reader", ActionReadStatus, ptr("grant-1")
	d := VerifyControl(p, r, requestSig(t, r))
	require.True(t, d.Accepted)
	assert.Equal(t, RoleGrantee, d.Role)
	assert.Empty(t, d.Reason)
	v := sampleRevoke()
	sig, err := SignGrantRevoke(keySigner(fixedKey(1)), v)
	require.NoError(t, err)
	require.NoError(t, RevokeGrant(p, v, sig))
	p.Grants, err = OpenFileGrantStore(path, p.Now)
	require.NoError(t, err)
	r.Nonce = "fresh01234567890"
	d = VerifyControl(p, r, requestSig(t, r))
	assert.False(t, d.Accepted)
	assert.Equal(t, ReasonGrantRevoked, d.Reason)
	require.ErrorIs(t, AddGrant(p, sampleGrant(), grantSig(t, sampleGrant())), ErrGrantRevoked)
}
