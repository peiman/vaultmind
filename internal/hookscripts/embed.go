// Package hookscripts is the single source of truth for VaultMind's
// Claude Code hook scripts. The .sh files in this directory are
// embedded into the vaultmind binary at build time and are also the
// scripts THIS repo's own SessionStart hook invokes via the local
// path under $CLAUDE_PROJECT_DIR.
//
// Two SSOT properties matter:
//
//   - **One canonical source.** No copy of these scripts exists
//     elsewhere in the vaultmind repo. The local repo's hook config
//     points at `internal/hookscripts/<name>.sh` directly; the
//     binary embeds the same files via `//go:embed`. Drift between
//     a "disk version" and an "embedded version" is structurally
//     impossible because there is no second disk version.
//
//   - **Drift-detect across consumers.** Other projects that run
//     `vaultmind hooks install` get COPIES written to their own
//     `.claude/scripts/`. Those copies CAN drift from the binary
//     as vaultmind ships new versions. The drift is bounded by
//     `vaultmind doctor`'s hook-drift check — it hashes the
//     installed copies against the embedded canonical and surfaces
//     mismatches. The resolution is one command:
//     `vaultmind hooks install --force`.
//
// Adding a new hook: place the .sh file in this directory; the
// embed FS picks it up automatically. Update `Names()` if the
// caller needs to enumerate them.
package hookscripts

import (
	"embed"
	"io/fs"
	"sort"
)

//go:embed *.sh
var fsys embed.FS

// All returns the embedded scripts as a map of base filename → bytes.
// Stable order via sorted keys when iterated; callers that need
// determinism should sort `Names()` or iterate the returned map via
// a sorted list.
func All() map[string][]byte {
	out := make(map[string][]byte)
	entries, err := fsys.ReadDir(".")
	if err != nil {
		return out
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := fsys.ReadFile(e.Name())
		if err != nil {
			continue
		}
		out[e.Name()] = data
	}
	return out
}

// Names returns the list of embedded hook script filenames in
// sorted order. Useful for callers that want a stable iteration
// (doctor's drift-check report, hooks install's per-file output).
func Names() []string {
	scripts := All()
	names := make([]string, 0, len(scripts))
	for name := range scripts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Get returns the bytes of one embedded hook script by filename
// (e.g. "load-persona.sh"). Returns nil + false if not embedded.
// Lookup is exact-match on the base filename; no path traversal.
func Get(name string) ([]byte, bool) {
	data, err := fsys.ReadFile(name)
	if err != nil {
		return nil, false
	}
	return data, true
}

// FS exposes the embed.FS for callers that need fs.FS semantics
// (range-walk, custom predicates). Most callers should prefer All
// or Get; this is the escape hatch.
func FS() fs.FS {
	return fsys
}
