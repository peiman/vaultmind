package propose

import (
	"testing"

	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublishStatus(t *testing.T) {
	cases := []struct {
		name     string
		statuses []string
		want     string
		wantErr  bool
	}{
		{name: "empty list", want: "accepted"},
		{name: "accepted is listed", statuses: []string{"proposed", "accepted", "rejected", "superseded"}, want: "accepted"},
		{name: "active when accepted is absent", statuses: []string{"active", "paused"}, want: "active"},
		{name: "neither word", statuses: []string{"proposed", "rejected"}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := publishStatus(vault.TypeDef{Statuses: tc.statuses})
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
