package mutation

import "fmt"

// OpType identifies the mutation operation.
type OpType int

const (
	OpSet       OpType = iota // set a single key=value
	OpUnset                   // remove a key
	OpMerge                   // merge multiple key=value pairs
	OpNormalize               // reformat frontmatter
)

func (o OpType) String() string {
	switch o {
	case OpSet:
		return "set"
	case OpUnset:
		return "unset"
	case OpMerge:
		return "merge"
	case OpNormalize:
		return "normalize"
	default:
		return fmt.Sprintf("unknown(%d)", int(o))
	}
}

// MutationRequest describes what to mutate.
type MutationRequest struct {
	Op         OpType
	Target     string
	Key        string
	Value      interface{}
	Fields     map[string]interface{}
	DryRun     bool
	Diff       bool
	Commit     bool
	AllowExtra bool
	StripTime  bool
}

// MutationResult is the JSON response for all mutation commands.
type MutationResult struct {
	Path            string          `json:"path"`
	ID              string          `json:"id"`
	Operation       string          `json:"operation"`
	Key             string          `json:"key,omitempty"`
	OldValue        interface{}     `json:"old_value,omitempty"`
	NewValue        interface{}     `json:"new_value,omitempty"`
	DryRun          bool            `json:"dry_run"`
	Diff            string          `json:"diff,omitempty"`
	WriteHash       string          `json:"write_hash,omitempty"`
	Git             GitInfo         `json:"git"`
	ReindexRequired bool            `json:"reindex_required"`
	Warnings        []PolicyWarning `json:"warnings,omitempty"`
}

// PolicyWarning describes a git policy warning that was triggered but didn't block.
type PolicyWarning struct {
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

// GitInfo reports git state in mutation responses.
type GitInfo struct {
	RepoDetected     bool   `json:"repo_detected"`
	WorkingTreeClean bool   `json:"working_tree_clean"`
	TargetFileClean  bool   `json:"target_file_clean"`
	CommitSHA        string `json:"commit_sha,omitempty"`
}

// ParsedNoteInfo holds the minimal note info needed for validation.
type ParsedNoteInfo struct {
	ID       string
	Type     string
	IsDomain bool
	Keys     []string
}

// MutationError is a structured error with an SRS error code.
type MutationError struct {
	Code    string
	Message string
	Field   string
}

func (e *MutationError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("%s: %s (field: %s)", e.Code, e.Message, e.Field)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}
