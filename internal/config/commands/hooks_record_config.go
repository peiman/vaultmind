package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// HooksRecordMetadata defines `vaultmind hooks record` — the did-it-run gate for
// hooks that prompt rather than query.
//
// The read-path hooks leave a trail: every recall and reach writes an ask or
// note_access row. The PreCompact write-path prompt left nothing at all, so a
// measurement window that ends with zero notes banked could not distinguish "the
// prompt fired and the agent ignored it" from "the prompt never fired" — a
// discipline problem and a wiring problem look identical, and only one of them
// is fixable by trying harder.
//
// The event name is an ALLOWLIST (internal/hooks.RecordableEvents), not free
// text. The usage log is the evidence base for every retrieval measurement, and
// a general "record any event" CLI would let any script or typo write rows that
// later read as agent behaviour — the hole #121 closed, reopened from the CLI.
var HooksRecordMetadata = config.CommandMetadata{
	Use:   "record <event>",
	Short: "Record that a hook fired, so a zero can be told apart from a no-show",
	Long: `Record that a VaultMind hook fired.

Some hooks PROMPT rather than QUERY. A prompt leaves no trace in the usage log,
so "no notes were banked" and "the prompt was never shown" produce identical
evidence. This writes the missing denominator.

It is a denominator, not a score. Firings are not success — the outcome metric
stays "were notes actually banked". This exists only so that a zero is
interpretable.

EVENTS

  Only names on the allowlist are accepted. Adding one is a reviewed diff, not
  a runtime string, because everything written here is later read as evidence.

    write_prompt   the PreCompact write-path prompt was shown to the agent

WHAT IS WRITTEN

  The event name, the vault path, and nothing else. No content, no query text.

EXAMPLES

  vaultmind hooks record write_prompt --vault ~/vault
  vaultmind hooks record write_prompt --vault ~/vault --json`,
	ConfigPrefix: "app.hooksrecord",
	FlagOverrides: map[string]string{
		"app.hooksrecord.json":  "json",
		"app.hooksrecord.vault": "vault",
	},
}

// HooksRecordOptions returns configuration options for `hooks record`.
func HooksRecordOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{
			Key:          "app.hooksrecord.json",
			DefaultValue: false,
			Description:  "Output in JSON format",
			Type:         "bool",
		},
		{
			Key:          "app.hooksrecord.vault",
			DefaultValue: ".",
			Description:  "Path to vault root",
			Type:         "string",
		},
	}
}

func init() {
	config.RegisterOptionsProvider(HooksRecordOptions)
}
