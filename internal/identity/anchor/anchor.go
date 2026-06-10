// Package anchor is the SSOT for the persisted Contract-B trust anchor: the
// out-of-band-confirmed {network_id, root_pubkey} pair that `identity enroll`
// pins on first enrollment and that a later `doctor` slice authenticates registry
// verification against. Persisting the anchor IS pinning the root, because
// network_id = vmnet1:hex(SHA-256(root_pubkey)[:16]) — the 128-bit fingerprint
// the member already confirmed OOB at enroll.
//
// Every stored anchor is INTERNALLY CONSISTENT: Upsert refuses any anchor whose
// network_id does not derive from its own root_pubkey (a stored anchor that lies
// about which root it pins is corrupt or hostile). Writes are ATOMIC (temp file
// in the same dir + chmod 0600 + rename) so a crash never leaves a half-written
// or corrupt anchor on disk.
package anchor

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/peiman/vaultmind/internal/identity/registry"
)

// Persisted-file shape + permission constants (SSOT — referenced, never inlined).
const (
	// anchorFilePerm is the mode the persisted anchor file is chmod'd to: owner
	// read/write only. The anchor is a trust-relevant artifact, so it is no more
	// world-readable than the identity key beside it.
	anchorFilePerm = 0600
	// anchorDirPerm is the mode an absent parent dir is created with: owner-only.
	anchorDirPerm = 0700
	// anchorTempPattern is the temp-file name pattern used for the atomic write.
	// The temp lives in the SAME dir as the target so os.Rename is atomic (a
	// cross-device rename is not).
	anchorTempPattern = "network-roots-*.json.tmp"
)

// Validation / decode error strings (SSOT). Each rejection is a distinct typed
// error so a caller (and a test) can tell WHY an anchor was refused.
const (
	// ErrEmptyNetworkID is returned by Upsert when the anchor's NetworkID is empty.
	ErrEmptyNetworkID = "anchor: network_id is empty"
	// ErrBadNetworkIDPrefix is returned when the NetworkID lacks the vmnet1: prefix.
	ErrBadNetworkIDPrefix = "anchor: network_id must carry the vmnet1: prefix"
	// ErrRootPubKeyDecode is returned when RootPubKey is not base64-std of a valid
	// ed25519 public key (bad base64 / wrong length / small-order reject).
	ErrRootPubKeyDecode = "anchor: root_pubkey is not a valid base64-std ed25519 public key"
	// ErrNetworkIDMismatch is returned when NetworkID != NetworkID(root_pubkey) —
	// the anchor is internally inconsistent (corrupt or hostile): its network_id
	// does not derive from the root it claims to pin.
	ErrNetworkIDMismatch = "anchor: network_id does not derive from root_pubkey — anchor is internally inconsistent"
	// errDecodeStore wraps a malformed on-disk anchor store.
	errDecodeStore = "anchor: decode anchor store"
	// errWriteTemp wraps a temp-file write failure during the atomic write.
	errWriteTemp = "anchor: write temp anchor file"
	// errRenameTemp wraps the temp→final rename failure during the atomic write.
	errRenameTemp = "anchor: rename temp anchor file into place"
	// errMkdirParent wraps a parent-dir creation failure.
	errMkdirParent = "anchor: create anchor parent directory"
	// errMarshalStore wraps a marshal failure of the anchor store (should not
	// happen for a struct of validated strings).
	errMarshalStore = "anchor: marshal anchor store"
)

// NetworkAnchor is one persisted trust anchor: the OOB-confirmed root of a single
// network. RootPubKey is base64-std (padded) of the 32-byte ed25519 ROOT public
// key — the same encoding the invite carries — and NetworkID is the vmnet1: id
// that derives from it.
type NetworkAnchor struct {
	NetworkID   string `json:"network_id"`
	RootPubKey  string `json:"root_pubkey"`
	ConfirmedAt int64  `json:"confirmed_at"`
	Relay       string `json:"relay,omitempty"`
}

// store is the on-disk envelope: a top-level "networks" array, multi-network
// forward-compatible so a member can be enrolled in more than one network.
type store struct {
	Networks []NetworkAnchor `json:"networks"`
}

// Load reads the anchor store at path and returns its anchors. A MISSING file
// returns (nil, nil) — a not-yet-enrolled member legitimately has no anchor, so
// absence is NOT an error (and nilnil does not fire: the return is a slice, not a
// pointer-error pair). A present-but-malformed file IS an error: the store is
// strictly decoded (unknown fields and trailing bytes are rejected) because a
// corrupt trust anchor must fail loud, never be silently tolerated.
func Load(path string) ([]NetworkAnchor, error) {
	// path is cmd-resolved via xdg.DataFile (defaultNetworkAnchorPath) or
	// test-supplied — never external/attacker-controlled input.
	// #nosec G304
	// nosemgrep: go-path-traversal
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	s, err := decodeStore(raw)
	if err != nil {
		return nil, err
	}
	return s.Networks, nil
}

// Upsert validates a, then replaces the existing anchor whose NetworkID matches
// (or appends a new one) and writes the store back ATOMICALLY. The anchor is
// rejected unless it is internally consistent — vmnet1: prefix, a valid
// (small-order-rejected) 32-byte ed25519 root pubkey, AND
// NetworkID == NetworkID(root_pubkey) — so a corrupt or hostile anchor never
// reaches disk. An absent parent dir is created (0700).
func Upsert(path string, a NetworkAnchor) error {
	if err := validate(a); err != nil {
		return err
	}
	existing, err := Load(path)
	if err != nil {
		return err
	}
	existing = upsertInto(existing, a)
	return writeStoreAtomic(path, store{Networks: existing})
}

// Find returns the anchor whose NetworkID equals networkID, and a found flag.
// It is the lookup helper a later doctor slice uses to resolve the pinned root
// for a given network.
func Find(anchors []NetworkAnchor, networkID string) (NetworkAnchor, bool) {
	for _, a := range anchors {
		if a.NetworkID == networkID {
			return a, true
		}
	}
	return NetworkAnchor{}, false
}

// upsertInto replaces the matching-NetworkID entry in anchors (preserving order)
// or appends a when none matches. It returns the updated slice.
func upsertInto(anchors []NetworkAnchor, a NetworkAnchor) []NetworkAnchor {
	for i := range anchors {
		if anchors[i].NetworkID == a.NetworkID {
			anchors[i] = a
			return anchors
		}
	}
	return append(anchors, a)
}

// validate enforces the anchor's internal consistency: non-empty vmnet1: id, a
// valid (small-order-rejected) 32-byte ed25519 root pubkey, and the binding
// NetworkID == NetworkID(root_pubkey). A stored anchor whose network_id does not
// derive from its root pins nothing — reject it.
func validate(a NetworkAnchor) error {
	if a.NetworkID == "" {
		return fmt.Errorf("%s", ErrEmptyNetworkID)
	}
	if !hasGlobalPrefix(a.NetworkID) {
		return fmt.Errorf("%s", ErrBadNetworkIDPrefix)
	}
	raw, err := base64.StdEncoding.DecodeString(a.RootPubKey)
	if err != nil {
		return fmt.Errorf("%s", ErrRootPubKeyDecode)
	}
	pub, err := registry.NewPublicKey(raw)
	if err != nil {
		return fmt.Errorf("%s", ErrRootPubKeyDecode)
	}
	if registry.NetworkID(pub.Bytes()) != a.NetworkID {
		return fmt.Errorf("%s", ErrNetworkIDMismatch)
	}
	return nil
}

// hasGlobalPrefix reports whether id carries the reserved vmnet1: global prefix.
func hasGlobalPrefix(id string) bool {
	return len(id) >= len(registry.GlobalPrefix) && id[:len(registry.GlobalPrefix)] == registry.GlobalPrefix
}

// decodeStore strictly decodes raw into a store: unknown fields are rejected and
// trailing bytes after the first JSON value are rejected, so a corrupt or
// tampered store fails loud instead of being partially accepted.
func decodeStore(raw []byte) (store, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var s store
	if err := dec.Decode(&s); err != nil {
		return store{}, fmt.Errorf("%s: %w", errDecodeStore, err)
	}
	if dec.More() {
		return store{}, fmt.Errorf("%s: trailing data after JSON value", errDecodeStore)
	}
	return s, nil
}

// writeStoreAtomic marshals s and writes it to path atomically: it creates an
// absent parent dir (0700), writes a temp file in the SAME dir, chmods it 0600,
// and renames it into place. A crash mid-write leaves either the old file or no
// new file — never a half-written anchor.
func writeStoreAtomic(path string, s store) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("%s: %w", errMarshalStore, err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, anchorDirPerm); err != nil {
		return fmt.Errorf("%s: %w", errMkdirParent, err)
	}
	tmp, err := os.CreateTemp(dir, anchorTempPattern)
	if err != nil {
		return fmt.Errorf("%s: %w", errWriteTemp, err)
	}
	tmpPath := tmp.Name()
	// Best-effort cleanup if we bail before the rename succeeds; after a
	// successful rename the temp name no longer exists, so the remove is a no-op.
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("%s: %w", errWriteTemp, err)
	}
	if err := tmp.Chmod(anchorFilePerm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("%s: %w", errWriteTemp, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("%s: %w", errWriteTemp, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("%s: %w", errRenameTemp, err)
	}
	return nil
}
