package cmd

import (
	"context"
	"fmt"
	"io"
	"runtime/debug"

	"github.com/peiman/vaultmind/internal/release"
	"github.com/peiman/vaultmind/internal/xdg"
)

// writeUpdateNotice prints one line when a newer VaultMind exists.
//
// Nothing used to say so. A release could ship a fix for a silent failure and
// the people it affected would keep running the broken version, because the only
// way to find out was to go and look — the same shape as hook drift: a real
// difference that no surface reports.
//
// Doctor is the right and ONLY home for it. It is the command someone runs to
// ask "is my setup healthy?", so a network call here is expected rather than a
// surprise, and putting it on every command would turn a courtesy into a tax on
// `ask`. Silent whenever the answer is not clearly useful: opted out, offline,
// unversioned build, already current.
// resolvedVersion is what `vaultmind version` would print — ldflags when a
// release binary was stamped, else the module version Go embeds for a
// `go install …@version` build. Resolved the same way in both places so the
// update notice can never disagree with the version command beside it.
func resolvedVersion() string {
	info, ok := debug.ReadBuildInfo()
	v, _, _ := buildVersionInfo(Version, Commit, Date, info, ok)
	return v
}

func writeUpdateNotice(w io.Writer, currentVersion string) error {
	cacheDir, err := xdg.CacheDir()
	if err != nil {
		cacheDir = "" // no cache: Check still works, it just re-asks next time
	}
	info, ok := release.Check(context.Background(), currentVersion, cacheDir)
	if !ok || !info.Newer {
		return nil
	}
	_, err = fmt.Fprintf(w,
		"⬆ VaultMind %s is available (running %s)\n"+
			"  go install github.com/peiman/vaultmind@%s   (or download the ORT archive from the release)\n"+
			"  silence this: %s=1\n",
		info.Latest, info.Current, info.Latest, release.DisableEnv)
	return err
}
