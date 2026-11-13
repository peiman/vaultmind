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

// fetchRegistry resolves the pin, names it on announce, and resolves hub and
// path — all BEFORE touching the network, so a misconfigured machine fails
// without fetching anything.
func fetchRegistry(cmd *cobra.Command, rep *fetchRegistryReport, announce io.Writer) error {
	// Only the flag itself is explicit here: the config pin is the shared
	// resolver's second source, so doctor reads the very same one.
	rootFlag, _ := cmd.Flags().GetString(fetchRegistryRootFlag)
	pin, err := resolveRootPin(rootFlag, "--"+fetchRegistryRootFlag)
	if err != nil {
		return err
	}
	if pin.pub == nil {
		return errFetchRegistryNoRoot
	}
	rep.pin = pin
	if _, err := fmt.Fprintln(announce, pin.line()); err != nil {
		return err
	}

	agents, project := registryPath(), projectPath()
	hub := getConfigValueWithFlags[string](cmd, "hub", config.KeyAppIdentityfetchregistryHub)
	if hub == "" {
		hub = resolveDoctorDaemonURL(os.Getenv(envDaemonURL), agents, project)
	}
	if hub == "" {
		return errFetchRegistryNoHub
	}
	rep.path = getConfigValueWithFlags[string](cmd, "registry-file", config.KeyAppIdentityfetchregistryRegistryFile)
	if rep.path == "" {
		rep.path = registryFileFromAgentsYAML(agents, project)
	}
	if rep.path == "" {
		return errFetchRegistryNoPath
	}

	var body []byte
	body, err = relayclient.FetchDirectory(cmd.Context(), nil, hub)
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
		// Re-encoded from the decoded key: the raw string may carry \r\n the
		// decoder skipped, so it need not read as the key it names.
		rep.hubRoot = base64.StdEncoding.EncodeToString(advPub)
		lines = append(lines, "  hub root: "+shortRoot(rep.hubRoot)+" (advertised at "+where+")",
			"  the hub advertises the pinned root, so the root did not change: the registry it serves is not signed by that root, or is past its valid_until")
	default:
		rep.hubRoot = base64.StdEncoding.EncodeToString(advPub)
		lines = append(lines, "  hub root: "+shortRoot(rep.hubRoot)+" (advertised at "+where+")",
			"  "+repinHint(rep.pin))
		// Conditional: the hub's word alone must not let us call a correct
		// anchor wrong — a hostile hub can advertise any root.
		if rep.pin.source == rootPinSourceAnchor {
			lines = append(lines, "  if the network's root changed, the enrollment anchor is stale: run `identity enroll` for the new root, which makes the new anchor the pinned one")
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
// (empty when no pin resolved), the fetch outcome on success, and on a refusal
// the root the hub advertises — UNTRUSTED, shown only to explain the refusal.
type fetchRegistryJSON struct {
	PinnedRoot        string `json:"pinned_root"`
	PinnedRootSource  string `json:"pinned_root_source"`
	PinnedRootOrigin  string `json:"pinned_root_origin"`
	HubAdvertisedRoot string `json:"hub_advertised_root,omitempty"`
	RegistryFile      string `json:"registry_file,omitempty"`
	Epoch             int    `json:"epoch,omitempty"`
	PreviousEpoch     int    `json:"previous_epoch,omitempty"`
	Changed           bool   `json:"changed"`
	ValidUntil        int64  `json:"valid_until,omitempty"`
}

func writeFetchRegistryJSON(w io.Writer, rep *fetchRegistryReport, runErr error) error {
	res := fetchRegistryJSON{
		PinnedRootSource:  rep.pin.source,
		PinnedRootOrigin:  rep.pin.origin,
		HubAdvertisedRoot: rep.hubRoot,
		RegistryFile:      rep.path,
		Epoch:             rep.res.Epoch,
		PreviousEpoch:     rep.res.PreviousEpoch,
		Changed:           rep.res.Changed,
		ValidUntil:        rep.res.ValidUntil,
	}
	if rep.pin.pub != nil {
		res.PinnedRoot = base64.StdEncoding.EncodeToString(rep.pin.pub)
	}
	if runErr == nil {
		return cmdutil.WriteJSON(w, fetchRegistryCommand, res, "", "")
	}
	env := envelope.Error(fetchRegistryCommand, fetchRegistryErrCode(runErr), runErr.Error(), "")
	env.Result = res
	return envelope.WriteError(w, env)
}

// fetchRegistryErrCode classifies a failure for the JSON envelope.
func fetchRegistryErrCode(err error) string {
	var invalid rootPinInvalidError
	switch {
	case errors.Is(err, registryfetch.ErrNotVerified):
		return "not_verified"
	case errors.Is(err, errFetchRegistryNoRoot), errors.Is(err, errFetchRegistryNoHub), errors.Is(err, errFetchRegistryNoPath):
		return "config_missing"
	case errors.As(err, &invalid):
		return "config_invalid"
	}
	return "fetch_failed"
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

// rootPinEnvVar is the environment variable viper reads config key from.
func rootPinEnvVar(key string) string {
	return EnvPrefix() + "_" + strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
}

// configPinOrigin says where config key's value came from: the env var when
// it holds that value, else the config file viper read, else the key itself.
func configPinOrigin(key, value string) (source, origin string) {
	if env := rootPinEnvVar(key); os.Getenv(env) == value {
		return rootPinSourceEnv, env
	}
	if configFileUsed != "" {
		return rootPinSourceConfig, configFileUsed
	}
	return rootPinSourceConfig, key
}

// explicitPin is a command's own pin value, where it came from, and the name
// a malformed value is reported under.
type explicitPin struct{ value, source, origin, name string }

// commandPin reads a command's own pin from its flag or the flag's config key.
// It is a flag only when the flag was passed: a value from ./config.yaml or
// the environment reported as "from flag" would blame a flag nobody typed.
func commandPin(cmd *cobra.Command, flag, key string) explicitPin {
	value := getConfigValueWithFlags[string](cmd, flag, key)
	if cmd.Flags().Changed(flag) {
		return explicitPin{value: value, source: rootPinSourceFlag, origin: "--" + flag, name: "--" + flag}
	}
	source, origin := configPinOrigin(key, value)
	return explicitPin{value: value, source: source, origin: origin, name: key}
}

// rootPinInvalidError names the source of a pin that does not decode.
type rootPinInvalidError struct{ name string }

func (e rootPinInvalidError) Error() string { return e.name + rootPinInvalidSuffix }

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
	return resolveRootPinFrom(explicitPin{value: explicit, source: rootPinSourceFlag, origin: explicitName, name: explicitName})
}

// resolveRootPinFrom is resolveRootPin for an explicit pin whose source the
// caller already knows (doctor's flag-or-config key; see commandPin).
func resolveRootPinFrom(explicit explicitPin) (rootPin, error) {
	shared := config.KeyAppIdentityfetchregistryRootPubkey
	sharedValue := getKeyValue[string](shared)
	sharedSource, sharedOrigin := configPinOrigin(shared, sharedValue)
	for _, src := range []explicitPin{
		explicit,
		{value: sharedValue, source: sharedSource, origin: sharedOrigin, name: shared},
	} {
		if src.value == "" {
			continue
		}
		pub, ok := decodeRootPin(src.value)
		if !ok {
			return rootPin{}, rootPinInvalidError{name: src.name}
		}
		return rootPin{pub: pub, networkID: registry.NetworkID(pub), declared: true,
			source: src.source, origin: src.origin}, nil
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
