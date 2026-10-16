package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The gap-log settings are a privacy choice the vault makes, so ask takes
// them from the config file or the environment and has no flag for them.
func TestAsk_HasNoFlagForConfigOnlySettings(t *testing.T) {
	assert.NotNil(t, askCmd.Flags().Lookup("budget"), "ask registers its ordinary flags")
	assert.Nil(t, askCmd.Flags().Lookup("gap-store-query"))
	assert.Nil(t, askCmd.Flags().Lookup("gap-log-limit"))
}
