package vault

import "path/filepath"

// CanonicalKey is the stable identity of a vault directory: absolute, and
// symlink-resolved when the path exists. Calibration stores this string on
// both the write side and the read side, and the possible-gap log hashes it
// for a per-vault file name. There is no separate vault id.
//
// The default vault path is "." (relative). Without this, two vaults indexed
// from different working directories would both store "." and collide.
// EvalSymlinks fails for a path that does not exist yet; Abs still yields a
// deterministic key.
func CanonicalKey(vaultPath string) string {
	abs, err := filepath.Abs(vaultPath)
	if err != nil {
		return filepath.Clean(vaultPath)
	}
	if resolved, evalErr := filepath.EvalSymlinks(abs); evalErr == nil {
		return resolved
	}
	return abs
}
