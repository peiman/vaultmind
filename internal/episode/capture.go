package episode

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Capture parses the transcript at transcriptPath, renders the episode
// markdown, and writes it to outputDir. Returns the written file path.
// Overwrites an existing file with the same derived ID — re-running against
// a finalized transcript is idempotent by design. Uses atomic temp-file +
// rename so concurrent SessionEnd hook invocations cannot produce torn files.
func Capture(transcriptPath, outputDir string) (string, error) {
	ep, err := ParseTranscript(transcriptPath)
	if err != nil {
		return "", err
	}
	if ep.SessionID == "" {
		return "", fmt.Errorf("transcript has no session id — empty or not a Claude Code transcript: %s", transcriptPath)
	}
	if err := os.MkdirAll(outputDir, 0o750); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}
	outPath := filepath.Join(outputDir, ep.ID+".md")
	if err := atomicWriteFile(outPath, []byte(RenderMarkdown(ep))); err != nil {
		return "", fmt.Errorf("write episode: %w", err)
	}
	return outPath, nil
}

// CaptureIncremental captures only the transcript segment written since the
// last CaptureIncremental call for this session (tracked via a cursor file in
// cursorDir), writing it as a new, uniquely-named episode segment file in
// outputDir. Returns "" with a nil error when there is nothing new to
// capture — a no-op SessionEnd, or a delta containing only filtered noise,
// must never write an empty episode file.
//
// This is the fix for a long-lived session that never closes: Capture
// re-renders the WHOLE transcript into the SAME file every SessionEnd, which
// silently grows one episode without bound. CaptureIncremental instead
// accumulates several small, bounded segment files, each covering only what
// was genuinely new since the last capture.
func CaptureIncremental(transcriptPath, outputDir, cursorDir string) (string, error) {
	sessionID, err := sessionIDOf(transcriptPath)
	if err != nil {
		return "", fmt.Errorf("read session id: %w", err)
	}
	if sessionID == "" {
		return "", fmt.Errorf("transcript has no session id — empty or not a Claude Code transcript: %s", transcriptPath)
	}

	startLine, err := ReadCursor(cursorDir, sessionID)
	if err != nil {
		return "", err
	}

	ep, endLine, err := ParseTranscriptFrom(transcriptPath, startLine)
	if err != nil {
		return "", err
	}
	if endLine == startLine {
		return "", nil // nothing new since the last capture
	}
	if !hasCapturableContent(ep) {
		// The transcript grew, but only with filtered noise (e.g. more
		// system-reminders) — advance the cursor so it's never re-scanned,
		// but don't write an episode file with nothing in it.
		return "", WriteCursor(cursorDir, sessionID, endLine)
	}

	if err := os.MkdirAll(outputDir, 0o750); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}
	ep.ID = deriveSegmentID(ep.StartedAt, sessionID, startLine)
	outPath := filepath.Join(outputDir, ep.ID+".md")
	if err := atomicWriteFile(outPath, []byte(RenderMarkdown(ep))); err != nil {
		return "", fmt.Errorf("write episode segment: %w", err)
	}
	if err := WriteCursor(cursorDir, sessionID, endLine); err != nil {
		return "", fmt.Errorf("advance cursor after writing %s: %w", outPath, err)
	}
	return outPath, nil
}

// hasCapturableContent reports whether ep carries anything worth writing an
// episode segment for. Checks every field the Episode struct can carry, not
// a subset — a delta that touched files via Read/Edit/Write but produced no
// text block (tool calls without prose) still has real, worth-keeping
// signal, and treating it as empty would silently drop it while still
// advancing the cursor past it.
func hasCapturableContent(ep *Episode) bool {
	return len(ep.UserMessages) > 0 || len(ep.AssistantMessages) > 0 ||
		len(ep.PRs) > 0 || len(ep.Commits) > 0 ||
		len(ep.FilesTouched) > 0 || len(ep.ToolCounts) > 0
}

// deriveSegmentID names one incremental-capture segment uniquely within its
// session. startLine is the cursor position the segment began at, which only
// ever advances forward, so two segments of the same session can never
// collide — unlike a full-transcript ID keyed only on (date, session
// prefix), which silently overwrites when more than one capture shares both.
func deriveSegmentID(startedAt, sessionID string, startLine int) string {
	return fmt.Sprintf("%s-part%08d", deriveID(startedAt, sessionID), startLine)
}

// CaptureBatch summarizes a directory capture: the episode files written and the
// transcripts that were skipped (with the reason). Skipping is deliberate — a noise
// or partial transcript in a large history must not abort the whole batch.
type CaptureBatch struct {
	Captured []string          // episode file paths written, in transcript-path order
	Skipped  map[string]string // transcript path -> reason (empty/malformed/parse error)
}

// CaptureDir captures every *.jsonl transcript found recursively under dir into
// outputDir. It is the bootstrap entry point: point it at a project's Claude Code
// transcript directory (e.g. ~/.claude/projects/<slug>) to seed an identity vault
// from sessions that already exist. Malformed/empty transcripts go into
// Skipped rather than failing the run; capture itself is idempotent.
func CaptureDir(dir, outputDir string) (CaptureBatch, error) {
	batch := CaptureBatch{Skipped: map[string]string{}}
	var transcripts []string
	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".jsonl") {
			transcripts = append(transcripts, path)
		}
		return nil
	})
	if walkErr != nil {
		return batch, fmt.Errorf("scanning %s: %w", dir, walkErr)
	}
	sort.Strings(transcripts)
	for _, t := range transcripts {
		out, err := Capture(t, outputDir)
		if err != nil {
			batch.Skipped[t] = err.Error()
			continue
		}
		batch.Captured = append(batch.Captured, out)
	}
	return batch, nil
}

// atomicWriteFile writes data to dst via a sibling temp file + rename, so
// concurrent writers cannot produce a torn file and dst is either fully
// written or untouched.
func atomicWriteFile(dst string, data []byte) error {
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
