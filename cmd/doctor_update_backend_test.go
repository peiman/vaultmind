package cmd

import (
	"bytes"
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/release"
	"github.com/stretchr/testify/require"
)

// TestRenderUpdateNotice_DoesNotRecommendADowngrade pins the fix for a defect a
// real adopter caught by using the tool.
//
// cgo cannot travel through the Go module proxy, so `go install` can only ever
// produce the pure-Go MiniLM binary. The notice printed that command to
// everyone — including users whose retrieval tier doctor had just printed as
// `ort+cpu` two lines above. Following it upgrades the version number and
// silently drops the sparse and ColBERT lanes: same tool, worse recall, no
// warning, no error.
//
// doctor already branches remedies on the backend (see minilmRemedy); the update
// notice knew the same fact and ignored it.
func TestRenderUpdateNotice_DoesNotRecommendADowngrade(t *testing.T) {
	info := release.Info{Current: "v0.6.0", Latest: "v0.7.0", Newer: true}

	t.Run("ORT build is never told to go install", func(t *testing.T) {
		var buf bytes.Buffer
		require.NoError(t, renderUpdateNotice(&buf, info, embedding.BackendNameORT))
		out := buf.String()

		// The invariant is that no runnable `go install …@version` is offered —
		// not that the phrase never appears. Naming it in order to warn against
		// it is the point of the line. Banning the substring asserted a proxy
		// for what I cared about rather than the thing itself.
		require.NotContains(t, out, "go install github.com/peiman/vaultmind@",
			"an ORT build that runs this loses sparse+ColBERT")
		require.Contains(t, out, "would drop you to MiniLM",
			"and it must say why, or the omission reads as an oversight")
		require.Contains(t, out, "ort.tar.gz", "the archive is the path that keeps BGE-M3")
		require.Contains(t, out, "task build", "source rebuild also keeps it")
		require.Contains(t, out, "v0.7.0")
	})

	t.Run("pure-Go build still gets the simplest correct path", func(t *testing.T) {
		var buf bytes.Buffer
		require.NoError(t, renderUpdateNotice(&buf, info, embedding.BackendNameGo))
		out := buf.String()

		require.Contains(t, out, "go install github.com/peiman/vaultmind@v0.7.0",
			"this build is already MiniLM, so go install costs it nothing")
	})

	t.Run("silent when already current, on either backend", func(t *testing.T) {
		for _, b := range []string{embedding.BackendNameORT, embedding.BackendNameGo} {
			var buf bytes.Buffer
			require.NoError(t, renderUpdateNotice(&buf, release.Info{
				Current: "v0.7.0", Latest: "v0.7.0", Newer: false,
			}, b))
			require.Empty(t, buf.String())
		}
	})
}
