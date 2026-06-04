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
