package vault

import (
	"io/fs"
	"path/filepath"
)

// SkipSymlink reports whether a walked directory entry must not be followed.
//
// Every walk over vault content needs this, and each one that skipped it was a
// hole. filepath.WalkDir does not descend into directory symlinks, but it DOES
// hand back a file symlink named *.md, and the stdlib calls that take a path
// follow it:
//
//   - os.ReadFile turns an untrusted vault into a READ primitive — `secrets.md
//     -> ~/.ssh/id_rsa` was hashed, parsed, stored in FTS, embedded, and
//     returned by `ask`.
//   - os.WriteFile turns it into a WRITE primitive — `notes.md -> ~/.zshrc` was
//     rewritten in place by the wikilink healer.
//
// The rule is the LINK, not the destination: an entry is skipped whatever it
// points at, including a target inside the vault. Resolving first and confining
// after would need EvalSymlinks, whose answer can change between the check and
// the call, and would let one file be handled under two paths — the duplicate-id
// class that took three fixes to close.
//
// This is NOT the confinement check. "Stays under the vault root" and "is not a
// link" are different predicates, and passing the first says nothing about the
// second: the mutator confines every path it writes and would still have
// overwritten ~/.zshrc, because notes.md really is inside the vault. Keep them
// separate; merging them is how one of the two quietly disappears.
//
// d.Type() comes from Lstat, so it describes the link itself rather than its
// target — no extra syscall, and no window between deciding and acting.
//
// Callers must REPORT what they skip, on their result rather than in a log
// line: from the outside, a file passed over and a file that was never there
// look identical. That is why the vault-relative path comes back from this
// call rather than being each caller's job — four hand-rolled copies of the
// same filepath.Rel fallback is how they drift.
func SkipSymlink(root, path string, d fs.DirEntry) (rel string, skip bool) {
	if d.Type()&fs.ModeSymlink == 0 {
		return "", false
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		// Only when root and path cannot be expressed relative to each other
		// (different volumes, or one relative and one absolute). Reporting the
		// absolute path is uglier than a relative one and still names the file,
		// which is the whole point of reporting it.
		rel = path
	}
	return rel, true
}
