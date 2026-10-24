package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/peiman/vaultmind/internal/hooks"
)

func writeHooksGitExclude(w io.Writer, res *hooks.GitExcludeResult) {
	if res == nil {
		return
	}
	switch {
	case res.SkippedReason != "":
		_, _ = fmt.Fprintf(w, "\n.claude/scripts/ and .claude/vaultmind-profile are in the tree; git exclusion skipped: %s\n", res.SkippedReason)
	case res.DryRun && len(res.Added) > 0:
		_, _ = fmt.Fprintf(w, "\nWould add to %s (nothing written):\n  %s\n", res.Path, strings.Join(res.Added, "\n  "))
	case len(res.Added) > 0:
		_, _ = fmt.Fprintf(w, "\nkept out of git: added %s to %s\n", strings.Join(res.Added, " and "), res.Path)
	case len(res.Removed) > 0:
		_, _ = fmt.Fprintf(w, "\nRemoved VaultMind's local git exclude block from %s\n", res.Path)
	default:
		_, _ = fmt.Fprintf(w, "\nGit exclude unchanged: %s\n", res.Path)
	}
}

// Cleanup has its own advisory wording: uninstall may have removed the files.
func writeHooksGitExcludeCleanup(w io.Writer, res *hooks.GitExcludeResult) {
	if res != nil && res.SkippedReason != "" {
		_, _ = fmt.Fprintf(w, "\nGit exclude cleanup skipped: %s\n", res.SkippedReason)
		return
	}
	writeHooksGitExclude(w, res)
}
