package cmd

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/identity/signerservice"
	"github.com/spf13/cobra"
)

const (
	// signerInstallOS is the only platform with a supervisor this command speaks.
	signerInstallOS = "darwin"
	// launchctlBin is the only program runLaunchctl will execute.
	launchctlBin = "launchctl"
	// signerSocketDialTimeout bounds the "is a signer already serving?" probe.
	signerSocketDialTimeout = 500 * time.Millisecond

	errSignerInstallUnsupported = "identity signer install: launchd is macOS-only (this is %s); " +
		"run `vaultmind identity signer` under your OS supervisor (e.g. a systemd --user unit with Restart=always)"
	signerInstalledMsg = "installed %s\n  job:   %s\n  logs:  %s\n" +
		"  starts at login and restarts if it dies.\n" +
		"  remove: launchctl bootout gui/%d/%s && rm %q\n"
)

var identitySignerInstallCmd = MustNewCommand(commands.IdentitySignerInstallMetadata, runIdentitySignerInstall)

func init() {
	identitySignerCmd.AddCommand(identitySignerInstallCmd)
	setupCommandConfig(identitySignerInstallCmd)
}

// runIdentitySignerInstall resolves the signer exactly as `identity signer`
// would, then either prints or installs the launchd job for it.
func runIdentitySignerInstall(cmd *cobra.Command, _ []string) error {
	spec, err := resolveSignerServiceSpec(cmd)
	if err != nil {
		return err
	}
	if getConfigValueWithFlags[bool](cmd, "print", config.KeyAppIdentitysignerinstallPrint) {
		body, err := signerservice.RenderPlist(spec)
		if err != nil {
			return err
		}
		_, err = cmd.OutOrStdout().Write(body)
		return err
	}
	if runtime.GOOS != signerInstallOS {
		return fmt.Errorf(errSignerInstallUnsupported, runtime.GOOS)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("identity signer install: resolving home: %w", err)
	}
	in := signerservice.Installer{Home: home, UID: os.Getuid(), Run: runLaunchctl, SocketLive: signerSocketLive}
	path, err := in.Install(cmd.Context(), spec)
	if err != nil {
		return err
	}
	return writeSignerInstalled(cmd.OutOrStdout(), spec, in.UID, path)
}

// resolveSignerServiceSpec pins every path NOW, as absolute paths, so the job
// launches the signer the operator's flags and --config-path-mode describe —
// not whatever launchd's environment would resolve.
func resolveSignerServiceSpec(cmd *cobra.Command) (signerservice.Spec, error) {
	keyPath := getConfigValueWithFlags[string](cmd, "signer-key", config.KeyAppIdentitysignerinstallSignerKey)
	sockPath := getConfigValueWithFlags[string](cmd, "signer-socket", config.KeyAppIdentitysignerinstallSignerSocket)
	var err error
	if keyPath == "" {
		if keyPath, err = defaultSignerKeyPath(); err != nil {
			return signerservice.Spec{}, fmt.Errorf("resolving signer key path: %w", err)
		}
	}
	if sockPath == "" {
		if sockPath, err = defaultSignerSocketPath(); err != nil {
			return signerservice.Spec{}, fmt.Errorf("resolving signer socket path: %w", err)
		}
	}
	bin, err := os.Executable()
	if err != nil {
		return signerservice.Spec{}, fmt.Errorf("resolving vaultmind binary: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(bin); err == nil {
		bin = resolved
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return signerservice.Spec{}, fmt.Errorf("resolving home: %w", err)
	}
	name := getConfigValueWithFlags[string](cmd, "name", config.KeyAppIdentitysignerinstallName)
	if name == "" {
		name = signerservice.NameFromKeyPath(keyPath)
	}
	return signerservice.Spec{
		Name:       name,
		Binary:     bin,
		KeyPath:    absOrSelf(keyPath),
		SocketPath: absOrSelf(sockPath),
		LogDir:     filepath.Join(home, "Library", "Logs"),
	}, nil
}

func absOrSelf(p string) string {
	if a, err := filepath.Abs(p); err == nil {
		return a
	}
	return p
}

// signerSocketLive reports whether something accepts connections on path.
func signerSocketLive(path string) bool {
	c, err := net.DialTimeout("unix", path, signerSocketDialTimeout)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// runLaunchctl runs launchctl, folding its output into the error — launchd's
// own message is usually the only explanation there is.
//
// The binary is fixed: anything but launchctl is refused, so the Installer's
// injectable runner can never become a way to execute something else.
func runLaunchctl(ctx context.Context, name string, args ...string) error {
	if name != launchctlBin {
		return fmt.Errorf("identity signer install: refusing to run %q (only %s)", name, launchctlBin)
	}
	out, err := exec.CommandContext(ctx, "launchctl", args...).CombinedOutput() //nolint:gosec // G204: program pinned above; args are a gui/<uid> domain, a job label and a plist path we wrote
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func writeSignerInstalled(w io.Writer, s signerservice.Spec, uid int, path string) error {
	label := signerservice.Label(s.Name)
	logs := filepath.Join(s.LogDir, "vaultmind-signer-"+s.Name+".{out,err}.log")
	_, err := fmt.Fprintf(w, signerInstalledMsg, label, path, logs, uid, label, path)
	return err
}
