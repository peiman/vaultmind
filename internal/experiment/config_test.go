package experiment

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseExperiments_Basic(t *testing.T) {
	raw := map[string]any{
		"retrieval_mode": map[string]any{
			"enabled": true,
			"primary": "hybrid",
			"shadows": []any{"dense", "sparse"},
		},
	}
	got := ParseExperiments(raw)
	assert.Len(t, got, 1)
	exp, ok := got["retrieval_mode"]
	assert.True(t, ok)
	assert.True(t, exp.Enabled)
	assert.Equal(t, "hybrid", exp.Primary)
	assert.Equal(t, []string{"dense", "sparse"}, exp.Shadows)
}

func TestParseExperiments_Disabled(t *testing.T) {
	raw := map[string]any{
		"retrieval_mode": map[string]any{
			"enabled": false,
			"primary": "hybrid",
			"shadows": []any{"dense"},
		},
	}
	got := ParseExperiments(raw)
	assert.Len(t, got, 1)
	exp := got["retrieval_mode"]
	assert.False(t, exp.Enabled)
	assert.Equal(t, "hybrid", exp.Primary)
}

func TestParseExperiments_EmptyMap(t *testing.T) {
	got := ParseExperiments(nil)
	assert.Empty(t, got)
}

func TestParseExperiments_MissingFields(t *testing.T) {
	raw := map[string]any{
		"my_experiment": map[string]any{
			"enabled": true,
		},
	}
	got := ParseExperiments(raw)
	assert.Len(t, got, 1)
	exp := got["my_experiment"]
	assert.True(t, exp.Enabled)
	assert.Empty(t, exp.Primary)
	assert.Empty(t, exp.Shadows)
}

func TestParseExperiments_SkipsReservedKeys(t *testing.T) {
	raw := map[string]any{
		"telemetry": map[string]any{
			"enabled": true,
			"primary": "should-be-skipped",
		},
		"outcome_window_sessions": map[string]any{
			"enabled": true,
			"primary": "also-skipped",
		},
		"real_experiment": map[string]any{
			"enabled": true,
			"primary": "control",
		},
	}
	got := ParseExperiments(raw)
	assert.Len(t, got, 1)
	_, hasTelemetry := got["telemetry"]
	assert.False(t, hasTelemetry)
	_, hasOutcomeWindow := got["outcome_window_sessions"]
	assert.False(t, hasOutcomeWindow)
	_, hasReal := got["real_experiment"]
	assert.True(t, hasReal)
}

func TestAllVariants(t *testing.T) {
	exp := ExperimentDef{
		Enabled: true,
		Primary: "control",
		Shadows: []string{"variant_a", "variant_b"},
	}
	got := exp.AllVariants()
	assert.Equal(t, []string{"control", "variant_a", "variant_b"}, got)
}

func TestAllVariants_NoShadows(t *testing.T) {
	exp := ExperimentDef{
		Enabled: true,
		Primary: "control",
	}
	got := exp.AllVariants()
	assert.Equal(t, []string{"control"}, got)
}
