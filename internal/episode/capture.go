package episode

import (
	"fmt"
	"os"
	"path/filepath"
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
	if ep.ID == "" {
		return "", fmt.Errorf("transcript produced empty episode ID (transcript may be empty or malformed): %s", transcriptPath)
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
