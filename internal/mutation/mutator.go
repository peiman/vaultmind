package mutation

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/peiman/vaultmind/internal/git"
	"github.com/peiman/vaultmind/internal/schema"
	"gopkg.in/yaml.v3"
)

// Mutator orchestrates the 7-step mutation pipeline:
// resolve target -> read file -> validate -> compute change -> generate diff -> atomic write -> post-write.
type Mutator struct {
	VaultPath string
	Detector  git.RepoStateDetector
	Checker   *git.PolicyChecker
	Committer *git.Committer
	Registry  *schema.Registry
}

// Run executes the mutation pipeline for the given request.
func (m *Mutator) Run(req MutationRequest) (*MutationResult, error) {
	// Step 1: Resolve target
	relPath, noteInfo, err := m.resolveTarget(req.Target)
	if err != nil {
		return nil, err
	}
	absPath := filepath.Clean(filepath.Join(m.VaultPath, relPath))

	// Step 2: Read file
	raw, err := os.ReadFile(absPath)
	if err != nil {
		return nil, &MutationError{Code: "read_error", Message: fmt.Sprintf("reading %s: %v", relPath, err)}
	}
	preHash := fileHash(raw)
	lineEnding := DetectLineEnding(raw)

	doc, bodyOffset, err := ParseFrontmatterNode(raw)
	if err != nil {
		return nil, &MutationError{Code: "parse_error", Message: fmt.Sprintf("parsing frontmatter: %v", err)}
	}

	// R1: Guard against empty or non-mapping frontmatter.
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, &MutationError{Code: "parse_error", Message: "frontmatter produced empty document node"}
	}
	mapping := doc.Content[0]
	if mapping.Kind != yaml.MappingNode {
		return nil, &MutationError{Code: "parse_error", Message: "frontmatter is not a YAML mapping"}
	}

	// Step 3: Validate
	if err := ValidateMutation(req, noteInfo, m.Registry); err != nil {
		return nil, err
	}

	// Step 4: Compute change
	result := &MutationResult{
		Path:      relPath,
		ID:        noteInfo.ID,
		Operation: req.Op.String(),
		DryRun:    req.DryRun,
		Warnings:  []PolicyWarning{}, // AX3: Always emit warnings array, never null.
	}

	if err := applyOperation(req, mapping, result); err != nil {
		return nil, err
	}

	// Step 5: Generate diff
	newFM, err := SerializeFrontmatter(doc, lineEnding)
	if err != nil {
		return nil, &MutationError{Code: "serialize_error", Message: fmt.Sprintf("serializing frontmatter: %v", err)}
	}
	newContent := SpliceFile(raw, newFM, bodyOffset)

	if req.Diff || req.DryRun {
		diff := GenerateDiff(relPath, string(raw), string(newContent))
		result.Diff = diff
	}

	if req.DryRun {
		result.Git = m.gitInfo(relPath)
		return result, nil
	}

	// Step 6: Check git policy and atomic write
	warnings, err := m.checkGitPolicy(req.Commit, relPath)
	if err != nil {
		return nil, err
	}
	result.Warnings = append(result.Warnings, warnings...)

	if err := atomicWrite(absPath, relPath, newContent, preHash); err != nil {
		return nil, err
	}

	result.WriteHash = fileHash(newContent)
	result.Git = m.gitInfo(relPath)

	// Step 7: Post-write — commit if requested
	if req.Commit && m.Committer != nil {
		msg := CommitMessage(req, noteInfo.ID)
		sha, err := m.Committer.CommitFiles(m.VaultPath, []string{relPath}, msg)
		if err != nil {
			return nil, &MutationError{Code: "commit_error", Message: fmt.Sprintf("committing: %v", err)}
		}
		result.Git.CommitSHA = sha
	}

	result.ReindexRequired = true
	return result, nil
}

// applyOperation dispatches the mutation operation on the YAML mapping and populates
// the result fields (Key, OldValue, NewValue, Warnings).
func applyOperation(req MutationRequest, mapping *yaml.Node, result *MutationResult) error {
	switch req.Op {
	case OpSet:
		result.Key = req.Key
		result.OldValue = getNodeValue(mapping, req.Key)
		if err := SetKey(mapping, req.Key, req.Value); err != nil {
			return &MutationError{Code: "set_error", Message: fmt.Sprintf("setting key %q: %v", req.Key, err)}
		}
		result.NewValue = req.Value

	case OpUnset:
		result.Key = req.Key
		result.OldValue = getNodeValue(mapping, req.Key)
		removed := UnsetKey(mapping, req.Key)
		if !removed {
			result.Warnings = append(result.Warnings, PolicyWarning{
				Rule:    "key_not_found",
				Message: fmt.Sprintf("key %q was not present in frontmatter", req.Key),
			})
		}

	case OpMerge:
		for key, value := range req.Fields {
			if err := SetKey(mapping, key, value); err != nil {
				return &MutationError{Code: "merge_error", Message: fmt.Sprintf("setting key %q: %v", key, err)}
			}
		}

	case OpNormalize:
		SortKeys(mapping)
		ScalarToList(mapping, "aliases")
		ScalarToList(mapping, "tags")
		NormalizeDates(mapping, req.StripTime)
		SnakeCaseKeys(mapping)
	}
	return nil
}

// atomicWrite performs conflict detection, writes content to a temp file, renames
// it into place, and restores the original file permissions.
func atomicWrite(absPath, relPath string, newContent []byte, preHash string) error {
	// Conflict detection: re-read file and verify hash unchanged.
	// absPath is validated against path traversal in resolveTarget before reaching here.
	reread, err := os.ReadFile(absPath) //nolint:gosec // path validated in resolveTarget
	if err != nil {
		return &MutationError{Code: "read_error", Message: fmt.Sprintf("re-reading %s for conflict check: %v", relPath, err)}
	}
	if fileHash(reread) != preHash {
		return &MutationError{Code: "conflict", Message: fmt.Sprintf("file %s was modified concurrently", relPath)}
	}

	// S2: Preserve file permissions during atomic write.
	origInfo, err := os.Stat(absPath)
	if err != nil {
		return &MutationError{Code: "write_error", Message: fmt.Sprintf("stat original: %v", err)}
	}

	dir := filepath.Dir(absPath)
	tmp, err := os.CreateTemp(dir, ".vaultmind-*.tmp")
	if err != nil {
		return &MutationError{Code: "write_error", Message: fmt.Sprintf("creating temp file: %v", err)}
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(newContent); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return &MutationError{Code: "write_error", Message: fmt.Sprintf("writing temp file: %v", err)}
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return &MutationError{Code: "write_error", Message: fmt.Sprintf("closing temp file: %v", err)}
	}
	if err := os.Rename(tmpName, absPath); err != nil {
		_ = os.Remove(tmpName)
		return &MutationError{Code: "write_error", Message: fmt.Sprintf("renaming temp file: %v", err)}
	}
	if err := os.Chmod(absPath, origInfo.Mode().Perm()); err != nil {
		return &MutationError{Code: "write_error", Message: fmt.Sprintf("restoring permissions: %v", err)}
	}
	return nil
}

// resolveTarget handles path-based targets (contains "/" or ends in ".md").
// For non-path targets (bare id/alias), returns unresolved_target for now.
func (m *Mutator) resolveTarget(target string) (string, ParsedNoteInfo, error) {
	if !strings.Contains(target, "/") && !strings.HasSuffix(target, ".md") {
		return "", ParsedNoteInfo{}, &MutationError{
			Code:    "unresolved_target",
			Message: fmt.Sprintf("cannot resolve target %q: entity resolution not yet available", target),
		}
	}

	absPath := filepath.Clean(filepath.Join(m.VaultPath, target))

	// S1: Validate resolved path stays within vault directory.
	cleanVault := filepath.Clean(m.VaultPath)
	cleanAbs := filepath.Clean(absPath)
	if !strings.HasPrefix(cleanAbs, cleanVault+string(filepath.Separator)) && cleanAbs != cleanVault {
		return "", ParsedNoteInfo{}, &MutationError{
			Code:    "path_traversal",
			Message: fmt.Sprintf("target path %q escapes vault directory", target),
		}
	}

	if _, err := os.Stat(absPath); err != nil {
		return "", ParsedNoteInfo{}, &MutationError{
			Code:    "unresolved_target",
			Message: fmt.Sprintf("file not found: %s", target),
		}
	}

	raw, err := os.ReadFile(absPath)
	if err != nil {
		return "", ParsedNoteInfo{}, &MutationError{
			Code:    "read_error",
			Message: fmt.Sprintf("reading %s: %v", target, err),
		}
	}

	doc, _, err := ParseFrontmatterNode(raw)
	if err != nil {
		return "", ParsedNoteInfo{}, &MutationError{
			Code:    "parse_error",
			Message: fmt.Sprintf("parsing frontmatter of %s: %v", target, err),
		}
	}

	// R1: Guard against empty or non-mapping frontmatter in resolveTarget.
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return "", ParsedNoteInfo{}, &MutationError{
			Code:    "parse_error",
			Message: fmt.Sprintf("frontmatter of %s produced empty document node", target),
		}
	}
	mapping := doc.Content[0]
	if mapping.Kind != yaml.MappingNode {
		return "", ParsedNoteInfo{}, &MutationError{
			Code:    "parse_error",
			Message: fmt.Sprintf("frontmatter of %s is not a YAML mapping", target),
		}
	}

	info := extractNoteInfo(mapping)
	return target, info, nil
}

// extractNoteInfo builds a ParsedNoteInfo from a yaml.Node mapping.
func extractNoteInfo(mapping *yaml.Node) ParsedNoteInfo {
	info := ParsedNoteInfo{}
	if mapping.Kind != yaml.MappingNode {
		return info
	}
	for i := 0; i < len(mapping.Content)-1; i += 2 {
		key := mapping.Content[i].Value
		info.Keys = append(info.Keys, key)
		switch key {
		case "id":
			info.ID = mapping.Content[i+1].Value
		case "type":
			info.Type = mapping.Content[i+1].Value
		}
	}
	if info.ID != "" && info.Type != "" {
		info.IsDomain = true
	}
	return info
}

// getNodeValue extracts the scalar string value of a key from a mapping, or nil if not found.
func getNodeValue(mapping *yaml.Node, key string) interface{} {
	if mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(mapping.Content)-1; i += 2 {
		if mapping.Content[i].Value == key {
			valNode := mapping.Content[i+1]
			if valNode.Kind == yaml.ScalarNode {
				return valNode.Value
			}
			// For non-scalar values, return a description.
			return nil
		}
	}
	return nil
}

// checkGitPolicy detects the repo state, evaluates the policy matrix for the
// given operation, and returns any policy warnings. Returns a *MutationError
// on Refuse or detection failure.
func (m *Mutator) checkGitPolicy(commit bool, relPath string) ([]PolicyWarning, error) {
	state, err := m.Detector.Detect(m.VaultPath)
	if err != nil {
		return nil, &MutationError{Code: "git_detect_error", Message: fmt.Sprintf("detecting git state: %v", err)}
	}

	op := git.OpWrite
	if commit {
		op = git.OpWriteCommit
	}
	policyResult := m.Checker.Check(state, op, relPath)
	if policyResult.Decision == git.Refuse {
		reason := "git policy refused"
		if len(policyResult.Reasons) > 0 {
			reason = policyResult.Reasons[0].Rule
		}
		return nil, &MutationError{Code: reason, Message: fmt.Sprintf("git policy refuses %s on %s", op, relPath)}
	}

	var warnings []PolicyWarning
	if policyResult.Decision == git.Warn {
		for _, r := range policyResult.Reasons {
			warnings = append(warnings, PolicyWarning{Rule: r.Rule, Message: r.Message})
		}
	}
	return warnings, nil
}

// gitInfo builds a GitInfo from the detector state.
func (m *Mutator) gitInfo(targetPath string) GitInfo {
	state, err := m.Detector.Detect(m.VaultPath)
	if err != nil {
		return GitInfo{}
	}
	info := GitInfo{
		RepoDetected:     state.RepoDetected,
		WorkingTreeClean: state.WorkingTreeClean,
		TargetFileClean:  true,
	}
	for _, f := range state.StagedFiles {
		if f == targetPath {
			info.TargetFileClean = false
			break
		}
	}
	if info.TargetFileClean {
		for _, f := range state.UnstagedFiles {
			if f == targetPath {
				info.TargetFileClean = false
				break
			}
		}
	}
	return info
}

// fileHash computes the SHA-256 hex digest of data.
func fileHash(data []byte) string {
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h)
}

// CommitMessage generates a structured commit message based on the operation.
func CommitMessage(req MutationRequest, id string) string {
	switch req.Op {
	case OpSet:
		return fmt.Sprintf("vaultmind: frontmatter set %s %s=%v \u2014 updated %s", id, req.Key, req.Value, req.Key)
	case OpUnset:
		return fmt.Sprintf("vaultmind: frontmatter unset %s %s \u2014 removed %s", id, req.Key, req.Key)
	case OpMerge:
		keys := make([]string, 0, len(req.Fields))
		for k := range req.Fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return fmt.Sprintf("vaultmind: frontmatter merge %s \u2014 updated %s", id, strings.Join(keys, ", "))
	case OpNormalize:
		return fmt.Sprintf("vaultmind: frontmatter normalize %s \u2014 reformatted frontmatter", id)
	default:
		return fmt.Sprintf("vaultmind: mutation on %s", id)
	}
}
