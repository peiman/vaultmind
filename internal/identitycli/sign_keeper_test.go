package identitycli

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/identity"
	"github.com/peiman/vaultmind/internal/identity/keeperctl"
	"github.com/stretchr/testify/require"
)

const (
	keeperControlJSON = `{"alg_version":1,"kind":"keeper.control","keeper":"worker","action":"message","from":"owner","key_epoch":1,"issued_at":100,"nonce":"original-nonce-1234","body":"<hello>&\u2028world"}`
	keeperGrantJSON   = `{"alg_version":1,"kind":"keeper.grant","grant_id":"read-1","from":"owner","key_epoch":1,"grantee":"reader","keepers":["worker"],"scopes":["context","status"],"issued_at":100,"expires_at":500}`
	keeperRevokeJSON  = `{"alg_version":1,"kind":"keeper.grant_revoke","grant_id":"read-1","from":"owner","key_epoch":1,"issued_at":100,"nonce":"original-nonce-1234"}`
)

func keeperTestSigner() *fakeSignerClient {
	return &fakeSignerClient{priv: ed25519.NewKeyFromSeed(bytes.Repeat([]byte{1}, ed25519.SeedSize))}
}

func keeperCanonical(t *testing.T, raw []byte) identity.CanonicalBytes {
	t.Helper()
	var kind struct{ Kind string }
	require.NoError(t, json.Unmarshal(raw, &kind))
	switch kind.Kind {
	case keeperctl.KindControl:
		r, err := keeperctl.ParseControlRequest(raw)
		require.NoError(t, err)
		c, err := keeperctl.CanonicalizeControlRequest(r)
		require.NoError(t, err)
		return c
	case keeperctl.KindGrant:
		g, err := keeperctl.ParseGrant(raw)
		require.NoError(t, err)
		c, err := keeperctl.CanonicalizeGrant(g)
		require.NoError(t, err)
		return c
	default:
		r, err := keeperctl.ParseGrantRevoke(raw)
		require.NoError(t, err)
		c, err := keeperctl.CanonicalizeGrantRevoke(r)
		require.NoError(t, err)
		return c
	}
}

func checkKeeperResult(t *testing.T, out []byte, fake *fakeSignerClient, expected []byte) map[string]json.RawMessage {
	t.Helper()
	var result struct {
		Document json.RawMessage `json:"document"`
		Sig      string          `json:"sig"`
	}
	require.NoError(t, json.Unmarshal(out, &result))
	require.Equal(t, 1, bytes.Count(out, []byte("\n")))
	require.True(t, bytes.HasSuffix(out, []byte("\n")))
	canonical := keeperCanonical(t, expected)
	require.Equal(t, canonical.Bytes(), []byte(result.Document))
	require.Equal(t, canonical.Bytes(), fake.gotCanonical)
	sig, err := base64.StdEncoding.DecodeString(result.Sig)
	require.NoError(t, err)
	ok, err := identity.VerifyCanonical(fake.priv.Public().(ed25519.PublicKey), canonical, sig)
	require.NoError(t, err)
	require.True(t, ok)
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(result.Document, &fields))
	return fields
}

func TestSignKeeperKinds(t *testing.T) {
	for _, raw := range []string{keeperControlJSON, keeperGrantJSON, keeperRevokeJSON} {
		t.Run(raw, func(t *testing.T) {
			fake := keeperTestSigner()
			var out bytes.Buffer
			require.NoError(t, SignKeeper(&out, fake, []byte(raw), false, nil, nil))
			checkKeeperResult(t, out.Bytes(), fake, []byte(raw))
		})
	}
}

func TestSignKeeperStamp(t *testing.T) {
	const nonce = "AAECAwQFBgcICQoLDA0ODxAR"
	random := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17}
	for _, raw := range []string{keeperControlJSON, keeperGrantJSON, keeperRevokeJSON} {
		for _, removeTime := range []bool{false, true} {
			for _, removeNonce := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/time=%t/nonce=%t", raw, removeTime, removeNonce), func(t *testing.T) {
					input, expected := raw, raw
					if removeTime {
						input = strings.Replace(input, `,"issued_at":100`, "", 1)
						expected = strings.Replace(expected, `"issued_at":100`, `"issued_at":200`, 1)
					}
					if removeNonce {
						input = strings.Replace(input, `,"nonce":"original-nonce-1234"`, "", 1)
						expected = strings.Replace(expected, "original-nonce-1234", nonce, 1)
					}
					fake := keeperTestSigner()
					reader := bytes.NewReader(random)
					var out bytes.Buffer
					require.NoError(t, SignKeeper(&out, fake, []byte(input), true, func() time.Time { return time.Unix(200, 999) }, reader))
					fields := checkKeeperResult(t, out.Bytes(), fake, []byte(expected))
					if strings.Contains(raw, keeperctl.KindGrant+`"`) {
						require.NotContains(t, fields, keeperctl.FieldNonce)
					}
					if !removeNonce || !strings.Contains(raw, `"nonce"`) {
						require.Equal(t, len(random), reader.Len(), "unused randomness must not be consumed")
					} else {
						require.Zero(t, reader.Len())
						require.Equal(t, `"`+nonce+`"`, string(fields[keeperctl.FieldNonce]))
					}
				})
			}
		}
	}
}

func TestSignKeeperFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name, raw, message string
		stamp              bool
		signerErr          error
	}{
		{"missing nonce", strings.Replace(keeperControlJSON, `,"nonce":"original-nonce-1234"`, "", 1), "missing_field", false, nil},
		{"unknown kind", `{"kind":"other"}`, "keeper.control, keeper.grant, keeper.grant_revoke", false, nil},
		{"missing kind", `{}`, "keeper.control, keeper.grant, keeper.grant_revoke", true, nil},
		{"wrong kind type", `{"kind":7}`, "keeper.control, keeper.grant, keeper.grant_revoke", false, nil},
		{"malformed", `{"kind":`, "JSON", false, nil},
		{"trailing document", keeperControlJSON + ` {}`, "JSON", true, nil},
		{"duplicate", strings.Replace(keeperControlJSON, `"issued_at":100`, `"issued_at":100,"issued_at":101`, 1), "unknown_field", false, nil},
		{"duplicate stamp", strings.Replace(keeperControlJSON, `"issued_at":100`, `"issued_at":100,"issued_at":101`, 1), identity.ErrSchemaDuplicateKey, true, nil},
		{"pause body", strings.Replace(keeperControlJSON, `"message"`, `"pause"`, 1), "body_forbidden", false, nil},
		{"null time", strings.Replace(keeperControlJSON, `"issued_at":100`, `"issued_at":null`, 1), "null_field", true, nil},
		{"null nonce", strings.Replace(keeperControlJSON, `"nonce":"original-nonce-1234"`, `"nonce":null`, 1), "null_field", true, nil},
		{"empty nonce", strings.Replace(keeperControlJSON, "original-nonce-1234", "", 1), "nonce_length", true, nil},
		{"grant nonce", strings.TrimSuffix(keeperGrantJSON, "}") + `,"nonce":"unexpected"}`, "unknown_field", true, nil},
		{"unsorted scopes", strings.Replace(keeperGrantJSON, `["context","status"]`, `["status","context"]`, 1), "array_not_sorted_unique", true, nil},
		{"signer", keeperControlJSON, "signer offline", false, errors.New("signer offline")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := keeperTestSigner()
			fake.failErr = tc.signerErr
			var out bytes.Buffer
			err := SignKeeper(&out, fake, []byte(tc.raw), tc.stamp, func() time.Time { return time.Unix(200, 0) }, strings.NewReader(strings.Repeat("x", 18)))
			require.ErrorContains(t, err, tc.message)
			require.Empty(t, out.String())
			require.Nil(t, fake.gotCanonical)
			if strings.Contains(tc.message, "_field") || tc.name == "pause body" {
				var gate *keeperctl.GateError
				if !strings.Contains(tc.name, "stamp") {
					require.ErrorAs(t, err, &gate)
					require.Contains(t, err.Error(), string(gate.Gate))
				}
			}
		})
	}
}

// A document the gates refuse never reached the signer, so its error must not
// say the signer failed; a real signer failure must say so.
func TestSignKeeperErrorNamesTheFailingStep(t *testing.T) {
	now := func() time.Time { return time.Unix(200, 0) }
	fake := keeperTestSigner()
	var out bytes.Buffer
	err := SignKeeper(&out, fake, []byte(strings.Replace(keeperControlJSON, `"message"`, `"pause"`, 1)), false, now, nil)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "signer")

	fake.failErr = errors.New("signer offline")
	err = SignKeeper(&out, fake, []byte(keeperControlJSON), false, now, nil)
	require.ErrorContains(t, err, "signing keeper document via signer")
}

func TestSignKeeperShortRandomRead(t *testing.T) {
	for _, raw := range []string{keeperControlJSON, keeperRevokeJSON} {
		fake := keeperTestSigner()
		var out bytes.Buffer
		input := strings.Replace(raw, `,"nonce":"original-nonce-1234"`, "", 1)
		err := SignKeeper(&out, fake, []byte(input), true, time.Now, strings.NewReader("short"))
		require.ErrorIs(t, err, io.ErrUnexpectedEOF)
		require.ErrorContains(t, err, "nonce")
		require.Empty(t, out.String())
		require.Nil(t, fake.gotCanonical)
	}
}

func TestKeeperStampPreservesExistingFields(t *testing.T) {
	// Keep field order, whitespace, escapes and a large integer's exact token.
	raw := []byte(` { "body" : "\u003chello\u003e", "kind":"keeper.control", "key_epoch":9007199254740992 } `)
	kind, stamped, err := keeperDocument(raw, true, func() time.Time { return time.Unix(200, 0) }, bytes.NewReader(make([]byte, 18)))
	require.NoError(t, err)
	require.Equal(t, keeperctl.KindControl, kind)
	require.Equal(t, ` { "body" : "\u003chello\u003e", "kind":"keeper.control", "key_epoch":9007199254740992 ,"issued_at":200,"nonce":"AAAAAAAAAAAAAAAAAAAAAAAA"}`, " "+string(stamped))
	require.Equal(t, ` { "body" : "\u003chello\u003e", "kind":"keeper.control", "key_epoch":9007199254740992 } `, string(raw), "input buffer must stay unchanged")
}

func TestSignKeeperStampKeepsZeroTime(t *testing.T) {
	for _, raw := range []string{keeperControlJSON, keeperGrantJSON, keeperRevokeJSON} {
		raw = strings.Replace(raw, `"issued_at":100`, `"issued_at":0`, 1)
		var out bytes.Buffer
		fake := keeperTestSigner()
		// All stamp fields are present; neither dependency should be used.
		require.NoError(t, SignKeeper(&out, fake, []byte(raw), true, nil, nil))
		checkKeeperResult(t, out.Bytes(), fake, []byte(raw))
	}
}
