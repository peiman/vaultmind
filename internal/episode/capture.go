package episode

import (
	"fmt"
	"os"
	"path/filepath"
)

// Capture parses the transcript at transcriptPath, renders the episode
// markdown, and writes it to outputDir. Returns the written file path.
// Overwrites an existing file with the same derived ID — re-running against
// a finalized transcript is idempotent by design.
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
	md := RenderMarkdown(ep)
	outPath := filepath.Join(outputDir, ep.ID+".md")
	if err := os.WriteFile(outPath, []byte(md), 0o600); err != nil {
		return "", fmt.Errorf("write episode: %w", err)
	}
	return outPath, nil
}
