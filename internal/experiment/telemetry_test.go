package experiment_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTelemetryChoice_Defaults(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"1", experiment.TelemetryAnonymous},
		{"2", experiment.TelemetryFull},
		{"3", experiment.TelemetryOff},
		{"", experiment.TelemetryAnonymous},
	}

	for _, tt := range tests {
		t.Run("input_"+tt.input, func(t *testing.T) {
			got := experiment.ParseTelemetryChoice(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestParseTelemetryChoice_Invalid(t *testing.T) {
	tests := []struct {
		input string
	}{
		{"4"},
		{"abc"},
	}

	for _, tt := range tests {
		t.Run("input_"+tt.input, func(t *testing.T) {
			got := experiment.ParseTelemetryChoice(tt.input)
			assert.Equal(t, experiment.TelemetryAnonymous, got)
		})
	}
}

func TestFormatTelemetryPrompt(t *testing.T) {
	var buf bytes.Buffer
	experiment.WriteTelemetryPrompt(&buf)
	output := buf.String()

	assert.Contains(t, output, "Help improve VaultMind")
	assert.Contains(t, output, "[1]")
	assert.Contains(t, output, "[2]")
	assert.Contains(t, output, "[3]")
}

func TestPromptTelemetry(t *testing.T) {
	var out bytes.Buffer
	result := experiment.PromptTelemetry(strings.NewReader("2\n"), &out)
	assert.Equal(t, experiment.TelemetryFull, result)
	assert.Contains(t, out.String(), "Help improve VaultMind")
}

func TestPromptTelemetry_Default(t *testing.T) {
	var out bytes.Buffer
	result := experiment.PromptTelemetry(strings.NewReader("\n"), &out)
	assert.Equal(t, experiment.TelemetryAnonymous, result)
}

func TestIsFirstRun_EmptyDB(t *testing.T) {
	db := openTestExpDB(t)
	first, err := db.IsFirstRun()
	require.NoError(t, err)
	assert.True(t, first)
}

func TestIsFirstRun_AfterSession(t *testing.T) {
	db := openTestExpDB(t)
	sid, err := db.StartSession("/vault")
	require.NoError(t, err)
	err = db.EndSession(sid)
	require.NoError(t, err)

	first, err := db.IsFirstRun()
	require.NoError(t, err)
	assert.False(t, first)
}

func TestUpdateSessionVaultPath(t *testing.T) {
	db := openTestExpDB(t)
	sid, err := db.StartSession("")
	require.NoError(t, err)

	err = db.UpdateSessionVaultPath(sid, "/vault/path")
	require.NoError(t, err)

	var vaultPath string
	err = db.QueryRow("SELECT vault_path FROM sessions WHERE session_id = ?", sid).Scan(&vaultPath)
	require.NoError(t, err)
	assert.Equal(t, "/vault/path", vaultPath)
}
