// Package autorag defines the auto-RAG drift catalog schema —
// the JSON shape consumers use to declare their project-specific
// drift signatures for `auto-rag-guard.sh`.
//
// The bash engine (internal/hookscripts/auto-rag-guard.sh, slice B)
// reads the catalog at runtime via the DRIFT_CATALOG env var. This
// package's Go types pin the same schema so consumers can:
//   - parse a catalog file programmatically
//   - lint a catalog at build time without spawning bash
//   - generate a catalog from typed Go data and serialize back to
//     the JSON shape the engine expects
//
// Origin: the companion project v0.3 stable handoff 2026-05-07. The shim outline
// in the companion project's v0.3 auto-RAG handoff outline showed
// DRIFT_CATALOG as a colon-tier env var; this Go package is the
// schema spec for that JSON.
package autorag

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Signature is one drift-detection rule. Required fields:
//   - Name      stable identifier; used in evaluator reports + sidecar
//     log filenames; must be unique within a catalog.
//   - Tool      the Claude Code PreToolUse tool matcher this rule
//     applies to. Currently supported: Bash, Write, Edit.
//   - Match     regex applied to the tool's target string (Bash
//     command line for Bash; file path for Write/Edit). The bash
//     engine compiles via `grep -E`; we validate via Go's regexp
//     (RE2) which is a strict subset.
//   - Decision  what the engine does on match. inject = warn-and-
//     allow via additionalContext; deny = block via
//     permissionDecision; allow = explicit no-op (rare; for
//     overriding broader signatures). "ask" is rejected at lint
//     time because Claude Code 2.1.129 silently drops it on
//     Write/Edit.
//   - Query     the vaultmind ask query string surfaced in the
//     guidance text and run against the consumer's vault.
//
// JSON field names match the companion project v0.3 shim outline.
type Signature struct {
	Name     string `json:"name"`
	Tool     string `json:"tool"`
	Match    string `json:"match"`
	Decision string `json:"decision"`
	Query    string `json:"query"`
}

// Catalog is a parsed drift-signature catalog. The on-disk JSON
// shape is a top-level array of Signature objects (matching the
// the companion project v0.3 DRIFT_CATALOG export shape).
type Catalog struct {
	Signatures []Signature
}

// allowedTools is the set of Claude Code PreToolUse tool matchers
// the bash engine knows how to dispatch on. Adding a new tool here
// requires engine work — both internal/hookscripts/auto-rag-guard.sh
// (new case branch) and any downstream consumer that filters on
// tool name. Catching unknown tools at lint time prevents shipping
// a catalog whose entries silently never fire.
var allowedTools = map[string]struct{}{
	"Bash":  {},
	"Write": {},
	"Edit":  {},
}

// allowedDecisions is the decisions the bash engine will dispatch.
// "ask" is intentionally absent: Claude Code 2.1.129 silently drops
// permissionDecision=ask on Write/Edit (companion project dogfood finding,
// 2026-05-07), so a catalog entry with Decision=ask would compile
// fine but would never gate the user — exactly the silent-failure
// shape doctor exists to catch.
var allowedDecisions = map[string]struct{}{
	"inject": {},
	"deny":   {},
	"allow":  {},
}

// ParseCatalog parses the on-disk JSON shape. Does NOT call Validate
// — call that separately so callers can choose strict (parse +
// validate) or permissive (parse only) modes.
func ParseCatalog(data []byte) (*Catalog, error) {
	var sigs []Signature
	if err := json.Unmarshal(data, &sigs); err != nil {
		return nil, fmt.Errorf("parse catalog: %w", err)
	}
	return &Catalog{Signatures: sigs}, nil
}

// Validate runs lint checks on every signature and returns the first
// error encountered. Returning the first error (rather than a
// multi-error) keeps the failure surface simple and matches the
// "fail fast on the first invariant break" pattern used across
// vaultmind's other validators (schema, frontmatter).
func (c *Catalog) Validate() error {
	seen := make(map[string]struct{}, len(c.Signatures))
	for i, sig := range c.Signatures {
		if err := validateSignature(sig); err != nil {
			return fmt.Errorf("signature %d (%q): %w", i, sig.Name, err)
		}
		if _, dup := seen[sig.Name]; dup {
			return fmt.Errorf("signature %d: duplicate name %q (names must be unique)", i, sig.Name)
		}
		seen[sig.Name] = struct{}{}
	}
	return nil
}

func validateSignature(sig Signature) error {
	if strings.TrimSpace(sig.Name) == "" {
		return fmt.Errorf("name is required")
	}
	// Bash engine emits results as TAB-separated fields (name<TAB>query<TAB>decision)
	// then `read -r DRIFT QUERY DECISION` parses them back. A TAB inside name
	// or query would corrupt the parse. Reject at lint time so the bash side
	// never has to defend against this shape.
	if strings.ContainsAny(sig.Name, "\t\n") {
		return fmt.Errorf("name must not contain TAB or newline (bash engine uses these as field/record separators)")
	}
	if sig.Tool == "" {
		return fmt.Errorf("tool is required (one of: Bash, Write, Edit)")
	}
	if _, ok := allowedTools[sig.Tool]; !ok {
		return fmt.Errorf("unknown tool %q (allowed: Bash, Write, Edit)", sig.Tool)
	}
	if sig.Match == "" {
		return fmt.Errorf("match (regex) is required")
	}
	if _, err := regexp.Compile(sig.Match); err != nil {
		return fmt.Errorf("invalid regex %q: %w", sig.Match, err)
	}
	if sig.Decision == "" {
		return fmt.Errorf("decision is required (one of: inject, deny, allow)")
	}
	if sig.Decision == "ask" {
		return fmt.Errorf("decision %q is rejected — Claude Code 2.1.129 silently drops `ask` on Write/Edit; use `deny` for hard gates", sig.Decision)
	}
	if _, ok := allowedDecisions[sig.Decision]; !ok {
		return fmt.Errorf("unknown decision %q (allowed: inject, deny, allow)", sig.Decision)
	}
	if strings.TrimSpace(sig.Query) == "" {
		return fmt.Errorf("query is required (the vaultmind ask query string)")
	}
	if strings.ContainsAny(sig.Query, "\t\n") {
		return fmt.Errorf("query must not contain TAB or newline (bash engine uses these as field/record separators)")
	}
	return nil
}
