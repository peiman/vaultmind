package cmd

import (
	"context"
	"fmt"
	"io"
	"runtime/debug"
	"strings"

	"github.com/peiman/vaultmind/internal/embedding"
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
	if !ok {
		return nil
	}
	return renderUpdateNotice(w, info, embedding.BackendName())
}

// renderUpdateNotice is the printing half, split from the deciding half so the
// notice itself is testable without a network stub reaching into another
// package. Silent when there is nothing to say.
//
// The upgrade path is branched on the backend, for the same reason minilmRemedy
// branches: cgo cannot travel through the Go module proxy, so `go install` can
// only ever produce the pure-Go MiniLM binary. Printing it to an ORT user —
// whose retrieval tier this same command reports two lines above — tells them to
// raise their version number and silently drop the sparse and ColBERT lanes.
// Nothing errors, nothing warns; recall just gets quietly worse. An adopter
// found this by reading the notice on a live ORT install and asking whether it
// was safe. It was not.
func renderUpdateNotice(w io.Writer, info release.Info, backend string) error {
	if !info.Newer {
		return nil
	}
	upgrade := fmt.Sprintf(
		"  go install github.com/peiman/vaultmind@%s   (or download the ORT archive from the release)",
		info.Latest)
	if backend == embedding.BackendNameORT {
		upgrade = fmt.Sprintf(
			"  This build runs the full BGE-M3 hybrid. Keep it — `go install` cannot "+
				"(cgo does not\n"+
				"  travel through the module proxy) and would drop you to MiniLM at the same version.\n"+
				"    • download  vaultmind_%s_<os>_<arch>_ort.tar.gz  from the release, or\n"+
				"    • from source:  git pull && task build",
			strings.TrimPrefix(info.Latest, "v"))
	}
	_, err := fmt.Fprintf(w,
		"⬆ VaultMind %s is available (running %s)\n%s\n  silence this: %s=1\n",
		info.Latest, info.Current, upgrade, release.DisableEnv)
	return err
}
