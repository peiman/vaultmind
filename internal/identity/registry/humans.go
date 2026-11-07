package registry

import (
	"fmt"
	"strings"
	"time"

	"github.com/peiman/vaultmind/internal/identity"
)

// People in the trust registry. A human binding is an AgentBinding carried in
// Registry.Humans with Class human. Agents stay in Registry.Agents with Class
// empty. The wire form is a top-level humans array, omitted when empty, so a
// registry with no people canonicalizes to the same bytes as before the array
// existed.
const (
	// ClassHuman is the class of a binding loaded from humans[].
	ClassHuman = "human"

	// CustodyShared is the default custody of a binding. It is omitted on the
	// wire. CustodyIsolated is the only other accepted value.
	CustodyShared   = "shared"
	CustodyIsolated = "isolated"

	// ErrGodOnlyOnHuman is returned when an agent binding holds the god role.
	ErrGodOnlyOnHuman = "registry: god role is allowed only on a human"
	// ErrAgentRoleOnlyOnAgent is returned when a human binding holds bridge or
	// code_signing.
	ErrAgentRoleOnlyOnAgent = "registry: bridge and code_signing roles are allowed only on an agent"
	// ErrMultipleGods is returned when more than one binding holds god.
	ErrMultipleGods = "registry: at most one binding may hold god"
	// ErrSlugAcrossClasses is returned when the same slug is in both agents and
	// humans. The rule is stricter than resolution needs: one slug, one class.
	ErrSlugAcrossClasses = "registry: slug is bound as both an agent and a human"
	// ErrBadCustody is returned when custody is not shared or isolated.
	ErrBadCustody = "registry: custody must be shared or isolated"
	// ErrSlugColon is returned when a binding slug contains a colon. A colon is
	// the class-qualifier separator in a principal id (human:<slug>), so allowing
	// it in a raw slug would let an agent slug like "human:peiman" be routed to
	// the humans array and silently discarded.
	ErrSlugColon = "registry: slug must not contain a colon"
)

// NormalizeCustody returns the in-memory custody of a binding. Empty and
// "shared" become empty, which the wire omits. "isolated" is kept. Any other
// value is ErrBadCustody, and the error names slug.
func NormalizeCustody(slug, custody string) (string, error) {
	switch custody {
	case "", CustodyShared:
		return "", nil
	case CustodyIsolated:
		return CustodyIsolated, nil
	default:
		return "", fmt.Errorf("%s: %q has %q", ErrBadCustody, slug, custody)
	}
}

// validateRegistryBindings is the shared schema gate for SignRegistry and
// VerifyAndLoad (Principle 7 — one schema, both doors). It refuses a duplicate
// tuple or a second live binding inside either array, the same slug in both
// arrays, a slug that contains a colon, a bad custody value, god on an agent,
// bridge or code_signing on a human, a second binding that holds god, and a
// registry that exceeds MaxBindings in total. Every refusal names the slug.
func validateRegistryBindings(reg Registry) error {
	if total := len(reg.Agents) + len(reg.Humans); total > MaxBindings {
		return fmt.Errorf("%s: got %d", ErrTooManyBindings, total)
	}
	if err := checkBindingUniqueness(reg.Agents); err != nil {
		return err
	}
	if err := checkBindingUniqueness(reg.Humans); err != nil {
		return err
	}
	if err := checkSlugDisjoint(reg.Agents, reg.Humans); err != nil {
		return err
	}
	if err := checkSlugNoColon(reg.Agents); err != nil {
		return err
	}
	if err := checkSlugNoColon(reg.Humans); err != nil {
		return err
	}
	if err := checkCustody(reg.Agents); err != nil {
		return err
	}
	if err := checkCustody(reg.Humans); err != nil {
		return err
	}
	return checkRolePlacement(reg)
}

// checkSlugNoColon rejects any binding whose slug contains a colon. A colon is
// the class-qualifier separator used in principal ids (human:<slug>), so a slug
// that embeds one would be silently mis-routed by envelopePrincipal.
func checkSlugNoColon(bindings []AgentBinding) error {
	for _, b := range bindings {
		if strings.Contains(b.Slug, ":") {
			return fmt.Errorf("%s: %q", ErrSlugColon, b.Slug)
		}
	}
	return nil
}

func checkCustody(bindings []AgentBinding) error {
	for _, b := range bindings {
		if _, err := NormalizeCustody(b.Slug, b.Custody); err != nil {
			return err
		}
	}
	return nil
}

func checkSlugDisjoint(agents, humans []AgentBinding) error {
	seen := make(map[string]struct{}, len(agents))
	for _, b := range agents {
		seen[b.Slug] = struct{}{}
	}
	for _, b := range humans {
		if _, ok := seen[b.Slug]; ok {
			return fmt.Errorf("%s: %q", ErrSlugAcrossClasses, b.Slug)
		}
	}
	return nil
}

func holdsGod(b AgentBinding) bool {
	for _, role := range b.Roles {
		if role.Kind() == RoleKindGod {
			return true
		}
	}
	return false
}

func checkRolePlacement(reg Registry) error {
	for _, b := range reg.Agents {
		if holdsGod(b) {
			return fmt.Errorf("%s: %q", ErrGodOnlyOnHuman, b.Slug)
		}
	}
	var gods []string
	for _, b := range reg.Humans {
		for _, role := range b.Roles {
			if role.Kind() == RoleKindBridge || role.Kind() == RoleKindCodeSigning {
				return fmt.Errorf("%s: %q has %s", ErrAgentRoleOnlyOnAgent, b.Slug, role.Kind())
			}
		}
		if holdsGod(b) {
			gods = append(gods, b.Slug)
		}
	}
	if len(gods) > 1 {
		return fmt.Errorf("%s: %s", ErrMultipleGods, strings.Join(gods, ", "))
	}
	return nil
}

func epochsInRange(bindings []AgentBinding) error {
	for _, b := range bindings {
		if !epochInRange(b.KeyEpoch) {
			return fmt.Errorf("%s", ErrEpochRange)
		}
	}
	return nil
}

// ResolveHuman returns the live human binding for slug at now. It applies the
// same liveness rules as Resolve and never returns an agent binding.
func ResolveHuman(reg Registry, slug string, now time.Time) (AgentBinding, error) {
	b, err := resolveSlug(reg.Humans, slug, now)
	if err != nil {
		return AgentBinding{}, err
	}
	return asHuman(b), nil
}

// ResolveHumanTuple returns the live human binding for the exact
// {slug, key_epoch} tuple. It applies the same default-deny as ResolveTuple
// and never returns an agent binding.
func ResolveHumanTuple(reg Registry, slug string, keyEpoch int, now time.Time) (AgentBinding, error) {
	b, err := resolveTupleIn(reg.Humans, slug, keyEpoch, now)
	if err != nil {
		return AgentBinding{}, err
	}
	return asHuman(b), nil
}

// isLive reports whether binding b is live at now: not revoked, within its
// validity window. This is the shared liveness predicate used by resolveSlug
// and GodBinding so both apply the same rule.
func isLive(b AgentBinding, now time.Time) bool {
	if b.RevokedAt != nil {
		return false
	}
	t := now.Unix()
	return t >= b.ValidFrom && t <= b.ValidUntil
}

// GodBinding returns the one live binding that holds god at now. The bool is
// false when no live binding holds god, or when more than one live binding
// does. A revoked, expired, or not-yet-valid god binding is not counted.
func GodBinding(reg Registry, now time.Time) (AgentBinding, bool) {
	var found AgentBinding
	n := 0
	for _, b := range reg.Humans {
		if !holdsGod(b) || !isLive(b, now) {
			continue
		}
		found = asHuman(b)
		n++
	}
	for _, b := range reg.Agents {
		if !holdsGod(b) || !isLive(b, now) {
			continue
		}
		found = b
		n++
	}
	if n != 1 {
		return AgentBinding{}, false
	}
	return found, true
}

// VerifyHumanMessage verifies sig over canonical under the live human binding
// for {slug, keyEpoch}. It is the human twin of VerifyMessage and shares that
// function's signature check. An agent slug is not a human slug.
func VerifyHumanMessage(
	reg Registry,
	slug string,
	keyEpoch int,
	canonical identity.CanonicalBytes,
	sig []byte,
	now time.Time,
) (bool, error) {
	b, err := ResolveHumanTuple(reg, slug, keyEpoch, now)
	if err != nil {
		return false, err
	}
	return identity.VerifyCanonical(b.PubKey.Bytes(), canonical, sig)
}

func asHuman(b AgentBinding) AgentBinding {
	if b.Class == "" {
		b.Class = ClassHuman
	}
	return b
}

// resolveSlug is Resolve's liveness scan over one binding array.
func resolveSlug(bindings []AgentBinding, slug string, now time.Time) (AgentBinding, error) {
	sawRevoked := false
	for _, b := range bindings {
		if b.Slug != slug {
			continue
		}
		if b.RevokedAt != nil {
			sawRevoked = true
			continue
		}
		if now.Unix() < b.ValidFrom {
			continue
		}
		if now.Unix() > b.ValidUntil {
			continue
		}
		return b, nil
	}
	if sawRevoked {
		return AgentBinding{}, fmt.Errorf("%s: %q", ErrRevoked, slug)
	}
	return AgentBinding{}, fmt.Errorf("%s: %q", ErrUnknownSlug, slug)
}

// resolveTupleIn is resolveTuple's default-deny scan over one binding array.
func resolveTupleIn(bindings []AgentBinding, slug string, keyEpoch int, now time.Time) (AgentBinding, error) {
	for _, b := range bindings {
		if b.Slug != slug || b.KeyEpoch != keyEpoch {
			continue
		}
		if b.RevokedAt != nil {
			return AgentBinding{}, fmt.Errorf("%s: %q", ErrRevoked, slug)
		}
		if now.Unix() < b.ValidFrom {
			return AgentBinding{}, fmt.Errorf("%s: %q", ErrNotYetValid, slug)
		}
		if now.Unix() > b.ValidUntil {
			return AgentBinding{}, fmt.Errorf("%s: %q", ErrExpiredBinding, slug)
		}
		return b, nil
	}
	for _, b := range bindings {
		if b.Slug == slug {
			return AgentBinding{}, fmt.Errorf("%s: %q", ErrKeyEpochMismatch, slug)
		}
	}
	return AgentBinding{}, fmt.Errorf("%s: %q", ErrUnknownSlug, slug)
}
