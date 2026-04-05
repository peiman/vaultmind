package plan

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/peiman/vaultmind/internal/git"
	"github.com/peiman/vaultmind/internal/marker"
	"github.com/peiman/vaultmind/internal/mutation"
	"github.com/peiman/vaultmind/internal/schema"
	"github.com/peiman/vaultmind/internal/vault"
)

// Executor orchestrates plan execution: validate, git policy check,
// execute ops in order, rollback on failure, optional batch commit.
type Executor struct {
	VaultPath string
	Detector  git.RepoStateDetector
	Checker   *git.PolicyChecker
	Committer *git.Committer
	Registry  *schema.Registry
	Config    *vault.Config
}

// backup stores the information needed to undo a single operation.
type backup struct {
	relPath string // relative path within vault
	data    []byte // original file bytes; nil means file was created (delete on rollback)
	created bool   // true if this op created the file
}

// Apply executes a plan: validate → git policy → execute ops → rollback on failure → optional commit.
func (e *Executor) Apply(p Plan, dryRun, diff, commit bool) (*ApplyResult, error) {
	result := &ApplyResult{
		PlanDescription: p.Description,
		OperationsTotal: len(p.Operations),
		Operations:      make([]OpResult, len(p.Operations)),
	}

	// Step 1: Validate
	if errs := ValidatePlan(p, e.Registry); len(errs) > 0 {
		result.Operations[0] = OpResult{
			Op:     firstOpName(p.Operations),
			Target: firstOpTarget(p.Operations),
			Status: "error",
			Error:  &errs[0],
		}
		for i := 1; i < len(p.Operations); i++ {
			result.Operations[i] = OpResult{
				Op:     p.Operations[i].Op,
				Target: opTarget(p.Operations[i]),
				Status: "skipped",
			}
		}
		return result, nil
	}

	// Step 2: Git policy check (skip for dry-run)
	if !dryRun {
		if err := e.checkGitPolicy(p.Operations); err != nil {
			return nil, err
		}
	}

	// Step 3: Execute ops
	var backups []backup
	for i, op := range p.Operations {
		// 3a: Backup current file bytes
		bk, err := e.backupFile(op)
		if err != nil && !os.IsNotExist(err) {
			// Non-not-exist errors are real failures
			result.Operations[i] = OpResult{
				Op:     op.Op,
				Target: opTarget(op),
				Status: "error",
				Error:  toOpError(fmt.Errorf("backup failed: %w", err)),
			}
			markSkipped(result, i+1, p.Operations)
			rollback(e.VaultPath, backups)
			return result, nil
		}

		// 3b: Dispatch to handler
		opResult, execErr := e.dispatch(op, dryRun, diff, commit)
		if execErr != nil {
			result.Operations[i] = OpResult{
				Op:     op.Op,
				Target: opTarget(op),
				Status: "error",
				Error:  toOpError(execErr),
			}
			result.OperationsCompleted = i
			markSkipped(result, i+1, p.Operations)
			rollback(e.VaultPath, backups)
			return result, nil
		}

		// 3c: Record success
		result.Operations[i] = *opResult
		result.OperationsCompleted = i + 1
		backups = append(backups, bk)
	}

	// Step 5: Commit if requested and all ops succeeded
	if commit && !dryRun && e.Committer != nil {
		paths := collectPaths(result.Operations)
		if len(paths) > 0 {
			msg := fmt.Sprintf("vaultmind: plan apply — %s", p.Description)
			sha, err := e.Committer.CommitFiles(e.VaultPath, paths, msg)
			if err != nil {
				return nil, fmt.Errorf("committing plan: %w", err)
			}
			result.Committed = true
			result.CommitSHA = sha
		}
	}

	return result, nil
}

// dispatch routes an operation to the appropriate handler.
func (e *Executor) dispatch(op Operation, dryRun, diff, commit bool) (*OpResult, error) {
	switch op.Op {
	case OpFrontmatterSet:
		return e.execMutation(op, mutation.OpSet, dryRun, diff, commit)
	case OpFrontmatterUnset:
		return e.execMutation(op, mutation.OpUnset, dryRun, diff, commit)
	case OpFrontmatterMerge:
		return e.execMutation(op, mutation.OpMerge, dryRun, diff, commit)
	case OpGeneratedRegion:
		return e.execRender(op, dryRun, diff, commit)
	case OpNoteCreate:
		return e.execNoteCreate(op, dryRun)
	default:
		return nil, fmt.Errorf("unhandled operation: %s", op.Op)
	}
}

// execMutation runs a frontmatter mutation via the Mutator pipeline.
func (e *Executor) execMutation(op Operation, mutOp mutation.OpType, dryRun, diff, commit bool) (*OpResult, error) {
	m := &mutation.Mutator{
		VaultPath: e.VaultPath,
		Detector:  e.Detector,
		Checker:   e.Checker,
		Committer: nil, // executor handles commit at plan level
		Registry:  e.Registry,
	}

	req := mutation.MutationRequest{
		Op:     mutOp,
		Target: op.Target,
		Key:    op.Key,
		Value:  op.Value,
		Fields: op.Fields,
		DryRun: dryRun,
		Diff:   diff,
		Commit: false, // never per-op commit; executor commits the batch
	}

	res, err := m.Run(req)
	if err != nil {
		return nil, err
	}

	return &OpResult{
		Op:        op.Op,
		Target:    op.Target,
		Status:    "ok",
		WriteHash: res.WriteHash,
	}, nil
}

// execRender runs the generated region render pipeline.
func (e *Executor) execRender(op Operation, dryRun, diff, commit bool) (*OpResult, error) {
	cfg := marker.RenderConfig{
		VaultPath:  e.VaultPath,
		Target:     op.Target,
		SectionKey: op.SectionKey,
		DryRun:     dryRun,
		Diff:       diff,
		Commit:     false, // executor handles commit at plan level
		Force:      false,
		Detector:   e.Detector,
		Checker:    e.Checker,
		Committer:  nil,
	}

	res, err := marker.RenderRegion(cfg)
	if err != nil {
		return nil, err
	}

	return &OpResult{
		Op:        op.Op,
		Target:    op.Target,
		Status:    "ok",
		WriteHash: res.WriteHash,
	}, nil
}

// execNoteCreate delegates to CreateNote and converts the result.
func (e *Executor) execNoteCreate(op Operation, dryRun bool) (*OpResult, error) {
	if dryRun {
		// For dry-run, synthesize a result without writing
		id := ""
		if rawID, ok := op.Frontmatter["id"]; ok {
			if s, ok := rawID.(string); ok && s != "" {
				id = s
			}
		}
		if id == "" {
			id = deriveID(op.Path, op.Type)
		}
		return &OpResult{
			Op:     OpNoteCreate,
			Path:   op.Path,
			ID:     id,
			Status: "ok",
		}, nil
	}

	return CreateNote(e.VaultPath, op)
}

// backupFile reads the current file content for rollback, or marks a created file.
func (e *Executor) backupFile(op Operation) (backup, error) {
	if op.Op == OpNoteCreate {
		return backup{relPath: op.Path, created: true}, nil
	}

	target := op.Target
	if target == "" {
		return backup{}, nil
	}

	absPath, err := safePath(e.VaultPath, target)
	if err != nil {
		return backup{}, err
	}
	data, err := os.ReadFile(absPath) //nolint:gosec // path validated by safePath vault boundary check
	if err != nil {
		return backup{}, err
	}
	return backup{relPath: target, data: data}, nil
}

// safePath validates that target stays within the vault directory and returns the absolute path.
func safePath(vaultPath, target string) (string, error) {
	absPath := filepath.Clean(filepath.Join(vaultPath, target))
	cleanVault := filepath.Clean(vaultPath)
	if !strings.HasPrefix(absPath, cleanVault+string(filepath.Separator)) && absPath != cleanVault {
		return "", fmt.Errorf("path %q escapes vault directory", target)
	}
	return absPath, nil
}

// rollback undoes completed operations in reverse order.
func rollback(vaultPath string, backups []backup) {
	for i := len(backups) - 1; i >= 0; i-- {
		bk := backups[i]
		absPath := filepath.Join(vaultPath, bk.relPath)
		if bk.created {
			_ = os.Remove(absPath)
		} else if bk.data != nil {
			_ = os.WriteFile(absPath, bk.data, 0o600)
		}
	}
}

// checkGitPolicy evaluates git policy for each operation target in the plan.
func (e *Executor) checkGitPolicy(ops []Operation) error {
	if e.Detector == nil || e.Checker == nil {
		return nil
	}

	state, err := e.Detector.Detect(e.VaultPath)
	if err != nil {
		return fmt.Errorf("git detect error: %w", err)
	}

	// Check each operation target individually so dirty_target rules fire correctly.
	checked := make(map[string]bool)
	for _, op := range ops {
		target := opTarget(op)
		if target == "" || checked[target] {
			continue
		}
		checked[target] = true

		policyResult := e.Checker.Check(state, git.OpWrite, target)
		if policyResult.Decision == git.Refuse {
			reason := "git policy refused"
			if len(policyResult.Reasons) > 0 {
				reason = policyResult.Reasons[0].Rule
			}
			return fmt.Errorf("%s: git policy refuses plan apply on %s", reason, target)
		}
	}
	return nil
}

// toOpError converts an error into an *OpError, handling planError and MutationError.
func toOpError(err error) *OpError {
	var pe *planError
	if errors.As(err, &pe) {
		return &OpError{Code: pe.Code, Message: pe.Message}
	}
	var me *mutation.MutationError
	if errors.As(err, &me) {
		return &OpError{Code: me.Code, Message: me.Message}
	}
	return &OpError{Code: "execution_error", Message: err.Error()}
}

// markSkipped fills remaining operations as "skipped".
func markSkipped(result *ApplyResult, fromIdx int, ops []Operation) {
	for i := fromIdx; i < len(ops); i++ {
		result.Operations[i] = OpResult{
			Op:     ops[i].Op,
			Target: opTarget(ops[i]),
			Status: "skipped",
		}
	}
}

// collectPaths gathers unique file paths from successful operations.
func collectPaths(ops []OpResult) []string {
	seen := make(map[string]bool)
	var paths []string
	for _, op := range ops {
		p := op.Target
		if p == "" {
			p = op.Path
		}
		if p != "" && !seen[p] {
			seen[p] = true
			paths = append(paths, p)
		}
	}
	return paths
}

// opTarget returns the target or path for an operation.
func opTarget(op Operation) string {
	if op.Target != "" {
		return op.Target
	}
	return op.Path
}

// firstOpName returns the op name of the first operation, or empty.
func firstOpName(ops []Operation) string {
	if len(ops) == 0 {
		return ""
	}
	return ops[0].Op
}

// firstOpTarget returns the target of the first operation, or empty.
func firstOpTarget(ops []Operation) string {
	if len(ops) == 0 {
		return ""
	}
	return opTarget(ops[0])
}
