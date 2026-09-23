package cmd

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/identity/anchor"
	"github.com/peiman/vaultmind/internal/identity/registry"
	"github.com/peiman/vaultmind/internal/identity/registryfetch"
	"github.com/peiman/vaultmind/internal/identity/relayclient"
	"github.com/spf13/cobra"
)

const (
	fetchRegistryErrNoRoot  = "no pinned root key: pass --root-pubkey, set " + config.KeyAppIdentityfetchregistryRootPubkey + " in the vaultmind config, or pin the network with `vaultmind identity enroll` — a registry cannot be trusted without one"
	fetchRegistryErrBadRoot = "--root-pubkey is not a valid base64 ed25519 public key"
	fetchRegistryErrNoHub   = "no hub address: pass --hub, set " + envDaemonURL + ", or declare daemon_url in agents.yaml"
	fetchRegistryErrNoPath  = "no registry path: pass --registry-file, or declare registry_path in agents.yaml"
)

var identityFetchRegistryCmd = MustNewCommand(commands.IdentityFetchRegistryMetadata, runIdentityFetchRegistry)

func init() {
	identityCmd.AddCommand(identityFetchRegistryCmd)
	setupCommandConfig(identityFetchRegistryCmd)
}

// runIdentityFetchRegistry resolves pin, hub, and path BEFORE touching the
// network, so a misconfigured machine fails without fetching anything.
func runIdentityFetchRegistry(cmd *cobra.Command, _ []string) error {
	root, err := resolveFetchRegistryRoot(getConfigValueWithFlags[string](cmd, "root-pubkey", config.KeyAppIdentityfetchregistryRootPubkey))
	if err != nil {
		return err
	}
	agents, project := registryPath(), projectPath()
	hub := getConfigValueWithFlags[string](cmd, "hub", config.KeyAppIdentityfetchregistryHub)
	if hub == "" {
		hub = resolveDoctorDaemonURL(os.Getenv(envDaemonURL), agents, project)
	}
	if hub == "" {
		return errors.New(fetchRegistryErrNoHub)
	}
	path := getConfigValueWithFlags[string](cmd, "registry-file", config.KeyAppIdentityfetchregistryRegistryFile)
	if path == "" {
		path = registryFileFromAgentsYAML(agents, project)
	}
	if path == "" {
		return errors.New(fetchRegistryErrNoPath)
	}

	body, err := relayclient.FetchDirectory(cmd.Context(), nil, hub)
	if err != nil {
		return err
	}
	now := time.Now()
	res, err := registryfetch.Update(root, body, path, now)
	if err != nil {
		return err
	}
	return writeFetchRegistryResult(cmd.OutOrStdout(), res, path, registryMaxStalenessFromAgentsYAML(agents, project), now)
}

// resolveFetchRegistryRoot returns the pin: the flag when given, else the first
// anchor `identity enroll` persisted. No pin is an error, never a skip.
func resolveFetchRegistryRoot(flag string) (ed25519.PublicKey, error) {
	if flag == "" {
		anchorPath, err := defaultNetworkAnchorPath()
		if err != nil {
			return nil, errors.New(fetchRegistryErrNoRoot)
		}
		anchors, err := anchor.Load(anchorPath)
		if err != nil || len(anchors) == 0 {
			return nil, errors.New(fetchRegistryErrNoRoot)
		}
		flag = anchors[0].RootPubKey
	}
	raw, err := base64.StdEncoding.DecodeString(flag)
	if err != nil {
		return nil, errors.New(fetchRegistryErrBadRoot)
	}
	pk, err := registry.NewPublicKey(raw)
	if err != nil {
		return nil, errors.New(fetchRegistryErrBadRoot)
	}
	return pk.Bytes(), nil
}

func writeFetchRegistryResult(w io.Writer, res registryfetch.Result, path string, maxStalenessSecs int64, now time.Time) error {
	var line string
	switch {
	case !res.Changed:
		line = fmt.Sprintf("registry already current: epoch %d (%s)", res.Epoch, path)
	case res.PreviousEpoch == 0:
		line = fmt.Sprintf("registry installed: epoch %d (%s)", res.Epoch, path)
	default:
		line = fmt.Sprintf("registry updated: epoch %d -> epoch %d (%s)", res.PreviousEpoch, res.Epoch, path)
	}
	if days, ok := registryDaysLeft(res.ValidFrom, res.ValidUntil, maxStalenessSecs, now); ok {
		line += fmt.Sprintf("; %d days left before the hub refuses it", days)
	}
	_, err := fmt.Fprintln(w, line)
	return err
}
