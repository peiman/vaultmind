package identitycli

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/peiman/vaultmind/internal/identity/enrollment"
	"github.com/peiman/vaultmind/internal/identity/invite"
	"github.com/peiman/vaultmind/internal/identity/registry"
	"github.com/peiman/vaultmind/internal/identity/relayclient"
	"github.com/peiman/vaultmind/internal/identity/signer"
)

// Enroll output-label / prompt constants (SSOT) — every printed string the
// member-enroll flow emits is a named constant here, never an inline literal.
const (
	// enrollNonceLen is the number of random bytes the enrollment nonce carries
	// (base64-std encoded into the request). 16 bytes = 128 bits of anti-replay
	// entropy.
	enrollNonceLen = 16

	// enrollFirstKeyEpoch is the key_epoch of a FIRST enrollment. Rotation (a
	// later epoch) is a future path — first enrollment is always epoch 1, so this
	// is hardcoded and intentionally NOT a flag.
	enrollFirstKeyEpoch = 1

	// EnrollConfirmPrompt is shown on errOut to ask the member to confirm the
	// fingerprint matches the one the admin gave them out of band.
	EnrollConfirmPrompt = "Confirm this matches the fingerprint your admin gave you out-of-band [y/N]: "
	// EnrollAutoConfirmedNote is printed instead of the prompt when --yes is set.
	EnrollAutoConfirmedNote = "auto-confirmed via --yes"
	// EnrollConfirmedNote is the interactive-confirm result word shown in the
	// summary when the member accepted the OOB fingerprint prompt.
	EnrollConfirmedNote = "confirmed"
	// EnrollNetworkLabel prefixes the printed network id.
	EnrollNetworkLabel = "network: "
	// EnrollRelayLabel prefixes the printed relay base URL.
	EnrollRelayLabel = "relay: "
	// EnrollFingerprintLabel prefixes the printed fingerprint (= network id).
	EnrollFingerprintLabel = "fingerprint: "
	// EnrollSignedNote confirms the request signed AND self-verified
	// (proof-of-possession passed).
	EnrollSignedNote = "✓ signed & self-verified"
	// EnrollNextStepNote tells the member to hand the emitted request to the admin.
	EnrollNextStepNote = "hand this to your admin out-of-band; they run `vaultmind identity enroll-add`."
)

// Enroll error strings (SSOT). Each failure mode is a distinct typed reject so
// the member can tell a transport/cross-check/proof failure apart.
const (
	// ErrEnrollEmptyInvite is returned when --invite is empty.
	ErrEnrollEmptyInvite = "identitycli: --invite is required (a vmenroll1: token or enroll URL)"
	// ErrEnrollEmptyDisplayName is returned when --display-name is empty.
	ErrEnrollEmptyDisplayName = "identitycli: --display-name is required"
	// ErrEnrollEmptySlug is returned when --slug is empty.
	ErrEnrollEmptySlug = "identitycli: --slug is required"
	// ErrEnrollEmptyPubKey is returned when --pubkey is empty.
	ErrEnrollEmptyPubKey = "identitycli: --pubkey is required (your base64-std ed25519 identity pubkey)"
	// ErrEnrollEmptyTransportPubKey is returned when --transport-pubkey is empty.
	ErrEnrollEmptyTransportPubKey = "identitycli: --transport-pubkey is required (your base64-std WireGuard pubkey)"

	// errEnrollDecodeInvite wraps an invite.Decode failure (bad prefix/base64/json
	// or network-id mismatch).
	errEnrollDecodeInvite = "identitycli: decode invite"
	// errEnrollFetchRoot wraps a well-known root fetch failure.
	errEnrollFetchRoot = "identitycli: fetch relay well-known root"
	// errEnrollRelayRootDecode is returned when the relay's advertised root pubkey
	// is not base64-std of a valid ed25519 key (bad base64 / wrong length /
	// small-order).
	errEnrollRelayRootDecode = "identitycli: relay root pubkey is not a valid ed25519 public key"
	// errEnrollRelaySelfInconsistent is returned when the relay's advertised
	// network_id != NetworkID(its advertised root pubkey) — the relay contradicts
	// itself (misconfig / MITM).
	errEnrollRelaySelfInconsistent = "identitycli: relay network_id does not match NetworkID(relay root pubkey) — relay is self-inconsistent (misconfig or MITM)"
	// errEnrollRootMismatch is returned when the relay's root pubkey != the
	// invite's root pubkey (the trust anchors disagree — wrong relay or MITM).
	errEnrollRootMismatch = "identitycli: relay root pubkey does not match the invite root pubkey — wrong relay or MITM, refusing to enroll"
	// errEnrollNetworkIDMismatch is returned when the relay network_id != the
	// invite network_id.
	errEnrollNetworkIDMismatch = "identitycli: relay network_id does not match the invite network_id — wrong relay or MITM, refusing to enroll"
	// errEnrollInviteRootDecode is returned when the invite root pubkey cannot be
	// base64-decoded (should not happen — invite.Decode already validated it).
	errEnrollInviteRootDecode = "identitycli: decode invite root pubkey"
	// errEnrollAborted is returned when the member declines the fingerprint
	// confirmation prompt.
	errEnrollAborted = "enrollment aborted: fingerprint not confirmed"
	// errEnrollNonce wraps a crypto/rand read failure for the nonce.
	errEnrollNonce = "identitycli: read enrollment nonce"
	// errEnrollSign wraps a signer failure with guidance.
	errEnrollSign = "identitycli: sign enrollment via signer (is `vaultmind identity signer` running?)"
	// errEnrollSelfVerifyDecode wraps a base64 decode failure of the returned sig.
	errEnrollSelfVerifyDecode = "identitycli: decode enrollment signature"
	// ErrEnrollSelfVerify is returned when proof-of-possession self-verification
	// fails: the running signer holds a DIFFERENT key than --pubkey.
	ErrEnrollSelfVerify = "self-signature failed to verify: the running signer's identity key does not match --pubkey (run `identity signer` with the key whose public half you passed to --pubkey)"
)

// EnrollConfig carries everything the member-enroll flow needs, with injectable
// seams so tests need no real daemon, network, or clock. Every seam defaults to
// the production implementation when nil/zero.
type EnrollConfig struct {
	InviteTokenOrURL   string // --invite (a vmenroll1: token OR an enroll URL)
	DisplayName        string // --display-name
	Slug               string // --slug
	PubKeyB64          string // --pubkey (base64-std ed25519 identity pubkey)
	TransportPubKeyB64 string // --transport-pubkey (base64-std WireGuard pubkey)
	TransportEndpoint  string // --transport-endpoint (optional host:port; "" => omitted)
	SignerSocket       string // --signer-socket

	AssumeYes bool // --yes (skip the OOB fingerprint confirmation prompt)

	// Seams (nil => production default).
	HTTPClient *http.Client            // well-known fetch client
	Signer     enrollment.SignerClient // keyless signer (default: &signer.Client{SocketPath})
	Now        func() time.Time        // created timestamp source (default: time.Now)
	RandReader io.Reader               // nonce entropy source (default: crypto/rand.Reader)
}

func (c EnrollConfig) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c EnrollConfig) rand() io.Reader {
	if c.RandReader != nil {
		return c.RandReader
	}
	return rand.Reader
}

func (c EnrollConfig) signer() enrollment.SignerClient {
	if c.Signer != nil {
		return c.Signer
	}
	return &signer.Client{SocketPath: c.SignerSocket}
}

func (c EnrollConfig) httpClient() *http.Client {
	return c.HTTPClient // nil is fine — relayclient.FetchRoot defaults it
}

// Enroll runs the Contract-B member-onboarding flow: decode the invite, fetch
// the relay's well-known root, CROSS-CHECK it against the invite (the security
// spine), confirm the fingerprint out of band, assemble + gate + sign the
// enrollment request via the keyless signer, self-verify proof-of-possession,
// and emit the signed wire JSON on out (human guidance goes to errOut). It FAILS
// CLOSED: any cross-check, gate, sign, or self-verify failure returns non-nil
// and emits NO request.
func Enroll(out, errOut io.Writer, in io.Reader, cfg EnrollConfig) error {
	if err := validateEnrollConfig(cfg); err != nil {
		return err
	}

	inv, err := invite.Decode(cfg.InviteTokenOrURL)
	if err != nil {
		return fmt.Errorf("%s: %w", errEnrollDecodeInvite, err)
	}

	root, err := relayclient.FetchRoot(context.Background(), cfg.httpClient(), inv.Relay)
	if err != nil {
		return fmt.Errorf("%s: %w", errEnrollFetchRoot, err)
	}

	if err := crossCheckRoot(inv, root); err != nil {
		return err
	}

	fp := invite.Fingerprint(inv)
	if err := confirmFingerprint(errOut, in, fp, cfg.AssumeYes); err != nil {
		return err
	}

	fields, err := assembleFields(cfg, inv)
	if err != nil {
		return err
	}

	// Fail-fast: surface gate errors with the exact enrollment.Err* message
	// BEFORE dialing the signer.
	if _, err := enrollment.CanonicalizeEnrollment(fields); err != nil {
		return err
	}

	res, err := enrollment.SignEnrollment(cfg.signer(), fields)
	if err != nil {
		return fmt.Errorf("%s: %w", errEnrollSign, err)
	}

	if err := selfVerify(fields, res.Sig); err != nil {
		return err
	}

	wire, err := enrollment.MarshalWire(fields, res.Sig)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "%s\n", wire); err != nil {
		return err
	}
	return writeEnrollGuidance(errOut, inv, fp, cfg.AssumeYes)
}

// validateEnrollConfig fails closed on any missing required field BEFORE any
// network or signer interaction.
func validateEnrollConfig(cfg EnrollConfig) error {
	switch {
	case cfg.InviteTokenOrURL == "":
		return fmt.Errorf("%s", ErrEnrollEmptyInvite)
	case cfg.DisplayName == "":
		return fmt.Errorf("%s", ErrEnrollEmptyDisplayName)
	case cfg.Slug == "":
		return fmt.Errorf("%s", ErrEnrollEmptySlug)
	case cfg.PubKeyB64 == "":
		return fmt.Errorf("%s", ErrEnrollEmptyPubKey)
	case cfg.TransportPubKeyB64 == "":
		return fmt.Errorf("%s", ErrEnrollEmptyTransportPubKey)
	}
	return nil
}

// crossCheckRoot is the security spine: the relay's advertised root MUST decode
// to a valid key, be self-consistent (network_id == NetworkID(pubkey)), and
// match the invite's anchor (raw bytes AND network id). Any mismatch is a
// distinct hard error — the flow NEVER proceeds on a mismatch.
func crossCheckRoot(inv invite.Invite, root relayclient.WellKnownRoot) error {
	relayRaw, err := base64.StdEncoding.DecodeString(root.RootPubKey)
	if err != nil {
		return fmt.Errorf("%s", errEnrollRelayRootDecode)
	}
	relayPub, err := registry.NewPublicKey(relayRaw)
	if err != nil {
		return fmt.Errorf("%s", errEnrollRelayRootDecode)
	}
	if registry.NetworkID(relayPub.Bytes()) != root.NetworkID {
		return fmt.Errorf("%s", errEnrollRelaySelfInconsistent)
	}

	inviteRaw, err := base64.StdEncoding.DecodeString(inv.RootPubKey)
	if err != nil {
		return fmt.Errorf("%s", errEnrollInviteRootDecode)
	}
	if !bytes.Equal(relayRaw, inviteRaw) {
		return fmt.Errorf("%s", errEnrollRootMismatch)
	}
	if root.NetworkID != inv.NetworkID {
		return fmt.Errorf("%s", errEnrollNetworkIDMismatch)
	}
	return nil
}

// confirmFingerprint prints the OOB-confirm step. With --yes it auto-confirms;
// otherwise it prompts on errOut and reads a line from in, accepting y/Y/yes
// (trim+lower). Anything else aborts.
func confirmFingerprint(errOut io.Writer, in io.Reader, fp string, assumeYes bool) error {
	if assumeYes {
		return nil
	}
	if _, err := fmt.Fprintf(errOut, "%s%s\n%s", EnrollFingerprintLabel, fp, EnrollConfirmPrompt); err != nil {
		return err
	}
	line, _ := readLine(in)
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return nil
	default:
		return fmt.Errorf("%s", errEnrollAborted)
	}
}

// readLine reads a single line (up to the first '\n') from in.
func readLine(in io.Reader) (string, error) {
	var sb strings.Builder
	buf := make([]byte, 1)
	for {
		n, err := in.Read(buf)
		if n > 0 {
			if buf[0] == '\n' {
				return sb.String(), nil
			}
			sb.WriteByte(buf[0])
		}
		if err != nil {
			return sb.String(), err
		}
	}
}

// assembleFields builds the enrollment.Fields from cfg + the decoded invite.
// transport_endpoint is nil (OMITTED) when --transport-endpoint is empty
// (absent != null).
func assembleFields(cfg EnrollConfig, inv invite.Invite) (enrollment.Fields, error) {
	nonce := make([]byte, enrollNonceLen)
	if _, err := io.ReadFull(cfg.rand(), nonce); err != nil {
		return enrollment.Fields{}, fmt.Errorf("%s: %w", errEnrollNonce, err)
	}
	var endpoint *string
	if cfg.TransportEndpoint != "" {
		endpoint = &cfg.TransportEndpoint
	}
	return enrollment.Fields{
		AlgVersion:        enrollment.AlgVersion,
		Created:           cfg.now().Unix(),
		DisplayName:       cfg.DisplayName,
		KeyEpoch:          enrollFirstKeyEpoch,
		NetworkID:         inv.NetworkID,
		Nonce:             base64.StdEncoding.EncodeToString(nonce),
		PubKey:            cfg.PubKeyB64,
		Slug:              cfg.Slug,
		TransportEndpoint: endpoint,
		TransportPubKey:   cfg.TransportPubKeyB64,
	}, nil
}

// selfVerify decodes the returned sig and re-verifies it against the request's
// OWN pubkey (proof-of-possession). A decode failure or a non-verifying sig is a
// LOUD error: the signer holds a different key than --pubkey.
func selfVerify(fields enrollment.Fields, sigB64 string) error {
	raw, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return fmt.Errorf("%s: %w", errEnrollSelfVerifyDecode, err)
	}
	ok, err := enrollment.VerifyEnrollment(fields, raw)
	if err != nil || !ok {
		return fmt.Errorf("%s", ErrEnrollSelfVerify)
	}
	return nil
}

// writeEnrollGuidance prints the human-readable summary to errOut: network,
// relay, fingerprint (+ how it was confirmed), the signed/self-verified note,
// and the next step.
func writeEnrollGuidance(errOut io.Writer, inv invite.Invite, fp string, assumeYes bool) error {
	confirm := EnrollConfirmedNote
	if assumeYes {
		confirm = EnrollAutoConfirmedNote
	}
	_, err := fmt.Fprintf(errOut, "%s%s\n%s%s\n%s%s (%s)\n%s\n%s\n",
		EnrollNetworkLabel, inv.NetworkID,
		EnrollRelayLabel, inv.Relay,
		EnrollFingerprintLabel, fp, confirm,
		EnrollSignedNote,
		EnrollNextStepNote,
	)
	return err
}
