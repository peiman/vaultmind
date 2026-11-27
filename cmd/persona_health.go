package cmd

import (
	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/personahealth"
	"github.com/spf13/cobra"
)

var personaHealthCmd = MustNewCommand(commands.PersonaHealthMetadata, runPersonaHealth)

func init() {
	personaCmd.AddCommand(personaHealthCmd)
}

func runPersonaHealth(cmd *cobra.Command, _ []string) error {
	keepers, err := personahealth.KeepersFromFlags(
		getConfigValueWithFlags[string](cmd, "name", config.KeyAppPersonaHealthName),
		getConfigValueWithFlags[string](cmd, "transcripts", config.KeyAppPersonaHealthTranscripts),
		getConfigValueWithFlags[string](cmd, "vault", config.KeyAppPersonaHealthVault),
		getConfigValueWithFlags[string](cmd, "anchor", config.KeyAppPersonaHealthAnchor),
		getConfigValueWithFlags[string](cmd, "keepers", config.KeyAppPersonaHealthKeepers),
	)
	if err != nil {
		return err
	}
	minCoverage := getConfigValueWithFlags[float64](cmd, "min-coverage", config.KeyAppPersonaHealthMinCoverage)
	if err := personahealth.ValidateMinCoverage(minCoverage); err != nil {
		return err
	}
	minArcCoverage := getConfigValueWithFlags[float64](cmd, "min-arc-coverage", config.KeyAppPersonaHealthMinArcCoverage)
	if err := personahealth.ValidateMinArcCoverage(minArcCoverage); err != nil {
		return err
	}
	minMomentCoverage := getConfigValueWithFlags[float64](cmd, "min-moment-coverage", config.KeyAppPersonaHealthMinMomentCoverage)
	if err := personahealth.ValidateMinMomentCoverage(minMomentCoverage); err != nil {
		return err
	}
	minKeyCoverage := getConfigValueWithFlags[float64](cmd, "min-key-coverage", config.KeyAppPersonaHealthMinKeyCoverage)
	if err := personahealth.ValidateMinKeyCoverage(minKeyCoverage); err != nil {
		return err
	}
	result := personahealth.Check(personahealth.Config{
		Keepers: keepers, MinCoverage: minCoverage, MinArcCoverage: minArcCoverage,
		MinMomentCoverage: minMomentCoverage, Full: getConfigValueWithFlags[string](cmd, "full", config.KeyAppPersonaHealthFull),
		MinKeyCoverage: minKeyCoverage,
	})
	return renderPersonaHealth(cmd, result, getConfigValueWithFlags[bool](cmd, "json", config.KeyAppPersonaHealthJson))
}
