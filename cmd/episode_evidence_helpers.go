package cmd

import (
	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// Capture hooks already pass transcript_path to episode capture. Evidence is
// best effort, respects telemetry=off (no session), and never delays recall.
func recordEpisodeUseEvidence(cmd *cobra.Command, transcriptPath string) {
	session := experiment.FromContext(cmd.Context())
	if session == nil {
		return
	}
	if err := session.DB.RecordUseEvidence(transcriptPath); err != nil {
		log.Debug().Err(err).Msg("episode capture use evidence unavailable")
	}
}
