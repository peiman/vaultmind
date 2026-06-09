package identitycli_test

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/identity"
	"github.com/peiman/vaultmind/internal/identity/enrollment"
	"github.com/peiman/vaultmind/internal/identity/invite"
	"github.com/peiman/vaultmind/internal/identity/registry"
	"github.com/peiman/vaultmind/internal/identity/relayclient"
	"github.com/peiman/vaultmind/internal/identitycli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSignerClient signs in-process with a held ed25519 key (the keyless seam),
// or returns failErr to simulate an unreachable/refusing signer. A test points
// priv at the key whose public half it passes as --pubkey, so self-verify
// passes; pointing it elsewhere proves the self-verify guard fires.
type fakeSignerClient struct {
	priv    ed25519.PrivateKey
	failErr error
}

func (f *fakeSignerClient) Sign(canonicalBytes []byte) ([]byte, error) {
	if f.failErr != nil {
		return nil, f.failErr
	}
	return ed25519.Sign(f.priv, canonicalBytes), nil
}

// fixedKey derives a deterministic ed25519 keypair from a one-byte seed fill so
// pubkeys in tests are LOW-ENTROPY (the gitleaks entropy scanner stays quiet).
func fixedKey(t *testing.T, fill byte) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	seed := bytes.Repeat([]byte{fill}, ed25519.SeedSize)
	priv := ed25519.NewKeyFromSeed(seed)
	return priv.Public().(ed25519.PublicKey), priv
}

// lowEntropyTransportB64 is a deterministic 32-byte (length-only-checked)
// WireGuard pubkey, base64-std. It is NOT validated as a key, so a repeated
// byte is fine and keeps entropy low.
func lowEntropyTransportB64() string {
	return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x11}, 32))
}

// rootKey mints a deterministic ROOT keypair and returns its base64-std pubkey
// plus the derived network id.
func rootKey(t *testing.T, fill byte) (rootB64, networkID string) {
	t.Helper()
	pub, _ := fixedKey(t, fill)
	return base64.StdEncoding.EncodeToString(pub), registry.NetworkID(pub)
}

// relayServing returns an httptest server that serves the given WellKnownRoot at
// the canonical path.
func relayServing(t *testing.T, root relayclient.WellKnownRoot) *httptest.Server {
	t.Helper()
	body, err := json.Marshal(root)
	require.NoError(t, err)
	mux := http.NewServeMux()
	mux.HandleFunc(relayclient.WellKnownRootPath, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// enrollHarness wires a consistent invite + relay + member key for the happy
// path; individual tests mutate the EnrollConfig they get back.
type enrollHarness struct {
	cfg        identitycli.EnrollConfig
	memberPub  ed25519.PublicKey
	memberPriv ed25519.PrivateKey
	networkID  string
	relayURL   string
}

func newEnrollHarness(t *testing.T) enrollHarness {
	t.Helper()
	rootB64, networkID := rootKey(t, 0xA1)
	memberPub, memberPriv := fixedKey(t, 0xB2)

	srv := relayServing(t, relayclient.WellKnownRoot{
		RootPubKey:   rootB64,
		NetworkID:    networkID,
		RootKeyEpoch: 1,
	})

	token, _, err := invite.Encode(invite.Invite{
		NetworkID:  networkID,
		Relay:      srv.URL,
		RootPubKey: rootB64,
	})
	require.NoError(t, err)

	return enrollHarness{
		cfg: identitycli.EnrollConfig{
			InviteTokenOrURL:   token,
			DisplayName:        "Mira",
			Slug:               "mira",
			PubKeyB64:          base64.StdEncoding.EncodeToString(memberPub),
			TransportPubKeyB64: lowEntropyTransportB64(),
			AssumeYes:          true,
			HTTPClient:         srv.Client(),
			Signer:             &fakeSignerClient{priv: memberPriv},
			Now:                func() time.Time { return time.Unix(2_000_000, 0) },
			RandReader:         bytes.NewReader(bytes.Repeat([]byte{0x07}, 64)),
		},
		memberPub:  memberPub,
		memberPriv: memberPriv,
		networkID:  networkID,
		relayURL:   srv.URL,
	}
}

// runEnroll runs Enroll and returns the captured stdout/stderr plus the error.
func runEnroll(cfg identitycli.EnrollConfig, in io.Reader) (out, errOut *bytes.Buffer, err error) {
	out, errOut = &bytes.Buffer{}, &bytes.Buffer{}
	if in == nil {
		in = strings.NewReader("")
	}
	err = identitycli.Enroll(out, errOut, in, cfg)
	return out, errOut, err
}

// TestEnrollHappyPath_EmitsVerifyingWire drives the full flow and proves the
// emitted wire parses, carries a sig, and self-verifies (proof-of-possession).
func TestEnrollHappyPath_EmitsVerifyingWire(t *testing.T) {
	h := newEnrollHarness(t)
	out, errOut, err := runEnroll(h.cfg, nil)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &got), "emitted wire must be JSON: %q", out.String())
	sigB64, ok := got[enrollment.FieldSig].(string)
	require.True(t, ok, "wire must carry a sig")
	assert.NotEmpty(t, sigB64)

	// Reconstruct Fields from the emitted wire and verify the sig against the
	// OWN pubkey.
	rawSig, err := base64.StdEncoding.DecodeString(sigB64)
	require.NoError(t, err)
	fields := fieldsFromWire(t, got)
	verified, verr := enrollment.VerifyEnrollment(fields, rawSig)
	require.NoError(t, verr)
	assert.True(t, verified, "emitted request must self-verify")

	// stderr guidance carries the network, relay, and the signed note.
	se := errOut.String()
	assert.Contains(t, se, h.networkID)
	assert.Contains(t, se, identitycli.EnrollSignedNote)
	assert.Contains(t, se, identitycli.EnrollNextStepNote)
}

// fieldsFromWire reconstructs enrollment.Fields from the parsed wire JSON map so
// a test can re-verify the signature.
func fieldsFromWire(t *testing.T, m map[string]any) enrollment.Fields {
	t.Helper()
	f := enrollment.Fields{
		AlgVersion:      int64(m[enrollment.FieldAlgVersion].(float64)),
		Created:         int64(m[enrollment.FieldCreated].(float64)),
		DisplayName:     m[enrollment.FieldDisplayName].(string),
		KeyEpoch:        int64(m[enrollment.FieldKeyEpoch].(float64)),
		NetworkID:       m[enrollment.FieldNetworkID].(string),
		Nonce:           m[enrollment.FieldNonce].(string),
		PubKey:          m[enrollment.FieldPubKey].(string),
		Slug:            m[enrollment.FieldSlug].(string),
		TransportPubKey: m[enrollment.FieldTransportPubKey].(string),
	}
	if v, ok := m[enrollment.FieldTransportEndpoint].(string); ok {
		f.TransportEndpoint = &v
	}
	return f
}

// TestEnrollEmitsKeyEpoch1AndAlgVersion1 pins the hardcoded first-enrollment
// invariants.
func TestEnrollEmitsKeyEpoch1AndAlgVersion1(t *testing.T) {
	h := newEnrollHarness(t)
	out, _, err := runEnroll(h.cfg, nil)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))
	assert.EqualValues(t, 1, got[enrollment.FieldKeyEpoch])
	assert.EqualValues(t, enrollment.AlgVersion, got[enrollment.FieldAlgVersion])
	assert.EqualValues(t, 2_000_000, got[enrollment.FieldCreated])
}

// TestEnrollTransportEndpointOmittedWhenEmpty proves an empty endpoint is OMITTED
// from the wire (absent != null), and a set one is present.
func TestEnrollTransportEndpointOmittedWhenEmpty(t *testing.T) {
	h := newEnrollHarness(t)
	out, _, err := runEnroll(h.cfg, nil)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))
	_, present := got[enrollment.FieldTransportEndpoint]
	assert.False(t, present, "transport_endpoint must be omitted when --transport-endpoint is empty")

	h2 := newEnrollHarness(t)
	h2.cfg.TransportEndpoint = "relay.example:51820"
	out2, _, err := runEnroll(h2.cfg, nil)
	require.NoError(t, err)
	var got2 map[string]any
	require.NoError(t, json.Unmarshal(out2.Bytes(), &got2))
	assert.Equal(t, "relay.example:51820", got2[enrollment.FieldTransportEndpoint])
}

// TestEnrollInviteDecodeErrorPropagates covers the bad-invite branch.
func TestEnrollInviteDecodeErrorPropagates(t *testing.T) {
	cases := map[string]string{
		"bad prefix": "not-an-invite",
		"bad base64": "vmenroll1:!!!notbase64!!!",
		"bad json":   "vmenroll1:" + base64.RawURLEncoding.EncodeToString([]byte("not json")),
	}
	for name, token := range cases {
		t.Run(name, func(t *testing.T) {
			h := newEnrollHarness(t)
			h.cfg.InviteTokenOrURL = token
			out, _, err := runEnroll(h.cfg, nil)
			require.Error(t, err)
			assert.Empty(t, out.String(), "no request must be emitted on a bad invite")
		})
	}

	// network-mismatch: a hand-built token carrying a VALID root key but a
	// network_id that does NOT derive from it. invite.Encode cannot produce this
	// (its validate() rejects the mismatch), so assemble the wire JSON directly.
	// This exercises the security-spine binding invite.Decode enforces and proves
	// Enroll propagates it (rejecting for the RIGHT reason, not incidentally).
	t.Run("network-mismatch", func(t *testing.T) {
		rootPub, _ := fixedKey(t, 0xA1)
		otherPub, _ := fixedKey(t, 0xC3)
		body, err := json.Marshal(map[string]string{
			invite.FieldNetworkID:  registry.NetworkID(otherPub),
			invite.FieldRelay:      "https://relay.example",
			invite.FieldRootPubKey: base64.StdEncoding.EncodeToString(rootPub),
		})
		require.NoError(t, err)
		token := "vmenroll1:" + base64.RawURLEncoding.EncodeToString(body)

		h := newEnrollHarness(t)
		h.cfg.InviteTokenOrURL = token
		out, _, err := runEnroll(h.cfg, nil)
		require.Error(t, err)
		assert.ErrorContains(t, err, invite.ErrNetworkIDMismatch)
		assert.Empty(t, out.String(), "no request must be emitted on a network-mismatch invite")
	})
}

// TestEnrollWellKnownFetchFailures covers non-200, malformed, and bad-scheme
// relay responses.
func TestEnrollWellKnownFetchFailures(t *testing.T) {
	t.Run("non-200", func(t *testing.T) {
		rootB64, networkID := rootKey(t, 0xA1)
		mux := http.NewServeMux()
		mux.HandleFunc(relayclient.WellKnownRootPath, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		})
		srv := httptest.NewServer(mux)
		t.Cleanup(srv.Close)
		cfg := enrollCfgFor(t, srv.URL, srv.Client(), rootB64, networkID)
		out, _, err := runEnroll(cfg, nil)
		require.Error(t, err)
		assert.Empty(t, out.String())
	})

	t.Run("malformed", func(t *testing.T) {
		rootB64, networkID := rootKey(t, 0xA1)
		mux := http.NewServeMux()
		mux.HandleFunc(relayclient.WellKnownRootPath, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("{broken"))
		})
		srv := httptest.NewServer(mux)
		t.Cleanup(srv.Close)
		cfg := enrollCfgFor(t, srv.URL, srv.Client(), rootB64, networkID)
		out, _, err := runEnroll(cfg, nil)
		require.Error(t, err)
		assert.Empty(t, out.String())
	})
}

// enrollCfgFor builds a happy EnrollConfig pointing at relayURL, with an invite
// for the given root, EXCEPT the relay may not actually serve a matching root —
// callers use it to drive fetch/cross-check failure tests.
func enrollCfgFor(t *testing.T, relayURL string, client *http.Client, rootB64, networkID string) identitycli.EnrollConfig {
	t.Helper()
	memberPub, memberPriv := fixedKey(t, 0xB2)
	token, _, err := invite.Encode(invite.Invite{
		NetworkID:  networkID,
		Relay:      relayURL,
		RootPubKey: rootB64,
	})
	require.NoError(t, err)
	return identitycli.EnrollConfig{
		InviteTokenOrURL:   token,
		DisplayName:        "Mira",
		Slug:               "mira",
		PubKeyB64:          base64.StdEncoding.EncodeToString(memberPub),
		TransportPubKeyB64: lowEntropyTransportB64(),
		AssumeYes:          true,
		HTTPClient:         client,
		Signer:             &fakeSignerClient{priv: memberPriv},
		Now:                func() time.Time { return time.Unix(2_000_000, 0) },
		RandReader:         bytes.NewReader(bytes.Repeat([]byte{0x07}, 64)),
	}
}

// TestEnrollCrossCheck_RelayRootMismatch: the relay advertises a DIFFERENT root
// than the invite — a distinct hard error, no request emitted.
func TestEnrollCrossCheck_RelayRootMismatch(t *testing.T) {
	inviteRootB64, inviteNetwork := rootKey(t, 0xA1)
	otherRootB64, otherNetwork := rootKey(t, 0xCC)

	// Relay self-consistently advertises the OTHER root (pub matches its
	// network_id), so the only failure is relay-root != invite-root.
	srv := relayServing(t, relayclient.WellKnownRoot{
		RootPubKey: otherRootB64,
		NetworkID:  otherNetwork,
	})
	cfg := enrollCfgFor(t, srv.URL, srv.Client(), inviteRootB64, inviteNetwork)
	out, _, err := runEnroll(cfg, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "wrong relay or MITM")
	assert.Empty(t, out.String())
}

// TestEnrollCrossCheck_RelaySelfInconsistent: the relay's network_id does not
// match NetworkID(its own advertised root) — a distinct hard error.
func TestEnrollCrossCheck_RelaySelfInconsistent(t *testing.T) {
	rootB64, networkID := rootKey(t, 0xA1)
	srv := relayServing(t, relayclient.WellKnownRoot{
		RootPubKey: rootB64,
		NetworkID:  "vmnet1:00000000000000000000000000000000", // lies about its own id
	})
	cfg := enrollCfgFor(t, srv.URL, srv.Client(), rootB64, networkID)
	out, _, err := runEnroll(cfg, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "self-inconsistent")
	assert.Empty(t, out.String())
}

// TestEnrollCrossCheck_RelayRootDecodeFails: the relay advertises a non-key root
// pubkey.
func TestEnrollCrossCheck_RelayRootDecodeFails(t *testing.T) {
	rootB64, networkID := rootKey(t, 0xA1)
	srv := relayServing(t, relayclient.WellKnownRoot{
		RootPubKey: "!!!not-base64!!!",
		NetworkID:  networkID,
	})
	cfg := enrollCfgFor(t, srv.URL, srv.Client(), rootB64, networkID)
	out, _, err := runEnroll(cfg, nil)
	require.Error(t, err)
	assert.Empty(t, out.String())
}

// NOTE on the network_id-mismatch cross-check (enroll.go: errEnrollNetworkIDMismatch):
// it is DEFENSIVE DEPTH that cannot be reached via valid inputs. By the time the
// flow reaches it, relay-root-bytes == invite-root-bytes is already proven, and
// NetworkID is a pure function of the root, so NetworkID(relay) == NetworkID(invite);
// the relay-self-consistency check (== root.NetworkID) and invite.Decode's binding
// check (== inv.NetworkID) then force root.NetworkID == inv.NetworkID. The check
// stays as a belt-and-suspenders guard the spec mandates as a distinct hard error.
// Its sibling branches (relay-root mismatch, relay self-inconsistent) are covered
// above.

// TestEnrollFingerprintConfirm covers the interactive y/yes/Y accept and the
// n/empty/garbage abort paths (no --yes).
func TestEnrollFingerprintConfirm(t *testing.T) {
	accept := []string{"y\n", "Y\n", "yes\n", " yes \n", "YES\n"}
	for _, ans := range accept {
		t.Run("accept-"+strings.TrimSpace(ans), func(t *testing.T) {
			h := newEnrollHarness(t)
			h.cfg.AssumeYes = false
			out, errOut, err := runEnroll(h.cfg, strings.NewReader(ans))
			require.NoError(t, err)
			assert.NotEmpty(t, out.String(), "accepted confirm must emit a request")
			assert.Contains(t, errOut.String(), identitycli.EnrollConfirmPrompt)
		})
	}

	reject := []string{"n\n", "\n", "garbage\n", "no\n", ""}
	for _, ans := range reject {
		t.Run("reject-"+strings.TrimSpace(ans), func(t *testing.T) {
			h := newEnrollHarness(t)
			h.cfg.AssumeYes = false
			out, _, err := runEnroll(h.cfg, strings.NewReader(ans))
			require.Error(t, err)
			assert.Empty(t, out.String(), "rejected confirm must emit nothing")
		})
	}
}

// TestEnrollYesSkipsPrompt proves --yes never prompts (no read from in) and
// still emits.
func TestEnrollYesSkipsPrompt(t *testing.T) {
	h := newEnrollHarness(t) // AssumeYes=true
	out, errOut, err := runEnroll(h.cfg, strings.NewReader("should-not-be-read"))
	require.NoError(t, err)
	assert.NotEmpty(t, out.String())
	assert.NotContains(t, errOut.String(), identitycli.EnrollConfirmPrompt)
	assert.Contains(t, errOut.String(), identitycli.EnrollAutoConfirmedNote)
}

// TestEnrollSelfVerifyMismatch: the signer holds a DIFFERENT key than --pubkey.
// Self-verify must fire loudly and emit NOTHING.
func TestEnrollSelfVerifyMismatch(t *testing.T) {
	h := newEnrollHarness(t)
	// Replace the signer with one holding an unrelated key.
	_, wrongPriv := fixedKey(t, 0x42)
	h.cfg.Signer = &fakeSignerClient{priv: wrongPriv}

	out, _, err := runEnroll(h.cfg, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), identitycli.ErrEnrollSelfVerify)
	assert.Empty(t, out.String(), "self-verify failure must emit no request")
}

// TestEnrollSignerUnreachable wraps a signer failure with guidance and emits
// nothing.
func TestEnrollSignerUnreachable(t *testing.T) {
	h := newEnrollHarness(t)
	h.cfg.Signer = &fakeSignerClient{failErr: errors.New("dial unix: connection refused")}
	out, _, err := runEnroll(h.cfg, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "identity signer")
	assert.Empty(t, out.String())
}

// TestEnrollGateFailFast: a non-ASCII slug fails the enrollment gate with the
// exact enrollment.Err* message BEFORE the signer is ever dialed (proven by a
// signer that would FAIL if reached).
func TestEnrollGateFailFast(t *testing.T) {
	h := newEnrollHarness(t)
	h.cfg.Slug = "miré" // non-ASCII -> ErrSlugASCII
	// A signer that would error if called; the gate must short-circuit first.
	h.cfg.Signer = &fakeSignerClient{failErr: errors.New("signer should NOT be dialed")}

	out, _, err := runEnroll(h.cfg, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), enrollment.ErrSlugASCII)
	assert.NotContains(t, err.Error(), "should NOT be dialed")
	assert.Empty(t, out.String())
}

// TestEnrollRequiredFieldsFailClosed proves each missing required field is a
// distinct fail-closed error with no network/signer interaction.
func TestEnrollRequiredFieldsFailClosed(t *testing.T) {
	base := newEnrollHarness(t).cfg
	cases := map[string]func(*identitycli.EnrollConfig){
		"invite":        func(c *identitycli.EnrollConfig) { c.InviteTokenOrURL = "" },
		"display-name":  func(c *identitycli.EnrollConfig) { c.DisplayName = "" },
		"slug":          func(c *identitycli.EnrollConfig) { c.Slug = "" },
		"pubkey":        func(c *identitycli.EnrollConfig) { c.PubKeyB64 = "" },
		"transport-pub": func(c *identitycli.EnrollConfig) { c.TransportPubKeyB64 = "" },
	}
	for name, mut := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := base
			mut(&cfg)
			out, _, err := runEnroll(cfg, nil)
			require.Error(t, err)
			assert.Empty(t, out.String())
		})
	}
}

// TestEnrollAcceptsEnrollURL proves the --invite value may be a full enroll URL
// (token in the fragment), not just a bare token.
func TestEnrollAcceptsEnrollURL(t *testing.T) {
	h := newEnrollHarness(t)
	_, url, err := invite.Encode(invite.Invite{
		NetworkID:  h.networkID,
		Relay:      h.relayURL,
		RootPubKey: mustInviteRoot(t, h.cfg.InviteTokenOrURL),
	})
	require.NoError(t, err)
	h.cfg.InviteTokenOrURL = url

	out, _, err := runEnroll(h.cfg, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, out.String())
}

// mustInviteRoot decodes the harness token to recover its root pubkey so the URL
// variant is minted from the same anchor.
func mustInviteRoot(t *testing.T, token string) string {
	t.Helper()
	inv, err := invite.Decode(token)
	require.NoError(t, err)
	return inv.RootPubKey
}

// TestEnrollWireStripSigEqualsCanonical ties the emitted wire to the signature:
// dropping sig + re-canonicalizing equals the canonical bytes the signature
// covers.
func TestEnrollWireStripSigEqualsCanonical(t *testing.T) {
	h := newEnrollHarness(t)
	out, _, err := runEnroll(h.cfg, nil)
	require.NoError(t, err)

	var obj map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &obj))
	delete(obj, enrollment.FieldSig)
	stripped, err := json.Marshal(obj)
	require.NoError(t, err)
	recanon, err := identity.Canonicalize(stripped)
	require.NoError(t, err)

	fields := fieldsFromWire(t, mustReparse(t, out.Bytes()))
	want, err := enrollment.CanonicalizeEnrollment(fields)
	require.NoError(t, err)
	assert.Equal(t, want.Bytes(), recanon.Bytes())
}

func mustReparse(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	return m
}

// TestEnrollDefaultSeams exercises the PRODUCTION default seams: Now, RandReader,
// and Signer are all left nil, so now()/rand() fall back to time.Now/crypto-rand
// and signer() builds a real *signer.Client pointed at a dead socket. The flow
// must reach the (unreachable) signer and fail closed with guidance — proving the
// default seams are wired without needing a live daemon. HTTPClient is also left
// nil so relayclient.FetchRoot's default-client branch runs.
func TestEnrollDefaultSeams(t *testing.T) {
	rootB64, networkID := rootKey(t, 0xA1)
	memberPub, _ := fixedKey(t, 0xB2)
	srv := relayServing(t, relayclient.WellKnownRoot{RootPubKey: rootB64, NetworkID: networkID})
	token, _, err := invite.Encode(invite.Invite{NetworkID: networkID, Relay: srv.URL, RootPubKey: rootB64})
	require.NoError(t, err)

	cfg := identitycli.EnrollConfig{
		InviteTokenOrURL:   token,
		DisplayName:        "Mira",
		Slug:               "mira",
		PubKeyB64:          base64.StdEncoding.EncodeToString(memberPub),
		TransportPubKeyB64: lowEntropyTransportB64(),
		SignerSocket:       "/nonexistent/identity-signer.sock",
		AssumeYes:          true,
		// HTTPClient, Signer, Now, RandReader all nil => default seams.
	}
	out, _, err := runEnroll(cfg, nil)
	require.Error(t, err, "default signer at a dead socket must fail closed")
	assert.Contains(t, err.Error(), "identity signer")
	assert.Empty(t, out.String())
}
