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
	"path/filepath"
	"sort"
	"strings"

	"github.com/peiman/vaultmind/internal/episode"
	"github.com/peiman/vaultmind/internal/xdg"
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

  --output-dir:  Directory to write the episode markdown file (string,
                 default: "vaultmind-identity/episodes"). Created if it
                 does not exist.

  --incremental: Capture only the transcript delta since the last incremental
                 capture of this session, instead of re-rendering the whole
                 transcript. Fixes the failure mode where a session that
                 never closes produces one ever-growing episode file: each
                 call writes a new, small, uniquely-named segment covering
                 only what's genuinely new. Tracks progress via a per-session
                 cursor file (--cursor-dir). Ignored for directory input
                 (bootstrap capture always wants the full history).

  --cursor-dir:  Where incremental-capture cursor files live (string,
                 default: the XDG state dir). Only meaningful with
                 --incremental.

OUTPUT (--incremental)

  Prints the new segment's path, same as a normal capture. When there is
  nothing new since the last call — the common case for a SessionEnd hook
  firing on a session that hasn't produced new content — prints one line
  saying so and exits 0; it never prints a blank line or writes an empty
  episode file, so "nothing happened" and "silently failed" are never the
  same output.

EXAMPLES

  vaultmind episode capture /tmp/session-abc123.jsonl
      # Parse the transcript and write to vaultmind-identity/episodes/

  vaultmind episode capture /tmp/session-abc123.jsonl --output-dir ./episodes
      # Write to a custom directory instead of the default

  vaultmind episode capture "$CLAUDE_SESSION_TRANSCRIPT"
      # Typical hook invocation; CLAUDE_SESSION_TRANSCRIPT set by Claude Code

  vaultmind episode capture "$CLAUDE_SESSION_TRANSCRIPT" --incremental
      # Long-lived-session-safe hook invocation: bounded segments, not one blob

  vaultmind episode capture ~/.claude/projects/my-project --output-dir vaultmind-identity/episodes
      # BOOTSTRAP: pass a DIRECTORY to capture every *.jsonl transcript under it
      # (recursively). Seed an identity vault from sessions that already exist —
      # then run 'vaultmind arc candidates'. Empty/non-transcript files are skipped,
      # and subagent/workflow transcripts are passed over: they carry the parent
      # session's id, so capturing them would overwrite the session itself.
      # Pass one of those files directly if you do want it captured.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		outputDir, _ := cmd.Flags().GetString("output-dir")
		if info, err := os.Stat(args[0]); err == nil && info.IsDir() {
			if incremental, _ := cmd.Flags().GetBool("incremental"); incremental {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "episode capture: --incremental is ignored for directory input (bootstrap capture always wants the full history)")
			}
			return runEpisodeCaptureDir(cmd, args[0], outputDir)
		}

		incremental, _ := cmd.Flags().GetBool("incremental")
		if !incremental {
			path, err := episode.Capture(args[0], outputDir)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), path)
			return err
		}

		return runEpisodeCaptureIncremental(cmd, args[0], outputDir)
	},
}

// runEpisodeCaptureIncremental resolves the cursor dir (defaulting to the XDG
// state dir when --cursor-dir is omitted) and prints either the new segment's
// path or a plain "nothing new" line, so a hook invocation's stdout never
// leaves "no output" ambiguous between success and silent failure.
func runEpisodeCaptureIncremental(cmd *cobra.Command, transcriptPath, outputDir string) error {
	cursorDir, _ := cmd.Flags().GetString("cursor-dir")
	if cursorDir == "" {
		dir, err := xdg.StateDir()
		if err != nil {
			return fmt.Errorf("resolve default cursor dir: %w", err)
		}
		cursorDir = filepath.Join(dir, "episode-cursors")
	}
	path, err := episode.CaptureIncremental(transcriptPath, outputDir, cursorDir)
	if err != nil {
		return err
	}
	if path == "" {
		_, err = fmt.Fprintln(cmd.OutOrStdout(), "(nothing new to capture since the last incremental capture)")
		return err
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), path)
	return err
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
	if batch.Sidechains > 0 {
		out += fmt.Sprintf("Passed over %d subagent/workflow transcript(s) — those are tool runs, not sessions.\n", batch.Sidechains)
	}
	if len(batch.Skipped) > 0 {
		out += fmt.Sprintf("Skipped %d file(s) (empty or not a Claude Code transcript).\n", len(batch.Skipped))
	}
	out += renderCollisions(batch.Collisions)
	if len(batch.Captured) > 0 {
		out += "\nNext: surface arc candidates with `vaultmind arc candidates`.\n"
	}
	_, err = fmt.Fprint(cmd.OutOrStdout(), out)
	return err
}

// maxListedCollisions bounds the named list. Collisions are rare by nature, so
// this only guards the pathological case; the count line still reports the true
// total, and truncation is stated rather than silent.
const maxListedCollisions = 5

// renderCollisions names the transcripts that lost a derived episode id. A bare
// count cannot be acted on — a collision means two transcripts existed and only
// one survived, so the reader needs to know WHICH, and the reason string already
// carries it. Computing that reason and then printing only a total is the same
// class of failure this whole command was fixed for.
func renderCollisions(collisions map[string]string) string {
	if len(collisions) == 0 {
		return ""
	}
	paths := make([]string, 0, len(collisions))
	for p := range collisions {
		paths = append(paths, p)
	}
	sort.Strings(paths) // map order is random; a summary that reorders itself run to run is unreadable
	out := fmt.Sprintf("%d transcript(s) collided on a derived episode id and were NOT captured:\n", len(collisions))
	for i, p := range paths {
		if i == maxListedCollisions {
			out += fmt.Sprintf("  … and %d more\n", len(paths)-maxListedCollisions)
			break
		}
		out += fmt.Sprintf("  %s — %s\n", p, collisions[p])
	}
	return out
}

func init() {
	episodeCaptureCmd.Flags().String("output-dir", "vaultmind-identity/episodes", "Directory to write the episode markdown file")
	episodeCaptureCmd.Flags().Bool("incremental", false, "Capture only the delta since the last incremental capture of this session (bounded segments, not one ever-growing file)")
	episodeCaptureCmd.Flags().String("cursor-dir", "", "Where incremental-capture cursor files live (default: XDG state dir)")
	episodeCmd.AddCommand(episodeCaptureCmd)
	MustAddToRoot(episodeCmd)
}
