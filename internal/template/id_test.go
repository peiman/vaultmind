package template

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateID_Basic(t *testing.T) {
	assert.Equal(t, "project-payment-retries", GenerateID("projects/payment-retries.md", "project"))
}

func TestGenerateID_NestedPath(t *testing.T) {
	assert.Equal(t, "decision-pause-billing", GenerateID("decisions/q3/pause-billing.md", "decision"))
}

func TestGenerateID_Spaces(t *testing.T) {
	assert.Equal(t, "concept-working-memory", GenerateID("concepts/Working Memory.md", "concept"))
}

func TestGenerateID_UpperCase(t *testing.T) {
	assert.Equal(t, "source-anderson-2023", GenerateID("sources/Anderson-2023.md", "source"))
}
