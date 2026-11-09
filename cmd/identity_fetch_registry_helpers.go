package cmd

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/identity/anchor"
	"github.com/peiman/vaultmind/internal/identity/registry"
	"github.com/peiman/vaultmind/internal/identity/registryfetch"
	"github.com/peiman/vaultmind/internal/identity/relayclient"
	"github.com/spf13/cobra"
)

// fetchRegistryReport is what one fetch-registry run decided, for the text or
// JSON writer.
type fetchRegistryReport struct {
	pin          rootPin
	hubRoot      string // the root the hub advertises; fetched only on a refusal
	res          registryfetch.Result
	path         string
	maxStaleness int64
	now          time.Time
}

// fetchRegistry resolves hub and path BEFORE touching the network, so a
// misconfigured machine fails without fetching anything.
func fetchRegistry(cmd *cobra.Command, rep *fetchRegistryReport) error {
	agents, project := registryPath(), projectPath()
	hub := getConfigValueWithFlags[string](cmd, "hub", config.KeyAppIdentityfetchregistryHub)
	if hub == "" {
		hub = resolveDoctorDaemonURL(os.Getenv(envDaemonURL), agents, project)
	}
	if hub == "" {
		return errors.New(fetchRegistryErrNoHub)
	}
	rep.path = getConfigValueWithFlags[string](cmd, "registry-file", config.KeyAppIdentityfetchregistryRegistryFile)
	if rep.path == "" {
		rep.path = registryFileFromAgentsYAML(agents, project)
	}
	if rep.path == "" {
		return errors.New(fetchRegistryErrNoPath)
	}

	body, err := relayclient.FetchDirectory(cmd.Context(), nil, hub)
	if err != nil {
		return err
	}
	rep.res, err = registryfetch.Update(rep.pin.pub, body, rep.path, rep.now)
	if errors.Is(err, registryfetch.ErrNotVerified) {
		return explainUnverified(cmd.Context(), err, rep, hub)
	}
	if err != nil {
		return err
	}
	rep.maxStaleness = registryMaxStalenessFromAgentsYAML(agents, project)
	return nil
}

// explainUnverified adds to a refusal the pinned root, where it came from, and
// the root the hub advertises (one bounded GET, only on this failure path).
// Diagnosis only: the hub's root is never trusted — the pin exists precisely
// so a hub cannot swap roots.
func explainUnverified(ctx context.Context, err error, rep *fetchRegistryReport, hub string) error {
	lines := []string{"  " + rep.pin.line()}
	where := strings.TrimRight(hub, "/") + relayclient.WellKnownRootPath
	adv, ferr := relayclient.FetchRoot(ctx, nil, hub)
	advPub, ok := decodeRootPin(adv.RootPubKey)
	switch {
	case ferr != nil:
		lines = append(lines, "  hub root: unavailable ("+ferr.Error()+")")
	case !ok:
		lines = append(lines, "  hub root: unavailable ("+where+" holds no valid root key)")
	case bytes.Equal(advPub, rep.pin.pub):
		rep.hubRoot = adv.RootPubKey
		lines = append(lines, "  hub root: "+shortRoot(adv.RootPubKey)+" (advertised at "+where+")",
			"  the hub advertises the pinned root, so the root did not change: the registry it serves is not signed by that root, or is past its valid_until")
	default:
		rep.hubRoot = adv.RootPubKey
		lines = append(lines, "  hub root: "+shortRoot(adv.RootPubKey)+" (advertised at "+where+")",
			"  "+repinHint(rep.pin))
		if rep.pin.source == rootPinSourceAnchor {
			lines = append(lines, "  the enrollment anchor is stale: only `identity enroll` writes it, and re-enrolling for a new root adds it after the old one, which stays first and stays pinned")
		}
		lines = append(lines, "  the hub's root is shown for diagnosis only, never trusted: confirm a new root out of band before pinning it")
	}
	return fmt.Errorf("%w\n%s", err, strings.Join(lines, "\n"))
}

// repinHint tells the operator where to change the pin if the root changed.
func repinHint(pin rootPin) string {
	const prefix = "the hub serves a different root; if the network's root changed, "
	if pin.source == rootPinSourceFlag {
		return prefix + "pass the new root to " + pin.origin
	}
	return prefix + "update " + pin.sourceLabel() + " or pass --" + fetchRegistryRootFlag
}

// fetchRegistryJSON is the --output-format json result: the pin and its source
// always, the fetch outcome on success, the hub's root on a refusal.
type fetchRegistryJSON struct {
	PinnedRoot       string `json:"pinned_root"`
	PinnedRootSource string `json:"pinned_root_source"`
	PinnedRootOrigin string `json:"pinned_root_origin"`
	HubRoot          string `json:"hub_root,omitempty"`
	RegistryFile     string `json:"registry_file,omitempty"`
	Epoch            int    `json:"epoch,omitempty"`
	PreviousEpoch    int    `json:"previous_epoch,omitempty"`
	Changed          bool   `json:"changed"`
	ValidUntil       int64  `json:"valid_until,omitempty"`
}

func writeFetchRegistryJSON(w io.Writer, rep fetchRegistryReport, runErr error) error {
	res := fetchRegistryJSON{
		PinnedRoot:       base64.StdEncoding.EncodeToString(rep.pin.pub),
		PinnedRootSource: rep.pin.source,
		PinnedRootOrigin: rep.pin.origin,
		HubRoot:          rep.hubRoot,
		RegistryFile:     rep.path,
		Epoch:            rep.res.Epoch,
		PreviousEpoch:    rep.res.PreviousEpoch,
		Changed:          rep.res.Changed,
		ValidUntil:       rep.res.ValidUntil,
	}
	if runErr == nil {
		return cmdutil.WriteJSON(w, fetchRegistryCommand, res, "", "")
	}
	code := "fetch_failed"
	if errors.Is(runErr, registryfetch.ErrNotVerified) {
		code = "not_verified"
	}
	env := envelope.Error(fetchRegistryCommand, code, runErr.Error(), "")
	env.Result = res
	return envelope.WriteError(w, env)
}

// rootPinInvalidSuffix completes "<source> is not a valid …" for every pin source.
const rootPinInvalidSuffix = " is not a valid base64 ed25519 public key"

// Pin sources, in resolution order.
const (
	rootPinSourceFlag   = "flag"
	rootPinSourceEnv    = "env"
	rootPinSourceConfig = "config"
	rootPinSourceAnchor = "anchor"
)

// rootPin is the network root this machine trusts.
type rootPin struct {
	pub       ed25519.PublicKey // nil when no source yields a usable root
	networkID string
	// declared: some source claims a pin, even one that did not decode. It is
	// a mesh signal on its own — the machine is configured for a network.
	declared bool
	// source (a rootPinSource*) and origin (the flag, env var, config file or
	// anchor file) say where the pin came from. After the 2026-11-08 root
	// changeover two agents failed on stale pins — a ./config.yaml and an enroll
	// anchor — and neither error said which root was used or why.
	source string
	origin string
}

// sourceLabel names the pin's source for a human: "config /path/config.yaml".
func (p rootPin) sourceLabel() string {
	if p.source == rootPinSourceAnchor {
		return "enrollment anchor " + p.origin
	}
	return p.source + " " + p.origin
}

// line is the one line every fetch prints: which root, and from where.
func (p rootPin) line() string {
	return pinnedRootLine(base64.StdEncoding.EncodeToString(p.pub), p.sourceLabel())
}

// pinnedRootLine formats a base64 root and its source label; fetch-registry
// and doctor share it so both name a pin the same way.
func pinnedRootLine(rootB64, sourceLabel string) string {
	return "pinned root: " + shortRoot(rootB64) + " (from " + sourceLabel + ")"
}

// shortRoot abbreviates a base64 root to its first and last four characters,
// enough to tell two roots apart at a glance; JSON carries the full key.
func shortRoot(b64 string) string {
	if len(b64) <= 9 {
		return b64
	}
	return b64[:4] + "…" + b64[len(b64)-4:]
}

// rootPinEnvVar is the environment variable viper reads the config pin from.
func rootPinEnvVar() string {
	return EnvPrefix() + "_" + strings.ToUpper(strings.ReplaceAll(config.KeyAppIdentityfetchregistryRootPubkey, ".", "_"))
}

// configPinOrigin says where the config pin's value came from: the env var
// when it holds that value, else the config file viper read, else the key.
func configPinOrigin(value string) (source, origin string) {
	if env := rootPinEnvVar(); os.Getenv(env) == value {
		return rootPinSourceEnv, env
	}
	if configFileUsed != "" {
		return rootPinSourceConfig, configFileUsed
	}
	return rootPinSourceConfig, config.KeyAppIdentityfetchregistryRootPubkey
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
	for i, src := range []struct{ value, name string }{
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
		pin := rootPin{pub: pub, networkID: registry.NetworkID(pub), declared: true,
			source: rootPinSourceFlag, origin: src.name}
		if i > 0 {
			pin.source, pin.origin = configPinOrigin(src.value)
		}
		return pin, nil
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
	return rootPin{pub: pub, networkID: anchors[0].NetworkID, declared: true,
		source: rootPinSourceAnchor, origin: anchorPath}, nil
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
