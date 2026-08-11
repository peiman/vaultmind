package episode

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// cursorSuffix names cursor files distinctly from anything else that might
// live in cursorDir.
const cursorSuffix = ".cursor"

// safeCursorKeyRe is deliberately strict: sessionID/cursor keys ultimately
// come from JSON content INSIDE a transcript file (parsed by sessionIDOf),
// not from a caller-controlled argument — a transcript carrying a
// sessionId like "../../../etc/passwd" must not be able to make
// ReadCursor/WriteCursor touch a path outside cursorDir.
var safeCursorKeyRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func cursorFilePath(cursorDir, key string) (string, error) {
	if key == "" || !safeCursorKeyRe.MatchString(key) {
		return "", fmt.Errorf("unsafe cursor key %q: must match %s", key, safeCursorKeyRe.String())
	}
	return filepath.Join(cursorDir, key+cursorSuffix), nil
}

// ReadCursor returns how many transcript lines have already been captured
// under key — the line CaptureIncremental should resume from. key is an
// opaque cursor-scoping identifier (CaptureIncremental combines the
// session id with the target vault so routing to a different --output-dir
// can't silently consult — or truncate — the wrong vault's progress).
// Absent (never captured before) returns 0, not an error. A present but
// corrupt cursor file errors instead of silently resetting to zero: a
// silent reset would re-render the whole transcript again, recreating the
// exact ever-growing-blob failure this mechanism exists to prevent.
func ReadCursor(cursorDir, key string) (int, error) {
	path, err := cursorFilePath(cursorDir, key)
	if err != nil {
		return 0, err
	}
	// #nosec G304 -- path is built from cursorDir (caller-controlled) plus a
	// key already validated against safeCursorKeyRe above; no traversal.
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read cursor for key %s: %w", key, err)
	}
	line, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("corrupt cursor for key %s: %w", key, err)
	}
	if line < 0 {
		// strconv.Atoi accepts a leading "-", so a corrupt cursor holding a
		// negative number would otherwise sail through this function's own
		// stated contract ("errors instead of silently resetting to zero").
		return 0, fmt.Errorf("corrupt cursor for key %s: negative line %d", key, line)
	}
	return line, nil
}

// WriteCursor persists how many transcript lines have been captured under
// key (see ReadCursor's doc for what key means), creating cursorDir if
// needed. Uses atomic temp-file + rename so a SessionEnd hook killed
// mid-write cannot leave a torn cursor that would silently mis-resume the
// next capture.
func WriteCursor(cursorDir, key string, line int) error {
	if err := os.MkdirAll(cursorDir, 0o750); err != nil {
		return fmt.Errorf("create cursor dir: %w", err)
	}
	dst, err := cursorFilePath(cursorDir, key)
	if err != nil {
		return err
	}
	if err := atomicWriteFile(dst, []byte(strconv.Itoa(line))); err != nil {
		return fmt.Errorf("write cursor for key %s: %w", key, err)
	}
	return nil
}
