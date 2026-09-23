package cmd

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/identity/anchor"
	"github.com/peiman/vaultmind/internal/identity/registry"
)

// rootPinInvalidSuffix completes "<source> is not a valid …" for every pin source.
const rootPinInvalidSuffix = " is not a valid base64 ed25519 public key"

// rootPin is the network root this machine trusts.
type rootPin struct {
	pub       ed25519.PublicKey // nil when no source yields a usable root
	networkID string
	// declared: some source claims a pin, even one that did not decode. It is
	// a mesh signal on its own — the machine is configured for a network.
	declared bool
}

// resolveRootPin is the ONE answer to "which root does this machine trust?",
// shared by doctor and fetch-registry. Two resolvers gave two answers on the
// same machine (#151): fetch-registry verified the hub's registry against the
// config pin while doctor, reading only the enroll anchor, called that same
// registry NOT authenticated.
//
// Order: explicit (a command's own flag, named by explicitName for errors),
// then the config pin, then the first enroll anchor. A malformed explicit or
// config value is an error naming its source — an operator typed it, and
// falling through to a weaker source would hide the typo. A malformed anchor
// is declared-but-unusable, as doctor has always treated it.
func resolveRootPin(explicit, explicitName string) (rootPin, error) {
	for _, src := range []struct{ value, name string }{
		{explicit, explicitName},
		{getKeyValue[string](config.KeyAppIdentityfetchregistryRootPubkey), config.KeyAppIdentityfetchregistryRootPubkey},
	} {
		if src.value == "" {
			continue
		}
		pub, ok := decodeRootPin(src.value)
		if !ok {
			return rootPin{}, fmt.Errorf("%s%s", src.name, rootPinInvalidSuffix)
		}
		return rootPin{pub: pub, networkID: registry.NetworkID(pub), declared: true}, nil
	}

	anchorPath, err := defaultNetworkAnchorPath()
	if err != nil {
		return rootPin{}, nil //nolint:nilerr // anchor path unresolved ⇒ no pin, not fatal
	}
	anchors, err := anchor.Load(anchorPath)
	if err != nil || len(anchors) == 0 {
		return rootPin{}, nil //nolint:nilerr // missing/corrupt anchor ⇒ unpinned
	}
	pub, ok := decodeRootPin(anchors[0].RootPubKey)
	if !ok {
		return rootPin{declared: true}, nil
	}
	return rootPin{pub: pub, networkID: anchors[0].NetworkID, declared: true}, nil
}

// decodeRootPin decodes a base64 ed25519 root, rejecting invalid keys.
func decodeRootPin(b64 string) (ed25519.PublicKey, bool) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, false
	}
	pk, err := registry.NewPublicKey(raw)
	if err != nil {
		return nil, false
	}
	return pk.Bytes(), true
}
