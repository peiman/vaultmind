package signer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/identity"
)

// onceRun is a once-mode signer serving in the background. done receives
// ServeOnce's result exactly once.
type onceRun struct {
	s        *Signer
	sockPath string
	done     chan onceOutcome
}

type onceOutcome struct {
	res *OnceResult
	err error
}

// startOnceSigner boots a Signer and runs ServeOnce in a goroutine.
func startOnceSigner(t *testing.T, keyPath string, allow []uint32) *onceRun {
	t.Helper()
	sockPath := shortSocketPath(t)
	s, err := New(Config{KeyPath: keyPath, SocketPath: sockPath, AllowedUIDs: allow})
	if err != nil {
		t.Fatalf("New signer: %v", err)
	}
	if err := s.Listen(); err != nil {
		t.Fatalf("Listen: %v", err)
	}
	r := &onceRun{s: s, sockPath: sockPath, done: make(chan onceOutcome, 1)}
	go func() {
		res, err := s.ServeOnce()
		r.done <- onceOutcome{res: res, err: err}
	}()
	t.Cleanup(func() { _ = s.Close() })
	return r
}

// wait returns ServeOnce's outcome, failing the test if it does not finish.
func (r *onceRun) wait(t *testing.T) onceOutcome {
	t.Helper()
	select {
	case o := <-r.done:
		return o
	case <-time.After(30 * time.Second):
		t.Fatal("ServeOnce did not return")
		return onceOutcome{}
	}
}

func currentUID() []uint32 { return []uint32{uint32(os.Getuid())} } //nolint:gosec // test: current uid fits uint32

func TestServeOnceSignsExactlyOnceThenReturns(t *testing.T) {
	keyPath, pub := newKeyFile(t)
	r := startOnceSigner(t, keyPath, currentUID())
	client := &Client{SocketPath: r.sockPath}

	cb := canonicalBytes(t)
	sig, err := client.Sign(cb.Bytes())
	if err != nil {
		t.Fatalf("first Sign: %v", err)
	}
	o := r.wait(t)
	if o.err != nil {
		t.Fatalf("ServeOnce err: %v", o.err)
	}
	if o.res == nil {
		t.Fatal("ServeOnce returned no result after a successful signature")
	}
	if string(o.res.Message) != string(cb.Bytes()) {
		t.Fatalf("result message = %q, want %q", o.res.Message, cb.Bytes())
	}
	if string(o.res.Sig) != string(sig) {
		t.Fatal("result signature differs from the one returned to the client")
	}
	ok, err := identity.VerifyCanonical(pub, identity.CanonicalBytesFromTrusted(o.res.Message), o.res.Sig)
	if err != nil || !ok {
		t.Fatalf("signature does not verify over the result message: ok=%v err=%v", ok, err)
	}
}

// TestServeOnceSocketGoneAfterSign checks the end-to-end outcome: after the one
// signature a later request cannot reach a signer at all. The signing guard
// itself is covered by TestSignGuardAllowsOneSignatureUnderRace.
func TestServeOnceSocketGoneAfterSign(t *testing.T) {
	keyPath, _ := newKeyFile(t)
	r := startOnceSigner(t, keyPath, currentUID())
	client := &Client{SocketPath: r.sockPath}

	if _, err := client.Sign(canonicalBytes(t).Bytes()); err != nil {
		t.Fatalf("first Sign: %v", err)
	}
	if o := r.wait(t); o.err != nil {
		t.Fatalf("ServeOnce err: %v", o.err)
	}
	if _, err := client.Sign([]byte(`{"agent":"evil","epoch":2}`)); err == nil {
		t.Fatal("second request was signed; once mode must refuse it")
	}
}

func TestServeOnceRemovesSocket(t *testing.T) {
	keyPath, _ := newKeyFile(t)
	r := startOnceSigner(t, keyPath, currentUID())
	client := &Client{SocketPath: r.sockPath}

	if _, err := client.Sign(canonicalBytes(t).Bytes()); err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if o := r.wait(t); o.err != nil {
		t.Fatalf("ServeOnce err: %v", o.err)
	}
	if _, err := os.Lstat(r.sockPath); !os.IsNotExist(err) {
		t.Fatalf("socket still present after the one signature: lstat err = %v", err)
	}
}

// TestServeOnceEndToEndConcurrentClients races real clients at the socket.
// Accepts are serial, so this proves the end-to-end outcome (one signature,
// the rest fail); the lock is covered by TestSignGuardAllowsOneSignatureUnderRace.
func TestServeOnceEndToEndConcurrentClients(t *testing.T) {
	keyPath, pub := newKeyFile(t)
	r := startOnceSigner(t, keyPath, currentUID())

	const n = 16
	var wg sync.WaitGroup
	var mu sync.Mutex
	var signed [][]byte
	var sigs [][]byte
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			msg := []byte(fmt.Sprintf(`{"agent":"a%d","epoch":1}`, i))
			c := &Client{SocketPath: r.sockPath}
			sig, err := c.Sign(msg)
			if err == nil {
				mu.Lock()
				signed = append(signed, msg)
				sigs = append(sigs, sig)
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	o := r.wait(t)
	if o.err != nil {
		t.Fatalf("ServeOnce err: %v", o.err)
	}
	if len(signed) != 1 {
		t.Fatalf("%d concurrent requests got a signature, want exactly 1", len(signed))
	}
	if string(o.res.Message) != string(signed[0]) {
		t.Fatalf("result message %q is not the request that was signed (%q)", o.res.Message, signed[0])
	}
	ok, err := identity.VerifyCanonical(pub, identity.CanonicalBytesFromTrusted(signed[0]), sigs[0])
	if err != nil || !ok {
		t.Fatalf("winning signature does not verify: ok=%v err=%v", ok, err)
	}
}

// TestSignGuardAllowsOneSignatureUnderRace drives the signing guard itself from
// many goroutines, so the lock is what is tested, not the serial accept loop.
func TestSignGuardAllowsOneSignatureUnderRace(t *testing.T) {
	keyPath, _ := newKeyFile(t)
	s, err := New(Config{KeyPath: keyPath, SocketPath: shortSocketPath(t), AllowedUIDs: currentUID()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.once = true

	const n = 32
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins, refusals := 0, 0
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := s.sign([]byte(fmt.Sprintf(`{"n":%d}`, i)))
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				wins++
			} else if strings.Contains(err.Error(), errOnceAlreadySigned) {
				refusals++
			}
		}(i)
	}
	wg.Wait()
	if wins != 1 || refusals != n-1 {
		t.Fatalf("wins=%d refusals=%d, want 1 and %d", wins, refusals, n-1)
	}
}

func TestSignGuardRefusesAfterClose(t *testing.T) {
	keyPath, _ := newKeyFile(t)
	s, err := New(Config{KeyPath: keyPath, SocketPath: shortSocketPath(t), AllowedUIDs: currentUID()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.once = true
	_ = s.Close()
	if _, err := s.sign([]byte(`{"n":1}`)); err == nil || err.Error() != errOnceClosed {
		t.Fatalf("sign after Close: err = %v, want %q", err, errOnceClosed)
	}
	if s.Signed() != nil {
		t.Fatal("Signed() reports a signature that was refused")
	}
}

func TestSignedReportsTheOneSignature(t *testing.T) {
	keyPath, _ := newKeyFile(t)
	r := startOnceSigner(t, keyPath, currentUID())
	cb := canonicalBytes(t)
	sig, err := (&Client{SocketPath: r.sockPath}).Sign(cb.Bytes())
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	r.wait(t)
	got := r.s.Signed()
	if got == nil || string(got.Message) != string(cb.Bytes()) || string(got.Sig) != string(sig) {
		t.Fatalf("Signed() = %+v, want the one signature", got)
	}
}

func TestServeOnceFailedRequestDoesNotCount(t *testing.T) {
	keyPath, pub := newKeyFile(t)
	r := startOnceSigner(t, keyPath, currentUID())

	// A malformed request: announce a frame, then hang up mid-payload.
	conn, err := net.Dial(network, r.sockPath)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	_, _ = conn.Write([]byte{0, 0, 0, 10, 'x'})
	_ = conn.Close()

	// The run must still be up and sign the next, well-formed request.
	cb := canonicalBytes(t)
	sig, err := (&Client{SocketPath: r.sockPath}).Sign(cb.Bytes())
	if err != nil {
		t.Fatalf("Sign after a failed request: %v", err)
	}
	o := r.wait(t)
	if o.err != nil || o.res == nil {
		t.Fatalf("ServeOnce: res=%v err=%v", o.res, o.err)
	}
	ok, err := identity.VerifyCanonical(pub, cb, sig)
	if err != nil || !ok {
		t.Fatalf("signature does not verify: ok=%v err=%v", ok, err)
	}
}

// TestServeOnceLivenessProbesDoNotCount: doctor, signer install and a second
// signer's stale-socket check connect and close without sending a byte. Those
// probes must not use up the failure bound.
func TestServeOnceLivenessProbesDoNotCount(t *testing.T) {
	keyPath, _ := newKeyFile(t)
	r := startOnceSigner(t, keyPath, currentUID())
	for i := 0; i < OnceMaxFailures+2; i++ {
		conn, err := net.Dial(network, r.sockPath)
		if err != nil {
			t.Fatalf("probe %d: %v", i, err)
		}
		_ = conn.Close()
	}
	if _, err := (&Client{SocketPath: r.sockPath}).Sign(canonicalBytes(t).Bytes()); err != nil {
		t.Fatalf("Sign after probes: %v", err)
	}
	if o := r.wait(t); o.err != nil || o.res == nil {
		t.Fatalf("ServeOnce: res=%v err=%v", o.res, o.err)
	}
}

// TestServeOnceHeaderOnlyCountsAsFailure: a peer that announces a frame and
// hangs up sent a malformed request, not a probe, so it counts.
func TestServeOnceHeaderOnlyCountsAsFailure(t *testing.T) {
	keyPath, _ := newKeyFile(t)
	r := startOnceSigner(t, keyPath, currentUID())
	for i := 0; i < OnceMaxFailures; i++ {
		conn, err := net.Dial(network, r.sockPath)
		if err != nil {
			t.Fatalf("Dial %d: %v", i, err)
		}
		_, _ = conn.Write([]byte{0, 0, 0, 10})
		_ = conn.(*net.UnixConn).CloseWrite()
		_, _ = io.ReadAll(conn)
		_ = conn.Close()
	}
	o := r.wait(t)
	if o.err == nil || !strings.Contains(o.err.Error(), errOnceTooManyFailures) {
		t.Fatalf("ServeOnce err = %v, want %q", o.err, errOnceTooManyFailures)
	}
}

func TestServeOnceRefusedPeerDoesNotCountAndBoundsFailures(t *testing.T) {
	keyPath, _ := newKeyFile(t)
	other := uint32(os.Getuid()) + 1 //nolint:gosec // test: current uid fits uint32
	r := startOnceSigner(t, keyPath, []uint32{other})
	client := &Client{SocketPath: r.sockPath}

	for i := 0; i < OnceMaxFailures; i++ {
		if _, err := client.Sign(canonicalBytes(t).Bytes()); err == nil {
			t.Fatalf("request %d from a non-allowlisted uid was signed", i)
		}
	}
	o := r.wait(t)
	if o.res != nil {
		t.Fatal("a result was returned although nothing was signed")
	}
	if o.err == nil || !strings.Contains(o.err.Error(), errOnceTooManyFailures) {
		t.Fatalf("ServeOnce err = %v, want %q", o.err, errOnceTooManyFailures)
	}
	if _, err := os.Lstat(r.sockPath); !os.IsNotExist(err) {
		t.Fatalf("socket still present after giving up: lstat err = %v", err)
	}
}

func TestServeOnceClosedBeforeSigningReturnsNoResult(t *testing.T) {
	keyPath, _ := newKeyFile(t)
	r := startOnceSigner(t, keyPath, currentUID())
	// Prove ServeOnce is in its accept loop (an oversize frame is answered with
	// an error at once), then stop it with nothing signed.
	conn, err := net.Dial(network, r.sockPath)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	_, _ = conn.Write([]byte{0xff, 0xff, 0xff, 0xff})
	status := make([]byte, 1)
	if _, err := conn.Read(status); err != nil || status[0] != respErr {
		t.Fatalf("oversize frame: status=%v err=%v, want an error response", status, err)
	}
	_ = conn.Close()
	_ = r.s.Close()
	o := r.wait(t)
	if o.err != nil || o.res != nil {
		t.Fatalf("closed before signing: res=%v err=%v, want nil, nil", o.res, o.err)
	}
}

func TestServeOnceRequiresListen(t *testing.T) {
	keyPath, _ := newKeyFile(t)
	s, err := New(Config{KeyPath: keyPath, SocketPath: shortSocketPath(t), AllowedUIDs: currentUID()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := s.ServeOnce(); err == nil || err.Error() != errNotListening {
		t.Fatalf("ServeOnce before Listen: err = %v, want %q", err, errNotListening)
	}
}

func TestOnceResultSummaryHashesTheSignedBytes(t *testing.T) {
	msg := []byte(`{"agent":"mira","epoch":1}`)
	sum := sha256.Sum256(msg)
	want := fmt.Sprintf("signed once: sha256=%s bytes=%d", hex.EncodeToString(sum[:]), len(msg))
	got := (&OnceResult{Message: msg}).Summary()
	if got != want {
		t.Fatalf("Summary() = %q, want %q", got, want)
	}
}
