package commands_test

import (
	"testing"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAskOptions_QueryMaxWords(t *testing.T) {
	const key = "app.ask.query_max_words"
	var opt config.ConfigOption
	for _, candidate := range commands.AskOptions() {
		if candidate.Key == key {
			opt = candidate
			break
		}
	}
	require.Equal(t, key, opt.Key)
	assert.Equal(t, 64, opt.DefaultValue)
	assert.Equal(t, "int", opt.Type)
	assert.Equal(t, "query-max-words", commands.AskMetadata.FlagOverrides[key])
	assert.Equal(t, "VAULTMIND_APP_ASK_QUERY_MAX_WORDS", opt.EnvVarName("VAULTMIND"))
	assert.Contains(t, commands.AskMetadata.Long, "--query-max-words")
	assert.Contains(t, opt.Description, "0")
}
