package commands_test

import (
	"testing"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/gap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAskOptions_QueryShapeBounds(t *testing.T) {
	tests := []struct {
		key  string
		flag string
		env  string
		def  int
	}{
		{
			key:  "app.ask.query_forward_tokens",
			flag: "query-forward-tokens",
			env:  "VAULTMIND_APP_ASK_QUERY_FORWARD_TOKENS",
			def:  1152,
		},
		{
			key:  "app.ask.query_salient_terms",
			flag: "query-salient-terms",
			env:  "VAULTMIND_APP_ASK_QUERY_SALIENT_TERMS",
			def:  64,
		},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			var opt config.ConfigOption
			for _, candidate := range commands.AskOptions() {
				if candidate.Key == tt.key {
					opt = candidate
					break
				}
			}
			require.Equal(t, tt.key, opt.Key)
			assert.Equal(t, tt.def, opt.DefaultValue)
			assert.Equal(t, "int", opt.Type)
			assert.Equal(t, tt.flag, commands.AskMetadata.FlagOverrides[tt.key])
			assert.Equal(t, tt.env, opt.EnvVarName("VAULTMIND"))
			assert.Contains(t, commands.AskMetadata.Long, "--"+tt.flag)
			assert.Contains(t, opt.Description, "0")
		})
	}
	assert.NotContains(t, commands.AskMetadata.Long, "query-max-words")
}

func TestAskOptions_GapHintLog(t *testing.T) {
	var store, limit config.ConfigOption
	for _, candidate := range commands.AskOptions() {
		switch candidate.Key {
		case "app.ask.gap_store_query":
			store = candidate
		case "app.ask.gap_log_limit":
			limit = candidate
		}
	}
	require.Equal(t, "app.ask.gap_store_query", store.Key)
	assert.Equal(t, false, store.DefaultValue)
	assert.True(t, store.NoFlag)
	require.Equal(t, "app.ask.gap_log_limit", limit.Key)
	assert.Equal(t, gap.DefaultLogLimit, limit.DefaultValue)
	assert.Equal(t, "int", limit.Type)
	assert.True(t, limit.NoFlag)
	assert.Empty(t, commands.AskMetadata.FlagOverrides["app.ask.gap_log_limit"])
}
