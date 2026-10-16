package cmd

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubEmbedGuard makes the guard read size from rss and record a stop instead
// of ending the test process.
func stubEmbedGuard(t *testing.T, rss func() int64) <-chan int64 {
	t.Helper()
	savedRSS, savedExit, savedEvery := embedRSS, exitOverMemory, embedGuardEvery
	stopped := make(chan int64, 1)
	embedRSS = rss
	exitOverMemory = func(_ io.Writer, _, limit int64) { stopped <- limit }
	embedGuardEvery = time.Millisecond
	t.Cleanup(func() { embedRSS, exitOverMemory, embedGuardEvery = savedRSS, savedExit, savedEvery })
	return stopped
}

// An embed pass that grows past app.embed_max_rss_mb is stopped. 0.10.13's
// re-embed grew one process past 100 GB with nothing to stop it.
func TestGuardEmbedMemory_StopsAPassOverTheLimit(t *testing.T) {
	setEmbedLimit(t, 6144)
	stopped := stubEmbedGuard(t, func() int64 { return 100 << 30 })
	var limit int64
	_, _ = guardEmbedMemory(&cobra.Command{}, func() (*index.EmbedResult, error) {
		select {
		case limit = <-stopped:
		case <-time.After(2 * time.Second):
		}
		return &index.EmbedResult{}, nil
	})
	require.NotZero(t, limit, "a pass at 100 GB was not stopped")
	assert.Equal(t, int64(6144)<<20, limit, "the limit is read in MiB")
}

func TestEmbedMaxRSS_DefaultsTo6144MiB(t *testing.T) {
	var def any
	for _, opt := range commands.WriteOptions() {
		if opt.Key == config.KeyAppEmbedMaxRssMb {
			def = opt.DefaultValue
		}
	}
	require.NotNil(t, def, "%s is not registered", config.KeyAppEmbedMaxRssMb)
	assert.Equal(t, 6144, def, "about twice the measured embed peak")
}

func setEmbedLimit(t *testing.T, mb int) {
	t.Helper()
	saved := viper.Get(config.KeyAppEmbedMaxRssMb)
	viper.Set(config.KeyAppEmbedMaxRssMb, mb)
	t.Cleanup(func() { viper.Set(config.KeyAppEmbedMaxRssMb, saved) })
}

func TestGuardEmbedMemory_ZeroTurnsItOff(t *testing.T) {
	setEmbedLimit(t, 0)
	stopped := stubEmbedGuard(t, func() int64 { return 100 << 30 })
	res, err := guardEmbedMemory(&cobra.Command{}, func() (*index.EmbedResult, error) {
		time.Sleep(20 * time.Millisecond)
		return &index.EmbedResult{Embedded: 3}, nil
	})
	require.NoError(t, err)
	assert.Equal(t, 3, res.Embedded, "the pass's own result comes back")
	assert.Empty(t, stopped, "a limit of 0 must not stop the pass")
}

func TestGuardEmbedMemory_UnderTheLimitRunsToTheEnd(t *testing.T) {
	setEmbedLimit(t, 6144)
	stopped := stubEmbedGuard(t, func() int64 { return 1 << 30 })
	_, err := guardEmbedMemory(&cobra.Command{}, func() (*index.EmbedResult, error) {
		time.Sleep(20 * time.Millisecond)
		return &index.EmbedResult{}, nil
	})
	require.NoError(t, err)
	assert.Empty(t, stopped)
}

// Every embed pass in cmd runs under the memory guard: a RunEmbed call
// anywhere but runEmbedGuarded would run unguarded.
func TestRunEmbed_OnlyThroughTheMemoryGuard(t *testing.T) {
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	fset := token.NewFileSet()
	calls := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		require.NoError(t, err)
		file, err := parser.ParseFile(fset, f, src, 0)
		require.NoError(t, err)
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			ast.Inspect(decl, func(n ast.Node) bool {
				call, isCall := n.(*ast.CallExpr)
				if !isCall {
					return true
				}
				sel, isSel := call.Fun.(*ast.SelectorExpr)
				if !isSel || sel.Sel.Name != "RunEmbed" {
					return true
				}
				calls++
				if !ok || fn.Name.Name != "runEmbedGuarded" {
					t.Errorf("%s: RunEmbed called outside runEmbedGuarded; route it through the memory guard", fset.Position(call.Pos()))
				}
				return true
			})
		}
	}
	require.Equal(t, 1, calls, "RunEmbed is called once, inside runEmbedGuarded")
}
