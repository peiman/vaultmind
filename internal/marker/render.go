package marker

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/peiman/vaultmind/internal/git"
	"github.com/peiman/vaultmind/internal/mutation"
	"gopkg.in/yaml.v3"
)

// RenderConfig configures the RenderRegion orchestrator.
type RenderConfig struct {
	VaultPath  string
	Target     string
	SectionKey string
	DryRun     bool
	Diff       bool
	Commit     bool
	Force      bool
	Detector   git.RepoStateDetector
	Checker    *git.PolicyChecker
	Committer  *git.Committer
}

// RenderResult is the response from RenderRegion.
type RenderResult struct {
	Path            string                   `json:"path"`
	ID              string                   `json:"id"`
	SectionKey      string                   `json:"section_key"`
	Operation       string                   `json:"operation"`
	DryRun          bool                     `json:"dry_run"`
	Diff            string                   `json:"diff,omitempty"`
	WriteHash       string                   `json:"write_hash,omitempty"`
	Git             mutation.GitInfo         `json:"git"`
	ReindexRequired bool                     `json:"reindex_required"`
	Warnings        []mutation.PolicyWarning `json:"warnings"`
}

// RenderRegion orchestrates the dataview region rendering pipeline:
//  1. Read file from vault
//  2. Extract note type from frontmatter
//  3. Load section template from .vaultmind/sections/{type}/{key}.md
//  4. Call ReplaceRegion to splice content
//  5. Generate diff if requested
//  6. If dry-run, return early
//  7. Git policy check
//  8. Conflict detection (re-read + hash compare)
//  9. Atomic write (temp + rename + chmod)
//  10. Return result
func RenderRegion(cfg RenderConfig) (*RenderResult, error) {
	// Step 1: Read file
	absPath := filepath.Clean(filepath.Join(cfg.VaultPath, cfg.Target))

	// Vault boundary check (I3: path traversal prevention)
	cleanVault := filepath.Clean(cfg.VaultPath)
	cleanAbs := filepath.Clean(absPath)
	if !strings.HasPrefix(cleanAbs, cleanVault+string(filepath.Separator)) && cleanAbs != cleanVault {
		return nil, &mutation.MutationError{
			Code:    "path_traversal",
			Message: fmt.Sprintf("target path %q escapes vault directory", cfg.Target),
		}
	}

	// Return unresolved_target if file does not exist
	if _, err := os.Stat(absPath); err != nil {
		return nil, &mutation.MutationError{
			Code:    "unresolved_target",
			Message: fmt.Sprintf("file not found: %s", cfg.Target),
		}
	}

	raw, err := os.ReadFile(absPath) //nolint:gosec // path validated by vault boundary check above
	if err != nil {
		return nil, &mutation.MutationError{
			Code:    "read_error",
			Message: fmt.Sprintf("reading %s: %v", cfg.Target, err),
		}
	}
	preHash := renderFileHash(raw)

	// Step 2: Extract note type from frontmatter
	noteType, noteID, err := extractNoteType(raw)
	if err != nil {
		return nil, err
	}

	// I1: when SectionKey is empty, render all sections found in the file.
	if cfg.SectionKey == "" {
		return renderAllSections(cfg, absPath, raw, preHash, noteType, noteID)
	}

	// Step 3: Load section template
	templateContent, err := LoadSectionTemplate(cfg.VaultPath, noteType, cfg.SectionKey)
	if err != nil {
		return nil, err
	}

	// Step 4: Call ReplaceRegion to splice content
	newContent, err := ReplaceRegion(raw, cfg.SectionKey, templateContent, cfg.Force)
	if err != nil {
		return nil, err
	}

	result := &RenderResult{
		Path:       cfg.Target,
		ID:         noteID,
		SectionKey: cfg.SectionKey,
		Operation:  "dataview_render",
		DryRun:     cfg.DryRun,
		Warnings:   []mutation.PolicyWarning{},
	}

	// Step 5: Generate diff if requested
	if cfg.Diff || cfg.DryRun {
		result.Diff = mutation.GenerateDiff(cfg.Target, string(raw), string(newContent))
	}

	// Step 6: If dry-run, return early
	if cfg.DryRun {
		result.Git = renderGitInfo(cfg)
		return result, nil
	}

	// Step 7: Git policy check
	warnings, err := renderCheckGitPolicy(cfg)
	if err != nil {
		return nil, err
	}
	result.Warnings = append(result.Warnings, warnings...)

	// Step 8: Conflict detection — re-read + hash compare
	reread, err := os.ReadFile(absPath) //nolint:gosec // path validated above
	if err != nil {
		return nil, &mutation.MutationError{
			Code:    "read_error",
			Message: fmt.Sprintf("re-reading %s for conflict check: %v", cfg.Target, err),
		}
	}
	if renderFileHash(reread) != preHash {
		return nil, &mutation.MutationError{
			Code:    "conflict",
			Message: fmt.Sprintf("file %s was modified concurrently", cfg.Target),
		}
	}

	// Step 9: Atomic write
	if err := renderAtomicWrite(absPath, cfg.Target, newContent); err != nil {
		return nil, err
	}

	// Step 10: Return result
	result.WriteHash = renderFileHash(newContent)
	result.Git = renderGitInfo(cfg)

	if cfg.Commit && cfg.Committer != nil {
		msg := fmt.Sprintf("vaultmind: render %s section %s — updated dataview region", noteID, cfg.SectionKey)
		sha, err := cfg.Committer.CommitFiles(cfg.VaultPath, []string{cfg.Target}, msg)
		if err != nil {
			return nil, &mutation.MutationError{
				Code:    "commit_error",
				Message: fmt.Sprintf("committing: %v", err),
			}
		}
		result.Git.CommitSHA = sha
	}

	result.ReindexRequired = true
	return result, nil
}

// renderAllSections renders every VAULTMIND marker found in raw, applying their
// section templates in sequence. It returns a single RenderResult whose
// SectionKey is set to the first marker's key, or "all" when no markers exist.
func renderAllSections(cfg RenderConfig, absPath string, raw []byte, preHash, noteType, noteID string) (*RenderResult, error) {
	markers, err := FindMarkers(raw)
	if err != nil {
		return nil, &mutation.MutationError{
			Code:    "parse_error",
			Message: fmt.Sprintf("finding markers: %v", err),
		}
	}

	firstKey := "all"
	if len(markers) > 0 {
		firstKey = markers[0].SectionKey
	}

	result := &RenderResult{
		Path:       cfg.Target,
		ID:         noteID,
		SectionKey: firstKey,
		Operation:  "dataview_render",
		DryRun:     cfg.DryRun,
		Warnings:   []mutation.PolicyWarning{},
	}

	current := raw
	for _, m := range markers {
		templateContent, err := LoadSectionTemplate(cfg.VaultPath, noteType, m.SectionKey)
		if err != nil {
			return nil, err
		}
		current, err = ReplaceRegion(current, m.SectionKey, templateContent, cfg.Force)
		if err != nil {
			return nil, err
		}
	}

	// Step 5: Generate diff if requested
	if cfg.Diff || cfg.DryRun {
		result.Diff = mutation.GenerateDiff(cfg.Target, string(raw), string(current))
	}

	// Step 6: If dry-run, return early
	if cfg.DryRun {
		result.Git = renderGitInfo(cfg)
		return result, nil
	}

	// Step 7: Git policy check
	warnings, err := renderCheckGitPolicy(cfg)
	if err != nil {
		return nil, err
	}
	result.Warnings = append(result.Warnings, warnings...)

	// Step 8: Conflict detection — re-read + hash compare
	reread, err := os.ReadFile(absPath) //nolint:gosec // path validated by caller
	if err != nil {
		return nil, &mutation.MutationError{
			Code:    "read_error",
			Message: fmt.Sprintf("re-reading %s for conflict check: %v", cfg.Target, err),
		}
	}
	if renderFileHash(reread) != preHash {
		return nil, &mutation.MutationError{
			Code:    "conflict",
			Message: fmt.Sprintf("file %s was modified concurrently", cfg.Target),
		}
	}

	// Step 9: Atomic write
	if err := renderAtomicWrite(absPath, cfg.Target, current); err != nil {
		return nil, err
	}

	// Step 10: Return result
	result.WriteHash = renderFileHash(current)
	result.Git = renderGitInfo(cfg)

	if cfg.Commit && cfg.Committer != nil {
		msg := fmt.Sprintf("vaultmind: render %s all sections — updated dataview regions", noteID)
		sha, err := cfg.Committer.CommitFiles(cfg.VaultPath, []string{cfg.Target}, msg)
		if err != nil {
			return nil, &mutation.MutationError{
				Code:    "commit_error",
				Message: fmt.Sprintf("committing: %v", err),
			}
		}
		result.Git.CommitSHA = sha
	}

	result.ReindexRequired = true
	return result, nil
}

// LoadSectionTemplate reads the section template file from
// .vaultmind/sections/{noteType}/{sectionKey}.md inside the vault.
// Returns MutationError{Code: "template_not_found"} if the file is absent.
func LoadSectionTemplate(vaultPath, noteType, sectionKey string) ([]byte, error) {
	tmplPath := filepath.Join(vaultPath, ".vaultmind", "sections", noteType, sectionKey+".md")
	data, err := os.ReadFile(tmplPath) //nolint:gosec // path is constructed from trusted vault path + config values
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &mutation.MutationError{
				Code:    "template_not_found",
				Message: fmt.Sprintf("section template %q not found for type %q", sectionKey, noteType),
				Field:   sectionKey,
			}
		}
		return nil, &mutation.MutationError{
			Code:    "read_error",
			Message: fmt.Sprintf("reading template %s: %v", tmplPath, err),
		}
	}
	return data, nil
}

// frontmatterFields holds the minimal frontmatter fields for rendering.
type frontmatterFields struct {
	ID   string `yaml:"id"`
	Type string `yaml:"type"`
}

// extractNoteType parses the frontmatter YAML of raw file bytes to get the
// note type and id. Returns an error if frontmatter is missing or type is absent.
func extractNoteType(raw []byte) (noteType, noteID string, err error) {
	// Find opening ---
	if !strings.HasPrefix(string(raw), "---") {
		return "", "", &mutation.MutationError{
			Code:    "parse_error",
			Message: "no frontmatter: file does not start with ---",
		}
	}

	// Find closing ---
	firstNewline := strings.Index(string(raw), "\n")
	if firstNewline < 0 {
		return "", "", &mutation.MutationError{
			Code:    "parse_error",
			Message: "no frontmatter: no newline after opening ---",
		}
	}
	rest := string(raw[firstNewline+1:])
	closeIdx := strings.Index(rest, "\n---")
	if closeIdx < 0 {
		return "", "", &mutation.MutationError{
			Code:    "parse_error",
			Message: "no frontmatter: closing --- not found",
		}
	}
	yamlContent := rest[:closeIdx]

	var fields frontmatterFields
	if err := yaml.Unmarshal([]byte(yamlContent), &fields); err != nil {
		return "", "", &mutation.MutationError{
			Code:    "parse_error",
			Message: fmt.Sprintf("invalid frontmatter YAML: %v", err),
		}
	}

	if fields.Type == "" {
		return "", "", &mutation.MutationError{
			Code:    "parse_error",
			Message: "frontmatter missing required field: type",
		}
	}

	return fields.Type, fields.ID, nil
}

// renderCheckGitPolicy evaluates the git policy for a write operation.
func renderCheckGitPolicy(cfg RenderConfig) ([]mutation.PolicyWarning, error) {
	if cfg.Detector == nil || cfg.Checker == nil {
		return nil, nil
	}

	state, err := cfg.Detector.Detect(cfg.VaultPath)
	if err != nil {
		return nil, &mutation.MutationError{
			Code:    "git_detect_error",
			Message: fmt.Sprintf("detecting git state: %v", err),
		}
	}

	op := git.OpWrite
	if cfg.Commit {
		op = git.OpWriteCommit
	}

	policyResult := cfg.Checker.Check(state, op, cfg.Target)
	if policyResult.Decision == git.Refuse {
		reason := "git policy refused"
		if len(policyResult.Reasons) > 0 {
			reason = policyResult.Reasons[0].Rule
		}
		return nil, &mutation.MutationError{
			Code:    reason,
			Message: fmt.Sprintf("git policy refuses %s on %s", op, cfg.Target),
		}
	}

	var warnings []mutation.PolicyWarning
	if policyResult.Decision == git.Warn {
		for _, r := range policyResult.Reasons {
			warnings = append(warnings, mutation.PolicyWarning{Rule: r.Rule, Message: r.Message})
		}
	}
	return warnings, nil
}

// renderGitInfo builds a GitInfo from the detector state.
func renderGitInfo(cfg RenderConfig) mutation.GitInfo {
	if cfg.Detector == nil {
		return mutation.GitInfo{}
	}
	state, err := cfg.Detector.Detect(cfg.VaultPath)
	if err != nil {
		return mutation.GitInfo{}
	}
	info := mutation.GitInfo{
		RepoDetected:     state.RepoDetected,
		WorkingTreeClean: state.WorkingTreeClean,
		TargetFileClean:  true,
	}
	for _, f := range state.StagedFiles {
		if f == cfg.Target {
			info.TargetFileClean = false
			break
		}
	}
	if info.TargetFileClean {
		for _, f := range state.UnstagedFiles {
			if f == cfg.Target {
				info.TargetFileClean = false
				break
			}
		}
	}
	return info
}

// renderAtomicWrite performs an atomic file write: preserve permissions,
// write to temp file, rename into place, restore permissions.
func renderAtomicWrite(absPath, relPath string, newContent []byte) error {
	origInfo, err := os.Stat(absPath)
	if err != nil {
		return &mutation.MutationError{
			Code:    "write_error",
			Message: fmt.Sprintf("stat original: %v", err),
		}
	}

	dir := filepath.Dir(absPath)
	tmp, err := os.CreateTemp(dir, ".vaultmind.tmp")
	if err != nil {
		return &mutation.MutationError{
			Code:    "write_error",
			Message: fmt.Sprintf("creating temp file: %v", err),
		}
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(newContent); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return &mutation.MutationError{
			Code:    "write_error",
			Message: fmt.Sprintf("writing temp file: %v", err),
		}
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return &mutation.MutationError{
			Code:    "write_error",
			Message: fmt.Sprintf("closing temp file: %v", err),
		}
	}
	if err := os.Rename(tmpName, absPath); err != nil {
		_ = os.Remove(tmpName)
		return &mutation.MutationError{
			Code:    "write_error",
			Message: fmt.Sprintf("renaming temp file %s: %v", relPath, err),
		}
	}
	if err := os.Chmod(absPath, origInfo.Mode().Perm()); err != nil {
		return &mutation.MutationError{
			Code:    "write_error",
			Message: fmt.Sprintf("restoring permissions: %v", err),
		}
	}
	return nil
}

// renderFileHash computes the SHA-256 hex digest of data.
func renderFileHash(data []byte) string {
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h)
}
