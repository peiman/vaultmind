package experiment_test

import (
	"context"
	"testing"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithSession_RoundTrip(t *testing.T) {
	db := openTestExpDB(t)

	sess := &experiment.Session{
		DB:        db,
		ID:        "test-session-id",
		VaultPath: "/tmp/test-vault",
	}

	ctx := experiment.WithSession(context.Background(), sess)
	got := experiment.FromContext(ctx)

	require.NotNil(t, got)
	assert.Equal(t, "test-session-id", got.ID)
	assert.Equal(t, "/tmp/test-vault", got.VaultPath)
	assert.Same(t, db, got.DB)
}

func TestFromContext_NilWhenMissing(t *testing.T) {
	got := experiment.FromContext(context.Background())
	assert.Nil(t, got)
}
