package cmd

// Helpers for identity sign-envelope. The *_helpers.go name keeps the
// command-pattern check from treating this resolver as its own command.

import (
	"fmt"
	"time"

	"github.com/peiman/vaultmind/internal/identity/registry"
	"github.com/peiman/vaultmind/internal/identitycli"
)

// signEnvelopeRegistry returns the verified registry sign-envelope checks the
// envelope's binding against, resolved the same way as sign-session's. ok is
// false when this machine has no mesh configured: no registry file in
// agents.yaml for this project, or no pinned root.
//
// Skipping is correct there. The hub is the gate and refuses a wrong
// key_epoch or key whatever this command does; the check only turns that
// later, unclear refusal into a clear one here. The root key ceremony runs
// sign-envelope as a separate user with no agents.yaml and no pin, and it
// must keep signing exactly as before.
//
// A registry file that is configured but missing, unreadable, or fails to
// parse or verify under the pin is an error naming the file.
func signEnvelopeRegistry(now time.Time) (reg registry.Registry, ok bool, err error) {
	path := registryFileFromAgentsYAML(registryPath(), projectPath())
	if path == "" {
		return registry.Registry{}, false, nil
	}
	pin, err := resolveRootPin("", "")
	if err != nil {
		return registry.Registry{}, false, err
	}
	if len(pin.pub) == 0 {
		return registry.Registry{}, false, nil
	}
	reg, err = readOwnRegistry(path, pin.pub, now)
	if err != nil {
		return registry.Registry{}, false, fmt.Errorf("%s %s: %w", identitycli.ErrEnvelopeRegistry, path, err)
	}
	return reg, true, nil
}
