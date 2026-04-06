package memory

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEstimateTokens_Basic(t *testing.T) {
	assert.Equal(t, 1, EstimateTokens("hi"))
	assert.Equal(t, 3, EstimateTokens("hello world"))
	assert.Equal(t, 0, EstimateTokens(""))
}

func TestEstimateTokens_CeilingDivision(t *testing.T) {
	assert.Equal(t, 1, EstimateTokens("abc"))
	assert.Equal(t, 1, EstimateTokens("abcd"))
	assert.Equal(t, 2, EstimateTokens("abcde"))
}
