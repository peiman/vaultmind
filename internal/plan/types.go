package plan

type Plan struct {
	Version     int         `json:"version"`
	Description string      `json:"description"`
	Operations  []Operation `json:"operations"`
}

type Operation struct {
	Op          string                 `json:"op"`
	Target      string                 `json:"target,omitempty"`
	Key         string                 `json:"key,omitempty"`
	Value       interface{}            `json:"value,omitempty"`
	Fields      map[string]interface{} `json:"fields,omitempty"`
	SectionKey  string                 `json:"section_key,omitempty"`
	Template    string                 `json:"template,omitempty"`
	Path        string                 `json:"path,omitempty"`
	Type        string                 `json:"type,omitempty"`
	Frontmatter map[string]interface{} `json:"frontmatter,omitempty"`
	Body        string                 `json:"body,omitempty"`
}

type OpResult struct {
	Op        string   `json:"op"`
	Target    string   `json:"target,omitempty"`
	Path      string   `json:"path,omitempty"`
	ID        string   `json:"id,omitempty"`
	Status    string   `json:"status"`
	WriteHash string   `json:"write_hash,omitempty"`
	Error     *OpError `json:"error,omitempty"`
}

type OpError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ApplyResult struct {
	PlanDescription     string     `json:"plan_description"`
	OperationsTotal     int        `json:"operations_total"`
	OperationsCompleted int        `json:"operations_completed"`
	Operations          []OpResult `json:"operations"`
	Committed           bool       `json:"committed"`
	CommitSHA           string     `json:"commit_sha,omitempty"`
}

const (
	OpFrontmatterSet   = "frontmatter_set"
	OpFrontmatterUnset = "frontmatter_unset"
	OpFrontmatterMerge = "frontmatter_merge"
	OpGeneratedRegion  = "generated_region_render"
	OpNoteCreate       = "note_create"
)

var KnownOps = map[string]bool{
	OpFrontmatterSet: true, OpFrontmatterUnset: true, OpFrontmatterMerge: true,
	OpGeneratedRegion: true, OpNoteCreate: true,
}
