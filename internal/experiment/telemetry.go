package experiment

// Telemetry tier constants.
//
// Write-path contract (as of this writing):
//   - TelemetryOff   → the experiment DB is never opened; no session is
//     started; no events are written. Gated in cmd/root.go.
//   - TelemetryAnonymous and TelemetryFull → identical write paths. Both
//     write everything locally. The distinction is
//     future-proofing for an uploader that does not yet
//     exist.
//
// Upload-path contract (for future implementation):
//
//	When a data-return mechanism is built (not yet), the uploader MUST
//	filter outbound payloads by tier. Under Anonymous, the following fields
//	MUST be stripped before transmission:
//	  - event_data.variants.*.results[].note_id
//	  - event_data.variants.*.results[].path
//	  - event_data.query.text (if/when query text is added to events)
//	  - vault_path
//	Aggregate structure (ranks, scores, counts, timestamps, variant names,
//	note_type) MAY be sent under Anonymous. Under Full, the uploader MAY
//	send everything.
//
// If you're adding the uploader: verify this contract still matches what the
// README's "The local usage log" section promises, and what `export --preview`
// prints. Those are the two places the promise is now made — the consent prompt
// this used to point at was deleted for asking about a transmission that does
// not happen.
const (
	TelemetryOff       = "off"
	TelemetryAnonymous = "anonymous"
	TelemetryFull      = "full"
)
