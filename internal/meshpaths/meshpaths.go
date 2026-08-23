// Package meshpaths is the SSOT for the mesh wake-watcher's on-disk state:
// which directory it lives in, and what each per-agent file is called.
//
// Before it existed there were four independent opinions — two of mira's
// watcher scripts, workhorse's, and doctor's checker — aligned only by comments
// asserting agreement. One of those comments ("matches vaultmind doctor's
// xdg.ConfigFile path") was false on darwin for months, which is how a watcher
// dead for seven days reported as "not found": the checker was reading a
// directory nothing writes to.
//
// The directory is xdg.ConfigDir(), which since the 2026-08-23 configBase fix
// resolves to ${XDG_CONFIG_HOME:-~/.config}/vaultmind on every Unix platform —
// the same expression the shell scripts have always used, now agreed on by
// mechanism (TestDir_MatchesTheShellDerivation) rather than by comment.
// $VAULTMIND_MESH_DIR overrides outright.
package meshpaths

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/peiman/vaultmind/internal/xdg"
)

const (
	// ListenFilename is the shared listen-control file. Deliberately NOT
	// slug-suffixed: it is one SSOT for every agent on the machine, with
	// per-agent sections inside it.
	ListenFilename = "mesh-listen.json"

	// RegistryFilename is chat-mcp's identity registry (slug → project path).
	// Written by a peer tool we do not own; named here so nothing else spells it.
	RegistryFilename = "agents.yaml"

	filePrefix = "mesh-watch-"
)

// ErrNoSlug means the per-agent paths cannot be derived. A path built from an
// empty slug would be a real file ("mesh-watch-.heartbeat") that a checker
// would then report on as if it meant something.
var ErrNoSlug = errors.New("meshpaths: no agent slug — cannot derive per-agent mesh paths")

// Paths is every mesh state path for one agent.
type Paths struct {
	Slug      string
	Dir       string
	Heartbeat string
	Pid       string
	Lastwake  string
	Lastarm   string
	Disarm    string
	Log       string
	Listen    string
	Registry  string
}

// Dir returns the mesh state directory: $VAULTMIND_MESH_DIR, else
// xdg.ConfigDir() (${XDG_CONFIG_HOME:-~/.config}/vaultmind on Unix).
func Dir() (string, error) {
	if dir := os.Getenv("VAULTMIND_MESH_DIR"); dir != "" {
		return dir, nil
	}
	return xdg.ConfigDir()
}

// For derives every per-agent path from the slug. ErrNoSlug on "".
func For(slug string) (Paths, error) {
	if slug == "" {
		return Paths{}, ErrNoSlug
	}
	dir, err := Dir()
	if err != nil {
		return Paths{}, err
	}
	per := func(ext string) string {
		return filepath.Join(dir, filePrefix+slug+"."+ext)
	}
	return Paths{
		Slug:      slug,
		Dir:       dir,
		Heartbeat: per("heartbeat"),
		Pid:       per("pid"),
		Lastwake:  per("lastwake"),
		Lastarm:   per("lastarm"),
		Disarm:    per("disarm"),
		Log:       per("log"),
		Listen:    filepath.Join(dir, ListenFilename),
		Registry:  filepath.Join(dir, RegistryFilename),
	}, nil
}

// Heartbeat is the one-path convenience used by doctor's default.
func Heartbeat(slug string) (string, error) {
	p, err := For(slug)
	if err != nil {
		return "", err
	}
	return p.Heartbeat, nil
}
