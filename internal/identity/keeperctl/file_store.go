package keeperctl

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/peiman/vaultmind/internal/identity"
)

type fileState struct {
	Version int                    `json:"version"`
	Grants  map[string]StoredGrant `json:"grants"`
	Revoked map[string]int64       `json:"revoked"`
}

// Keep the signed document raw until ParseGrant has enforced presence and null
// rules; decoding directly into Grant would erase that distinction.
type diskGrant struct {
	Grant   json.RawMessage `json:"grant"`
	Sig     []byte          `json:"sig"`
	Revoked *bool           `json:"revoked"`
}
type diskState struct {
	Version int                  `json:"version"`
	Grants  map[string]diskGrant `json:"grants"`
	Revoked map[string]int64     `json:"revoked"`
}

// FileGrantStore persists an atomic JSON snapshot. Use one open instance per
// path; concurrent calls on that instance are safe. Cross-process writers need
// external serialization. The containing directory must be owner-controlled.
type FileGrantStore struct {
	mu     sync.Mutex
	path   string
	now    func() time.Time
	state  fileState
	failed error
}

// OpenFileGrantStore fails closed on corrupt state, and lazily prunes expired
// grants and tombstones only after validating the entire persisted document.
func OpenFileGrantStore(path string, now func() time.Time) (*FileGrantStore, error) {
	if now == nil {
		now = time.Now
	}
	s := &FileGrantStore{path: path, now: now, state: fileState{Version: 1,
		Grants: make(map[string]StoredGrant), Revoked: make(map[string]int64)}}
	raw, err := os.ReadFile(path) //nolint:gosec // nosemgrep: go-path-traversal -- operator-configured grant store path (watchdog config), never request input
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil {
		if err := decodeState(raw, &s.state); err != nil {
			return nil, err
		}
	}
	pruneState(&s.state, now().Unix())
	if err := saveState(path, s.state); err != nil {
		return nil, err
	}
	return s, nil
}

func decodeState(raw []byte, state *fileState) error {
	if err := identity.ValidateSchema(raw); err != nil {
		return fmt.Errorf("keeperctl: corrupt grant store: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var disk diskState
	if err := dec.Decode(&disk); err != nil {
		return err
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("keeperctl: trailing grant store data")
	}
	if disk.Version != 1 || disk.Grants == nil || disk.Revoked == nil {
		return fmt.Errorf("keeperctl: invalid grant store shape")
	}
	*state = fileState{Version: disk.Version, Grants: make(map[string]StoredGrant), Revoked: disk.Revoked}
	for id, stored := range disk.Grants {
		g, err := decodeStored(id, stored, disk.Revoked)
		if err != nil {
			return err
		}
		state.Grants[id] = g
	}
	for id := range state.Revoked {
		if _, ok := state.Grants[id]; !ok {
			return fmt.Errorf("keeperctl: orphan revocation %q", id)
		}
	}
	return nil
}

func decodeStored(id string, stored diskGrant, revokedIDs map[string]int64) (StoredGrant, error) {
	g, err := ParseGrant(stored.Grant)
	if err != nil {
		return StoredGrant{}, err
	}
	if id != g.GrantID || len(stored.Sig) != ed25519.SignatureSize || stored.Revoked == nil {
		return StoredGrant{}, fmt.Errorf("keeperctl: corrupt stored grant %q", id)
	}
	expiry, revoked := revokedIDs[id]
	if *stored.Revoked != revoked || (revoked && expiry != g.ExpiresAt) {
		return StoredGrant{}, fmt.Errorf("keeperctl: inconsistent revocation %q", id)
	}
	return StoredGrant{Grant: g, Sig: stored.Sig, Revoked: revoked}, nil
}

func pruneState(state *fileState, now int64) {
	for id, stored := range state.Grants {
		if now >= stored.Grant.ExpiresAt {
			delete(state.Grants, id)
			delete(state.Revoked, id)
		}
	}
}

func copyState(state fileState) fileState {
	next := fileState{Version: state.Version, Grants: make(map[string]StoredGrant), Revoked: make(map[string]int64)}
	for id, grant := range state.Grants {
		next.Grants[id] = cloneGrant(grant)
	}
	for id, expiry := range state.Revoked {
		next.Revoked[id] = expiry
	}
	return next
}

func saveState(path string, state fileState) error {
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".keeperctl-grants-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }() // after a successful rename the temp name is gone
	if err := writeSnapshot(f, append(raw, '\n')); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path)) // nosemgrep: go-path-traversal -- directory of the operator-configured store path, opened only to fsync
	if err != nil {
		return err
	}
	defer func() { _ = dir.Close() }()
	return dir.Sync()
}

func writeSnapshot(f *os.File, raw []byte) error {
	defer func() { _ = f.Close() }() // closes on early return; the explicit Close below reports errors
	if err := f.Chmod(0600); err != nil {
		return err
	}
	if _, err := f.Write(raw); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	return f.Close()
}

func (s *FileGrantStore) Get(id string) (StoredGrant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failed != nil {
		return StoredGrant{}, s.failed
	}
	g, ok := s.state.Grants[id]
	if !ok {
		return StoredGrant{}, ErrGrantMissing
	}
	return cloneGrant(g), nil
}

func (s *FileGrantStore) publish(next fileState) error {
	if err := saveState(s.path, next); err != nil {
		// A directory-fsync failure can follow a successful rename. Refuse
		// subsequent use until reopen reconciles the durable snapshot.
		s.failed = err
		return err
	}
	s.state = next
	return nil
}

func (s *FileGrantStore) Put(g Grant, sig []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failed != nil {
		return s.failed
	}
	next := copyState(s.state)
	pruneState(&next, s.now().Unix())
	if err := putGrant(next.Grants, g, sig); err != nil {
		return err
	}
	return s.publish(next)
}

func (s *FileGrantStore) MarkRevoked(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failed != nil {
		return s.failed
	}
	next := copyState(s.state)
	stored, ok := next.Grants[id]
	if !ok {
		return ErrGrantMissing
	}
	stored.Revoked = true
	next.Grants[id], next.Revoked[id] = stored, stored.Grant.ExpiresAt
	return s.publish(next)
}
