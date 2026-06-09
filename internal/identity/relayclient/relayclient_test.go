package relayclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/identity/relayclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lowEntropyRootB64 is a deterministic, LOW-ENTROPY base64-std pubkey-shaped
// value used in fixtures so the gitleaks entropy scanner does not trip. It is
// NOT a real key — these tests never verify against it.
const lowEntropyRootB64 = "oaGhoaGhoaGhoaGhoaGhoaGhoaGhoaGhoaGhoaGhoaE=" // base64-std of 32 × 0xA1

const wellKnownBody = `{"root_pubkey":"` + lowEntropyRootB64 +
	`","network_id":"vmnet1:deadbeefdeadbeefdeadbeefdeadbeef","root_key_epoch":3}`

// newRelay spins up an httptest server that serves the well-known root at the
// canonical path and 404s everything else. handler lets a test override the
// well-known response.
func newRelay(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc(relayclient.WellKnownRootPath, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// TestFetchRootHappy fetches a well-formed well-known root and decodes every
// field, including the optional root_key_epoch.
func TestFetchRootHappy(t *testing.T) {
	srv := newRelay(t, http.StatusOK, wellKnownBody)

	got, err := relayclient.FetchRoot(context.Background(), srv.Client(), srv.URL)
	require.NoError(t, err)
	assert.Equal(t, lowEntropyRootB64, got.RootPubKey)
	assert.Equal(t, "vmnet1:deadbeefdeadbeefdeadbeefdeadbeef", got.NetworkID)
	assert.Equal(t, int64(3), got.RootKeyEpoch)
}

// TestFetchRootForwardCompatible proves UNKNOWN fields in the relay response are
// tolerated (the relay may add fields without breaking older members).
func TestFetchRootForwardCompatible(t *testing.T) {
	body := `{"root_pubkey":"` + lowEntropyRootB64 +
		`","network_id":"vmnet1:deadbeefdeadbeefdeadbeefdeadbeef","future_field":"ignored"}`
	srv := newRelay(t, http.StatusOK, body)

	got, err := relayclient.FetchRoot(context.Background(), srv.Client(), srv.URL)
	require.NoError(t, err)
	assert.Equal(t, lowEntropyRootB64, got.RootPubKey)
}

// TestFetchRootNon200 maps a non-200 status to an error that names the status.
func TestFetchRootNon200(t *testing.T) {
	srv := newRelay(t, http.StatusInternalServerError, "boom")

	_, err := relayclient.FetchRoot(context.Background(), srv.Client(), srv.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

// TestFetchRootMalformedJSON rejects a body that is not a JSON object.
func TestFetchRootMalformedJSON(t *testing.T) {
	srv := newRelay(t, http.StatusOK, "{not json")

	_, err := relayclient.FetchRoot(context.Background(), srv.Client(), srv.URL)
	require.Error(t, err)
}

// TestFetchRootWrongPath404 hits a relay whose well-known path is unmapped; the
// default mux returns 404, which must surface as a non-200 error.
func TestFetchRootWrongPath404(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)

	_, err := relayclient.FetchRoot(context.Background(), srv.Client(), srv.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

// TestFetchRootBadScheme rejects a relay base URL whose scheme is not http(s)
// BEFORE dialing.
func TestFetchRootBadScheme(t *testing.T) {
	for _, base := range []string{"ftp://relay.example", "file:///etc/passwd", "://no-scheme", "not a url at all"} {
		_, err := relayclient.FetchRoot(context.Background(), http.DefaultClient, base)
		require.Error(t, err, "base %q must be rejected", base)
	}
}

// TestFetchRootContextTimeout proves a cancelled/expired context aborts the
// fetch (the per-request deadline is honored).
func TestFetchRootContextTimeout(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(relayclient.WellKnownRootPath, func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte(wellKnownBody))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := relayclient.FetchRoot(ctx, srv.Client(), srv.URL)
	require.Error(t, err)
}

// TestFetchRootNilClientUsesDefault proves a nil *http.Client is tolerated (the
// fetch falls back to a timeout-bounded default client).
func TestFetchRootNilClientUsesDefault(t *testing.T) {
	srv := newRelay(t, http.StatusOK, wellKnownBody)

	got, err := relayclient.FetchRoot(context.Background(), nil, srv.URL)
	require.NoError(t, err)
	assert.Equal(t, lowEntropyRootB64, got.RootPubKey)
}

// TestFetchRootDecodesTestdataFixture serves the committed testdata fixture and
// asserts it decodes to the same fields the inline body uses, pinning the
// on-disk well-known shape to the struct.
func TestFetchRootDecodesTestdataFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "well_known_root.json"))
	require.NoError(t, err)
	srv := newRelay(t, http.StatusOK, string(raw))

	got, err := relayclient.FetchRoot(context.Background(), srv.Client(), srv.URL)
	require.NoError(t, err)
	assert.Equal(t, lowEntropyRootB64, got.RootPubKey)
	assert.Equal(t, "vmnet1:deadbeefdeadbeefdeadbeefdeadbeef", got.NetworkID)
	assert.Equal(t, int64(3), got.RootKeyEpoch)
}
