package cmd

// Helpers for identity sign-session. The *_helpers.go name keeps the
// command-pattern check from treating this resolver as its own command.

import (
	"crypto/ed25519"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/peiman/vaultmind/internal/identity/registry"
	"github.com/peiman/vaultmind/internal/identitycli"
)

// resolveOwnSessionBinding is the sign-session view of the existing mesh
// identity: the slug is the agents.yaml project-path match (resolveSlug with
// an empty flag, so app.doctor.mesh_slug cannot name another agent), and
// key_epoch plus the pubkey are the live registry.Resolve binding under the
// pinned root. There is no second resolver.
func resolveOwnSessionBinding(now time.Time) (identitycli.OwnBinding, error) {
	slug := resolveSlug("")
	if slug == "" {
		return identitycli.OwnBinding{}, fmt.Errorf("%s", identitycli.ErrSessionNoSlug)
	}
	pin, err := resolveRootPin("", "")
	if err != nil {
		return identitycli.OwnBinding{}, err
	}
	if len(pin.pub) == 0 {
		return identitycli.OwnBinding{}, fmt.Errorf("%s", identitycli.ErrSessionNoPin)
	}
	reg, err := loadOwnSessionRegistry(pin.pub, now)
	if err != nil {
		return identitycli.OwnBinding{}, err
	}
	b, err := registry.Resolve(reg, slug, now)
	if err != nil {
		return identitycli.OwnBinding{}, fmt.Errorf("%s: %w", identitycli.ErrSessionNotEnrolled, err)
	}
	return identitycli.OwnBinding{
		Slug:     b.Slug,
		KeyEpoch: int64(b.KeyEpoch),
		PubKey:   b.PubKey.Bytes(),
	}, nil
}

func loadOwnSessionRegistry(root ed25519.PublicKey, now time.Time) (registry.Registry, error) {
	path := registryFileFromAgentsYAML(registryPath(), projectPath())
	if path == "" {
		return registry.Registry{}, fmt.Errorf("%s", identitycli.ErrSessionNoRegistry)
	}
	// path comes from the operator's agents.yaml, the same file doctor reads.
	// #nosec G304 G703
	// nosemgrep: go-path-traversal
	raw, err := os.ReadFile(path)
	if err != nil {
		return registry.Registry{}, fmt.Errorf("%s: %w", identitycli.ErrSessionRegistry, err)
	}
	env, err := registry.ParseDistribution(raw)
	if err != nil {
		return registry.Registry{}, fmt.Errorf("%s: %w", identitycli.ErrSessionRegistry, err)
	}
	reg, _, err := registry.VerifyAndLoad(root, env, 0, now, ownSessionMaxStaleness())
	if err != nil {
		return registry.Registry{}, fmt.Errorf("%s: %w", identitycli.ErrSessionRegistry, err)
	}
	return reg, nil
}

// ownSessionMaxStaleness uses the hub bound agents.yaml declares. When none is
// declared, only the registry's own valid_from/valid_until apply — the same
// age bound doctor's signature check uses, not a second policy.
func ownSessionMaxStaleness() time.Duration {
	secs := registryMaxStalenessFromAgentsYAML(registryPath(), projectPath())
	if secs <= 0 {
		return time.Duration(math.MaxInt64)
	}
	return time.Duration(secs) * time.Second
}
