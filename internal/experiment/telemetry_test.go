package experiment_test

import (
	"bytes"
	"testing"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/stretchr/testify/assert"
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
