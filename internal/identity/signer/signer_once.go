package signer

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
)

// OnceMaxFailures is how many failed requests (peer uid refused, malformed
// frame, policy refusal; not a connection that sends nothing) a once-mode signer
// tolerates before it gives up with
// nothing signed. Failures never produce a signature, so they do not use up the
// one signature; the bound only stops a run that is being probed or misused from
// waiting forever.
const OnceMaxFailures = 3

const (
	// errOnceAlreadySigned refuses any signature after the first in once mode.
	errOnceAlreadySigned = "signer: once: already signed one request (refusing another)"
	// errOnceClosed refuses a signature after Close in once mode, so a run being
	// shut down cannot produce a signature it never reports.
	errOnceClosed = "signer: once: stopped (refusing to sign)"
	// errOnceTooManyFailures ends a once-mode run that saw OnceMaxFailures
	// failed requests and signed nothing.
	errOnceTooManyFailures = "signer: once: too many failed requests, nothing signed"
	// onceSummaryFormat is the line a once-mode run prints after its signature.
	onceSummaryFormat = "signed once: sha256=%s bytes=%d"
)

// OnceResult is the single signature a once-mode signer produced.
type OnceResult struct {
	// Message is exactly the bytes ed25519 signed. The signer uses pure Ed25519
	// with no prefix or domain separation, so these are also exactly the bytes
	// the client sent (for sign-envelope: the envelope's JCS canonical bytes).
	Message []byte
	// Sig is the signature over Message, as returned to the client.
	Sig []byte
}

// Summary is the line to show the operator: the lowercase-hex sha256 of the
// signed bytes and their length.
func (r *OnceResult) Summary() string {
	sum := sha256.Sum256(r.Message)
	return fmt.Sprintf(onceSummaryFormat, hex.EncodeToString(sum[:]), len(r.Message))
}

// ServeOnce serves until ONE request has been signed, then closes the listener
// (removing the socket) and returns what was signed. Failed requests do not
// count; after OnceMaxFailures of them it closes and returns an error with no
// result. A connection closed before sending any byte is a liveness probe and
// is not a failure. Requests that race the first are refused by the signing guard (see
// sign), so no second signature can be produced. A nil result with a nil error
// means the listener was closed (Close) before anything was signed.
func (s *Signer) ServeOnce() (*OnceResult, error) {
	s.mu.Lock()
	ln := s.ln
	s.mu.Unlock()
	if ln == nil {
		return nil, errors.New(errNotListening)
	}
	s.signMu.Lock()
	s.once = true
	s.signMu.Unlock()

	for failures := 0; failures < OnceMaxFailures; {
		conn, err := ln.AcceptUnix()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				// Closed (Close was called). Accepts are serial, so nothing was
				// signed; Signed reports nil.
				return s.Signed(), nil
			}
			return nil, errors.Join(fmt.Errorf("signer: accept: %w", err), s.Close())
		}
		switch s.handle(conn) {
		case outcomeSigned:
			return s.Signed(), s.Close()
		case outcomeFailed:
			failures++
		case outcomeProbe:
		}
	}
	return nil, errors.Join(fmt.Errorf("%s (%d)", errOnceTooManyFailures, OnceMaxFailures), s.Close())
}

// Signed returns the once-mode signature, or nil if none has been produced. It
// is safe to call after Close: a run stopped by a signal still reports the
// signature if one was made before it stopped.
func (s *Signer) Signed() *OnceResult {
	s.signMu.Lock()
	defer s.signMu.Unlock()
	return s.signed
}
