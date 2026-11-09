package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/identity/signer"
	"github.com/spf13/cobra"
)

// Signer-daemon startup/shutdown lines (SSOT). These are the ONLY things the
// command prints — never the key, never key bytes.
const (
	signerListeningMsg = "custody signer listening on %s (uid-gated, dev-interim)\n"
	signerShutdownMsg  = "shutting down, removing socket %s\n"

	// errSignerEmptyAllowlist is the command-level fail-closed message when the
	// computed uid allowlist is empty (defense-in-depth alongside signer.New's
	// own guard). An empty allowlist would otherwise deny every caller silently.
	errSignerEmptyAllowlist = "signer: refusing to start with an empty uid allowlist"
)

var identitySignerCmd = MustNewCommand(commands.IdentitySignerMetadata, runIdentitySignerCmd)

func init() {
	identityCmd.AddCommand(identitySignerCmd)
	setupCommandConfig(identitySignerCmd)
}

// runIdentitySignerCmd is the cobra entrypoint: it resolves the key/socket
// paths, installs a SIGINT/SIGTERM-cancelled context, and runs the signer in
// the foreground until a signal arrives.
func runIdentitySignerCmd(cmd *cobra.Command, _ []string) error {
	keyPath, sockPath, err := resolveSignerPaths(cmd)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if getConfigValueWithFlags[bool](cmd, "once", config.KeyAppIdentitysignerOnce) {
		return serveIdentitySignerOnce(ctx, cmd.OutOrStdout(), cmd.ErrOrStderr(), currentUserSignerConfig(keyPath, sockPath))
	}
	return runIdentitySigner(ctx, cmd.OutOrStdout(), keyPath, sockPath)
}

// resolveSignerPaths reads --signer-key / --signer-socket, falling back to the
// XDG default paths when unset.
func resolveSignerPaths(cmd *cobra.Command) (keyPath, sockPath string, err error) {
	keyPath = getConfigValueWithFlags[string](cmd, "signer-key", config.KeyAppIdentitysignerSignerKey)
	if keyPath == "" {
		if keyPath, err = defaultSignerKeyPath(); err != nil {
			return "", "", fmt.Errorf("resolving signer key path: %w", err)
		}
	}
	sockPath = getConfigValueWithFlags[string](cmd, "signer-socket", config.KeyAppIdentitysignerSignerSocket)
	if sockPath == "" {
		if sockPath, err = defaultSignerSocketPath(); err != nil {
			return "", "", fmt.Errorf("resolving signer socket path: %w", err)
		}
	}
	return keyPath, sockPath, nil
}

// runIdentitySigner builds the single-uid (current user) dev-interim signer
// config and serves it until ctx is cancelled. The allowlist is the current
// uid only — fail-closed on an empty/zero allowlist is enforced by serve.
func runIdentitySigner(ctx context.Context, out io.Writer, keyPath, sockPath string) error {
	return serveIdentitySigner(ctx, out, currentUserSignerConfig(keyPath, sockPath))
}

// currentUserSignerConfig is the dev-interim signer config: the given paths and
// an allowlist of the current uid only.
func currentUserSignerConfig(keyPath, sockPath string) signer.Config {
	return signer.Config{
		KeyPath:     keyPath,
		SocketPath:  sockPath,
		AllowedUIDs: []uint32{uint32(os.Getuid())}, //nolint:gosec // G115: os.Getuid() is the current real uid, always non-negative and within uint32
	}
}

// serveIdentitySigner runs a signer with the given config until ctx is
// cancelled, then Closes it (removing the socket). It FAILS CLOSED: an empty
// allowlist, a missing/unreadable key, or a bind failure returns an error
// having printed nothing sensitive.
func serveIdentitySigner(ctx context.Context, out io.Writer, cfg signer.Config) error {
	s, err := startIdentitySigner(out, cfg)
	if err != nil {
		return err
	}

	serveErr := make(chan error, 1)
	go func() { serveErr <- s.Serve() }()

	select {
	case <-ctx.Done():
		// Cleanup precedes cosmetics: Close (load-bearing socket removal) FIRST,
		// so a SIGPIPE on a closed stdout can't kill us before cleanup; then
		// best-effort print. Drain any Serve error that raced the signal so a real
		// failure isn't silently dropped on the exit-0 path.
		err := s.Close()
		select {
		case se := <-serveErr:
			err = errors.Join(err, se)
		default:
		}
		_, _ = fmt.Fprintf(out, signerShutdownMsg, cfg.SocketPath)
		return err
	case err := <-serveErr:
		return errors.Join(err, s.Close())
	}
}

// startIdentitySigner checks the allowlist, loads the key, binds the socket and
// prints the startup line. It FAILS CLOSED and leaves no socket on any error.
func startIdentitySigner(out io.Writer, cfg signer.Config) (*signer.Signer, error) {
	if len(cfg.AllowedUIDs) == 0 {
		return nil, fmt.Errorf("%s", errSignerEmptyAllowlist)
	}
	s, err := signer.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("signer: init: %w", err)
	}
	if err := s.Listen(); err != nil {
		return nil, fmt.Errorf("signer: listen: %w", err)
	}
	if _, err := fmt.Fprintf(out, signerListeningMsg, cfg.SocketPath); err != nil {
		return nil, errors.Join(fmt.Errorf("signer: write startup line: %w", err), s.Close())
	}
	return s, nil
}

// serveIdentitySignerOnce signs exactly one request, then prints the sha256 of
// the signed bytes to errOut and returns. A signal stops the run; Close makes
// the signer refuse any further signature, so a signature made before the stop
// is still reported and none can be made after it.
func serveIdentitySignerOnce(ctx context.Context, out, errOut io.Writer, cfg signer.Config) error {
	s, err := startIdentitySigner(out, cfg)
	if err != nil {
		return err
	}
	serveErr := make(chan error, 1)
	go func() {
		_, err := s.ServeOnce()
		serveErr <- err
	}()
	select {
	case <-ctx.Done():
		err = s.Close()
		select {
		case se := <-serveErr:
			err = errors.Join(err, se)
		default:
		}
	case err = <-serveErr:
	}
	if res := s.Signed(); res != nil {
		_, _ = fmt.Fprintln(errOut, res.Summary())
	}
	return err
}
