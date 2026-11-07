package registry

import (
	"crypto/ed25519"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func humanBinding(t *testing.T, fill byte, slug, name string, roles []Role, custody string) AgentBinding {
	t.Helper()
	pub, _ := fixedEd25519(t, fill)
	pk, err := NewPublicKey(pub)
	require.NoError(t, err)
	return AgentBinding{
		Slug: slug, DisplayName: name, PubKey: pk, KeyEpoch: 1,
		ValidFrom: 1_770_000_000, ValidUntil: 1_780_000_000,
		AuthorizedOriginDaemons: []string{"daemon-eu-1"},
		Roles:                   roles,
		Custody:                 custody,
		Class:                   ClassHuman,
	}
}

func peopleRegistry(t *testing.T) Registry {
	t.Helper()
	reg := rolesFreeRegistry(t, []Role{NewBridgeRole([]string{"human:peiman"})})
	reg.Humans = []AgentBinding{
		humanBinding(t, 0x31, "peiman", "Peiman", []Role{NewGodRole()}, CustodyIsolated),
		humanBinding(t, 0x32, "siavoush", "Siavoush", nil, ""),
	}
	return reg
}

// cloneRegistry copies both binding slices so a test can mutate one case
// without changing the registry the next case signs.
func cloneRegistry(reg Registry) Registry {
	out := reg
	out.Agents = cloneBindings(reg.Agents)
	out.Humans = cloneBindings(reg.Humans)
	return out
}

func cloneBindings(in []AgentBinding) []AgentBinding {
	if in == nil {
		return nil
	}
	out := make([]AgentBinding, len(in))
	for i, b := range in {
		out[i] = b
		if b.RevokedAt != nil {
			v := *b.RevokedAt
			out[i].RevokedAt = &v
		}
		if b.Roles != nil {
			out[i].Roles = append([]Role(nil), b.Roles...)
		}
		if b.AuthorizedOriginDaemons != nil {
			out[i].AuthorizedOriginDaemons = append([]string(nil), b.AuthorizedOriginDaemons...)
		}
		if b.VouchAllowlist != nil {
			out[i].VouchAllowlist = append([]string(nil), b.VouchAllowlist...)
		}
	}
	return out
}

func TestCanonicalBytes_HumansOmittedMatchesPreHumansGolden(t *testing.T) {
	for _, humans := range [][]AgentBinding{nil, {}} {
		reg := rolesFreeRegistry(t, nil)
		reg.Humans = humans
		cb, err := canonicalBytes(reg)
		require.NoError(t, err)
		assert.Equal(t, rolesFreeCanonicalHex, hex.EncodeToString(cb.Bytes()))
		assert.NotContains(t, string(cb.Bytes()), "humans")
		assert.NotContains(t, string(cb.Bytes()), "custody")
	}
}

func TestCanonicalBytes_SharedCustodyIsOmitted(t *testing.T) {
	for _, custody := range []string{"", CustodyShared} {
		reg := rolesFreeRegistry(t, nil)
		reg.Agents[0].Custody = custody
		cb, err := canonicalBytes(reg)
		require.NoError(t, err)
		assert.Equal(t, rolesFreeCanonicalHex, hex.EncodeToString(cb.Bytes()))
		assert.NotContains(t, string(cb.Bytes()), "custody")
	}
}

func TestCanonicalBytes_EmitsHumansGodAndIsolatedCustody(t *testing.T) {
	cb, err := canonicalBytes(peopleRegistry(t))
	require.NoError(t, err)
	body := string(cb.Bytes())
	assert.Contains(t, body, `"humans":[`)
	assert.Contains(t, body, `"slug":"peiman"`)
	assert.Contains(t, body, `"custody":"isolated"`)
	assert.Contains(t, body, `"roles":[{"kind":"god"}]`)
	assert.Contains(t, body, `"slug":"siavoush"`)
	assert.NotContains(t, body, `"class"`)
	// siavoush has no roles and the default custody, so neither key is on that binding.
	var obj map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(cb.Bytes(), &obj))
	var humans []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(obj["humans"], &humans))
	require.Len(t, humans, 2)
	_, hasRoles := humans[1]["roles"]
	_, hasCustody := humans[1]["custody"]
	assert.False(t, hasRoles, "a human with no roles omits roles")
	assert.False(t, hasCustody, "shared custody is omitted")
}

func TestExistingSignedFixtures_ReCanonicalizeWithoutHumans(t *testing.T) {
	t.Run("cross_language_vectors", func(t *testing.T) {
		assertFixtureRecanonicalizes(t, loadFixture(t).RootPubKey, loadFixture(t).Now,
			loadFixture(t).MaxStalenessSec, loadFixture(t).Valid.Envelope, loadFixture(t).Valid.PersistedEpoch)
	})
	t.Run("cross_language_roles_vectors", func(t *testing.T) {
		fx := loadRolesFixture(t)
		assertFixtureRecanonicalizes(t, fx.RootPubKey, fx.Now, fx.MaxStalenessSec,
			fx.Valid.Envelope, fx.Valid.PersistedEpoch)
	})
}

func assertFixtureRecanonicalizes(t *testing.T, rootB64 string, now, maxStale int64, envelope json.RawMessage, persisted int) {
	t.Helper()
	parsed, err := ParseDistribution(envelope)
	require.NoError(t, err)
	assert.NotContains(t, string(parsed.Registry), `"humans"`)
	rootPub := mustB64Pub(t, rootB64)
	reg, _, err := VerifyAndLoad(rootPub, parsed, persisted, time.Unix(now, 0), time.Duration(maxStale)*time.Second)
	require.NoError(t, err)
	require.Empty(t, reg.Humans)
	cb, err := canonicalBytes(reg)
	require.NoError(t, err)
	assert.Equal(t, parsed.Registry, cb.Bytes(), "a registry with no humans must canonicalize to the bytes that were signed")
}

func TestSignAndLoad_HumansRoundTrip(t *testing.T) {
	rootPub, rootPriv := fixedEd25519(t, 0x11)
	reg := peopleRegistry(t)
	env, err := SignRegistry(rootPriv, reg)
	require.NoError(t, err)
	assert.NotContains(t, string(env.Registry), `"class"`)

	loaded, _, err := VerifyAndLoad(rootPub, env, 4, fixedNow, time.Hour*24*365)
	require.NoError(t, err)
	require.Len(t, loaded.Humans, 2)
	assert.Equal(t, ClassHuman, loaded.Humans[0].Class)
	assert.Equal(t, "peiman", loaded.Humans[0].Slug)
	assert.Equal(t, CustodyIsolated, loaded.Humans[0].Custody)
	require.Len(t, loaded.Humans[0].Roles, 1)
	assert.Equal(t, RoleKindGod, loaded.Humans[0].Roles[0].Kind())
	assert.Equal(t, ClassHuman, loaded.Humans[1].Class)
	assert.Equal(t, "siavoush", loaded.Humans[1].Slug)
	assert.Empty(t, loaded.Humans[1].Custody)
	assert.Empty(t, loaded.Humans[1].Roles)
	assert.Empty(t, loaded.Agents[0].Class)
	assert.Equal(t, ClassAgent, loaded.Agents[0].EffectiveClass())

	god, ok := GodBinding(loaded, fixedNow)
	require.True(t, ok)
	assert.Equal(t, "human:peiman", god.EffectiveClass()+":"+god.Slug)
	assert.Equal(t, "peiman", god.Slug)
}

func TestSignRegistry_RejectsHumanRules(t *testing.T) {
	_, rootPriv := fixedEd25519(t, 0x11)
	base := peopleRegistry(t)
	agentGod := rolesFreeRegistry(t, []Role{NewGodRole()})
	twoGods := cloneRegistry(base)
	twoGods.Humans[1].Roles = []Role{NewGodRole()}
	dup := cloneRegistry(base)
	dup.Humans[1].Slug = "mira"
	badCustody := cloneRegistry(base)
	badCustody.Humans[0].Custody = "private"
	bridgeHuman := cloneRegistry(base)
	bridgeHuman.Humans[1].Roles = []Role{NewBridgeRole([]string{"human:ada"})}
	signingHuman := cloneRegistry(base)
	signingHuman.Humans[1].Roles = []Role{NewCodeSigningRole([]string{"darwin-arm64"})}
	badEpoch := cloneRegistry(base)
	badEpoch.Humans[0].KeyEpoch = 0
	dupHuman := cloneRegistry(base)
	dupHuman.Humans = append(dupHuman.Humans, cloneBindings(base.Humans[:1])...)
	agentColonSlug := rolesFreeRegistry(t, nil)
	agentColonSlug.Agents[0].Slug = "human:peiman"
	humanColonSlug := cloneRegistry(base)
	humanColonSlug.Humans[1].Slug = "foo:bar"

	cases := []struct {
		name string
		reg  Registry
		want string
		slug string
	}{
		{name: "god_on_agent", reg: agentGod, want: ErrGodOnlyOnHuman, slug: "mira"},
		{name: "bridge_on_human", reg: bridgeHuman, want: ErrAgentRoleOnlyOnAgent, slug: "siavoush"},
		{name: "code_signing_on_human", reg: signingHuman, want: ErrAgentRoleOnlyOnAgent, slug: "siavoush"},
		{name: "two_gods", reg: twoGods, want: ErrMultipleGods, slug: "peiman"},
		{name: "slug_across_classes", reg: dup, want: ErrSlugAcrossClasses, slug: "mira"},
		{name: "bad_custody", reg: badCustody, want: ErrBadCustody, slug: "peiman"},
		{name: "human_epoch", reg: badEpoch, want: ErrEpochRange},
		{name: "duplicate_human", reg: dupHuman, want: ErrDuplicateBinding, slug: "peiman"},
		{name: "agent_slug_colon", reg: agentColonSlug, want: ErrSlugColon, slug: "human:peiman"},
		{name: "human_slug_colon", reg: humanColonSlug, want: ErrSlugColon, slug: "foo:bar"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := SignRegistry(rootPriv, tc.reg)
			require.ErrorContains(t, err, tc.want)
			if tc.slug != "" {
				assert.ErrorContains(t, err, tc.slug)
			}
			if tc.want == ErrMultipleGods {
				assert.ErrorContains(t, err, "siavoush")
			}
			signer := &keyBackedSigner{priv: rootPriv}
			_, err = SignRegistryWithSigner(signer, tc.reg)
			require.ErrorContains(t, err, tc.want)
			assert.Nil(t, signer.gotCanonical, "signer must not see a registry that breaks a human rule")
		})
	}
}

func TestVerifyAndLoad_RejectsHumanRules(t *testing.T) {
	rootPub, rootPriv := fixedEd25519(t, 0x11)
	base := peopleRegistry(t)
	cases := []struct {
		name  string
		patch func(t *testing.T, obj map[string]any)
		want  string
		slug  string
	}{
		{
			name: "god_on_agent", slug: "mira", want: ErrGodOnlyOnHuman,
			patch: func(t *testing.T, obj map[string]any) {
				agents := obj["agents"].([]any)
				agents[0].(map[string]any)["roles"] = []any{map[string]any{"kind": "god"}}
			},
		},
		{
			name: "bridge_on_human", slug: "siavoush", want: ErrAgentRoleOnlyOnAgent,
			patch: func(t *testing.T, obj map[string]any) {
				humans := obj["humans"].([]any)
				humans[1].(map[string]any)["roles"] = []any{
					map[string]any{"kind": "bridge", "vouches_for": []any{"human:ada"}},
				}
			},
		},
		{
			name: "code_signing_on_human", slug: "siavoush", want: ErrAgentRoleOnlyOnAgent,
			patch: func(t *testing.T, obj map[string]any) {
				humans := obj["humans"].([]any)
				humans[1].(map[string]any)["roles"] = []any{
					map[string]any{"kind": "code_signing", "platforms": []any{"darwin-arm64"}},
				}
			},
		},
		{
			name: "two_gods", slug: "siavoush", want: ErrMultipleGods,
			patch: func(t *testing.T, obj map[string]any) {
				humans := obj["humans"].([]any)
				humans[1].(map[string]any)["roles"] = []any{map[string]any{"kind": "god"}}
			},
		},
		{
			name: "slug_across_classes", slug: "mira", want: ErrSlugAcrossClasses,
			patch: func(t *testing.T, obj map[string]any) {
				humans := obj["humans"].([]any)
				humans[1].(map[string]any)["slug"] = "mira"
			},
		},
		{
			name: "bad_custody", slug: "peiman", want: ErrBadCustody,
			patch: func(t *testing.T, obj map[string]any) {
				humans := obj["humans"].([]any)
				humans[0].(map[string]any)["custody"] = "private"
			},
		},
		{
			name: "god_extra_field", slug: "peiman", want: ErrMalformedRole,
			patch: func(t *testing.T, obj map[string]any) {
				humans := obj["humans"].([]any)
				humans[0].(map[string]any)["roles"] = []any{
					map[string]any{"kind": "god", "extra": true},
				}
			},
		},
		{
			name: "human_missing_pubkey", slug: "peiman", want: "binding \"peiman\"",
			patch: func(t *testing.T, obj map[string]any) {
				humans := obj["humans"].([]any)
				delete(humans[0].(map[string]any), "pubkey")
			},
		},
		{
			name: "agent_slug_colon", slug: "human:peiman", want: ErrSlugColon,
			patch: func(t *testing.T, obj map[string]any) {
				agents := obj["agents"].([]any)
				agents[0].(map[string]any)["slug"] = "human:peiman"
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := signPatchedObject(t, rootPriv, base, tc.patch)
			ok, err := identity.VerifyCanonical(rootPub, identity.CanonicalBytesFromTrusted(env.Registry), env.RootSig)
			require.NoError(t, err)
			assert.True(t, ok, "root signature must hold; the reject is the rule")
			_, _, err = VerifyAndLoad(rootPub, env, 4, fixedNow, time.Hour*24*365)
			require.ErrorContains(t, err, tc.want)
			assert.ErrorContains(t, err, tc.slug)
		})
	}
}

func TestVerifyAndLoad_AcceptsExplicitSharedCustodyAndEmptyHumans(t *testing.T) {
	rootPub, rootPriv := fixedEd25519(t, 0x11)
	reg := rolesFreeRegistry(t, nil)
	env := signPatchedObject(t, rootPriv, reg, func(t *testing.T, obj map[string]any) {
		agents := obj["agents"].([]any)
		agents[0].(map[string]any)["custody"] = CustodyShared
		obj["humans"] = []any{}
	})
	loaded, _, err := VerifyAndLoad(rootPub, env, 4, fixedNow, time.Hour*24*365)
	require.NoError(t, err)
	assert.Empty(t, loaded.Agents[0].Custody)
	assert.Empty(t, loaded.Humans)
	cb, err := canonicalBytes(loaded)
	require.NoError(t, err)
	assert.NotContains(t, string(cb.Bytes()), "custody")
	assert.NotContains(t, string(cb.Bytes()), "humans")
}

func TestResolveHuman_LivenessAndClassSeparation(t *testing.T) {
	reg := peopleRegistry(t)
	got, err := ResolveHuman(reg, "peiman", fixedNow)
	require.NoError(t, err)
	assert.Equal(t, ClassHuman, got.Class)
	assert.Equal(t, CustodyIsolated, got.Custody)

	_, err = Resolve(reg, "peiman", fixedNow)
	require.ErrorContains(t, err, ErrUnknownSlug)
	_, err = ResolveHuman(reg, "mira", fixedNow)
	require.ErrorContains(t, err, ErrUnknownSlug)
	_, err = ResolveTuple(reg, "peiman", 1, fixedNow)
	require.ErrorContains(t, err, ErrUnknownSlug)
	human, err := ResolveHumanTuple(reg, "peiman", 1, fixedNow)
	require.NoError(t, err)
	assert.Equal(t, "peiman", human.Slug)

	revoked := cloneRegistry(reg)
	revokedAt := 1
	revoked.Humans[0].RevokedAt = &revokedAt
	_, err = ResolveHuman(revoked, "peiman", fixedNow)
	require.ErrorContains(t, err, ErrRevoked)
	_, err = ResolveHumanTuple(revoked, "peiman", 1, fixedNow)
	require.ErrorContains(t, err, ErrRevoked)

	early := cloneRegistry(reg)
	early.Humans[0].ValidFrom = fixedNow.Unix() + 10
	_, err = ResolveHuman(early, "peiman", fixedNow)
	require.ErrorContains(t, err, ErrUnknownSlug)
	_, err = ResolveHumanTuple(early, "peiman", 1, fixedNow)
	require.ErrorContains(t, err, ErrNotYetValid)

	expired := cloneRegistry(reg)
	expired.Humans[0].ValidUntil = fixedNow.Unix() - 1
	_, err = ResolveHuman(expired, "peiman", fixedNow)
	require.ErrorContains(t, err, ErrUnknownSlug)
	_, err = ResolveHumanTuple(expired, "peiman", 1, fixedNow)
	require.ErrorContains(t, err, ErrExpiredBinding)

	_, err = ResolveHumanTuple(reg, "peiman", 2, fixedNow)
	require.ErrorContains(t, err, ErrKeyEpochMismatch)

	_, ok := GodBinding(rolesFreeRegistry(t, nil), fixedNow)
	assert.False(t, ok)
	two := cloneRegistry(reg)
	two.Humans[1].Roles = []Role{NewGodRole()}
	_, ok = GodBinding(two, fixedNow)
	assert.False(t, ok, "two gods is not a god binding")
}

func TestGodBinding_LivenessRequired(t *testing.T) {
	base := peopleRegistry(t) // peiman holds god; siavoush does not

	// A revoked god is not counted as the god binding.
	revoked := cloneRegistry(base)
	revokedAt := 1
	revoked.Humans[0].RevokedAt = &revokedAt
	_, ok := GodBinding(revoked, fixedNow)
	assert.False(t, ok, "revoked god must not count")

	// An expired god (ValidUntil before now) is not counted.
	expired := cloneRegistry(base)
	expired.Humans[0].ValidUntil = fixedNow.Unix() - 1
	_, ok = GodBinding(expired, fixedNow)
	assert.False(t, ok, "expired god must not count")

	// A not-yet-valid god (ValidFrom after now) is not counted.
	future := cloneRegistry(base)
	future.Humans[0].ValidFrom = fixedNow.Unix() + 1
	_, ok = GodBinding(future, fixedNow)
	assert.False(t, ok, "not-yet-valid god must not count")

	// One live god plus one revoked god: returns the live binding.
	liveAndRevoked := cloneRegistry(base)
	rotated := cloneBindings(base.Humans[:1]) // copy of peiman, epoch 1
	rotated[0].KeyEpoch = 2
	revokedAt2 := int(fixedNow.Unix() - 1)
	rotated[0].RevokedAt = &revokedAt2
	liveAndRevoked.Humans = append(liveAndRevoked.Humans, rotated...)
	got, ok := GodBinding(liveAndRevoked, fixedNow)
	require.True(t, ok, "one live god should be found even when another binding for the same slug is revoked")
	assert.Equal(t, "peiman", got.Slug)
	assert.Nil(t, got.RevokedAt)
}

func TestSignRegistry_MaxBindings(t *testing.T) {
	_, rootPriv := fixedEd25519(t, 0x11)

	// indexedBinding returns a deterministic binding for slot i using a 2-byte
	// counter embedded in the ed25519 seed, giving 65536 unique keys.
	indexedBinding := func(t *testing.T, i int) AgentBinding {
		t.Helper()
		seed := make([]byte, ed25519.SeedSize)
		binary.BigEndian.PutUint16(seed, uint16(i+1)) // keep seed != all-zeros
		priv := ed25519.NewKeyFromSeed(seed)
		pub := priv.Public().(ed25519.PublicKey)
		pk, err := NewPublicKey(pub)
		require.NoError(t, err)
		return AgentBinding{
			Slug: fmt.Sprintf("agent%d", i), DisplayName: fmt.Sprintf("Agent%d", i),
			PubKey: pk, KeyEpoch: 1,
			ValidFrom: 1_770_000_000, ValidUntil: 1_780_000_000,
			AuthorizedOriginDaemons: []string{"d"},
		}
	}

	// Exactly MaxBindings bindings: must be accepted.
	regExact := Registry{Epoch: 5, ValidFrom: 1_770_000_000, ValidUntil: 1_780_000_000}
	for i := range MaxBindings {
		regExact.Agents = append(regExact.Agents, indexedBinding(t, i))
	}
	_, err := SignRegistry(rootPriv, regExact)
	require.NoError(t, err, "exactly MaxBindings bindings must be accepted")

	// MaxBindings+1 bindings: must be refused.
	regOver := regExact
	regOver.Agents = append(append([]AgentBinding(nil), regExact.Agents...), indexedBinding(t, MaxBindings))
	_, err = SignRegistry(rootPriv, regOver)
	require.ErrorContains(t, err, ErrTooManyBindings, "1025 bindings must be refused on sign")
}

func TestVerifyAndLoad_MaxBindings(t *testing.T) {
	rootPub, rootPriv := fixedEd25519(t, 0x11)

	// Build a registry at exactly MaxBindings by signing regExact from above,
	// then verify it loads cleanly; then patch in a 1025th binding and verify
	// VerifyAndLoad rejects it even with a valid root signature.
	seed := make([]byte, ed25519.SeedSize)
	binary.BigEndian.PutUint16(seed, uint16(MaxBindings+1))
	extra := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	extraPK, err := NewPublicKey(extra)
	require.NoError(t, err)

	reg := Registry{Epoch: 5, ValidFrom: 1_770_000_000, ValidUntil: 1_780_000_000}
	for i := range MaxBindings {
		seed := make([]byte, ed25519.SeedSize)
		binary.BigEndian.PutUint16(seed, uint16(i+1))
		priv := ed25519.NewKeyFromSeed(seed)
		pub := priv.Public().(ed25519.PublicKey)
		pk, err2 := NewPublicKey(pub)
		require.NoError(t, err2)
		reg.Agents = append(reg.Agents, AgentBinding{
			Slug: fmt.Sprintf("agent%d", i), DisplayName: fmt.Sprintf("Agent%d", i),
			PubKey: pk, KeyEpoch: 1,
			ValidFrom: 1_770_000_000, ValidUntil: 1_780_000_000,
			AuthorizedOriginDaemons: []string{"d"},
		})
	}
	env, err := SignRegistry(rootPriv, reg)
	require.NoError(t, err)
	_, _, err = VerifyAndLoad(rootPub, env, 4, fixedNow, 365*24*time.Hour)
	require.NoError(t, err, "exactly MaxBindings must load cleanly")

	// Patch a 1025th binding directly into the signed bytes and re-sign.
	env2 := signPatchedObject(t, rootPriv, reg, func(t *testing.T, obj map[string]any) {
		agents := obj["agents"].([]any)
		obj["agents"] = append(agents, map[string]any{
			"slug": "extra", "display_name": "Extra",
			"pubkey":     b64(extraPK.Bytes()),
			"key_epoch":  float64(1),
			"valid_from": float64(1_770_000_000), "valid_until": float64(1_780_000_000),
			"authorized_origin_daemons": []any{"d"},
		})
	})
	_, _, err = VerifyAndLoad(rootPub, env2, 4, fixedNow, 365*24*time.Hour)
	require.ErrorContains(t, err, ErrTooManyBindings, "1025 bindings must be refused on load")
}

func TestVerifyHumanMessage_UsesTheHumanKey(t *testing.T) {
	reg := peopleRegistry(t)
	_, humanPriv := fixedEd25519(t, 0x31)
	_, agentPriv := fixedEd25519(t, 0x22)
	msg := identity.CanonicalBytesFromTrusted([]byte("hello from peiman"))
	sig, err := identity.SignCanonical(humanPriv, msg)
	require.NoError(t, err)

	ok, err := VerifyHumanMessage(reg, "peiman", 1, msg, sig, fixedNow)
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = VerifyMessage(reg, "peiman", 1, msg, sig, fixedNow)
	require.ErrorContains(t, err, ErrUnknownSlug)
	assert.False(t, ok)

	agentSig, err := identity.SignCanonical(agentPriv, msg)
	require.NoError(t, err)
	ok, err = VerifyHumanMessage(reg, "peiman", 1, msg, agentSig, fixedNow)
	require.NoError(t, err)
	assert.False(t, ok, "an agent key must not verify as the human")

	ok, err = VerifyMessage(reg, "mira", 1, msg, agentSig, fixedNow)
	require.NoError(t, err)
	assert.True(t, ok, "agent verify is unchanged")
}

func signPatchedObject(t *testing.T, rootPriv ed25519.PrivateKey, reg Registry, patch func(t *testing.T, obj map[string]any)) SignedRegistry {
	t.Helper()
	wire, err := toWire(reg)
	require.NoError(t, err)
	raw, err := json.Marshal(wire)
	require.NoError(t, err)
	var obj map[string]any
	require.NoError(t, json.Unmarshal(raw, &obj))
	patch(t, obj)
	patched, err := json.Marshal(obj)
	require.NoError(t, err)
	canonical, err := identity.Canonicalize(patched)
	require.NoError(t, err)
	sig, err := identity.SignCanonical(rootPriv, canonical)
	require.NoError(t, err)
	return SignedRegistry{Registry: canonical.Bytes(), RootSig: sig}
}
