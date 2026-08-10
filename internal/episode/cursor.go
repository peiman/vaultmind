package episode

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// cursorSuffix names cursor files distinctly from anything else that might
// live in cursorDir.
const cursorSuffix = ".cursor"

func cursorFilePath(cursorDir, sessionID string) string {
	return filepath.Join(cursorDir, sessionID+cursorSuffix)
}

// ReadCursor returns how many transcript lines have already been captured
// for sessionID — the line CaptureIncremental should resume from. Absent
// (never captured before) returns 0, not an error. A present but corrupt
// cursor file errors instead of silently resetting to zero: a silent reset
// would re-render the whole transcript again, recreating the exact
// ever-growing-blob failure this mechanism exists to prevent.
func ReadCursor(cursorDir, sessionID string) (int, error) {
	// #nosec G304 -- cursorDir is caller-controlled (CLI/hook), not user input from a request.
	data, err := os.ReadFile(cursorFilePath(cursorDir, sessionID))
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read cursor for session %s: %w", sessionID, err)
	}
	line, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("corrupt cursor for session %s: %w", sessionID, err)
	}
	return line, nil
}

// WriteCursor persists how many transcript lines have been captured for
// sessionID, creating cursorDir if needed. Uses atomic temp-file + rename so
// a SessionEnd hook killed mid-write cannot leave a torn cursor that would
// silently mis-resume the next capture.
func WriteCursor(cursorDir, sessionID string, line int) error {
	if err := os.MkdirAll(cursorDir, 0o750); err != nil {
		return fmt.Errorf("create cursor dir: %w", err)
	}
	dst := cursorFilePath(cursorDir, sessionID)
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, []byte(strconv.Itoa(line)), 0o600); err != nil {
		return fmt.Errorf("write cursor for session %s: %w", sessionID, err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("commit cursor for session %s: %w", sessionID, err)
	}
	return nil
}
