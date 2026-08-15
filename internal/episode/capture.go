package episode

import (
	"crypto/sha256"
	"encoding/hex"
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
	ep, outPath, err := prepareEpisode(transcriptPath, outputDir)
	if err != nil {
		return "", err
	}
	return outPath, writeEpisode(ep, outPath)
}

// prepareEpisode parses and validates a transcript and derives the path its
// episode would be written to, without writing anything. CaptureDir needs the
// destination BEFORE the write in order to detect that two transcripts claim
// the same one — a check that is worthless after the fact, since by then the
// second has already overwritten the first.
func prepareEpisode(transcriptPath, outputDir string) (*Episode, string, error) {
	ep, err := ParseTranscript(transcriptPath)
	if err != nil {
		return nil, "", err
	}
	if ep.SessionID == "" {
		return nil, "", fmt.Errorf("transcript has no session id — empty or not a Claude Code transcript: %s", transcriptPath)
	}
	return ep, filepath.Join(outputDir, ep.ID+".md"), nil
}

func writeEpisode(ep *Episode, outPath string) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o750); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	if err := atomicWriteFile(outPath, []byte(RenderMarkdown(ep))); err != nil {
		return fmt.Errorf("write episode: %w", err)
	}
	return nil
}

// vaultScopedCursorKey combines outputDir and sessionID into a single cursor
// key so that capturing the same session into two different --output-dirs
// (vaults) never shares — or silently truncates — the same cursor. Keying
// purely on sessionID would mean pointing capture at a second vault while
// reusing the same --cursor-dir resumes from wherever the FIRST vault's
// capture left off, silently skipping content the second vault has never
// actually captured.
//
// outputDir is hashed rather than embedded verbatim because it's an
// arbitrary filesystem path — spaces, unicode, and other characters
// cursorFilePath's safeCursorKeyRe rejects are all valid there. sessionID
// stays in the clear for debuggability; cursorFilePath still validates the
// composed result, so a transcript carrying a malicious sessionId is
// rejected exactly as before.
func vaultScopedCursorKey(outputDir, sessionID string) (string, error) {
	abs, err := filepath.Abs(outputDir)
	if err != nil {
		return "", fmt.Errorf("resolve output dir: %w", err)
	}
	sum := sha256.Sum256([]byte(filepath.Clean(abs)))
	return sessionID + "-" + hex.EncodeToString(sum[:])[:12], nil
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
	cursorKey, err := vaultScopedCursorKey(outputDir, sessionID)
	if err != nil {
		return "", err
	}

	startLine, err := ReadCursor(cursorDir, cursorKey)
	if err != nil {
		return "", err
	}

	ep, endLine, err := ParseTranscriptFrom(transcriptPath, startLine)
	if err != nil {
		return "", err
	}
	if endLine == startLine {
		// Looks like a no-op, but ParseTranscriptFrom can't tell "genuinely
		// nothing new" apart from "the file shrank below startLine" — both
		// return endLine == startLine (it never regresses on its own). This
		// is the one place that CAN tell: startLine came from a real cursor
		// WE persisted for THIS session, so if the file's true current line
		// count is now less than that, the transcript was truncated,
		// rotated, or replaced underneath us. Left unhandled, the cursor
		// would stay stuck there forever, reporting "nothing new" on every
		// future call while real content silently piles up uncaptured.
		//
		// CountLines is a cheap line-count-only pass (no JSON parsing), paid
		// only on this branch — the hook also fires on Stop (every turn),
		// not just SessionEnd, so a true no-op is the common case and must
		// not pay for a second full parse every time.
		if total, cErr := CountLines(transcriptPath); cErr == nil && total < startLine {
			// Reset the cursor to 0 FIRST, then recurse — the recursive
			// call's own ReadCursor must see the reset, not the stale
			// value, or this would loop forever re-detecting the same
			// shrinkage instead of actually re-capturing.
			if err := WriteCursor(cursorDir, cursorKey, 0); err != nil {
				return "", fmt.Errorf("reset cursor after detecting a shrunken transcript: %w", err)
			}
			return CaptureIncremental(transcriptPath, outputDir, cursorDir)
		}
		return "", nil // genuinely nothing new since the last capture
	}
	if !hasCapturableContent(ep) {
		// The transcript grew, but only with filtered noise (e.g. more
		// system-reminders) — advance the cursor so it's never re-scanned,
		// but don't write an episode file with nothing in it.
		return "", WriteCursor(cursorDir, cursorKey, endLine)
	}

	if err := os.MkdirAll(outputDir, 0o750); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}
	ep.ID = deriveSegmentID(ep.StartedAt, sessionID, startLine)
	outPath := filepath.Join(outputDir, ep.ID+".md")
	if err := atomicWriteFile(outPath, []byte(RenderMarkdown(ep))); err != nil {
		return "", fmt.Errorf("write episode segment: %w", err)
	}
	if err := WriteCursor(cursorDir, cursorKey, endLine); err != nil {
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
//
// The "-partNNNNNNNN" suffix is that raw transcript line number, not a
// sequential 0/1/2/… segment index — two segments from the same session can
// legitimately jump from, say, -part00000012 to -part00000340 if a lot of
// filtered noise separated them, with nothing in between.
func deriveSegmentID(startedAt, sessionID string, startLine int) string {
	return fmt.Sprintf("%s-part%08d", deriveID(startedAt, sessionID), startLine)
}

// CaptureBatch summarizes a directory capture: the episode files written and the
// transcripts that were skipped (with the reason). Skipping is deliberate — a noise
// or partial transcript in a large history must not abort the whole batch.
//
// Sidechains is counted separately from Skipped because it is not a fault:
// subagent transcripts are routine, they outnumber real sessions by an order of
// magnitude (1,759 to 141 across four measured project histories), and listing
// them as problems would bury the handful that are.
// Collisions is separate from Skipped because it is the one outcome where the
// user may actually have lost something: two transcripts existed and only one
// is representable. That calls for naming the pair, where a malformed file only
// calls for a count.
type CaptureBatch struct {
	Captured   []string          // episode file paths written, in transcript-path order
	Skipped    map[string]string // transcript path -> reason (empty/malformed/write error)
	Collisions map[string]string // transcript path -> the episode id it lost, and to whom
	Sidechains int               // subagent/workflow transcripts passed over by design
}

// CaptureDir captures every *.jsonl transcript found recursively under dir into
// outputDir. It is the bootstrap entry point: point it at a project's Claude Code
// transcript directory (e.g. ~/.claude/projects/<slug>) to seed an identity vault
// from sessions that already exist. Malformed/empty transcripts go into
// Skipped rather than failing the run; capture itself is idempotent.
//
// Subagent and workflow transcripts are passed over (counted in Sidechains).
// They are records of tools running, not of the collaboration, and because they
// carry the parent session's id they derive the same episode filename as the
// real session — capturing them overwrites it. Pointing `episode capture` at a
// single sidechain file still captures it: an explicit path is a choice, a
// directory sweep is not.
//
// Two transcripts can still derive one episode id (same date, same session id
// prefix). The first wins and the rest are reported as collisions, so Captured
// always equals the files on disk — a count the user can check with `ls`.
func CaptureDir(dir, outputDir string) (CaptureBatch, error) {
	batch := CaptureBatch{Skipped: map[string]string{}, Collisions: map[string]string{}}
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
	writtenBy := map[string]string{} // episode path -> the transcript that claimed it
	for _, t := range transcripts {
		if isSidechainTranscript(t) {
			batch.Sidechains++
			continue
		}
		ep, out, err := prepareEpisode(t, outputDir)
		if err != nil {
			batch.Skipped[t] = err.Error()
			continue
		}
		if first, taken := writtenBy[out]; taken {
			batch.Collisions[t] = fmt.Sprintf("derives the same episode id as %s (%s) — kept the first", first, filepath.Base(out))
			continue
		}
		if err := writeEpisode(ep, out); err != nil {
			batch.Skipped[t] = err.Error()
			continue
		}
		writtenBy[out] = t
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
