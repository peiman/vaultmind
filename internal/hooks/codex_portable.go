package hooks

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Portable Codex wiring (#372). .codex/hooks.json is normally committed, and an
// absolute project path in it is the operator's home directory published to
// every teammate, and a command that is wrong in every other checkout.
//
// Codex sets no project variable, runs a hook with the session's cwd (which may
// be a subdirectory), and its docs name the git root as the way to find a
// repo-local hook. So when the project directory IS a git work tree's root, the
// command asks git at run time instead of carrying a path.
//
// A vault inside the project already travels as $CLAUDE_PROJECT_DIR/<rel>. A
// vault OUTSIDE the project stays absolute: nothing relative to the project can
// name it, and that path is the operator's choice, not ours to hide.
const (
	// codexGitRootExport resolves the project dir from git at run time.
	codexGitRootExport = "export " + envProjectDir + `="$(git rev-parse --show-toplevel)"; `
	// codexClaudeAliasExport is added only when a vault ref names
	// $CLAUDE_PROJECT_DIR, which Codex does not set.
	codexClaudeAliasExport = "export CLAUDE_PROJECT_DIR=\"$" + envProjectDir + "\"; "
	// codexPortableScriptDir is the script directory in a portable command; the
	// script name follows it.
	codexPortableScriptDir = `"$` + envProjectDir + `"/` + codexBaseDir + "/" + scriptsSubdir + "/"
)

// isGitRoot reports whether projectDir is the top level of a git work tree,
// comparing both sides with symlinks resolved (macOS /var is /private/var).
func isGitRoot(projectDir string) bool {
	cmd := exec.Command("git", "-C", projectDir, "rev-parse", "--show-toplevel") // #nosec G204 -- fixed git args; projectDir is an argument, not shell
	// A git hook exports GIT_DIR and friends; they would redirect this query
	// into whichever repository is running the hook.
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	top, err := filepath.EvalSymlinks(strings.TrimSpace(string(out)))
	if err != nil {
		return false
	}
	proj, err := filepath.EvalSymlinks(projectDir)
	return err == nil && top == proj
}

// codexPortableExports is the export prefix of a portable command.
func codexPortableExports(vaultPath string, vaults []string) string {
	if usesClaudeProjectDir(vaultPath, vaults) {
		return codexGitRootExport + codexClaudeAliasExport
	}
	return codexGitRootExport
}

// codexPortableScriptRef names script through the exported project dir.
func codexPortableScriptRef(script string) string {
	return codexPortableScriptDir + script
}

// isPortableExports reports whether text is exactly the export prefix this
// package writes, so a hand-written variant is never taken for ours.
func isPortableExports(text string) bool {
	text = strings.TrimSpace(text)
	return text == strings.TrimSpace(codexGitRootExport) ||
		text == strings.TrimSpace(codexGitRootExport+codexClaudeAliasExport)
}

// isPortableExportWord reports whether raw is the assignment word of
// codexGitRootExport, for migrating a generated command to user scope.
func isPortableExportWord(raw string) bool {
	rest, ok := strings.CutPrefix(codexGitRootExport, "export ")
	return ok && raw == strings.TrimSuffix(rest, "; ")
}
