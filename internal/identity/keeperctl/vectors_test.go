package keeperctl

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/identity"
	"github.com/peiman/vaultmind/internal/identity/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var updateFixture = flag.Bool("update", false, "regenerate keeper control cross-language vectors")

const fixturePath = "testdata/keeper_control_vectors.json"

type signedVector struct {
	Name      string          `json:"name"`
	Kind      string          `json:"kind"`
	Input     json.RawMessage `json:"input"`
	Canonical string          `json:"canonical"`
	SHA256    string          `json:"sha256_hex"`
	PubKey    string          `json:"pubkey_hex"`
	Signature string          `json:"signature_base64"`
}

type bindingVector struct {
	Slug       string `json:"slug"`
	PubKey     string `json:"pubkey_hex"`
	KeyEpoch   int    `json:"key_epoch"`
	ValidFrom  int64  `json:"valid_from"`
	ValidUntil int64  `json:"valid_until"`
	RevokedAt  *int   `json:"revoked_at,omitempty"`
}
type registryVector struct {
	ValidFrom  int64           `json:"valid_from"`
	ValidUntil int64           `json:"valid_until"`
	Bindings   []bindingVector `json:"bindings"`
}
type priorRequest struct {
	From     string `json:"from"`
	Nonce    string `json:"nonce"`
	IssuedAt int64  `json:"issued_at"`
}
type installedVector struct {
	Grant     Grant  `json:"grant"`
	Signature string `json:"signature_base64"`
}
type verifierVector struct {
	Name              string            `json:"name"`
	Registry          registryVector    `json:"registry"`
	Now               int64             `json:"now"`
	NotBefore         int64             `json:"not_before"`
	MaxAgeSeconds     int64             `json:"max_age_seconds"`
	MaxSkewSeconds    int64             `json:"max_skew_seconds"`
	KeeperOwners      map[string]string `json:"keeper_owners"`
	Installed         []installedVector `json:"installed_grants"`
	Revoked           []string          `json:"revoked_grant_ids"`
	Prior             []priorRequest    `json:"prior_requests"`
	NonceStoreFailure bool              `json:"nonce_store_failure"`
	Request           ControlRequest    `json:"request"`
	Signature         string            `json:"signature_base64"`
	Expected          Decision          `json:"expected"`
}
type controlFixture struct {
	Version int              `json:"version"`
	Signed  []signedVector   `json:"signed_forms"`
	Reject  []gateCase       `json:"gate_rejects"`
	Verify  []verifierVector `json:"verifier_cases"`
}

type recordingNonces struct {
	NonceStore
	prior []priorRequest
}

func (s *recordingNonces) CheckAndRecord(from, nonce string, issued int64) error {
	s.prior = append(s.prior, priorRequest{from, nonce, issued})
	return s.NonceStore.CheckAndRecord(from, nonce, issued)
}

func signedVectors(t *testing.T) []signedVector {
	t.Helper()
	r := sampleRequest()
	r.Action = ActionMessage
	r.Body = ptr(strings.Repeat("x", MaxBodyBytes))
	u := sampleRequest()
	u.Action = ActionRebrief
	u.Body = ptr("Résumé ⭐\nWatch the keeper.")
	var vectors []signedVector
	for _, tc := range []struct {
		name, kind string
		form       any
	}{
		{"pause", KindControl, sampleRequest()}, {"max_body", KindControl, r},
		{"unicode_rebrief", KindControl, u}, {"grant", KindGrant, sampleGrant()},
		{"grant_revoke", KindGrantRevoke, sampleRevoke()},
	} {
		c, err := canonicalForm(tc.form)
		require.NoError(t, err)
		sig, err := identity.SignCanonical(fixedKey(1), c)
		require.NoError(t, err)
		hash := sha256.Sum256(c.Bytes())
		vectors = append(vectors, signedVector{tc.name, tc.kind, rawJSON(t, tc.form), string(c.Bytes()),
			hex.EncodeToString(hash[:]), hex.EncodeToString(fixedKey(1).Public().(ed25519.PublicKey)), base64.StdEncoding.EncodeToString(sig)})
	}
	return vectors
}

func scenarioVector(t *testing.T, tc verifyCase) verifierVector {
	t.Helper()
	p, req := testPolicy(t), sampleRequest()
	sig := requestSig(t, req)
	recorder := &recordingNonces{NonceStore: p.Nonces}
	p.Nonces = recorder
	if tc.edit != nil {
		tc.edit(t, &p, &req, &sig)
	}
	v := verifierVector{Name: tc.name, Now: p.Now().Unix(), NotBefore: p.NotBefore,
		MaxAgeSeconds: 300, MaxSkewSeconds: 30, KeeperOwners: p.KeeperOwners,
		Registry:  registryVector{ValidFrom: p.Registry.ValidFrom, ValidUntil: p.Registry.ValidUntil},
		Installed: []installedVector{}, Revoked: []string{}, Prior: append([]priorRequest{}, recorder.prior...),
		NonceStoreFailure: tc.reason == ReasonNonceStoreFailed, Request: req,
		Signature: base64.StdEncoding.EncodeToString(sig), Expected: Decision{Accepted: tc.role != "", Role: tc.role, Reason: tc.reason}}
	for _, b := range p.Registry.Agents {
		v.Registry.Bindings = append(v.Registry.Bindings, bindingVector{b.Slug, hex.EncodeToString(b.PubKey.Bytes()), b.KeyEpoch, b.ValidFrom, b.ValidUntil, b.RevokedAt})
	}
	s := p.Grants.(*MemoryGrantStore)
	for _, id := range sortedKeys(s.grants) {
		g := s.grants[id]
		v.Installed = append(v.Installed, installedVector{g.Grant, base64.StdEncoding.EncodeToString(g.Sig)})
		if g.Revoked {
			v.Revoked = append(v.Revoked, id)
		}
	}
	return v
}

func buildFixture(t *testing.T) controlFixture {
	t.Helper()
	f := controlFixture{Version: 1, Signed: signedVectors(t), Reject: gateCases(t)}
	for _, tc := range verifierCases() {
		f.Verify = append(f.Verify, scenarioVector(t, tc))
	}
	return f
}

func fixtureBytes(t *testing.T) []byte {
	t.Helper()
	raw, err := json.MarshalIndent(buildFixture(t), "", "  ")
	require.NoError(t, err)
	return append(raw, '\n')
}

func TestGenerateKeeperControlFixture(t *testing.T) {
	if !*updateFixture {
		t.Skip("run with -update to regenerate the committed fixture")
	}
	raw := fixtureBytes(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(fixturePath), 0755))
	require.NoError(t, os.WriteFile(fixturePath, raw, 0644)) //nolint:gosec // public committed test vectors
	t.Logf("wrote %s (%d bytes)", fixturePath, len(raw))
}

func TestKeeperControlFixtureMatchesGenerator(t *testing.T) {
	if *updateFixture {
		t.Skip("skip comparison during fixture generation")
	}
	raw, err := os.ReadFile(fixturePath)
	require.NoError(t, err, "generate with -run TestGenerateKeeperControlFixture -update")
	assert.True(t, bytes.Equal(raw, fixtureBytes(t)), "frozen cross-language contract changed; review and regenerate with -update")
}

func loadFixture(t *testing.T) controlFixture {
	t.Helper()
	raw, err := os.ReadFile(fixturePath)
	require.NoError(t, err)
	var f controlFixture
	require.NoError(t, json.Unmarshal(raw, &f))
	require.Equal(t, 1, f.Version)
	return f
}

func TestFixtureSignedFormsAndGates(t *testing.T) {
	f := loadFixture(t)
	require.NotEmpty(t, f.Signed)
	require.NotEmpty(t, f.Reject)
	for _, v := range f.Signed {
		t.Run(v.Name, func(t *testing.T) {
			c := parseCanonical(t, v.Kind, v.Input)
			assert.Equal(t, v.Canonical, string(c.Bytes()))
			hash := sha256.Sum256(c.Bytes())
			assert.Equal(t, v.SHA256, hex.EncodeToString(hash[:]))
			pub, err := hex.DecodeString(v.PubKey)
			require.NoError(t, err)
			sig, err := base64.StdEncoding.DecodeString(v.Signature)
			require.NoError(t, err)
			ok, err := identity.VerifyCanonical(pub, c, sig)
			require.NoError(t, err)
			assert.True(t, ok)
		})
	}
	for _, v := range f.Reject {
		t.Run(v.Name, func(t *testing.T) {
			var gate *GateError
			require.ErrorAs(t, parseForm(v.Kind, v.raw()), &gate)
			assert.Equal(t, v.Gate, gate.Gate)
		})
	}
}

func parseCanonical(t *testing.T, kind string, raw []byte) identity.CanonicalBytes {
	t.Helper()
	var form any
	switch kind {
	case KindControl:
		f, err := ParseControlRequest(raw)
		require.NoError(t, err)
		form = f
	case KindGrant:
		f, err := ParseGrant(raw)
		require.NoError(t, err)
		form = f
	default:
		f, err := ParseGrantRevoke(raw)
		require.NoError(t, err)
		form = f
	}
	c, err := canonicalForm(form)
	require.NoError(t, err)
	return c
}

func policyFromVector(t *testing.T, v verifierVector) Policy {
	t.Helper()
	now := func() time.Time { return time.Unix(v.Now, 0) }
	p := Policy{Registry: registry.Registry{ValidFrom: v.Registry.ValidFrom, ValidUntil: v.Registry.ValidUntil},
		Now: now, NotBefore: v.NotBefore, KeeperOwners: v.KeeperOwners, Grants: NewMemoryGrantStore(),
		Nonces: NewMemoryNonceStore(now, time.Duration(v.MaxAgeSeconds)*time.Second, time.Duration(v.MaxSkewSeconds)*time.Second),
		MaxAge: time.Duration(v.MaxAgeSeconds) * time.Second, MaxSkew: time.Duration(v.MaxSkewSeconds) * time.Second}
	require.NoError(t, seedPolicy(&p, v))
	if v.NonceStoreFailure {
		p.Nonces = brokenNonces{}
	}
	return p
}

// seedPolicy loads a scenario's registry bindings, installed and revoked grants,
// and already-seen nonces. Any of those lists may legitimately be empty, so it
// returns the first failure for the caller to assert once.
func seedPolicy(p *Policy, v verifierVector) error {
	for _, b := range v.Registry.Bindings {
		raw, err := hex.DecodeString(b.PubKey)
		if err != nil {
			return err
		}
		pub, err := registry.NewPublicKey(raw)
		if err != nil {
			return err
		}
		p.Registry.Agents = append(p.Registry.Agents, registry.AgentBinding{Slug: b.Slug, PubKey: pub, KeyEpoch: b.KeyEpoch,
			ValidFrom: b.ValidFrom, ValidUntil: b.ValidUntil, RevokedAt: b.RevokedAt})
	}
	return seedStores(p, v)
}

func seedStores(p *Policy, v verifierVector) error {
	for _, g := range v.Installed {
		sig, err := base64.StdEncoding.DecodeString(g.Signature)
		if err != nil {
			return err
		}
		if err := p.Grants.Put(g.Grant, sig); err != nil {
			return err
		}
	}
	for _, id := range v.Revoked {
		if err := p.Grants.MarkRevoked(id); err != nil {
			return err
		}
	}
	for _, prior := range v.Prior {
		if err := p.Nonces.CheckAndRecord(prior.From, prior.Nonce, prior.IssuedAt); err != nil {
			return err
		}
	}
	return nil
}

func TestFixtureVerifierCases(t *testing.T) {
	covered := make(map[Reason]bool)
	for _, v := range loadFixture(t).Verify {
		t.Run(v.Name, func(t *testing.T) {
			p := policyFromVector(t, v)
			sig, err := base64.StdEncoding.DecodeString(v.Signature)
			require.NoError(t, err)
			d := VerifyControl(p, v.Request, sig)
			d.Detail = ""
			assert.Equal(t, v.Expected, d)
			covered[v.Expected.Reason] = true
		})
	}
	for _, reason := range []Reason{ReasonSchemaInvalid, ReasonRegistryExpired, ReasonSignatureInvalid, ReasonBindingRejected,
		ReasonUnknownKeeper, ReasonStale, ReasonFromFuture, ReasonReplayed, ReasonGrantMissing, ReasonGrantExpired,
		ReasonGrantRevoked, ReasonGrantNotForKeeper, ReasonGrantScopeMissing, ReasonGrantWrongGrantee,
		ReasonGrantBadSignature, ReasonControlNeedsOwner, ReasonNonceStoreFailed, ReasonBeforeStart} {
		assert.True(t, covered[reason], "fixture must cover %s", reason)
	}
}
