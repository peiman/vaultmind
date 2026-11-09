package cmd

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/.ckeletin/pkg/output"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/identity/registryfetch"
	"github.com/spf13/cobra"
)

const (
	fetchRegistryErrNoRoot  = "no pinned root key: pass --root-pubkey, set " + config.KeyAppIdentityfetchregistryRootPubkey + " in the vaultmind config, or pin the network with `vaultmind identity enroll` — a registry cannot be trusted without one"
	fetchRegistryRootFlag   = "root-pubkey"
	fetchRegistryErrBadRoot = "--" + fetchRegistryRootFlag + rootPinInvalidSuffix
	fetchRegistryErrNoHub   = "no hub address: pass --hub, set " + envDaemonURL + ", or declare daemon_url in agents.yaml"
	fetchRegistryErrNoPath  = "no registry path: pass --registry-file, or declare registry_path in agents.yaml"
	fetchRegistryCommand    = "identity fetch-registry"
)

var identityFetchRegistryCmd = MustNewCommand(commands.IdentityFetchRegistryMetadata, runIdentityFetchRegistry)

func init() {
	identityCmd.AddCommand(identityFetchRegistryCmd)
	setupCommandConfig(identityFetchRegistryCmd)
}

// Sentinels for the config errors, so JSON mode can classify them.
var (
	errFetchRegistryNoRoot = errors.New(fetchRegistryErrNoRoot)
	errFetchRegistryNoHub  = errors.New(fetchRegistryErrNoHub)
	errFetchRegistryNoPath = errors.New(fetchRegistryErrNoPath)
)

// runIdentityFetchRegistry names the pinned root and its source on every text
// run (stderr); JSON mode answers every outcome with one envelope shape.
func runIdentityFetchRegistry(cmd *cobra.Command, _ []string) error {
	rep := fetchRegistryReport{now: time.Now()}
	if output.IsJSONMode() {
		err := fetchRegistry(cmd, &rep, io.Discard)
		return writeFetchRegistryJSON(cmd.OutOrStdout(), &rep, err)
	}
	if err := fetchRegistry(cmd, &rep, cmd.ErrOrStderr()); err != nil {
		return err
	}
	return writeFetchRegistryResult(cmd.OutOrStdout(), rep.res, rep.path, rep.maxStaleness, rep.now)
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
