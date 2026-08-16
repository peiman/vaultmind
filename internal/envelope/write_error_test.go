package envelope_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The whole point of the helper: writing an error envelope and reporting
// success must not be a state a caller can express.
func TestWriteError_SignalsFailureToTheCaller(t *testing.T) {
	var buf bytes.Buffer
	err := envelope.WriteError(&buf, envelope.Error("note get", "not_found", "no note matches \"x\"", ""))

	require.ErrorIs(t, err, envelope.ErrAlreadyWritten,
		"the caller must be told to exit non-zero")

	var env envelope.Envelope
	require.NoError(t, json.Unmarshal(buf.Bytes(), &env))
	assert.Equal(t, "error", env.Status, "and the envelope the reader gets says the same thing")
	require.NotEmpty(t, env.Errors)
	assert.Equal(t, "not_found", env.Errors[0].Code)
}

type failingWriter struct{ err error }

func (f failingWriter) Write([]byte) (int, error) { return 0, f.err }

// If the envelope could not be written, the sentinel would be a lie: it means
// "the failure is already described in the output", and here nothing reached
// the reader. Return the write error so main describes the failure itself.
func TestWriteError_ReturnsTheWriteFailureItself(t *testing.T) {
	boom := errors.New("disk full")
	err := envelope.WriteError(failingWriter{err: boom}, envelope.Error("cmd", "code", "msg", ""))

	require.ErrorIs(t, err, boom)
	assert.NotErrorIs(t, err, envelope.ErrAlreadyWritten,
		"nothing was written, so there is no envelope for the exit code to agree with")
}
