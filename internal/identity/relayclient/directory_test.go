package relayclient_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/peiman/vaultmind/internal/identity/relayclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The signed registry is fetched from the (remote) hub so member machines can
// verify it locally. The bytes are UNTRUSTED until the caller verifies the root
// signature — so they must come back byte-exact: a truncated body would fail
// that check for the wrong reason, or worse, be mistaken for a different file.

func TestFetchDirectory_ReturnsTheExactBytes(t *testing.T) {
	body := []byte(`{"registry":"abc","root_sig":"def","root_key_epoch":0}`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, relayclient.WellKnownDirectoryPath, r.URL.Path)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	got, err := relayclient.FetchDirectory(context.Background(), nil, srv.URL)
	require.NoError(t, err)
	assert.Equal(t, body, got)
}

func TestFetchDirectory_Non200IsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	_, err := relayclient.FetchDirectory(context.Background(), nil, srv.URL)
	require.Error(t, err)
}

// Over the cap is an error, never a silent truncation.
func TestFetchDirectory_OversizedBodyFailsLoud(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte("x"), relayclient.MaxDirectoryBytes+1))
	}))
	defer srv.Close()
	_, err := relayclient.FetchDirectory(context.Background(), nil, srv.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds")
}

func TestFetchDirectory_RejectsNonHTTPSchemes(t *testing.T) {
	_, err := relayclient.FetchDirectory(context.Background(), nil, "file:///etc/passwd")
	require.Error(t, err)
}
