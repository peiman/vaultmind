package cmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/peiman/vaultmind/internal/mcpserver"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seventyWords() string {
	parts := make([]string, 70)
	for i := range parts {
		parts[i] = "w"
	}
	parts[69] = "LASTWORD"
	return strings.Join(parts, " ")
}

type askCapEnvelope struct {
	SchemaVersion string `json:"schema_version"`
	Result        struct {
		Query       string `json:"query"`
		QueryCapped *struct {
			UsedWords  int `json:"used_words"`
			TotalWords int `json:"total_words"`
		} `json:"query_capped"`
	} `json:"result"`
}

func askCap(t *testing.T, vault string, extra ...string) askCapEnvelope {
	t.Helper()
	args := []string{"ask", seventyWords(), "--vault", vault, "--json", "--pointers-only", "--max-items", "1", "--budget", "200"}
	args = append(args, extra...)
	out, errOut, err := runRootCmd(t, args...)
	require.NoError(t, err, "stderr: %s", errOut.String())
	var env askCapEnvelope
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	return env
}

func isolateAskCap(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
}

func TestAskQueryCap_JSONAndText(t *testing.T) {
	isolateAskCap(t)
	vault := indexedBaselineVault(t)
	query := seventyWords()

	env := askCap(t, vault)
	assert.Equal(t, query, env.Result.Query)
	assert.Equal(t, "v1", env.SchemaVersion)
	require.NotNil(t, env.Result.QueryCapped)
	assert.Equal(t, 64, env.Result.QueryCapped.UsedWords)
	assert.Equal(t, 70, env.Result.QueryCapped.TotalWords)

	out, _, err := runRootCmd(t, "ask", query, "--vault", vault, "--pointers-only", "--max-items", "1", "--budget", "200")
	require.NoError(t, err)
	text := out.String()
	searchAt := strings.Index(text, "Search:")
	require.GreaterOrEqual(t, searchAt, 0)
	capAt := strings.Index(text, "  [query capped: ranked on the first 64 of 70 words]\n")
	require.Greater(t, capAt, searchAt)
	assert.Contains(t, text, "LASTWORD")
}

func TestAskQueryMaxWords_Precedence(t *testing.T) {
	isolateAskCap(t)
	// --config sticks in configFileUsed for the life of the process, and the
	// next test would validate that deleted temp file. Clearing the two
	// package vars is the whole fix: viper.Reset in the subtests already
	// drops flag bindings, and rebinding them (resetCLIState) would resurrect
	// a --vaults value an earlier test had blanked with viper.Set.
	t.Cleanup(func() {
		configFileUsed, configFileStatus = "", ""
		cfgFile = ""
	})
	vault := indexedBaselineVault(t)
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("app:\n  ask:\n    query_max_words: 10\n"), 0o644))

	t.Run("default is 64", func(t *testing.T) {
		viper.Reset()
		env := askCap(t, vault)
		require.NotNil(t, env.Result.QueryCapped)
		assert.Equal(t, 64, env.Result.QueryCapped.UsedWords)
		assert.Equal(t, 70, env.Result.QueryCapped.TotalWords)
	})

	t.Run("config file beats the default", func(t *testing.T) {
		viper.Reset()
		env := askCap(t, vault, "--config", configPath)
		require.NotNil(t, env.Result.QueryCapped)
		assert.Equal(t, 10, env.Result.QueryCapped.UsedWords)
	})

	t.Run("env beats the config file", func(t *testing.T) {
		viper.Reset()
		t.Setenv("VAULTMIND_APP_ASK_QUERY_MAX_WORDS", "5")
		env := askCap(t, vault, "--config", configPath)
		require.NotNil(t, env.Result.QueryCapped)
		assert.Equal(t, 5, env.Result.QueryCapped.UsedWords)
	})

	t.Run("flag beats env", func(t *testing.T) {
		viper.Reset()
		t.Setenv("VAULTMIND_APP_ASK_QUERY_MAX_WORDS", "5")
		env := askCap(t, vault, "--config", configPath, "--query-max-words", "3")
		require.NotNil(t, env.Result.QueryCapped)
		assert.Equal(t, 3, env.Result.QueryCapped.UsedWords)
	})

	t.Run("flag zero turns the cap off", func(t *testing.T) {
		viper.Reset()
		t.Setenv("VAULTMIND_APP_ASK_QUERY_MAX_WORDS", "5")
		env := askCap(t, vault, "--query-max-words", "0")
		assert.Nil(t, env.Result.QueryCapped)
		assert.Equal(t, seventyWords(), env.Result.Query)
	})
}

func TestAskHelp_MentionsQueryMaxWords(t *testing.T) {
	isolateAskCap(t)
	out, _, err := runRootCmd(t, "ask", "--help")
	require.NoError(t, err)
	help := out.String()
	assert.Contains(t, help, "--query-max-words")
	assert.Contains(t, help, "0")
}

func TestMCPAsk_UsesTheSameCap(t *testing.T) {
	isolateAskCap(t)
	vault := indexedBaselineVault(t)
	runner := func(_ context.Context, args []string) ([]byte, error) {
		out, errOut, err := runRootCmd(t, args...)
		if err != nil {
			return nil, err
		}
		if errOut.Len() > 0 && out.Len() == 0 {
			return nil, err
		}
		return out.Bytes(), nil
	}
	ctx := context.Background()
	serverT, clientT := mcp.NewInMemoryTransports()
	server := mcpserver.New(mcpserver.Config{Vault: vault, Run: runner})
	ss, err := server.Connect(ctx, serverT, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, clientT, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "ask",
		Arguments: map[string]any{"query": seventyWords()},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)
	var body strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			body.WriteString(tc.Text)
		}
	}
	var env askCapEnvelope
	require.NoError(t, json.Unmarshal([]byte(body.String()), &env))
	assert.Equal(t, seventyWords(), env.Result.Query)
	require.NotNil(t, env.Result.QueryCapped)
	assert.Equal(t, 64, env.Result.QueryCapped.UsedWords)
	assert.Equal(t, 70, env.Result.QueryCapped.TotalWords)
}
