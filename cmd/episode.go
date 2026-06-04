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
	Use:   "capture <transcript-path>",
	Short: "Convert a Claude Code session transcript into a markdown episode file",
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
      # Typical hook invocation; CLAUDE_SESSION_TRANSCRIPT set by Claude Code`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		outputDir, _ := cmd.Flags().GetString("output-dir")
		path, err := episode.Capture(args[0], outputDir)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), path)
		return err
	},
}

func init() {
	episodeCaptureCmd.Flags().String("output-dir", "vaultmind-identity/episodes", "Directory to write the episode markdown file")
	episodeCmd.AddCommand(episodeCaptureCmd)
	MustAddToRoot(episodeCmd)
}
