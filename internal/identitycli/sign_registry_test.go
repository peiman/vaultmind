package identitycli

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/identity/registry"
)

// testRegistryJSON is a valid unsigned registry (one live binding) whose pubkey
// is filled in per-test (the agent key is generated fresh so we can verify the
// emitted distribution envelope).
func testRegistryJSON(pubB64 string) string {
	return `{"agents":[{"authorized_origin_daemons":["daemon-eu-1"],"display_name":"Mira",` +
		`"key_epoch":1,"pubkey":"` + pubB64 + `","slug":"mira",` +
		`"valid_from":1770000000,"valid_until":1780000000}],` +
		`"epoch":5,"valid_from":1770000000,"valid_until":1780000000}`
}

// TestSignRegistryProducesVerifyingDistributionKeylessly proves SignRegistry
// signs the JCS-canonical registry through the SignerClient seam (no key file)
// and emits a distribution envelope that ParseDistribution+VerifyAndLoad accept.
func TestSignRegistryProducesVerifyingDistributionKeylessly(t *testing.T) {
	rootPub, rootPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey root: %v", err)
	}
	agentPub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey agent: %v", err)
	}
	fake := &fakeSignerClient{priv: rootPriv}

	var out bytes.Buffer
	in := testRegistryJSON(base64.StdEncoding.EncodeToString(agentPub))
	if err := SignRegistry(&out, fake, []byte(in)); err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}

	env, err := registry.ParseDistribution(out.Bytes())
	if err != nil {
		t.Fatalf("ParseDistribution(output): %v\noutput=%q", err, out.String())
	}
	now := time.Unix(1_770_000_500, 0)
	loaded, newHighest, err := registry.VerifyAndLoad(rootPub, env, 4, now, time.Hour*24*365)
	if err != nil {
		t.Fatalf("VerifyAndLoad: %v", err)
	}
	if newHighest != 5 {
		t.Fatalf("newHighest = %d, want 5", newHighest)
	}
	if len(loaded.Agents) != 1 || loaded.Agents[0].Slug != "mira" {
		t.Fatalf("loaded registry mismatch: %+v", loaded.Agents)
	}
}

// TestSignRegistryByteIdenticalToDomain proves the CLI path yields the SAME
// distribution as registry.SignRegistryWithSigner over the same registry — the
// CLI wire-decode introduces no drift.
func TestSignRegistryByteIdenticalToDomain(t *testing.T) {
	_, rootPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	agentPub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey agent: %v", err)
	}
	fake := &fakeSignerClient{priv: rootPriv}

	var out bytes.Buffer
	in := testRegistryJSON(base64.StdEncoding.EncodeToString(agentPub))
	if err := SignRegistry(&out, fake, []byte(in)); err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}
	cliEnv, err := registry.ParseDistribution(out.Bytes())
	if err != nil {
		t.Fatalf("ParseDistribution: %v", err)
	}

	pk, err := registry.NewPublicKey(agentPub)
	if err != nil {
		t.Fatalf("NewPublicKey: %v", err)
	}
	reg := registry.Registry{
		Epoch: 5, ValidFrom: 1770000000, ValidUntil: 1780000000,
		Agents: []registry.AgentBinding{{
			Slug: "mira", DisplayName: "Mira", PubKey: pk, KeyEpoch: 1,
			ValidFrom: 1770000000, ValidUntil: 1780000000,
			AuthorizedOriginDaemons: []string{"daemon-eu-1"},
		}},
	}
	domEnv, err := registry.SignRegistryWithSigner(&keyBackedSigner{priv: rootPriv}, reg)
	if err != nil {
		t.Fatalf("SignRegistryWithSigner: %v", err)
	}
	if !bytes.Equal(cliEnv.Registry, domEnv.Registry) {
		t.Fatalf("canonical bytes differ:\n cli=%q\n dom=%q", cliEnv.Registry, domEnv.Registry)
	}
	if !bytes.Equal(cliEnv.RootSig, domEnv.RootSig) {
		t.Fatal("root sigs differ")
	}
}

// keyBackedSigner signs with a held root key (mirrors the registry-package test
// helper) so the CLI-vs-domain parity test can use a deterministic signer.
type keyBackedSigner struct{ priv ed25519.PrivateKey }

func (k *keyBackedSigner) Sign(b []byte) ([]byte, error) { return ed25519.Sign(k.priv, b), nil }

// TestSignRegistryCarriesRevokedAt proves a binding with revoked_at set (a
// revoked rotation tuple) round-trips through the CLI into the signed
// distribution. It covers the revoked_at narrowing branch. The registry pairs a
// live mira@2 with a revoked mira@1 (uniqueness allows one live + one revoked).
func TestSignRegistryCarriesRevokedAt(t *testing.T) {
	rootPub, rootPriv, _ := ed25519.GenerateKey(rand.Reader)
	livePub, _, _ := ed25519.GenerateKey(rand.Reader)
	oldPub, _, _ := ed25519.GenerateKey(rand.Reader)
	fake := &fakeSignerClient{priv: rootPriv}

	in := `{"agents":[` +
		`{"authorized_origin_daemons":["d"],"display_name":"Mira","key_epoch":2,` +
		`"pubkey":"` + base64.StdEncoding.EncodeToString(livePub) + `","slug":"mira",` +
		`"valid_from":1770000000,"valid_until":1780000000},` +
		`{"authorized_origin_daemons":["d"],"display_name":"Mira","key_epoch":1,` +
		`"pubkey":"` + base64.StdEncoding.EncodeToString(oldPub) + `","revoked_at":1770000100,` +
		`"slug":"mira","valid_from":1770000000,"valid_until":1780000000}],` +
		`"epoch":5,"valid_from":1770000000,"valid_until":1780000000}`

	var out bytes.Buffer
	if err := SignRegistry(&out, fake, []byte(in)); err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}
	env, err := registry.ParseDistribution(out.Bytes())
	if err != nil {
		t.Fatalf("ParseDistribution: %v", err)
	}
	now := time.Unix(1_770_000_500, 0)
	loaded, _, err := registry.VerifyAndLoad(rootPub, env, 4, now, time.Hour*24*365)
	if err != nil {
		t.Fatalf("VerifyAndLoad: %v", err)
	}
	var sawRevoked bool
	for _, b := range loaded.Agents {
		if b.KeyEpoch == 1 {
			if b.RevokedAt == nil || *b.RevokedAt != 1770000100 {
				t.Fatalf("revoked_at did not round-trip: %+v", b.RevokedAt)
			}
			sawRevoked = true
		}
	}
	if !sawRevoked {
		t.Fatal("revoked binding (key_epoch 1) missing from loaded registry")
	}
}

func TestSignRegistryRejectsBadBase64PubKey(t *testing.T) {
	fake := &fakeSignerClient{}
	var out bytes.Buffer
	in := testRegistryJSON("not-base64!!!")
	if err := SignRegistry(&out, fake, []byte(in)); err == nil {
		t.Fatal("expected bad-base64 pubkey rejection, got nil")
	}
	if fake.gotCanonical != nil {
		t.Fatal("signer was called for a bad-base64 pubkey")
	}
	if out.Len() != 0 {
		t.Fatalf("reject must print nothing, got %q", out.String())
	}
}

func TestSignRegistryRejectsSmallOrderPubKey(t *testing.T) {
	fake := &fakeSignerClient{}
	var out bytes.Buffer
	// 32 zero bytes is a small-order point -> NewPublicKey rejects it.
	zero := base64.StdEncoding.EncodeToString(make([]byte, ed25519.PublicKeySize))
	in := testRegistryJSON(zero)
	if err := SignRegistry(&out, fake, []byte(in)); err == nil {
		t.Fatal("expected small-order pubkey rejection, got nil")
	}
	if fake.gotCanonical != nil {
		t.Fatal("signer was called for a small-order pubkey")
	}
}

func TestSignRegistryRejectsUnknownField(t *testing.T) {
	fake := &fakeSignerClient{}
	var out bytes.Buffer
	bad := `{"agents":[],"epoch":5,"valid_from":1,"valid_until":2,"evil":true}`
	if err := SignRegistry(&out, fake, []byte(bad)); err == nil {
		t.Fatal("expected unknown-field rejection, got nil")
	}
	if out.Len() != 0 {
		t.Fatalf("parse reject must print nothing, got %q", out.String())
	}
}

func TestSignRegistryRejectsTrailingData(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	fake := &fakeSignerClient{}
	var out bytes.Buffer
	in := testRegistryJSON(base64.StdEncoding.EncodeToString(pub)) + ` {"trailing":true}`
	if err := SignRegistry(&out, fake, []byte(in)); err == nil {
		t.Fatal("expected trailing-data rejection, got nil")
	}
	if out.Len() != 0 {
		t.Fatalf("parse reject must print nothing, got %q", out.String())
	}
}

func TestSignRegistryFailsClosedOnSignerError(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	fake := &fakeSignerClient{failErr: sentinelErr("signer unreachable")}
	var out bytes.Buffer
	in := testRegistryJSON(base64.StdEncoding.EncodeToString(pub))
	if err := SignRegistry(&out, fake, []byte(in)); err == nil {
		t.Fatal("expected fail-closed error, got nil")
	}
	if out.Len() != 0 {
		t.Fatalf("fail-closed must print nothing, got %q", out.String())
	}
}

func TestSignRegistryRejectsOutOfRangeEpoch(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	fake := &fakeSignerClient{}
	var out bytes.Buffer
	// epoch 0 is below the [1, 2^53] floor -> rejected by the registry gate.
	in := `{"agents":[{"authorized_origin_daemons":["d"],"display_name":"M",` +
		`"key_epoch":1,"pubkey":"` + base64.StdEncoding.EncodeToString(pub) + `","slug":"mira",` +
		`"valid_from":1,"valid_until":2}],"epoch":0,"valid_from":1,"valid_until":2}`
	if err := SignRegistry(&out, fake, []byte(in)); err == nil {
		t.Fatal("expected out-of-range epoch rejection, got nil")
	}
	if fake.gotCanonical != nil {
		t.Fatal("signer was called for an out-of-range epoch")
	}
}

// TestSignRegistryRejectsRegistryEpochAbove2Pow53 covers the registry-epoch gate
// at the JCS-safe ceiling. The gate runs on the int64 wire value BEFORE narrowing
// (epochInRangeI64), so the wire boundary is the range authority — a >2^53 value
// cannot reach the signer.
func TestSignRegistryRejectsRegistryEpochAbove2Pow53(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	fake := &fakeSignerClient{}
	var out bytes.Buffer
	// 2^53 + 1 = 9007199254740993, one past MaxSafeEpoch.
	in := `{"agents":[{"authorized_origin_daemons":["d"],"display_name":"M",` +
		`"key_epoch":1,"pubkey":"` + base64.StdEncoding.EncodeToString(pub) + `","slug":"mira",` +
		`"valid_from":1,"valid_until":2}],"epoch":9007199254740993,"valid_from":1,"valid_until":2}`
	if err := SignRegistry(&out, fake, []byte(in)); err == nil {
		t.Fatal("expected >2^53 registry epoch rejection, got nil")
	}
	if fake.gotCanonical != nil {
		t.Fatal("signer was called for a >2^53 registry epoch")
	}
	if out.Len() != 0 {
		t.Fatalf("reject must print nothing, got %q", out.String())
	}
}

// TestSignRegistryRejectsBindingKeyEpochAbove2Pow53 covers the BINDING key_epoch
// gate (distinct from the registry epoch) — the path the original out-of-range
// test did not exercise.
func TestSignRegistryRejectsBindingKeyEpochAbove2Pow53(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	fake := &fakeSignerClient{}
	var out bytes.Buffer
	in := `{"agents":[{"authorized_origin_daemons":["d"],"display_name":"M",` +
		`"key_epoch":9007199254740993,"pubkey":"` + base64.StdEncoding.EncodeToString(pub) + `","slug":"mira",` +
		`"valid_from":1,"valid_until":2}],"epoch":1,"valid_from":1,"valid_until":2}`
	if err := SignRegistry(&out, fake, []byte(in)); err == nil {
		t.Fatal("expected >2^53 binding key_epoch rejection, got nil")
	}
	if fake.gotCanonical != nil {
		t.Fatal("signer was called for a >2^53 binding key_epoch")
	}
}

// TestSignRegistryRejectsWrongLengthPubKey covers a well-formed base64 string
// that decodes to the wrong number of bytes (not 32) — a distinct NewPublicKey
// reject path from bad-base64 and small-order, exercised at the CLI boundary.
func TestSignRegistryRejectsWrongLengthPubKey(t *testing.T) {
	fake := &fakeSignerClient{}
	var out bytes.Buffer
	short := base64.StdEncoding.EncodeToString(make([]byte, 16)) // 16 != 32
	in := testRegistryJSON(short)
	if err := SignRegistry(&out, fake, []byte(in)); err == nil {
		t.Fatal("expected wrong-length pubkey rejection, got nil")
	}
	if fake.gotCanonical != nil {
		t.Fatal("signer was called for a wrong-length pubkey")
	}
	if out.Len() != 0 {
		t.Fatalf("reject must print nothing, got %q", out.String())
	}
}

// TestSignRegistryRejectsNegativeRevokedAt covers the revoked_at sanity gate: a
// negative (pre-epoch) revocation timestamp fails closed.
func TestSignRegistryRejectsNegativeRevokedAt(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	fake := &fakeSignerClient{}
	var out bytes.Buffer
	in := `{"agents":[{"authorized_origin_daemons":["d"],"display_name":"M",` +
		`"key_epoch":1,"pubkey":"` + base64.StdEncoding.EncodeToString(pub) + `","revoked_at":-1,` +
		`"slug":"mira","valid_from":1,"valid_until":2}],"epoch":1,"valid_from":1,"valid_until":2}`
	if err := SignRegistry(&out, fake, []byte(in)); err == nil {
		t.Fatal("expected negative revoked_at rejection, got nil")
	}
	if fake.gotCanonical != nil {
		t.Fatal("signer was called for a negative revoked_at")
	}
}
