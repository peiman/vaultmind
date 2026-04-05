package git

import (
	"fmt"
	"slices"

	"github.com/peiman/vaultmind/internal/vault"
)

// Known rule names for the policy matrix.
var knownRules = []string{
	"dirty_unrelated",
	"dirty_target",
	"detached_head",
	"merge_in_progress",
	"no_repo",
}

// policyRule defines one row of the policy matrix.
type policyRule struct {
	name      string
	condition func(state RepoState, targetPath string) bool
	defaults  [4]PolicyDecision // indexed by OperationType: Read, DryRun, Write, WriteCommit
}

// commitGuardRules are rules where WriteCommit is always Refuse regardless of overrides.
var commitGuardRules = map[string]bool{
	"detached_head": true,
	"no_repo":       true,
}

// defaultRules returns the SRS policy matrix as rules.
func defaultRules() []policyRule {
	return []policyRule{
		{
			name: "dirty_unrelated",
			condition: func(state RepoState, targetPath string) bool {
				if !state.RepoDetected || state.WorkingTreeClean || targetPath == "" {
					return false
				}
				// Dirty, but target is NOT among dirty files
				return !slices.Contains(state.StagedFiles, targetPath) &&
					!slices.Contains(state.UnstagedFiles, targetPath)
			},
			defaults: [4]PolicyDecision{Allow, Allow, Warn, Warn},
		},
		{
			name: "dirty_target",
			condition: func(state RepoState, targetPath string) bool {
				if !state.RepoDetected || targetPath == "" {
					return false
				}
				return slices.Contains(state.StagedFiles, targetPath) ||
					slices.Contains(state.UnstagedFiles, targetPath)
			},
			defaults: [4]PolicyDecision{Allow, Allow, Refuse, Refuse},
		},
		{
			name: "detached_head",
			condition: func(state RepoState, _ string) bool {
				return state.RepoDetected && state.Detached
			},
			defaults: [4]PolicyDecision{Allow, Allow, Warn, Refuse},
		},
		{
			name: "merge_in_progress",
			condition: func(state RepoState, _ string) bool {
				return state.MergeInProgress || state.RebaseInProgress
			},
			defaults: [4]PolicyDecision{Allow, Allow, Refuse, Refuse},
		},
		{
			name: "no_repo",
			condition: func(state RepoState, _ string) bool {
				return !state.RepoDetected
			},
			defaults: [4]PolicyDecision{Warn, Warn, Warn, Refuse},
		},
	}
}

// PolicyChecker evaluates the git policy matrix.
type PolicyChecker struct {
	rules     []policyRule
	overrides map[string]PolicyDecision
}

// NewPolicyChecker creates a PolicyChecker from config.
// Returns error if config contains invalid policy values or unknown rule names.
func NewPolicyChecker(cfg vault.GitPolicyConfig) (*PolicyChecker, error) {
	overrides := make(map[string]PolicyDecision)

	for ruleName, value := range cfg.Policy {
		if !slices.Contains(knownRules, ruleName) {
			return nil, fmt.Errorf("unknown git policy rule: %q (valid: %v)", ruleName, knownRules)
		}
		d, err := ParsePolicyDecision(value)
		if err != nil {
			return nil, fmt.Errorf("git policy %q: %w", ruleName, err)
		}
		overrides[ruleName] = d
	}

	return &PolicyChecker{
		rules:     defaultRules(),
		overrides: overrides,
	}, nil
}

// Check evaluates all policy rules for the given state and operation.
// targetPath identifies the file being mutated (empty for read-only operations).
func (pc *PolicyChecker) Check(state RepoState, op OperationType, targetPath string) PolicyResult {
	if op < OpRead || op > OpWriteCommit {
		return PolicyResult{
			Decision: Refuse,
			Reasons:  []PolicyReason{{Rule: "invalid_operation", Message: fmt.Sprintf("unknown operation type: %d", op)}},
		}
	}

	result := PolicyResult{Decision: Allow}

	for _, rule := range pc.rules {
		if !rule.condition(state, targetPath) {
			continue
		}

		decision := rule.defaults[op]

		// Apply override for Write column
		if override, ok := pc.overrides[rule.name]; ok && (op == OpWrite || op == OpWriteCommit) {
			decision = override
			// WriteCommit inherits from override, but commit guards are not overridable
			if op == OpWriteCommit && commitGuardRules[rule.name] {
				decision = Refuse
			}
		}

		reason := PolicyReason{
			Rule:    rule.name,
			Message: fmt.Sprintf("git policy %q triggers %s for %s operation", rule.name, decision, op),
		}
		result.Reasons = append(result.Reasons, reason)

		if decision > result.Decision {
			result.Decision = decision
		}
	}

	return result
}
