package git

import (
	"testing"

	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPolicyChecker_Defaults(t *testing.T) {
	pc, err := NewPolicyChecker(vault.GitPolicyConfig{})
	require.NoError(t, err)
	assert.NotNil(t, pc)
}

func TestNewPolicyChecker_ValidOverrides(t *testing.T) {
	cfg := vault.GitPolicyConfig{
		Policy: map[string]string{
			"dirty_unrelated": "refuse",
			"dirty_target":    "allow",
		},
	}
	pc, err := NewPolicyChecker(cfg)
	require.NoError(t, err)
	assert.NotNil(t, pc)
}

func TestNewPolicyChecker_InvalidOverride(t *testing.T) {
	cfg := vault.GitPolicyConfig{
		Policy: map[string]string{
			"dirty_target": "block",
		},
	}
	_, err := NewPolicyChecker(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "block")
}

func TestNewPolicyChecker_UnknownRule(t *testing.T) {
	cfg := vault.GitPolicyConfig{
		Policy: map[string]string{
			"unknown_rule": "warn",
		},
	}
	_, err := NewPolicyChecker(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown_rule")
}

func TestPolicyChecker_CleanRepo(t *testing.T) {
	pc, err := NewPolicyChecker(vault.GitPolicyConfig{})
	require.NoError(t, err)

	state := RepoState{
		RepoDetected:     true,
		WorkingTreeClean: true,
	}

	for _, op := range []OperationType{OpRead, OpDryRun, OpWrite, OpWriteCommit} {
		result := pc.Check(state, op, "notes/test.md")
		assert.Equal(t, Allow, result.Decision, "op=%s", op)
		assert.Empty(t, result.Reasons, "op=%s", op)
	}
}

func TestPolicyChecker_DirtyUnrelated(t *testing.T) {
	pc, err := NewPolicyChecker(vault.GitPolicyConfig{})
	require.NoError(t, err)

	state := RepoState{
		RepoDetected:     true,
		WorkingTreeClean: false,
		UnstagedFiles:    []string{"other/file.md"},
	}

	// Read and DryRun: Allow
	assert.Equal(t, Allow, pc.Check(state, OpRead, "notes/test.md").Decision)
	assert.Equal(t, Allow, pc.Check(state, OpDryRun, "notes/test.md").Decision)

	// Write and WriteCommit: Warn
	result := pc.Check(state, OpWrite, "notes/test.md")
	assert.Equal(t, Warn, result.Decision)
	assert.Len(t, result.Reasons, 1)
	assert.Equal(t, "dirty_unrelated", result.Reasons[0].Rule)

	result = pc.Check(state, OpWriteCommit, "notes/test.md")
	assert.Equal(t, Warn, result.Decision)
}

func TestPolicyChecker_DirtyTarget_Unstaged(t *testing.T) {
	pc, err := NewPolicyChecker(vault.GitPolicyConfig{})
	require.NoError(t, err)

	state := RepoState{
		RepoDetected:     true,
		WorkingTreeClean: false,
		UnstagedFiles:    []string{"notes/test.md"},
	}

	assert.Equal(t, Allow, pc.Check(state, OpRead, "notes/test.md").Decision)
	assert.Equal(t, Allow, pc.Check(state, OpDryRun, "notes/test.md").Decision)
	assert.Equal(t, Refuse, pc.Check(state, OpWrite, "notes/test.md").Decision)
	assert.Equal(t, Refuse, pc.Check(state, OpWriteCommit, "notes/test.md").Decision)
}

func TestPolicyChecker_DirtyTarget_Staged(t *testing.T) {
	pc, err := NewPolicyChecker(vault.GitPolicyConfig{})
	require.NoError(t, err)

	state := RepoState{
		RepoDetected:     true,
		WorkingTreeClean: false,
		StagedFiles:      []string{"notes/test.md"},
	}

	assert.Equal(t, Refuse, pc.Check(state, OpWrite, "notes/test.md").Decision)
	assert.Equal(t, Refuse, pc.Check(state, OpWriteCommit, "notes/test.md").Decision)
}

func TestPolicyChecker_DetachedHead(t *testing.T) {
	pc, err := NewPolicyChecker(vault.GitPolicyConfig{})
	require.NoError(t, err)

	state := RepoState{
		RepoDetected:     true,
		Detached:         true,
		WorkingTreeClean: true,
	}

	assert.Equal(t, Allow, pc.Check(state, OpRead, "").Decision)
	assert.Equal(t, Allow, pc.Check(state, OpDryRun, "").Decision)
	assert.Equal(t, Warn, pc.Check(state, OpWrite, "notes/test.md").Decision)
	assert.Equal(t, Refuse, pc.Check(state, OpWriteCommit, "notes/test.md").Decision)
}

func TestPolicyChecker_MergeInProgress(t *testing.T) {
	pc, err := NewPolicyChecker(vault.GitPolicyConfig{})
	require.NoError(t, err)

	state := RepoState{
		RepoDetected:     true,
		MergeInProgress:  true,
		WorkingTreeClean: true,
	}

	assert.Equal(t, Allow, pc.Check(state, OpRead, "").Decision)
	assert.Equal(t, Allow, pc.Check(state, OpDryRun, "").Decision)
	assert.Equal(t, Refuse, pc.Check(state, OpWrite, "notes/test.md").Decision)
	assert.Equal(t, Refuse, pc.Check(state, OpWriteCommit, "notes/test.md").Decision)
}

func TestPolicyChecker_RebaseInProgress(t *testing.T) {
	pc, err := NewPolicyChecker(vault.GitPolicyConfig{})
	require.NoError(t, err)

	state := RepoState{
		RepoDetected:     true,
		RebaseInProgress: true,
		WorkingTreeClean: true,
	}

	assert.Equal(t, Refuse, pc.Check(state, OpWrite, "notes/test.md").Decision)
	assert.Equal(t, Refuse, pc.Check(state, OpWriteCommit, "notes/test.md").Decision)
}

func TestPolicyChecker_NoRepo(t *testing.T) {
	pc, err := NewPolicyChecker(vault.GitPolicyConfig{})
	require.NoError(t, err)

	state := RepoState{
		RepoDetected: false,
	}

	// Read and DryRun: Warn
	result := pc.Check(state, OpRead, "")
	assert.Equal(t, Warn, result.Decision)
	assert.Equal(t, "no_repo", result.Reasons[0].Rule)

	assert.Equal(t, Warn, pc.Check(state, OpDryRun, "").Decision)
	assert.Equal(t, Warn, pc.Check(state, OpWrite, "notes/test.md").Decision)

	// WriteCommit: Refuse (always, not overridable)
	assert.Equal(t, Refuse, pc.Check(state, OpWriteCommit, "notes/test.md").Decision)
}

func TestPolicyChecker_Override_DirtyUnrelated_Refuse(t *testing.T) {
	cfg := vault.GitPolicyConfig{
		Policy: map[string]string{"dirty_unrelated": "refuse"},
	}
	pc, err := NewPolicyChecker(cfg)
	require.NoError(t, err)

	state := RepoState{
		RepoDetected:     true,
		WorkingTreeClean: false,
		UnstagedFiles:    []string{"other/file.md"},
	}

	// Override changes Write from Warn to Refuse
	assert.Equal(t, Refuse, pc.Check(state, OpWrite, "notes/test.md").Decision)
	// WriteCommit inherits: also Refuse
	assert.Equal(t, Refuse, pc.Check(state, OpWriteCommit, "notes/test.md").Decision)
}

func TestPolicyChecker_Override_DetachedHead_Allow(t *testing.T) {
	cfg := vault.GitPolicyConfig{
		Policy: map[string]string{"detached_head": "allow"},
	}
	pc, err := NewPolicyChecker(cfg)
	require.NoError(t, err)

	state := RepoState{
		RepoDetected:     true,
		Detached:         true,
		WorkingTreeClean: true,
	}

	// Override changes Write from Warn to Allow
	assert.Equal(t, Allow, pc.Check(state, OpWrite, "notes/test.md").Decision)
	// WriteCommit: still Refuse (non-overridable guard)
	assert.Equal(t, Refuse, pc.Check(state, OpWriteCommit, "notes/test.md").Decision)
}

func TestPolicyChecker_Override_NoRepo_Allow(t *testing.T) {
	cfg := vault.GitPolicyConfig{
		Policy: map[string]string{"no_repo": "allow"},
	}
	pc, err := NewPolicyChecker(cfg)
	require.NoError(t, err)

	state := RepoState{RepoDetected: false}

	// Override changes Write from Warn to Allow
	assert.Equal(t, Allow, pc.Check(state, OpWrite, "notes/test.md").Decision)
	// WriteCommit: still Refuse (non-overridable guard)
	assert.Equal(t, Refuse, pc.Check(state, OpWriteCommit, "notes/test.md").Decision)
}

func TestPolicyChecker_MultipleRules_StrictestWins(t *testing.T) {
	pc, err := NewPolicyChecker(vault.GitPolicyConfig{})
	require.NoError(t, err)

	// Detached HEAD + dirty target
	state := RepoState{
		RepoDetected:     true,
		Detached:         true,
		WorkingTreeClean: false,
		UnstagedFiles:    []string{"notes/test.md"},
	}

	result := pc.Check(state, OpWrite, "notes/test.md")
	assert.Equal(t, Refuse, result.Decision)
	// Should have reasons from both rules
	assert.GreaterOrEqual(t, len(result.Reasons), 2)
}

func TestPolicyChecker_InvalidOperationType(t *testing.T) {
	pc, err := NewPolicyChecker(vault.GitPolicyConfig{})
	require.NoError(t, err)

	state := RepoState{RepoDetected: true, WorkingTreeClean: true}
	result := pc.Check(state, OperationType(99), "notes/test.md")
	assert.Equal(t, Refuse, result.Decision)
	assert.Equal(t, "invalid_operation", result.Reasons[0].Rule)
}

func TestPolicyChecker_ReadOps_NoTarget(t *testing.T) {
	pc, err := NewPolicyChecker(vault.GitPolicyConfig{})
	require.NoError(t, err)

	state := RepoState{
		RepoDetected:     true,
		WorkingTreeClean: false,
		UnstagedFiles:    []string{"notes/test.md"},
	}

	// Read with empty target — dirty_target rule should not trigger
	result := pc.Check(state, OpRead, "")
	assert.Equal(t, Allow, result.Decision)
}
