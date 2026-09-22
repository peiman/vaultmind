package cmd

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseHookVaults(t *testing.T) {
	got, err := parseHookVaults("")
	require.NoError(t, err)
	assert.Nil(t, got, "no flag, no federation")

	_, err = parseHookVaults("/only/one")
	require.Error(t, err, "one vault is not a federation — say so rather than accept it")

	got, err = parseHookVaults(" /a , rel/b ,")
	require.NoError(t, err)
	abs, _ := filepath.Abs("rel/b")
	assert.Equal(t, []string{"/a", abs}, got,
		"relative paths are pinned absolute: hooks run from wherever the agent is")
}
