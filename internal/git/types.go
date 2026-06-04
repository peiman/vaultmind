package git

import (
	"fmt"
	"strings"
)

// OperationType classifies the caller's intended action.
type OperationType int

const (
	OpRead        OperationType = iota // query commands
	OpDryRun                           // --dry-run / --diff
	OpWrite                            // apply mutation to disk
	OpWriteCommit                      // apply + git commit
)

func (o OperationType) String() string {
	switch o {
	case OpRead:
		return "read"
	case OpDryRun:
		return "dry_run"
	case OpWrite:
		return "write"
	case OpWriteCommit:
		return "write_commit"
	default:
		return fmt.Sprintf("unknown(%d)", int(o))
	}
}

// PolicyDecision is the outcome of a policy rule evaluation.
type PolicyDecision int

const (
	Allow PolicyDecision = iota
	Warn
	Refuse
)

func (d PolicyDecision) String() string {
	switch d {
	case Allow:
		return "allow"
	case Warn:
		return "warn"
	case Refuse:
		return "refuse"
	default:
		return fmt.Sprintf("unknown(%d)", int(d))
	}
}

// ParsePolicyDecision parses a string into a PolicyDecision.
func ParsePolicyDecision(s string) (PolicyDecision, error) {
	switch strings.ToLower(s) {
	case "allow":
		return Allow, nil
	case "warn":
		return Warn, nil
	case "refuse":
		return Refuse, nil
	default:
		return 0, fmt.Errorf("invalid policy decision: %q (valid: allow, warn, refuse)", s)
	}
}

// PolicyResult is the aggregate outcome of a policy check.
type PolicyResult struct {
	Decision PolicyDecision
	Reasons  []PolicyReason
}

// PolicyReason describes one triggered policy rule.
type PolicyReason struct {
	Rule    string // stable identifier: "dirty_target", "detached_head", etc.
	Message string
}

// RepoState captures git repository state at a point in time.
type RepoState struct {
	RepoDetected     bool
	Branch           string
	Detached         bool
	MergeInProgress  bool
	RebaseInProgress bool
	WorkingTreeClean bool
	StagedFiles      []string
	UnstagedFiles    []string
	UntrackedFiles   []string
}
