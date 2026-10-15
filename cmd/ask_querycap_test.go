package cmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/mcpserver"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func nWords(n int, last string) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "w"
	}
	if last != "" && n > 0 {
		parts[n-1] = last
	}
	return strings.Join(parts, " ")
}

type askCapEnvelope struct {
	SchemaVersion string `json:"schema_version"`
	Result        struct {
		Query       string `json:"query"`
		QueryCapped *struct {
			EmbeddedTokens int `json:"embedded_tokens"`
			TotalTokens    int `json:"total_tokens"`
			SalientTerms   int `json:"salient_terms"`
		} `json:"query_capped"`
	} `json:"result"`
}

func askCapQuery(t *testing.T, vault, query string, extra ...string) (askCapEnvelope, []byte) {
	t.Helper()
	args := []string{"ask", query, "--vault", vault, "--json", "--pointers-only", "--max-items", "1", "--budget", "200"}
	args = append(args, extra...)
	out, errOut, err := runRootCmd(t, args...)
	require.NoError(t, err, "stderr: %s", errOut.String())
	var env askCapEnvelope
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	return env, out.Bytes()
}

func isolateAskCap(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
}

func TestAskQueryCap_JSONAndText(t *testing.T) {
	isolateAskCap(t)
	vault := indexedBaselineVault(t)
	query := nWords(1160, "LASTWORD")

	env, raw := askCapQuery(t, vault, query)
	assert.Equal(t, query, env.Result.Query)
	assert.Equal(t, "v1", env.SchemaVersion)
	require.NotNil(t, env.Result.QueryCapped)
	assert.Equal(t, 1152, env.Result.QueryCapped.EmbeddedTokens)
	assert.Equal(t, 1160, env.Result.QueryCapped.TotalTokens)
	assert.Equal(t, 0, env.Result.QueryCapped.SalientTerms, "a keyword vault has no sparse weights")
	assert.Contains(t, string(raw), `"salient_terms":0`)
	assert.Contains(t, string(raw), `"total_tokens":1160`)
	assert.NotContains(t, string(raw), "total_chars")
	assert.NotContains(t, string(raw), "used_words")

	out, _, err := runRootCmd(t, "ask", query, "--vault", vault, "--pointers-only", "--max-items", "1", "--budget", "200")
	require.NoError(t, err)
	text := out.String()
	searchAt := strings.Index(text, "Search:")
	require.GreaterOrEqual(t, searchAt, 0)
	capAt := strings.Index(text, "  [long query: ranked on its first 1152 of 1160 tokens]\n")
	require.Greater(t, capAt, searchAt)
	assert.NotContains(t, text, "salient")
	assert.NotContains(t, text, "LASTWORD", "the Search line does not echo the tail of a long query")
	assert.Contains(t, text, "chars)")
}

func TestAskQueryForwardTokens_Precedence(t *testing.T) {
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
	query := nWords(1160, "LASTWORD")
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("app:\n  ask:\n    query_forward_tokens: 10\n"), 0o644))

	t.Run("default is 1152", func(t *testing.T) {
		viper.Reset()
		env, _ := askCapQuery(t, vault, query)
		require.NotNil(t, env.Result.QueryCapped)
		assert.Equal(t, 1152, env.Result.QueryCapped.EmbeddedTokens)
		assert.Equal(t, 1160, env.Result.QueryCapped.TotalTokens)
	})

	t.Run("config file beats the default", func(t *testing.T) {
		viper.Reset()
		env, _ := askCapQuery(t, vault, query, "--config", configPath)
		require.NotNil(t, env.Result.QueryCapped)
		assert.Equal(t, 10, env.Result.QueryCapped.EmbeddedTokens)
	})

	t.Run("env beats the config file", func(t *testing.T) {
		viper.Reset()
		t.Setenv("VAULTMIND_APP_ASK_QUERY_FORWARD_TOKENS", "5")
		env, _ := askCapQuery(t, vault, query, "--config", configPath)
		require.NotNil(t, env.Result.QueryCapped)
		assert.Equal(t, 5, env.Result.QueryCapped.EmbeddedTokens)
	})

	t.Run("flag beats env", func(t *testing.T) {
		viper.Reset()
		t.Setenv("VAULTMIND_APP_ASK_QUERY_FORWARD_TOKENS", "5")
		env, _ := askCapQuery(t, vault, query, "--config", configPath, "--query-forward-tokens", "3")
		require.NotNil(t, env.Result.QueryCapped)
		assert.Equal(t, 3, env.Result.QueryCapped.EmbeddedTokens)
	})

	t.Run("flag zero turns the forward bound off", func(t *testing.T) {
		viper.Reset()
		t.Setenv("VAULTMIND_APP_ASK_QUERY_FORWARD_TOKENS", "5")
		env, _ := askCapQuery(t, vault, query, "--query-forward-tokens", "0")
		assert.Nil(t, env.Result.QueryCapped)
		assert.Equal(t, query, env.Result.Query)
	})

	t.Run("salient zero does not turn the forward bound off", func(t *testing.T) {
		viper.Reset()
		env, _ := askCapQuery(t, vault, query, "--query-salient-terms", "0")
		require.NotNil(t, env.Result.QueryCapped)
		assert.Equal(t, 1152, env.Result.QueryCapped.EmbeddedTokens)
		assert.Equal(t, 0, env.Result.QueryCapped.SalientTerms)
	})
}

func TestAskQuerySalientTerms_Precedence(t *testing.T) {
	isolateAskCap(t)
	t.Cleanup(func() {
		configFileUsed, configFileStatus = "", ""
		cfgFile = ""
	})
	vault := indexedBaselineVault(t)
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("app:\n  ask:\n    query_salient_terms: 10\n"), 0o644))
	const key = config.KeyAppAskQuerySalientTerms

	t.Run("default is 64", func(t *testing.T) {
		viper.Reset()
		_, _ = askCapQuery(t, vault, "spreading activation")
		assert.Equal(t, 64, getConfigValueWithFlags[int](askCmd, "query-salient-terms", key))
	})
	t.Run("config file beats the default", func(t *testing.T) {
		viper.Reset()
		_, _ = askCapQuery(t, vault, "spreading activation", "--config", configPath)
		assert.Equal(t, 10, getConfigValueWithFlags[int](askCmd, "query-salient-terms", key))
	})
	t.Run("env beats the config file", func(t *testing.T) {
		viper.Reset()
		t.Setenv("VAULTMIND_APP_ASK_QUERY_SALIENT_TERMS", "5")
		_, _ = askCapQuery(t, vault, "spreading activation", "--config", configPath)
		assert.Equal(t, 5, getConfigValueWithFlags[int](askCmd, "query-salient-terms", key))
	})
	t.Run("flag beats env", func(t *testing.T) {
		viper.Reset()
		t.Setenv("VAULTMIND_APP_ASK_QUERY_SALIENT_TERMS", "5")
		_, _ = askCapQuery(t, vault, "spreading activation", "--config", configPath, "--query-salient-terms", "3")
		assert.Equal(t, 3, getConfigValueWithFlags[int](askCmd, "query-salient-terms", key))
	})
	t.Run("flag zero is off", func(t *testing.T) {
		viper.Reset()
		t.Setenv("VAULTMIND_APP_ASK_QUERY_SALIENT_TERMS", "5")
		_, _ = askCapQuery(t, vault, "spreading activation", "--query-salient-terms", "0")
		assert.Equal(t, 0, getConfigValueWithFlags[int](askCmd, "query-salient-terms", key))
	})
}

func TestAskHelp_MentionsQueryShapeBounds(t *testing.T) {
	isolateAskCap(t)
	out, _, err := runRootCmd(t, "ask", "--help")
	require.NoError(t, err)
	help := out.String()
	assert.Contains(t, help, "--query-forward-tokens")
	assert.NotContains(t, help, "--query-forward-words")
	assert.Contains(t, help, "--query-salient-terms")
	assert.Contains(t, help, "0")
	assert.NotContains(t, help, "--query-max-words")
}

// ask stamps note_accesses with time.Now before it hashes the sqlite file, so
// two asks of byte-identical vaults still differ in index_hash. Each run gets
// its own clone; the comparison blanks the clock, the hash, and the path.
var askRunFields = regexp.MustCompile(`"(timestamp|index_hash|vault_path)":"[^"]*"`)

func withoutRunFields(b []byte) []byte {
	return askRunFields.ReplaceAll(b, []byte(`"$1":""`))
}

func cloneIndexedVault(t *testing.T, src string) string {
	t.Helper()
	dst := t.TempDir()
	require.NoError(t, os.CopyFS(dst, os.DirFS(src)))
	return dst
}

func TestAskQueryCap_ShortQueryIsByteIdenticalToOff(t *testing.T) {
	isolateAskCap(t)
	src := indexedBaselineVault(t)
	const query = "spreading activation"
	onVault := cloneIndexedVault(t, src)
	offVault := cloneIndexedVault(t, src)

	on, _, err := runRootCmd(t, "ask", query, "--vault", onVault, "--json", "--pointers-only", "--max-items", "1", "--budget", "200")
	require.NoError(t, err)
	off, _, err := runRootCmd(t, "ask", query, "--vault", offVault, "--json", "--pointers-only", "--max-items", "1", "--budget", "200",
		"--query-forward-tokens", "0", "--query-salient-terms", "0")
	require.NoError(t, err)
	assert.Equal(t, withoutRunFields(on.Bytes()), withoutRunFields(off.Bytes()))
	assert.NotContains(t, on.String(), "query_capped")
	assert.NotContains(t, on.String(), "long query")
	assert.Regexp(t, `"index_hash":"[0-9a-f]{64}"`, on.String())

	textOn, _, err := runRootCmd(t, "ask", query, "--vault", cloneIndexedVault(t, src), "--pointers-only", "--max-items", "1", "--budget", "200")
	require.NoError(t, err)
	textOff, _, err := runRootCmd(t, "ask", query, "--vault", cloneIndexedVault(t, src), "--pointers-only", "--max-items", "1", "--budget", "200",
		"--query-forward-tokens", "0", "--query-salient-terms", "0")
	require.NoError(t, err)
	assert.Equal(t, textOn.Bytes(), textOff.Bytes())
	assert.NotContains(t, textOn.String(), "long query")
}

func TestMCPAsk_ShortQueryMatchesTheCLI(t *testing.T) {
	isolateAskCap(t)
	src := indexedBaselineVault(t)
	const query = "spreading activation"
	onVault := cloneIndexedVault(t, src)
	offVault := cloneIndexedVault(t, src)
	mcpVault := cloneIndexedVault(t, src)
	// The same argv the ask tool builds: ask --json --vault=<path> -- <query>.
	on, _, err := runRootCmd(t, "ask", "--json", "--vault="+onVault, "--", query)
	require.NoError(t, err)
	off, _, err := runRootCmd(t, "ask", "--json", "--vault="+offVault,
		"--query-forward-tokens", "0", "--query-salient-terms", "0", "--", query)
	require.NoError(t, err)
	assert.Equal(t, withoutRunFields(on.Bytes()), withoutRunFields(off.Bytes()),
		"a short MCP query is the CLI query, and that query is unchanged with both bounds off")

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
	server := mcpserver.New(mcpserver.Config{Vault: mcpVault, Run: runner})
	ss, err := server.Connect(ctx, serverT, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, clientT, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "ask",
		Arguments: map[string]any{"query": query},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)
	var body strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			body.WriteString(tc.Text)
		}
	}
	assert.Equal(t, withoutRunFields(on.Bytes()), withoutRunFields([]byte(body.String())))
	assert.NotContains(t, body.String(), "query_capped")
}
