package hooks

import (
	"path/filepath"
	"strings"
)

// claudeProjectDirVar is the variable Claude Code sets for every hook. A
// vault inside the project is stored against it so a committed settings file
// works in every checkout.
const claudeProjectDirVar = "$CLAUDE_PROJECT_DIR"

// PortableVaultRef is the vault path hooks install bakes into a command.
// Project scope writes $CLAUDE_PROJECT_DIR/<rel> when vault is inside
// projectDir. A vault outside the project, and every user-scope vault, stays
// absolute: there is no single project to be relative to, and a hook's
// working directory is not the directory the flag was typed in.
// Symlinks are not evaluated. The path that was named is the path a teammate's
// checkout has.
func PortableVaultRef(projectDir, vault string, scope Scope) (string, error) {
	vault = strings.TrimSpace(vault)
	if vault == "" {
		return "", nil
	}
	// Agent-specific wiring may already have normalized this reference before
	// handing its install to Provision. Preserve it on subsequent passes.
	if scope != ScopeUser && isProjectVaultRef(vault) {
		return vault, nil
	}
	abs, err := filepath.Abs(vault)
	if err != nil {
		return "", err
	}
	if scope == ScopeUser || strings.TrimSpace(projectDir) == "" {
		return abs, nil
	}
	proj, err := filepath.Abs(projectDir)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(filepath.VolumeName(proj), filepath.VolumeName(abs)) {
		return abs, nil
	}
	rel, err := filepath.Rel(proj, abs)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return abs, nil
	}
	if rel == "." {
		return claudeProjectDirVar, nil
	}
	return claudeProjectDirVar + "/" + filepath.ToSlash(rel), nil
}

// PortableVaultRefs applies PortableVaultRef to a primary vault and a --vaults list.
func PortableVaultRefs(projectDir, vault string, vaults []string, scope Scope) (string, []string, error) {
	baked, err := PortableVaultRef(projectDir, vault, scope)
	if err != nil {
		return "", nil, err
	}
	if len(vaults) == 0 {
		return baked, nil, nil
	}
	out := make([]string, len(vaults))
	for i, v := range vaults {
		out[i], err = PortableVaultRef(projectDir, v, scope)
		if err != nil {
			return "", nil, err
		}
	}
	return baked, out, nil
}

// projectDirExports is the prefix Codex and Cursor put on a project-scope
// command. User scope keeps Codex's "$PWD" export. When the vault assignment
// names $CLAUDE_PROJECT_DIR, that variable is exported too: neither agent sets
// it, and an empty expansion would point the hook at /knowledge.
func projectDirExports(projectDir, vaultPath string, vaults []string, scope Scope) string {
	if scope == ScopeUser {
		return `export VAULTMIND_PROJECT_DIR="$PWD"; `
	}
	prefix := "export " + envProjectDir + "=" + singleQuote(projectDir) + "; "
	if usesClaudeProjectDir(vaultPath, vaults) {
		prefix += "export CLAUDE_PROJECT_DIR=" + singleQuote(projectDir) + "; "
	}
	return prefix
}

func usesClaudeProjectDir(vault string, vaults []string) bool {
	if isProjectVaultRef(vault) {
		return true
	}
	for _, v := range vaults {
		if isProjectVaultRef(v) {
			return true
		}
	}
	return false
}

func isProjectVaultRef(v string) bool {
	return v == claudeProjectDirVar || strings.HasPrefix(v, claudeProjectDirVar+"/")
}
