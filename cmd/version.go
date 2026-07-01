// cmd/version.go
// ckeletin:allow-custom-command

package cmd

import (
	"fmt"
	"runtime/debug"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/spf13/cobra"
)

// buildVersionInfo resolves the version/commit/date to display. ldflags-injected
// values (set on release binaries by goreleaser/Taskfile) always win. For a
// `go install …@version` build — which carries no ldflags and would otherwise
// show the "dev" default with empty commit/date — it falls back to the module
// version and VCS stamps Go embeds in the build info. Kept as a pure function
// (build info injected) so the resolution rules stay unit-testable.
func buildVersionInfo(ldVersion, ldCommit, ldDate string, info *debug.BuildInfo, ok bool) (version, commit, date string) {
	version, commit, date = ldVersion, ldCommit, ldDate
	if !ok || info == nil {
		return version, commit, date
	}
	if (version == "" || version == "dev") && info.Main.Version != "" && info.Main.Version != "(devel)" {
		version = info.Main.Version
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			if commit == "" {
				commit = s.Value
				if len(commit) > 12 {
					commit = commit[:12]
				}
			}
		case "vcs.time":
			if date == "" {
				date = s.Value
			}
		}
	}
	return version, commit, date
}

// versionCmd prints the build version, mirroring the global --version flag.
// Added because `vaultmind version` previously errored with "unknown command"
// while `--help` listed it under Setup — a first-impression papercut surfaced
// by the first knowledge-vault adopter (focalc field report, P2). Checking the
// version is often the very first thing a new user does.
//
// The trailing "(backend: …)" names the embedding backend the binary was built
// against (ort+cpu / ort+coreml / go-cpu). Whether a binary is ORT- or pure-Go
// is invisible without running an embed pass and reading doctor; an adopter
// hitting a silent MiniLM degrade (Siavoush field report, 2026-06-19) can now
// confirm at a glance which binary is on PATH.
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version, commit, build date, and embedding backend",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		info, ok := debug.ReadBuildInfo()
		v, c, d := buildVersionInfo(Version, Commit, Date, info, ok)
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s version %s, commit %s, built at %s (backend: %s)\n",
			binaryName, v, c, d, embedding.Acceleration())
		return err
	},
}

func init() {
	RootCmd.AddCommand(versionCmd)
}
