// ckeletin:allow-custom-command
//
// `episode` is a utility command with no persistent user-facing config keys:
// its single flag (--output-dir) defaults to a path inside the project's
// identity vault, and its positional arg is a path provided per-invocation.
// There are no viper-bound settings worth the ceremony of the ckeletin
// config registry + generated constants. The marker above documents the
// deliberate exception to the MustNewCommand pattern used by config-driven
// commands elsewhere in cmd/.
package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/peiman/vaultmind/internal/episode"
	"github.com/spf13/cobra"
)

var episodeCmd = &cobra.Command{
	Use:   "episode",
	Short: "Capture Claude Code sessions as episodic-memory artifacts",
	Long: `Record Claude Code sessions as structured markdown episodes for long-term memory.

An episode is a parsed, structured record of one Claude Code session: the user
and assistant exchanges, tool calls, commits, PRs opened, and files touched.
Episodes are the raw substrate for arc distillation — the process of surfacing
growth moments and behavioral patterns across many sessions over time.

Episodes are not indexed into the vault's search layer (v0 design); they are
stored as markdown files for review and downstream arc processing.

SUBCOMMANDS

  capture   Parse a Claude Code JSONL transcript and write a markdown episode.
            Invoked automatically by the capture-episode.sh SessionEnd hook,
            or manually after a session ends.`,
}

var episodeCaptureCmd = &cobra.Command{
	Use:   "capture <transcript-or-dir>",
	Short: "Convert a Claude Code session transcript (or a directory of them) into episodes",
	Long: `Parse a Claude Code JSONL transcript and write a structured markdown episode.

Episode capture is the pipeline entry point from a live Claude Code session into
vaultmind's episodic-memory substrate. The capture-episode.sh SessionEnd hook
calls this command automatically; you can also run it manually against any saved
transcript.

INPUT

  A Claude Code session transcript in JSONL format. Each line is one JSON event
  (user turn, assistant turn, tool call, pr-link, etc.). Claude Code writes this
  file during a session; the hook passes its path here at session end.

  Noise records (system reminders, tool results, thinking blocks) are filtered
  automatically — only real exchanges and structural signals are kept.

OUTPUT

  Prints the written file path to stdout, e.g.:
    vaultmind-identity/episodes/episode-2026-05-01-a1b2c3d4.md

  The filename is derived from the session start timestamp and the first 8
  characters of the session ID. Re-running against the same transcript is
  idempotent — it overwrites the existing episode file.

  The markdown file contains:
    - YAML frontmatter (id, session_id, started_at, ended_at, cwd, git_branch)
    - Metadata summary (message counts, tool call counts, files touched)
    - Commits made during the session
    - PRs opened during the session
    - Files touched (Read, Edit, Write tool calls)
    - User messages (verbatim, block-quoted)
    - Assistant responses (verbatim)

FLAGS

  --output-dir: Directory to write the episode markdown file (string,
                default: "vaultmind-identity/episodes"). Created if it
                does not exist.

EXAMPLES

  vaultmind episode capture /tmp/session-abc123.jsonl
      # Parse the transcript and write to vaultmind-identity/episodes/

  vaultmind episode capture /tmp/session-abc123.jsonl --output-dir ./episodes
      # Write to a custom directory instead of the default

  vaultmind episode capture "$CLAUDE_SESSION_TRANSCRIPT"
      # Typical hook invocation; CLAUDE_SESSION_TRANSCRIPT set by Claude Code

  vaultmind episode capture ~/.claude/projects/my-project --output-dir vaultmind-identity/episodes
      # BOOTSTRAP: pass a DIRECTORY to capture every *.jsonl transcript under it
      # (recursively). Seed an identity vault from sessions that already exist —
      # then run 'vaultmind arc candidates'. Empty/non-transcript files are skipped.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		outputDir, _ := cmd.Flags().GetString("output-dir")
		if info, err := os.Stat(args[0]); err == nil && info.IsDir() {
			return runEpisodeCaptureDir(cmd, args[0], outputDir)
		}
		path, err := episode.Capture(args[0], outputDir)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), path)
		return err
	},
}

// runEpisodeCaptureDir batch-captures every *.jsonl transcript under dir and prints
// a summary — so seeding an identity vault from an existing session history (e.g.
// ~/.claude/projects/<slug>) is one command instead of a hand-rolled loop. Empty or
// non-transcript files are reported, not fatal.
func runEpisodeCaptureDir(cmd *cobra.Command, dir, outputDir string) error {
	batch, err := episode.CaptureDir(dir, outputDir)
	if err != nil {
		return err
	}
	out := strings.Join(batch.Captured, "\n")
	if len(batch.Captured) > 0 {
		out += "\n"
	}
	out += fmt.Sprintf("Captured %d episode(s) from %s\n", len(batch.Captured), dir)
	if len(batch.Skipped) > 0 {
		out += fmt.Sprintf("Skipped %d file(s) (empty or not a Claude Code transcript).\n", len(batch.Skipped))
	}
	if len(batch.Captured) > 0 {
		out += "\nNext: surface arc candidates with `vaultmind arc candidates`.\n"
	}
	_, err = fmt.Fprint(cmd.OutOrStdout(), out)
	return err
}

func init() {
	episodeCaptureCmd.Flags().String("output-dir", "vaultmind-identity/episodes", "Directory to write the episode markdown file")
	episodeCmd.AddCommand(episodeCaptureCmd)
	MustAddToRoot(episodeCmd)
}
