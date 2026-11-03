package keeperctl

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrNonceSeen    = errors.New("keeperctl: nonce already seen")
	ErrGrantMissing = errors.New("keeperctl: grant missing")
	ErrGrantRevoked = errors.New("keeperctl: grant revoked")
	ErrGrantExists  = errors.New("keeperctl: grant id already installed with different contents")
)

// StoredGrant retains all signed fields and the signature for live re-verification.
type StoredGrant struct {
	Grant   Grant  `json:"grant"`
	Sig     []byte `json:"sig"`
	Revoked bool   `json:"revoked"`
}

// GrantStore operations must be atomic and return defensive copies. Put must
// refuse revoked IDs and replacement of an existing ID's signed contents.
// Get returns ErrGrantMissing when absent; all other errors also fail closed.
type GrantStore interface {
	Get(id string) (StoredGrant, error)
	Put(g Grant, sig []byte) error
	MarkRevoked(id string) error
}

// NonceStore atomically tests and records the (signer, nonce) tuple.
type NonceStore interface {
	CheckAndRecord(signer, nonce string, issuedAt int64) error
}

// MemoryNonceStore retains nonces through issuedAt + MaxAge + MaxSkew, inclusive.
// Its window must match (or exceed) the policy window. Use a fresh store only
// with a watchdog NotBefore floor at the new start time.
type MemoryNonceStore struct {
	mu              sync.Mutex
	entries         map[nonceKey]time.Time
	now             func() time.Time
	maxAge, maxSkew time.Duration
}
type nonceKey struct{ signer, nonce string }

func windows(age, skew time.Duration) (time.Duration, time.Duration) {
	if age == 0 {
		age = 300 * time.Second
	}
	if skew == 0 {
		skew = 30 * time.Second
	}
	return age, skew
}

// NewMemoryNonceStore defaults zero windows to 300s and 30s.
func NewMemoryNonceStore(now func() time.Time, maxAge, maxSkew time.Duration) *MemoryNonceStore {
	if now == nil {
		now = time.Now
	}
	maxAge, maxSkew = windows(maxAge, maxSkew)
	return &MemoryNonceStore{entries: make(map[nonceKey]time.Time), now: now, maxAge: maxAge, maxSkew: maxSkew}
}

func (s *MemoryNonceStore) CheckAndRecord(signer, nonce string, issuedAt int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.maxAge < 0 || s.maxSkew < 0 {
		return errors.New("keeperctl: negative nonce retention window")
	}
	now := s.now()
	for key, until := range s.entries {
		if now.After(until) {
			delete(s.entries, key)
		}
	}
	key := nonceKey{signer, nonce}
	if _, ok := s.entries[key]; ok {
		return ErrNonceSeen
	}
	s.entries[key] = time.Unix(issuedAt, 0).Add(s.maxAge).Add(s.maxSkew)
	return nil
}

// MemoryGrantStore is a concurrency-safe, non-pruning grant store.
type MemoryGrantStore struct {
	mu     sync.Mutex
	grants map[string]StoredGrant
}

func NewMemoryGrantStore() *MemoryGrantStore {
	return &MemoryGrantStore{grants: make(map[string]StoredGrant)}
}

func cloneGrant(s StoredGrant) StoredGrant {
	s.Sig = bytes.Clone(s.Sig)
	s.Grant.Keepers = append([]string{}, s.Grant.Keepers...)
	s.Grant.Scopes = append([]string{}, s.Grant.Scopes...)
	return s
}

func putGrant(m map[string]StoredGrant, g Grant, sig []byte) error {
	if len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("keeperctl: invalid grant signature length")
	}
	canonical, err := CanonicalizeGrant(g)
	if err != nil {
		return err
	}
	if old, ok := m[g.GrantID]; ok {
		if old.Revoked {
			return ErrGrantRevoked
		}
		previous, err := CanonicalizeGrant(old.Grant)
		if err != nil {
			return err
		}
		if !bytes.Equal(previous.Bytes(), canonical.Bytes()) || !bytes.Equal(old.Sig, sig) {
			return ErrGrantExists
		}
		return nil
	}
	m[g.GrantID] = cloneGrant(StoredGrant{Grant: g, Sig: sig})
	return nil
}

func (s *MemoryGrantStore) Get(id string) (StoredGrant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.grants[id]
	if !ok {
		return StoredGrant{}, ErrGrantMissing
	}
	return cloneGrant(g), nil
}

func (s *MemoryGrantStore) Put(g Grant, sig []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return putGrant(s.grants, g, sig)
}

func (s *MemoryGrantStore) MarkRevoked(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.grants[id]
	if !ok {
		return ErrGrantMissing
	}
	g.Revoked = true
	s.grants[id] = g
	return nil
}
