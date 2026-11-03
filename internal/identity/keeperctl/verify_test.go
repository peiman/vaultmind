package keeperctl

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/identity"
	"github.com/peiman/vaultmind/internal/identity/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type keySigner ed25519.PrivateKey

func (k keySigner) Sign(b []byte) ([]byte, error) {
	return identity.SignCanonical(ed25519.PrivateKey(k), identity.CanonicalBytesFromTrusted(b))
}
func fixedKey(seed byte) ed25519.PrivateKey {
	return ed25519.NewKeyFromSeed(bytes.Repeat([]byte{seed}, 32))
}

func testPolicy(t *testing.T) Policy {
	t.Helper()
	now := func() time.Time { return time.Unix(2000000, 0) }
	p := Policy{Registry: registry.Registry{ValidFrom: 1900000, ValidUntil: 2100000},
		KeeperOwners: map[string]string{"keeper-a": "owner", "keeper-b": "owner"},
		Grants:       NewMemoryGrantStore(), Nonces: NewMemoryNonceStore(now, 0, 0), Now: now, NotBefore: 1999000}
	for i, slug := range []string{"owner", "reader", "stranger"} {
		pub, err := registry.NewPublicKey(fixedKey(byte(i + 1)).Public().(ed25519.PublicKey))
		require.NoError(t, err)
		p.Registry.Agents = append(p.Registry.Agents, registry.AgentBinding{Slug: slug, PubKey: pub,
			KeyEpoch: 1, ValidFrom: 1900000, ValidUntil: 2100000})
	}
	return p
}

func requestSig(t *testing.T, r ControlRequest) []byte {
	t.Helper()
	seed := map[string]byte{"owner": 1, "reader": 2, "stranger": 3}[r.From]
	sig, err := SignControlRequest(keySigner(fixedKey(seed)), r)
	require.NoError(t, err)
	return sig
}
func grantSig(t *testing.T, g Grant) []byte {
	t.Helper()
	seed := byte(1)
	if g.From == "stranger" {
		seed = 3
	}
	sig, err := SignGrant(keySigner(fixedKey(seed)), g)
	require.NoError(t, err)
	return sig
}
func install(t *testing.T, p Policy, g Grant) {
	t.Helper()
	require.NoError(t, AddGrant(p, g, grantSig(t, g)))
}

type brokenNonces struct{}

func (brokenNonces) CheckAndRecord(string, string, int64) error {
	return errors.New("storage unavailable")
}

type verifyCase struct {
	name   string
	reason Reason
	role   Role
	edit   func(*testing.T, *Policy, *ControlRequest, *[]byte)
}

func verifierCases() []verifyCase {
	return []verifyCase{
		{name: "owner_pause", role: RoleOwner},
		{name: "at_start", role: RoleOwner, edit: func(_ *testing.T, p *Policy, r *ControlRequest, _ *[]byte) { p.NotBefore = r.IssuedAt }},
		{name: "registry_at_start", role: RoleOwner, edit: func(_ *testing.T, p *Policy, _ *ControlRequest, _ *[]byte) { p.Registry.ValidFrom = p.Now().Unix() }},
		{name: "registry_at_end", role: RoleOwner, edit: func(_ *testing.T, p *Policy, _ *ControlRequest, _ *[]byte) { p.Registry.ValidUntil = p.Now().Unix() }},
		{name: "schema_invalid", reason: ReasonSchemaInvalid, edit: func(_ *testing.T, _ *Policy, r *ControlRequest, _ *[]byte) { r.AlgVersion = 2 }},
		{name: "unsigned", reason: ReasonBindingRejected, edit: func(_ *testing.T, _ *Policy, _ *ControlRequest, s *[]byte) { *s = nil }},
		{name: "signature_invalid", reason: ReasonSignatureInvalid, edit: func(_ *testing.T, _ *Policy, r *ControlRequest, _ *[]byte) { r.Keeper = "keeper-b" }},
		{name: "registry_expired", reason: ReasonRegistryExpired, edit: func(_ *testing.T, p *Policy, _ *ControlRequest, _ *[]byte) { p.Registry.ValidUntil = 1999999 }},
		{name: "registry_future", reason: ReasonRegistryExpired, edit: func(_ *testing.T, p *Policy, _ *ControlRequest, _ *[]byte) { p.Registry.ValidFrom = 2000001 }},
		{name: "owner_revoked", reason: ReasonBindingRejected, edit: func(_ *testing.T, p *Policy, _ *ControlRequest, _ *[]byte) {
			v := 1999999
			p.Registry.Agents[0].RevokedAt = &v
		}},
		{name: "unknown_keeper", reason: ReasonUnknownKeeper, edit: func(t *testing.T, _ *Policy, r *ControlRequest, s *[]byte) {
			r.Keeper = "missing"
			*s = requestSig(t, *r)
		}},
		{name: "before_start", reason: ReasonBeforeStart, edit: func(t *testing.T, p *Policy, r *ControlRequest, s *[]byte) {
			r.IssuedAt = p.NotBefore - 1
			*s = requestSig(t, *r)
		}},
		{name: "stale", reason: ReasonStale, edit: func(t *testing.T, _ *Policy, r *ControlRequest, s *[]byte) { r.IssuedAt -= 301; *s = requestSig(t, *r) }},
		{name: "oldest_accepted", role: RoleOwner, edit: func(t *testing.T, _ *Policy, r *ControlRequest, s *[]byte) { r.IssuedAt -= 300; *s = requestSig(t, *r) }},
		{name: "future", reason: ReasonFromFuture, edit: func(t *testing.T, _ *Policy, r *ControlRequest, s *[]byte) { r.IssuedAt += 31; *s = requestSig(t, *r) }},
		{name: "newest_accepted", role: RoleOwner, edit: func(t *testing.T, _ *Policy, r *ControlRequest, s *[]byte) { r.IssuedAt += 30; *s = requestSig(t, *r) }},
		{name: "replayed", reason: ReasonReplayed, edit: func(t *testing.T, p *Policy, r *ControlRequest, _ *[]byte) {
			require.NoError(t, p.Nonces.CheckAndRecord(r.From, r.Nonce, r.IssuedAt))
		}},
		{name: "nonce_failure", reason: ReasonNonceStoreFailed, edit: func(_ *testing.T, p *Policy, _ *ControlRequest, _ *[]byte) { p.Nonces = brokenNonces{} }},
		{name: "other_slug_pause", reason: ReasonControlNeedsOwner, edit: func(t *testing.T, _ *Policy, r *ControlRequest, s *[]byte) { r.From = "reader"; *s = requestSig(t, *r) }},
		{name: "grant_missing_id", reason: ReasonGrantMissing, edit: readCase(nil, nil)},
		{name: "grant_missing_store", reason: ReasonGrantMissing, edit: readCase(ptr("absent"), nil)},
		{name: "grantee_status", role: RoleGrantee, edit: readCase(ptr("grant-1"), func(t *testing.T, p *Policy) { install(t, *p, sampleGrant()) })},
		{name: "grantee_pause", reason: ReasonControlNeedsOwner, edit: func(t *testing.T, p *Policy, r *ControlRequest, s *[]byte) {
			install(t, *p, sampleGrant())
			r.From = "reader"
			*s = requestSig(t, *r)
		}},
		{name: "grantee_context", role: RoleGrantee, edit: func(t *testing.T, p *Policy, r *ControlRequest, s *[]byte) {
			g := sampleGrant()
			g.Scopes = []string{ScopeContext, ScopeStatus}
			install(t, *p, g)
			r.From, r.Action, r.GrantID = "reader", ActionReadContext, ptr(g.GrantID)
			*s = requestSig(t, *r)
		}},
		{name: "grant_revoked", reason: ReasonGrantRevoked, edit: readCase(ptr("grant-1"), func(t *testing.T, p *Policy) {
			install(t, *p, sampleGrant())
			require.NoError(t, p.Grants.MarkRevoked("grant-1"))
		})},
		{name: "grant_expired", reason: ReasonGrantExpired, edit: readCase(ptr("grant-1"), func(t *testing.T, p *Policy) {
			g := sampleGrant()
			g.ExpiresAt = 2000000
			require.NoError(t, p.Grants.Put(g, grantSig(t, g)))
		})},
		{name: "grant_wrong_grantee", reason: ReasonGrantWrongGrantee, edit: readCase(ptr("grant-1"), func(t *testing.T, p *Policy) { g := sampleGrant(); g.Grantee = "stranger"; install(t, *p, g) })},
		{name: "grant_not_for_keeper", reason: ReasonGrantNotForKeeper, edit: readCase(ptr("grant-1"), func(t *testing.T, p *Policy) { g := sampleGrant(); g.Keepers = []string{"keeper-b"}; install(t, *p, g) })},
		{name: "grant_scope_missing", reason: ReasonGrantScopeMissing, edit: func(t *testing.T, p *Policy, r *ControlRequest, s *[]byte) {
			readCase(ptr("grant-1"), func(t *testing.T, p *Policy) { install(t, *p, sampleGrant()) })(t, p, r, s)
			r.Action = ActionReadContext
			*s = requestSig(t, *r)
		}},
		{name: "grant_owner_rotated", reason: ReasonGrantBadSignature, edit: readCase(ptr("grant-1"), func(t *testing.T, p *Policy) {
			install(t, *p, sampleGrant())
			v := 1999999
			p.Registry.Agents[0].RevokedAt = &v
			pub, err := registry.NewPublicKey(fixedKey(4).Public().(ed25519.PublicKey))
			require.NoError(t, err)
			rotated := p.Registry.Agents[0]
			rotated.PubKey, rotated.KeyEpoch, rotated.RevokedAt = pub, 2, nil
			p.Registry.Agents = append(p.Registry.Agents, rotated)
		})},
		{name: "grant_signature_mismatch", reason: ReasonGrantBadSignature, edit: readCase(ptr("grant-1"), func(t *testing.T, p *Policy) {
			g := sampleGrant()
			sig, err := SignGrant(keySigner(fixedKey(2)), g)
			require.NoError(t, err)
			require.NoError(t, p.Grants.Put(g, sig))
		})},
		{name: "grant_owner_changed", reason: ReasonGrantBadSignature, edit: readCase(ptr("grant-1"), func(t *testing.T, p *Policy) { install(t, *p, sampleGrant()); p.KeeperOwners["keeper-a"] = "stranger" })},
		{name: "owner_ignores_grant_id", role: RoleOwner, edit: func(t *testing.T, _ *Policy, r *ControlRequest, s *[]byte) {
			r.Action = ActionReadStatus
			r.GrantID = ptr("absent")
			*s = requestSig(t, *r)
		}},
	}
}

func readCase(id *string, setup func(*testing.T, *Policy)) func(*testing.T, *Policy, *ControlRequest, *[]byte) {
	return func(t *testing.T, p *Policy, r *ControlRequest, s *[]byte) {
		if setup != nil {
			setup(t, p)
		}
		r.From, r.Action, r.GrantID = "reader", ActionReadStatus, id
		*s = requestSig(t, *r)
	}
}

func TestVerifyControl(t *testing.T) {
	cases := verifierCases()
	require.NotEmpty(t, cases)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, r := testPolicy(t), sampleRequest()
			sig := requestSig(t, r)
			if tc.edit != nil {
				tc.edit(t, &p, &r, &sig)
			}
			d := VerifyControl(p, r, sig)
			assert.Equal(t, tc.role != "", d.Accepted)
			assert.Equal(t, tc.reason, d.Reason)
			assert.Equal(t, tc.role, d.Role)
		})
	}
}

func TestGrantLifecycle(t *testing.T) {
	p, g := testPolicy(t), sampleGrant()
	bad := g
	bad.From = "stranger"
	var denied *DecisionError
	require.ErrorAs(t, AddGrant(p, bad, grantSig(t, bad)), &denied)
	assert.Equal(t, ReasonControlNeedsOwner, denied.Reason)
	install(t, p, g)
	r := sampleRevoke()
	sig, err := SignGrantRevoke(keySigner(fixedKey(1)), r)
	require.NoError(t, err)
	require.NoError(t, RevokeGrant(p, r, sig))
	require.ErrorAs(t, RevokeGrant(p, r, sig), &denied)
	assert.Equal(t, ReasonReplayed, denied.Reason)
	require.ErrorIs(t, AddGrant(p, g, grantSig(t, g)), ErrGrantRevoked)
}

func TestGrantAdministrationFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		reason Reason
		edit   func(*Policy, *Grant)
	}{
		{"expired_registry", ReasonRegistryExpired, func(p *Policy, _ *Grant) { p.Registry.ValidUntil = 1 }},
		{"revoked_binding", ReasonBindingRejected, func(p *Policy, _ *Grant) { v := 1999999; p.Registry.Agents[0].RevokedAt = &v }},
		{"unknown_keeper", ReasonUnknownKeeper, func(_ *Policy, g *Grant) { g.Keepers = []string{"keeper-a", "missing"} }},
		{"every_keeper_must_be_owned", ReasonControlNeedsOwner, func(p *Policy, g *Grant) {
			g.Keepers = []string{"keeper-a", "keeper-b"}
			p.KeeperOwners["keeper-b"] = "stranger"
		}},
		{"expired_grant", ReasonGrantExpired, func(_ *Policy, g *Grant) { g.ExpiresAt = 2000000 }},
		{"missing_store", ReasonGrantMissing, func(p *Policy, _ *Grant) { p.Grants = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, g := testPolicy(t), sampleGrant()
			tc.edit(&p, &g)
			var denied *DecisionError
			require.ErrorAs(t, AddGrant(p, g, grantSig(t, g)), &denied)
			assert.Equal(t, tc.reason, denied.Reason)
			assert.NotEmpty(t, denied.Error())
		})
	}
}

func TestRevokeGrantFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		reason Reason
		edit   func(*Policy, *GrantRevoke)
	}{
		{"registry_expired", ReasonRegistryExpired, func(p *Policy, _ *GrantRevoke) { p.Registry.ValidUntil = 1 }},
		{"stale", ReasonStale, func(_ *Policy, r *GrantRevoke) { r.IssuedAt -= 301 }},
		{"future", ReasonFromFuture, func(_ *Policy, r *GrantRevoke) { r.IssuedAt += 31 }},
		{"before_start", ReasonBeforeStart, func(p *Policy, r *GrantRevoke) { p.NotBefore = r.IssuedAt + 1 }},
		{"nonce_failure", ReasonNonceStoreFailed, func(p *Policy, _ *GrantRevoke) { p.Nonces = brokenNonces{} }},
		{"wrong_issuer", ReasonControlNeedsOwner, func(_ *Policy, r *GrantRevoke) { r.From = "reader" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, r := testPolicy(t), sampleRevoke()
			install(t, p, sampleGrant())
			tc.edit(&p, &r)
			seed := byte(1)
			if r.From == "reader" {
				seed = 2
			}
			sig, err := SignGrantRevoke(keySigner(fixedKey(seed)), r)
			require.NoError(t, err)
			var denied *DecisionError
			require.ErrorAs(t, RevokeGrant(p, r, sig), &denied)
			assert.Equal(t, tc.reason, denied.Reason)
			stored, err := p.Grants.Get(r.GrantID)
			require.NoError(t, err)
			assert.False(t, stored.Revoked)
		})
	}
}

func TestVerifyOrderAndInvalidPolicies(t *testing.T) {
	for _, tc := range []struct {
		name   string
		reason Reason
		edit   func(*Policy, *ControlRequest, *[]byte)
	}{
		{"schema_before_registry", ReasonSchemaInvalid, func(p *Policy, r *ControlRequest, _ *[]byte) { r.AlgVersion = 2; p.Registry.ValidUntil = 0 }},
		{"registry_before_signature", ReasonRegistryExpired, func(p *Policy, _ *ControlRequest, sig *[]byte) { p.Registry.ValidUntil = 0; *sig = nil }},
		{"signature_before_keeper", ReasonSignatureInvalid, func(_ *Policy, r *ControlRequest, _ *[]byte) { r.Keeper = "missing" }},
		{"time_before_nonce", ReasonStale, func(p *Policy, _ *ControlRequest, _ *[]byte) {
			p.Now = func() time.Time { return time.Unix(2000301, 0) }
			p.Nonces = brokenNonces{}
		}},
		{"nonce_before_authorization", ReasonNonceStoreFailed, func(p *Policy, _ *ControlRequest, _ *[]byte) {
			p.KeeperOwners["keeper-a"] = "stranger"
			p.Nonces = brokenNonces{}
		}},
		{"missing_not_before", ReasonSchemaInvalid, func(p *Policy, _ *ControlRequest, _ *[]byte) { p.NotBefore = 0 }},
		{"negative_age", ReasonSchemaInvalid, func(p *Policy, _ *ControlRequest, _ *[]byte) { p.MaxAge = -time.Second }},
		{"missing_nonce_store", ReasonNonceStoreFailed, func(p *Policy, _ *ControlRequest, _ *[]byte) { p.Nonces = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, r := testPolicy(t), sampleRequest()
			sig := requestSig(t, r)
			tc.edit(&p, &r, &sig)
			d := VerifyControl(p, r, sig)
			assert.False(t, d.Accepted)
			assert.Equal(t, tc.reason, d.Reason)
		})
	}
}

func TestAllOwnerActionsAndSignatureSeam(t *testing.T) {
	for _, action := range []string{ActionMessage, ActionRebrief, ActionPause, ActionDrain, ActionResume,
		ActionRestart, ActionStop, ActionReadStatus, ActionReadContext} {
		t.Run(action, func(t *testing.T) {
			p, r := testPolicy(t), sampleRequest()
			r.Action = action
			if action == ActionMessage || action == ActionRebrief {
				r.Body = ptr("Hello ⭐")
			}
			sig := requestSig(t, r)
			d := VerifyControl(p, r, sig)
			require.True(t, d.Accepted)
			assert.Equal(t, RoleOwner, d.Role)
			assert.Empty(t, d.Reason)
			assert.Equal(t, ReasonReplayed, VerifyControl(p, r, sig).Reason)
		})
	}
}
