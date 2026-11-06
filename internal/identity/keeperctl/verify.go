package keeperctl

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/peiman/vaultmind/internal/identity"
	"github.com/peiman/vaultmind/internal/identity/registry"
)

type Role string

const (
	RoleOwner   Role = "owner"
	RoleGrantee Role = "grantee"
)

// Reason is the frozen verifier rejection vocabulary. Acceptance has no Reason.
type Reason string

const (
	ReasonSchemaInvalid     Reason = "schema_invalid"
	ReasonRegistryExpired   Reason = "registry_expired"
	ReasonSignatureInvalid  Reason = "signature_invalid"
	ReasonBindingRejected   Reason = "binding_rejected"
	ReasonUnknownKeeper     Reason = "unknown_keeper"
	ReasonStale             Reason = "stale"
	ReasonFromFuture        Reason = "from_future"
	ReasonReplayed          Reason = "replayed"
	ReasonGrantMissing      Reason = "grant_missing"
	ReasonGrantExpired      Reason = "grant_expired"
	ReasonGrantRevoked      Reason = "grant_revoked"
	ReasonGrantNotForKeeper Reason = "grant_not_for_keeper"
	ReasonGrantScopeMissing Reason = "grant_scope_missing"
	ReasonGrantWrongGrantee Reason = "grant_wrong_grantee"
	ReasonGrantBadSignature Reason = "grant_bad_signature"
	ReasonControlNeedsOwner Reason = "control_needs_owner"
	ReasonNonceStoreFailed  Reason = "nonce_store_failed"
	ReasonBeforeStart       Reason = "before_start"
)

type Decision struct {
	Accepted bool   `json:"accepted"`
	Role     Role   `json:"role,omitempty"`
	Reason   Reason `json:"reason,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

// Policy uses an already root-verified registry and the watchdog's own manifest.
// NotBefore is required and must be positive. Share store instances across calls.
type Policy struct {
	Registry     registry.Registry
	KeeperOwners map[string]string
	Grants       GrantStore
	Nonces       NonceStore
	Now          func() time.Time
	MaxAge       time.Duration
	MaxSkew      time.Duration
	NotBefore    int64
}

// DecisionError lets grant administration expose the same typed reasons.
type DecisionError struct {
	Reason Reason
	Detail string
}

func (e *DecisionError) Error() string             { return fmt.Sprintf("keeperctl: %s: %s", e.Reason, e.Detail) }
func reject(reason Reason, detail string) Decision { return Decision{Reason: reason, Detail: detail} }
func (d Decision) err() error {
	if d.Reason == "" {
		return nil
	}
	return &DecisionError{Reason: d.Reason, Detail: d.Detail}
}
func policyNow(p Policy) time.Time {
	if p.Now == nil {
		return time.Now()
	}
	return p.Now()
}

func verifySigned(p Policy, from string, epoch int64, c identity.CanonicalBytes, sig []byte, now time.Time) Decision {
	if now.Unix() > p.Registry.ValidUntil || now.Unix() < p.Registry.ValidFrom {
		return reject(ReasonRegistryExpired, "registry outside validity window")
	}
	// registry's API uses int. Prevent truncation on 32-bit builds.
	if int64(int(epoch)) != epoch {
		return reject(ReasonBindingRejected, "key epoch cannot match a registry tuple")
	}
	ok, err := registry.VerifyMessage(p.Registry, from, int(epoch), c, sig, now)
	if err != nil {
		return reject(ReasonBindingRejected, err.Error())
	}
	if !ok {
		return reject(ReasonSignatureInvalid, "signature does not verify")
	}
	return Decision{}
}

func fresh(p Policy, issued int64, now time.Time) Decision {
	if p.NotBefore <= 0 {
		return reject(ReasonSchemaInvalid, "NotBefore must be positive")
	}
	age, skew := windows(p.MaxAge, p.MaxSkew)
	if age < 0 || skew < 0 {
		return reject(ReasonSchemaInvalid, "negative freshness window")
	}
	if issued < p.NotBefore {
		return reject(ReasonBeforeStart, "issued before watchdog start")
	}
	// Whole epoch seconds on both sides, so an integer-clock verifier (Rust)
	// reaches the same verdict; sub-second wall time never moves a bound.
	nowSec := now.Unix()
	if issued < nowSec-int64(age/time.Second) {
		return reject(ReasonStale, "request too old")
	}
	if issued > nowSec+int64(skew/time.Second) {
		return reject(ReasonFromFuture, "request too far in future")
	}
	return Decision{}
}

func recordNonce(p Policy, from, nonce string, issued int64) Decision {
	if p.Nonces == nil {
		return reject(ReasonNonceStoreFailed, "nonce store required")
	}
	err := p.Nonces.CheckAndRecord(from, nonce, issued)
	if errors.Is(err, ErrNonceSeen) {
		return reject(ReasonReplayed, err.Error())
	}
	if err != nil {
		return reject(ReasonNonceStoreFailed, err.Error())
	}
	return Decision{}
}

// VerifyControl follows the frozen gate, registry, signature, keeper, time,
// nonce, authorization order. A valid owner's read ignores its grant_id.
func VerifyControl(p Policy, req ControlRequest, sig []byte) Decision {
	c, err := CanonicalizeControlRequest(req)
	if err != nil {
		return reject(ReasonSchemaInvalid, err.Error())
	}
	now := policyNow(p)
	if d := verifySigned(p, req.From, req.KeyEpoch, c, sig, now); d.Reason != "" {
		return d
	}
	owner, ok := p.KeeperOwners[req.Keeper]
	if !ok {
		return reject(ReasonUnknownKeeper, req.Keeper)
	}
	if d := fresh(p, req.IssuedAt, now); d.Reason != "" {
		return d
	}
	if d := recordNonce(p, req.From, req.Nonce, req.IssuedAt); d.Reason != "" {
		return d
	}
	if req.From == owner {
		return Decision{Accepted: true, Role: RoleOwner}
	}
	if !isRead(req.Action) {
		return reject(ReasonControlNeedsOwner, req.Action)
	}
	return authorizeRead(p, req, owner, now)
}

func authorizeRead(p Policy, req ControlRequest, owner string, now time.Time) Decision {
	if req.GrantID == nil || p.Grants == nil {
		return reject(ReasonGrantMissing, "grant required")
	}
	s, err := p.Grants.Get(*req.GrantID)
	if err != nil {
		return reject(ReasonGrantMissing, err.Error())
	}
	if d := grantAccess(s, req, now); d.Reason != "" {
		return d
	}
	g := s.Grant
	c, err := CanonicalizeGrant(g)
	if err != nil {
		return reject(ReasonGrantBadSignature, err.Error())
	}
	if g.GrantID != *req.GrantID || g.From != owner {
		return reject(ReasonGrantBadSignature, "grant owner or id mismatch")
	}
	if d := verifySigned(p, g.From, g.KeyEpoch, c, s.Sig, now); d.Reason != "" {
		return reject(ReasonGrantBadSignature, d.Detail)
	}
	return Decision{Accepted: true, Role: RoleGrantee}
}

func grantAccess(s StoredGrant, req ControlRequest, now time.Time) Decision {
	g := s.Grant
	if s.Revoked {
		return reject(ReasonGrantRevoked, g.GrantID)
	}
	if g.Grantee != req.From {
		return reject(ReasonGrantWrongGrantee, g.Grantee)
	}
	if !slices.Contains(g.Keepers, req.Keeper) {
		return reject(ReasonGrantNotForKeeper, req.Keeper)
	}
	if now.Unix() >= g.ExpiresAt {
		return reject(ReasonGrantExpired, g.GrantID)
	}
	scope := ScopeStatus
	if req.Action == ActionReadContext {
		scope = ScopeContext
	}
	if !slices.Contains(g.Scopes, scope) {
		return reject(ReasonGrantScopeMissing, scope)
	}
	return Decision{}
}

// AddGrant authenticates a delivered signed grant and checks every keeper owner.
func AddGrant(p Policy, g Grant, sig []byte) error {
	c, err := CanonicalizeGrant(g)
	if err != nil {
		return err
	}
	now := policyNow(p)
	if d := verifySigned(p, g.From, g.KeyEpoch, c, sig, now); d.Reason != "" {
		return d.err()
	}
	for _, keeper := range g.Keepers {
		owner, ok := p.KeeperOwners[keeper]
		if !ok {
			return reject(ReasonUnknownKeeper, keeper).err()
		}
		if owner != g.From {
			return reject(ReasonControlNeedsOwner, keeper).err()
		}
	}
	if now.Unix() >= g.ExpiresAt {
		return reject(ReasonGrantExpired, g.GrantID).err()
	}
	if p.Grants == nil {
		return reject(ReasonGrantMissing, "grant store required").err()
	}
	return p.Grants.Put(g, sig)
}

// RevokeGrant authenticates, checks freshness, consumes a nonce, then authorizes
// the original issuer. A revocation cannot be undone by reinstalling the grant.
func RevokeGrant(p Policy, r GrantRevoke, sig []byte) error {
	c, err := CanonicalizeGrantRevoke(r)
	if err != nil {
		return err
	}
	now := policyNow(p)
	if d := verifySigned(p, r.From, r.KeyEpoch, c, sig, now); d.Reason != "" {
		return d.err()
	}
	if d := fresh(p, r.IssuedAt, now); d.Reason != "" {
		return d.err()
	}
	if d := recordNonce(p, r.From, r.Nonce, r.IssuedAt); d.Reason != "" {
		return d.err()
	}
	if p.Grants == nil {
		return ErrGrantMissing
	}
	g, err := p.Grants.Get(r.GrantID)
	if err != nil {
		return err
	}
	if g.Grant.From != r.From {
		return reject(ReasonControlNeedsOwner, "only grant issuer may revoke").err()
	}
	return p.Grants.MarkRevoked(r.GrantID)
}
