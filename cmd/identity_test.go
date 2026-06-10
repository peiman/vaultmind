package cmd

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/identity"
	"github.com/peiman/vaultmind/internal/identity/enrollment"
	"github.com/peiman/vaultmind/internal/identity/envelope"
	"github.com/peiman/vaultmind/internal/identity/invite"
	"github.com/peiman/vaultmind/internal/identity/registry"
	"github.com/peiman/vaultmind/internal/identity/relayclient"
	"github.com/peiman/vaultmind/internal/identity/signer"
	"github.com/peiman/vaultmind/internal/identitycli"
)

// startCmdSigner boots a real signer on a short UDS allowlisting the test uid
// and returns the socket path plus the public key for verification. It is the
// fixture the cmd-level sign tests use to drive `identity sign` end to end.
func startCmdSigner(t *testing.T) (sockPath string, pub ed25519.PublicKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	// Short dirs: a t.TempDir() under /var/folders can exceed the ~104-char UDS
	// path limit on darwin.
	keyDir, err := os.MkdirTemp("", "vmck")
	if err != nil {
		t.Fatalf("MkdirTemp key: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(keyDir) })
	keyPath := filepath.Join(keyDir, "k.key")
	if err := signer.SealPrivateKey(keyPath, priv); err != nil {
		t.Fatalf("SealPrivateKey: %v", err)
	}
	sockDir, err := os.MkdirTemp("", "vmcs")
	if err != nil {
		t.Fatalf("MkdirTemp sock: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sockDir) })
	sockPath = filepath.Join(sockDir, "s.sock")
	s, err := signer.New(signer.Config{
		KeyPath:     keyPath,
		SocketPath:  sockPath,
		AllowedUIDs: []uint32{uint32(os.Getuid())},
	})
	if err != nil {
		t.Fatalf("signer.New: %v", err)
	}
	if err := s.Listen(); err != nil {
		t.Fatalf("Listen: %v", err)
	}
	go func() { _ = s.Serve() }()
	t.Cleanup(func() { _ = s.Close() })
	return sockPath, pub
}

// TestIdentityInitCommandWiring drives `identity init` through RootCmd and
// asserts the thin command seals a 0600 key and prints only the public key.
func TestIdentityInitCommandWiring(t *testing.T) {
	keyDir, err := os.MkdirTemp("", "vmcmdinit")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(keyDir) })
	keyPath := filepath.Join(keyDir, "k.key")

	out, _, err := runRootCmd(t, "identity", "init", "--signer-key", keyPath)
	if err != nil {
		t.Fatalf("identity init: %v", err)
	}
	if !strings.Contains(out.String(), identitycli.PubKeyLabel) {
		t.Fatalf("output missing public-key label: %q", out.String())
	}

	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("Stat key: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("key perm = %o, want 0600", perm)
	}
	keyBytes, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("ReadFile key: %v", err)
	}
	if len(keyBytes) != ed25519.PrivateKeySize {
		t.Fatalf("sealed key len = %d, want %d", len(keyBytes), ed25519.PrivateKeySize)
	}
}

// TestIdentitySignCommandFailsClosedWhenSignerUnreachable drives `identity sign`
// through RootCmd pointed at a non-existent socket and asserts it fails closed.
func TestIdentitySignCommandFailsClosedWhenSignerUnreachable(t *testing.T) {
	dir, err := os.MkdirTemp("", "vmcmdsign")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	entryPath := filepath.Join(dir, "entry.json")
	if err := os.WriteFile(entryPath, []byte(`{"agent":"mira","epoch":1}`), 0o600); err != nil {
		t.Fatalf("write entry: %v", err)
	}
	sockPath := filepath.Join(dir, "nope.sock")

	_, _, err = runRootCmd(t, "identity", "sign", "--file", entryPath, "--signer-socket", sockPath)
	if err == nil {
		t.Fatal("expected fail-closed error when signer unreachable, got nil")
	}
}

// TestIdentitySignCommandViaStdin drives `identity sign` reading the entry from
// STDIN (the default agent flow — no --file) against a live signer and asserts a
// VERIFYING signature is printed. This covers readIdentityEntryJSON's stdin
// branch, which only --file tests previously exercised.
func TestIdentitySignCommandViaStdin(t *testing.T) {
	sockPath, pub := startCmdSigner(t)
	const entry = `{"agent":"mira","epoch":1}`

	RootCmd.SetIn(strings.NewReader(entry))
	defer RootCmd.SetIn(os.Stdin)

	out, _, err := runRootCmd(t, "identity", "sign", "--signer-socket", sockPath)
	if err != nil {
		t.Fatalf("identity sign via stdin: %v", err)
	}

	line := strings.TrimSpace(out.String())
	if !strings.HasPrefix(line, identitycli.SigLabel) {
		t.Fatalf("output %q missing signature label %q", line, identitycli.SigLabel)
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(line, identitycli.SigLabel))
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	canonical, err := identity.Canonicalize([]byte(entry))
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}
	ok, err := identity.VerifyCanonical(pub, canonical, sig)
	if err != nil || !ok {
		t.Fatalf("stdin-signed signature did not verify: ok=%v err=%v", ok, err)
	}
}

// TestIdentitySignCommandFileNotFound asserts `identity sign --file <missing>`
// FAILS CLOSED with a clear error and prints no signature, rather than silently
// falling back to stdin or emitting an empty result.
func TestIdentitySignCommandFileNotFound(t *testing.T) {
	dir, err := os.MkdirTemp("", "vmcmdsignnf")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	missing := filepath.Join(dir, "does-not-exist.json")

	// No --signer-socket: this also exercises runIdentitySign's default-socket
	// resolution branch (socket is resolved before the entry is read).
	out, _, err := runRootCmd(t, "identity", "sign", "--file", missing)
	if err == nil {
		t.Fatal("expected error for nonexistent --file, got nil")
	}
	if strings.Contains(out.String(), identitycli.SigLabel) {
		t.Fatalf("file-not-found path must not print a signature, got %q", out.String())
	}
}

// TestIdentitySignEnvelopeCommandViaStdin drives `identity sign-envelope` reading
// the envelope from STDIN against a live signer, then proves the emitted sig
// verifies under a registry binding via VerifyEnvelope (the daemon's check).
func TestIdentitySignEnvelopeCommandViaStdin(t *testing.T) {
	sockPath, pub := startCmdSigner(t)
	const env = `{"alg_version":1,"body":"hello","from_agent":"mira","key_epoch":1,"nonce":"YWJjZGVmZ2hpamtsbW5vcA==","room":"dev","seq":7,"ts":2000000}`

	RootCmd.SetIn(strings.NewReader(env))
	defer RootCmd.SetIn(os.Stdin)

	out, _, err := runRootCmd(t, "identity", "sign-envelope",
		"--signer-socket", sockPath, "--from-pubkey", base64.StdEncoding.EncodeToString(pub))
	if err != nil {
		t.Fatalf("identity sign-envelope via stdin: %v", err)
	}

	var res map[string]any
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("output is not JSON: %v (%q)", err, out.String())
	}
	sig, err := base64.StdEncoding.DecodeString(res[envelope.FieldSig].(string))
	if err != nil {
		t.Fatalf("decode sig: %v", err)
	}

	pk, err := registry.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("NewPublicKey: %v", err)
	}
	reg := registry.Registry{
		Epoch: 1, ValidFrom: 0, ValidUntil: 1 << 40,
		Agents: []registry.AgentBinding{{
			Slug: "mira", DisplayName: "Mira", PubKey: pk, KeyEpoch: 1,
			ValidFrom: 0, ValidUntil: 1 << 40,
		}},
	}
	room := "dev"
	ok, verr := envelope.VerifyEnvelope(reg, envelope.Fields{
		AlgVersion: 1, Body: "hello", FromAgent: "mira", KeyEpoch: 1,
		Nonce: "YWJjZGVmZ2hpamtsbW5vcA==", Room: &room, Seq: 7, TS: 2000000,
	}, sig, time.Unix(2000000, 0))
	if verr != nil || !ok {
		t.Fatalf("cmd-signed envelope did not verify: ok=%v err=%v", ok, verr)
	}
}

// TestIdentitySignEnvelopeCommandFailsClosedWhenSignerUnreachable drives the
// command at a non-existent socket and asserts it fails closed (no output).
func TestIdentitySignEnvelopeCommandFailsClosedWhenSignerUnreachable(t *testing.T) {
	dir, err := os.MkdirTemp("", "vmcmdsignenv")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	envPath := filepath.Join(dir, "env.json")
	const env = `{"alg_version":1,"body":"hello","from_agent":"mira","key_epoch":1,"nonce":"abc","room":"dev","seq":1,"ts":1}`
	if err := os.WriteFile(envPath, []byte(env), 0o600); err != nil {
		t.Fatalf("write envelope: %v", err)
	}
	sockPath := filepath.Join(dir, "nope.sock")

	out, _, err := runRootCmd(t, "identity", "sign-envelope", "--file", envPath, "--signer-socket", sockPath)
	if err == nil {
		t.Fatal("expected fail-closed error when signer unreachable, got nil")
	}
	if strings.Contains(out.String(), envelope.FieldSig) {
		t.Fatalf("fail-closed path must not print a result, got %q", out.String())
	}
}

// TestIdentitySignEnvelopeCommandFileNotFound asserts the --file branch fails
// closed (and exercises runIdentitySignEnvelope's default-socket resolution,
// since no --signer-socket is passed) when the envelope file is missing.
func TestIdentitySignEnvelopeCommandFileNotFound(t *testing.T) {
	dir, err := os.MkdirTemp("", "vmcmdsignenvnf")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	missing := filepath.Join(dir, "does-not-exist.json")

	out, _, err := runRootCmd(t, "identity", "sign-envelope", "--file", missing)
	if err == nil {
		t.Fatal("expected error for nonexistent --file, got nil")
	}
	if strings.Contains(out.String(), envelope.FieldSig) {
		t.Fatalf("file-not-found path must not print a result, got %q", out.String())
	}
}

// signRegistryCmdInput builds a valid unsigned-registry JSON for the cmd tests,
// with the given agent pubkey (base64-std) bound under slug "mira".
func signRegistryCmdInput(pubB64 string) string {
	return `{"agents":[{"authorized_origin_daemons":["daemon-eu-1"],"display_name":"Mira",` +
		`"key_epoch":1,"pubkey":"` + pubB64 + `","slug":"mira",` +
		`"valid_from":1770000000,"valid_until":1780000000}],` +
		`"epoch":5,"valid_from":1770000000,"valid_until":1780000000}`
}

// TestIdentitySignRegistryCommandViaStdin drives `identity sign-registry` reading
// the registry from STDIN against a live signer, then proves the emitted
// distribution envelope verifies under the root key via ParseDistribution +
// VerifyAndLoad (the consumer's trust gate).
func TestIdentitySignRegistryCommandViaStdin(t *testing.T) {
	sockPath, rootPub := startCmdSigner(t)
	agentPub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey agent: %v", err)
	}
	in := signRegistryCmdInput(base64.StdEncoding.EncodeToString(agentPub))

	RootCmd.SetIn(strings.NewReader(in))
	defer RootCmd.SetIn(os.Stdin)

	out, _, err := runRootCmd(t, "identity", "sign-registry", "--signer-socket", sockPath)
	if err != nil {
		t.Fatalf("identity sign-registry via stdin: %v", err)
	}

	env, err := registry.ParseDistribution(out.Bytes())
	if err != nil {
		t.Fatalf("ParseDistribution(output): %v\noutput=%q", err, out.String())
	}
	loaded, _, err := registry.VerifyAndLoad(
		rootPub, env, 4, time.Unix(1_770_000_500, 0), time.Hour*24*365)
	if err != nil {
		t.Fatalf("VerifyAndLoad: %v", err)
	}
	if len(loaded.Agents) != 1 || loaded.Agents[0].Slug != "mira" {
		t.Fatalf("loaded registry mismatch: %+v", loaded.Agents)
	}
}

// TestIdentitySignRegistryCommandFailsClosedWhenSignerUnreachable drives the
// command at a non-existent socket and asserts it fails closed (no output).
func TestIdentitySignRegistryCommandFailsClosedWhenSignerUnreachable(t *testing.T) {
	dir, err := os.MkdirTemp("", "vmcmdsignreg")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	agentPub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey agent: %v", err)
	}
	regPath := filepath.Join(dir, "reg.json")
	if err := os.WriteFile(regPath, []byte(signRegistryCmdInput(base64.StdEncoding.EncodeToString(agentPub))), 0o600); err != nil {
		t.Fatalf("write registry: %v", err)
	}
	sockPath := filepath.Join(dir, "nope.sock")

	out, _, err := runRootCmd(t, "identity", "sign-registry", "--file", regPath, "--signer-socket", sockPath)
	if err == nil {
		t.Fatal("expected fail-closed error when signer unreachable, got nil")
	}
	if strings.Contains(out.String(), "root_sig") {
		t.Fatalf("fail-closed path must not print a result, got %q", out.String())
	}
}

// TestIdentitySignRegistryCommandFileNotFound asserts the --file branch fails
// closed (and exercises runIdentitySignRegistry's default-socket resolution,
// since no --signer-socket is passed) when the registry file is missing.
func TestIdentitySignRegistryCommandFileNotFound(t *testing.T) {
	dir, err := os.MkdirTemp("", "vmcmdsignregnf")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	missing := filepath.Join(dir, "does-not-exist.json")

	out, _, err := runRootCmd(t, "identity", "sign-registry", "--file", missing)
	if err == nil {
		t.Fatal("expected error for nonexistent --file, got nil")
	}
	if strings.Contains(out.String(), "root_sig") {
		t.Fatalf("file-not-found path must not print a result, got %q", out.String())
	}
}

// TestDefaultSignerPathsResolveUnderXDG asserts the XDG path helpers return
// non-empty, distinct paths ending in the SSOT filenames. This covers the
// happy path of defaultSignerKeyPath / defaultSignerSocketPath.
func TestDefaultSignerPathsResolveUnderXDG(t *testing.T) {
	keyPath, err := defaultSignerKeyPath()
	if err != nil {
		t.Fatalf("defaultSignerKeyPath: %v", err)
	}
	if !strings.HasSuffix(keyPath, signerKeyFilename) {
		t.Fatalf("key path %q does not end in %q", keyPath, signerKeyFilename)
	}

	sockPath, err := defaultSignerSocketPath()
	if err != nil {
		t.Fatalf("defaultSignerSocketPath: %v", err)
	}
	if !strings.HasSuffix(sockPath, signerSocketFilename) {
		t.Fatalf("socket path %q does not end in %q", sockPath, signerSocketFilename)
	}

	if keyPath == sockPath {
		t.Fatal("key and socket default paths must differ")
	}
}

// TestIdentityInviteCommandRoundTrips drives `identity invite` through RootCmd
// and asserts it prints the token/url/fingerprint blocks and that the printed
// token Decodes back to the same root key, relay, and network id.
func TestIdentityInviteCommandRoundTrips(t *testing.T) {
	rootPub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	rootB64 := base64.StdEncoding.EncodeToString(rootPub)
	const relay = "https://chat.acme.com"

	out, _, err := runRootCmd(t, "identity", "invite", "--root-pubkey", rootB64, "--relay", relay)
	if err != nil {
		t.Fatalf("identity invite: %v", err)
	}
	got := out.String()
	for _, label := range []string{identitycli.InviteTokenLabel, identitycli.InviteURLLabel, identitycli.InviteFingerprintLabel} {
		if !strings.Contains(got, label) {
			t.Fatalf("output missing label %q:\n%s", label, got)
		}
	}

	token := ""
	for _, line := range strings.Split(got, "\n") {
		if i := strings.Index(line, identitycli.InviteTokenLabel); i >= 0 {
			token = strings.TrimSpace(line[i+len(identitycli.InviteTokenLabel):])
		}
	}
	dec, err := invite.Decode(token)
	if err != nil {
		t.Fatalf("Decode(printed token): %v\noutput=%s", err, got)
	}
	if dec.RootPubKey != rootB64 || dec.Relay != relay || dec.NetworkID != registry.NetworkID(rootPub) {
		t.Fatalf("decoded invite mismatch: %+v", dec)
	}
}

// TestIdentityInviteCommandFailsClosed asserts the command rejects a missing or
// invalid --root-pubkey and an empty --relay, printing no partial output.
func TestIdentityInviteCommandFailsClosed(t *testing.T) {
	rootPub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	validB64 := base64.StdEncoding.EncodeToString(rootPub)

	cases := map[string][]string{
		"missing root-pubkey": {"identity", "invite", "--relay", "https://r"},
		"invalid root-pubkey": {"identity", "invite", "--root-pubkey", "!!!", "--relay", "https://r"},
		"empty relay":         {"identity", "invite", "--root-pubkey", validB64, "--relay", ""},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			out, _, err := runRootCmd(t, args...)
			if err == nil {
				t.Fatalf("%s: expected fail-closed error, got nil", name)
			}
			if strings.Contains(out.String(), identitycli.InviteTokenLabel) {
				t.Fatalf("%s: fail-closed path must not print a token, got %q", name, out.String())
			}
		})
	}
}

// TestIdentityInviteCommandHelp is a registration smoke test: `identity invite
// --help` resolves the command and prints its required flags.
func TestIdentityInviteCommandHelp(t *testing.T) {
	out, _, err := runRootCmd(t, "identity", "invite", "--help")
	if err != nil {
		t.Fatalf("identity invite --help: %v", err)
	}
	help := out.String()
	for _, want := range []string{"--root-pubkey", "--relay"} {
		if !strings.Contains(help, want) {
			t.Fatalf("help missing flag %q:\n%s", want, help)
		}
	}
}

// TestIdentityEnrollCommandHelp is a registration smoke test: `identity enroll
// --help` resolves the command and prints all its flags.
func TestIdentityEnrollCommandHelp(t *testing.T) {
	out, _, err := runRootCmd(t, "identity", "enroll", "--help")
	if err != nil {
		t.Fatalf("identity enroll --help: %v", err)
	}
	help := out.String()
	for _, want := range []string{
		"--invite", "--display-name", "--slug", "--pubkey",
		"--transport-pubkey", "--transport-endpoint", "--signer-socket", "--yes",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("enroll help missing flag %q:\n%s", want, help)
		}
	}
}

// enrollRelayServer spins up an httptest relay serving the well-known root for
// rootPub and returns the server plus the derived base64 root and network id.
func enrollRelayServer(t *testing.T, rootPub ed25519.PublicKey) (srv *httptest.Server, rootB64, networkID string) {
	t.Helper()
	rootB64 = base64.StdEncoding.EncodeToString(rootPub)
	networkID = registry.NetworkID(rootPub)
	body, err := json.Marshal(relayclient.WellKnownRoot{RootPubKey: rootB64, NetworkID: networkID})
	if err != nil {
		t.Fatalf("marshal well-known: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc(relayclient.WellKnownRootPath, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, rootB64, networkID
}

// TestIdentityEnrollCommandHappyPath drives `identity enroll` through RootCmd
// against a LIVE signer and an httptest relay. The signer holds the member key,
// so the emitted wire self-verifies (proof-of-possession). This exercises the
// production cmd wiring (no seam injection): real signer.Client + real HTTP.
func TestIdentityEnrollCommandHappyPath(t *testing.T) {
	sockPath, memberPub := startCmdSigner(t)
	rootPub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey root: %v", err)
	}
	srv, rootB64, networkID := enrollRelayServer(t, rootPub)

	token, _, err := invite.Encode(invite.Invite{NetworkID: networkID, Relay: srv.URL, RootPubKey: rootB64})
	if err != nil {
		t.Fatalf("invite.Encode: %v", err)
	}

	out, _, err := runRootCmd(t, "identity", "enroll",
		"--invite", token,
		"--display-name", "Mira",
		"--slug", "mira",
		"--pubkey", base64.StdEncoding.EncodeToString(memberPub),
		"--transport-pubkey", base64.StdEncoding.EncodeToString(make([]byte, 32)),
		"--signer-socket", sockPath,
		"--yes",
	)
	if err != nil {
		t.Fatalf("identity enroll: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("emitted wire is not JSON: %v (%q)", err, out.String())
	}
	if got[enrollment.FieldSig] == nil {
		t.Fatalf("emitted wire missing sig: %q", out.String())
	}
	if got[enrollment.FieldNetworkID] != networkID {
		t.Fatalf("emitted network_id = %v, want %s", got[enrollment.FieldNetworkID], networkID)
	}
}

// TestIdentityEnrollCommandFailsClosedWhenSignerUnreachable drives the command
// with a valid invite + relay but a dead signer socket, and asserts it fails
// closed with no request emitted.
func TestIdentityEnrollCommandFailsClosedWhenSignerUnreachable(t *testing.T) {
	rootPub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey root: %v", err)
	}
	srv, rootB64, networkID := enrollRelayServer(t, rootPub)
	token, _, err := invite.Encode(invite.Invite{NetworkID: networkID, Relay: srv.URL, RootPubKey: rootB64})
	if err != nil {
		t.Fatalf("invite.Encode: %v", err)
	}
	memberPub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey member: %v", err)
	}
	dir, err := os.MkdirTemp("", "vmcmdenroll")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	out, _, err := runRootCmd(t, "identity", "enroll",
		"--invite", token,
		"--display-name", "Mira",
		"--slug", "mira",
		"--pubkey", base64.StdEncoding.EncodeToString(memberPub),
		"--transport-pubkey", base64.StdEncoding.EncodeToString(make([]byte, 32)),
		"--signer-socket", filepath.Join(dir, "nope.sock"),
		"--yes",
	)
	if err == nil {
		t.Fatal("expected fail-closed error when signer unreachable, got nil")
	}
	if strings.Contains(out.String(), enrollment.FieldSig) {
		t.Fatalf("fail-closed path must not emit a request, got %q", out.String())
	}
}

// TestIdentityEnrollAddCommandViaFile drives `identity enroll-add` through
// RootCmd: a member's signed enrollment request file is added to a fresh
// registry, and the emitted UNSIGNED registry carries the new binding at epoch 1.
func TestIdentityEnrollAddCommandViaFile(t *testing.T) {
	rootPub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey root: %v", err)
	}
	memberPub, memberPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey member: %v", err)
	}
	net := registry.NetworkID(rootPub)
	f := enrollment.Fields{
		AlgVersion: enrollment.AlgVersion, Created: 1_700_000_000, DisplayName: "Mira",
		KeyEpoch: 1, NetworkID: net, Nonce: "YWJjZGVm",
		PubKey:          base64.StdEncoding.EncodeToString(memberPub),
		Slug:            "mira",
		TransportPubKey: base64.StdEncoding.EncodeToString(make([]byte, 32)),
	}
	canon, err := enrollment.CanonicalizeEnrollment(f)
	if err != nil {
		t.Fatalf("CanonicalizeEnrollment: %v", err)
	}
	reqJSON, err := enrollment.MarshalWire(f, base64.StdEncoding.EncodeToString(ed25519.Sign(memberPriv, canon.Bytes())))
	if err != nil {
		t.Fatalf("MarshalWire: %v", err)
	}
	dir := t.TempDir()
	reqPath := filepath.Join(dir, "request.json")
	if err := os.WriteFile(reqPath, reqJSON, 0o600); err != nil {
		t.Fatalf("write request: %v", err)
	}

	out, _, err := runRootCmd(t, "identity", "enroll-add",
		"--request", reqPath,
		"--root-pubkey", base64.StdEncoding.EncodeToString(rootPub),
	)
	if err != nil {
		t.Fatalf("identity enroll-add: %v", err)
	}
	var emitted struct {
		Agents []struct {
			Slug string `json:"slug"`
		} `json:"agents"`
		Epoch int `json:"epoch"`
	}
	if err := json.Unmarshal(out.Bytes(), &emitted); err != nil {
		t.Fatalf("decode emitted registry: %v\nout=%q", err, out.String())
	}
	if emitted.Epoch != 1 {
		t.Fatalf("emitted epoch = %d, want 1", emitted.Epoch)
	}
	if len(emitted.Agents) != 1 || emitted.Agents[0].Slug != "mira" {
		t.Fatalf("emitted agents mismatch: %+v", emitted.Agents)
	}
}

// TestIdentityEnrollAddCommandRejectsCrossNetwork drives the command with a
// request whose network_id is not the admin network and asserts it fails closed
// (no emitted registry).
func TestIdentityEnrollAddCommandRejectsCrossNetwork(t *testing.T) {
	rootPub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey root: %v", err)
	}
	memberPub, memberPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey member: %v", err)
	}
	f := enrollment.Fields{
		AlgVersion: enrollment.AlgVersion, Created: 1_700_000_000, DisplayName: "Mira",
		KeyEpoch: 1, NetworkID: "vmnet1:deadbeefdeadbeefdeadbeefdeadbeef", Nonce: "YWJj",
		PubKey:          base64.StdEncoding.EncodeToString(memberPub),
		Slug:            "mira",
		TransportPubKey: base64.StdEncoding.EncodeToString(make([]byte, 32)),
	}
	canon, _ := enrollment.CanonicalizeEnrollment(f)
	reqJSON, _ := enrollment.MarshalWire(f, base64.StdEncoding.EncodeToString(ed25519.Sign(memberPriv, canon.Bytes())))
	dir := t.TempDir()
	reqPath := filepath.Join(dir, "request.json")
	if err := os.WriteFile(reqPath, reqJSON, 0o600); err != nil {
		t.Fatalf("write request: %v", err)
	}

	out, _, err := runRootCmd(t, "identity", "enroll-add",
		"--request", reqPath,
		"--root-pubkey", base64.StdEncoding.EncodeToString(rootPub),
	)
	if err == nil {
		t.Fatal("expected cross-network rejection, got nil")
	}
	if out.Len() != 0 {
		t.Fatalf("reject must emit nothing, got %q", out.String())
	}
}
