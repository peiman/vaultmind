package identity

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Hub-session signing domain (SSOT). A session proof is a signature over
// SessionDomainTag immediately followed by the JCS bytes of a SessionStatement.
// Envelope and registry signatures cover untagged JCS. Doctor self-verify
// covers "vaultmind-doctor-selfverify-v1:" plus a nonce. None of those byte
// strings verifies as a session proof, and a session proof does not verify as
// any of them.
const (
	// SessionDomainTag is the UTF-8 prefix of every hub-session signature.
	SessionDomainTag = "vaultmind-hub-session-v1:"
	// SessionPurpose is the only purpose a session statement may carry.
	SessionPurpose = "hub-session"
)

const (
	sessionChallengeMin = 16
	sessionChallengeMax = 64
	sessionNetworkIDMax = 128
	// sessionMaxSafeInt is the inclusive JCS ceiling (2^53). The same bound
	// envelope and registry use so an integer round-trips in every JSON
	// implementation. The hub's freshness window is a separate, tighter check.
	sessionMaxSafeInt = int64(1) << 53
)

// Structural session-statement rejects (SSOT). SessionTaggedBytes and
// VerifySessionStatement return these as the error text.
const (
	// ErrSessionChallenge is returned when challenge is not standard base64
	// (RFC 4648 §4) of 16 to 64 decoded bytes.
	ErrSessionChallenge = "identity: session: challenge must be standard base64 of 16 to 64 bytes"
	// ErrSessionNetworkID is returned when network_id is empty, longer than
	// 128 bytes, not valid UTF-8, not NFC, or contains an ASCII control
	// (U+0000–U+001F or U+007F). It is rejected, never normalized.
	ErrSessionNetworkID = "identity: session: network_id must be non-empty NFC UTF-8 of at most 128 bytes with no ASCII control"
	// ErrSessionPurpose is returned when purpose is not SessionPurpose.
	ErrSessionPurpose = "identity: session: purpose must be hub-session"
	// ErrSessionSlug is returned when slug is empty, not valid UTF-8, not NFC,
	// or contains an ASCII control (U+0000–U+001F or U+007F).
	ErrSessionSlug = "identity: session: slug must be non-empty NFC UTF-8 with no ASCII control"
	// ErrSessionKeyEpoch is returned when key_epoch is outside [1, 2^53].
	ErrSessionKeyEpoch = "identity: session: key_epoch must be in [1, 2^53]"
	// ErrSessionTS is returned when ts is outside [0, 2^53].
	ErrSessionTS = "identity: session: ts must be in [0, 2^53]"
)

// SessionStatement is the JSON object a hub-session signature covers, after
// the domain tag. Field names are the canonical keys. Challenge is the
// standard-base64 text the hub issued, stored exactly as issued (not
// re-encoded). Slug and KeyEpoch are the signer's own binding.
type SessionStatement struct {
	Challenge string `json:"challenge"`
	KeyEpoch  int64  `json:"key_epoch"`
	NetworkID string `json:"network_id"`
	Purpose   string `json:"purpose"`
	Slug      string `json:"slug"`
	TS        int64  `json:"ts"`
}

// SessionTaggedBytes gates st and returns the exact bytes a hub-session
// signature covers: the UTF-8 SessionDomainTag immediately followed by the
// RFC 8785 (JCS) canonical encoding of st. The tag is outside the JSON. A
// gate failure returns a named error and no bytes.
func SessionTaggedBytes(st SessionStatement) (CanonicalBytes, error) {
	if err := checkSessionStatement(st); err != nil {
		return CanonicalBytes{}, err
	}
	raw, err := json.Marshal(st)
	if err != nil {
		return CanonicalBytes{}, fmt.Errorf("identity: session: marshal statement: %w", err)
	}
	canon, err := Canonicalize(raw)
	if err != nil {
		return CanonicalBytes{}, err
	}
	tagged := make([]byte, len(SessionDomainTag)+len(canon.Bytes()))
	copy(tagged, SessionDomainTag)
	copy(tagged[len(SessionDomainTag):], canon.Bytes())
	// The tag is not JCS. CanonicalBytesFromTrusted is the signable-bytes
	// token, the same hatch the doctor self-verify challenge uses.
	return CanonicalBytesFromTrusted(tagged), nil
}

// VerifySessionStatement reports whether sig is an ed25519 signature by pub
// over the hub-session tagged bytes of st.
//
// Those bytes are rebuilt from st on every call: SessionDomainTag
// ("vaultmind-hub-session-v1:") immediately followed by the JCS canonical
// encoding of st. A field changed after signing does not verify. A correct
// signature over any other domain — bare JCS (what an envelope or registry
// signature covers), the doctor self-verify tag, or no tag — does not verify.
//
// A structural reject (a failed gate, or a malformed key or signature) returns
// (false, error). An honest non-match returns (false, nil). A match returns
// (true, nil). This function does not check challenge single-use, network
// identity, or the time window; those belong to the hub.
func VerifySessionStatement(pub ed25519.PublicKey, st SessionStatement, sig []byte) (bool, error) {
	tagged, err := SessionTaggedBytes(st)
	if err != nil {
		return false, err
	}
	return VerifyCanonical(pub, tagged, sig)
}

func checkSessionStatement(st SessionStatement) error {
	if err := checkSessionChallenge(st.Challenge); err != nil {
		return err
	}
	if err := checkSessionNetworkID(st.NetworkID); err != nil {
		return err
	}
	if st.Purpose != SessionPurpose {
		return fmt.Errorf("%s", ErrSessionPurpose)
	}
	if err := checkSessionSlug(st.Slug); err != nil {
		return err
	}
	return checkSessionInts(st.KeyEpoch, st.TS)
}

func checkSessionChallenge(challenge string) error {
	raw, err := base64.StdEncoding.DecodeString(challenge)
	if err != nil || len(raw) < sessionChallengeMin || len(raw) > sessionChallengeMax {
		return fmt.Errorf("%s", ErrSessionChallenge)
	}
	return nil
}

func checkSessionNetworkID(id string) error {
	if id == "" || len(id) > sessionNetworkIDMax || !utf8.ValidString(id) || !norm.NFC.IsNormalString(id) || hasSessionControl(id) {
		return fmt.Errorf("%s", ErrSessionNetworkID)
	}
	return nil
}

func checkSessionSlug(slug string) error {
	if slug == "" || !utf8.ValidString(slug) || !norm.NFC.IsNormalString(slug) || hasSessionControl(slug) {
		return fmt.Errorf("%s", ErrSessionSlug)
	}
	return nil
}

// hasSessionControl reports whether s contains an ASCII control
// (U+0000–U+001F or U+007F). Those bytes never occur inside a multibyte
// UTF-8 sequence, so a byte scan is the whole check. RFC 8785 escapes
// controls, and that escaping is where Go and Rust JCS libraries can diverge.
func hasSessionControl(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] <= 0x1F || s[i] == 0x7F {
			return true
		}
	}
	return false
}

func checkSessionInts(epoch, ts int64) error {
	if epoch < 1 || epoch > sessionMaxSafeInt {
		return fmt.Errorf("%s", ErrSessionKeyEpoch)
	}
	if ts < 0 || ts > sessionMaxSafeInt {
		return fmt.Errorf("%s", ErrSessionTS)
	}
	return nil
}
