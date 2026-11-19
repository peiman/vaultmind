package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// IdentitySignHealthMetadata defines metadata for `identity sign-health`.
var IdentitySignHealthMetadata = config.CommandMetadata{
	Use:   "sign-health",
	Short: "Report the latest sign-envelope outcome for every agent",
	Long: `Read the latest outcome recorded by identity sign-envelope for each
sender on this machine. A failed latest attempt is a hard failure, so an agent
that cannot sign is noticed before its messages silently disappear.

--max-age marks older records stale. Stale records remain visible but do not
count as failing. With --output-format json, the result is an agents array
whose records contain agent, ok, ts, error when present, and version when
present.`,
	ConfigPrefix: "app.identitysignhealth",
	FlagOverrides: map[string]string{
		"app.identitysignhealth.max_age": "max-age",
	},
}

// IdentitySignHealthOptions returns config options for `identity sign-health`.
func IdentitySignHealthOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.identitysignhealth.max_age", DefaultValue: "0", Description: "Ignore records older than this duration (default: off)", Type: "string"},
	}
}

func init() {
	config.RegisterOptionsProvider(IdentitySignHealthOptions)
}
