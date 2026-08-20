package identitycli

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readLine had no test. It was rewritten to satisfy gosec's G602, and a rewrite
// with no test is a change nobody can verify — so it gets one now rather than
// later.
//
// The property that matters is the reason it reads a byte at a time instead of
// wrapping a bufio.Reader: it reads interactive prompts off a SHARED stdin, so it
// must not consume a single byte past the newline. A buffered reader would swallow
// whatever the next prompt was going to read.
func TestReadLine(t *testing.T) {
	t.Run("stops at the newline and does not include it", func(t *testing.T) {
		got, err := readLine(strings.NewReader("hello\n"))
		require.NoError(t, err)
		assert.Equal(t, "hello", got)
	})

	t.Run("leaves everything after the newline for the next reader", func(t *testing.T) {
		in := strings.NewReader("first\nsecond\nthird\n")

		first, err := readLine(in)
		require.NoError(t, err)
		assert.Equal(t, "first", first)

		// THE load-bearing assertion. If this ever reads ahead, an enrollment
		// prompt silently eats the answer to the prompt after it.
		rest, err := io.ReadAll(in)
		require.NoError(t, err)
		assert.Equal(t, "second\nthird\n", string(rest),
			"readLine consumed past the newline; a shared stdin cannot survive that")
	})

	t.Run("returns what it has when the input ends without a newline", func(t *testing.T) {
		got, err := readLine(strings.NewReader("no trailing newline"))
		assert.ErrorIs(t, err, io.EOF)
		assert.Equal(t, "no trailing newline", got,
			"the caller still needs the text it did read")
	})

	t.Run("empty line", func(t *testing.T) {
		got, err := readLine(strings.NewReader("\nnext"))
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("empty input", func(t *testing.T) {
		got, err := readLine(strings.NewReader(""))
		assert.ErrorIs(t, err, io.EOF)
		assert.Empty(t, got)
	})
}
