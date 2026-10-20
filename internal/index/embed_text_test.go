package index

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNoteEmbedText(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "body", noteEmbedText("", "body"), "an empty title leaves the body")
	assert.Equal(t, "body", noteEmbedText(" \t", "body"), "a whitespace title leaves the body")
	assert.Equal(t, "title\n\nbody", noteEmbedText("title", "body"))
	assert.Equal(t, "title\n\n", noteEmbedText("title", ""))
}
