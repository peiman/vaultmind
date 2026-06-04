package marker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateMarkers_Clean(t *testing.T) {
	raw := []byte("# Title\n\n<!-- VAULTMIND:GENERATED:related:START -->\ncontent\n<!-- VAULTMIND:GENERATED:related:END -->\n")
	issues := ValidateMarkers(raw)
	assert.Empty(t, issues)
}

func TestValidateMarkers_NoMarkers(t *testing.T) {
	raw := []byte("# Title\n\nNo markers here.\n")
	issues := ValidateMarkers(raw)
	assert.Empty(t, issues)
}

func TestValidateMarkers_MalformedUnpairedStart(t *testing.T) {
	raw := []byte("<!-- VAULTMIND:GENERATED:related:START -->\ncontent\n")
	issues := ValidateMarkers(raw)
	assert.Len(t, issues, 1)
	assert.Equal(t, "malformed_markers", issues[0].Rule)
	assert.Equal(t, "related", issues[0].SectionKey)
}

func TestValidateMarkers_MalformedEndWithoutStart(t *testing.T) {
	raw := []byte("content\n<!-- VAULTMIND:GENERATED:related:END -->\n")
	issues := ValidateMarkers(raw)
	assert.Len(t, issues, 1)
	assert.Equal(t, "malformed_markers", issues[0].Rule)
}

func TestValidateMarkers_DuplicateSectionKey(t *testing.T) {
	raw := []byte("<!-- VAULTMIND:GENERATED:related:START -->\nc1\n<!-- VAULTMIND:GENERATED:related:END -->\n\n<!-- VAULTMIND:GENERATED:related:START -->\nc2\n<!-- VAULTMIND:GENERATED:related:END -->\n")
	issues := ValidateMarkers(raw)
	assert.Len(t, issues, 1)
	assert.Equal(t, "duplicate_markers", issues[0].Rule)
	assert.Equal(t, "related", issues[0].SectionKey)
}
