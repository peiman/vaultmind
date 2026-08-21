package hooks

import "fmt"

// RecordableEvents is the closed set of event names `vaultmind hooks record`
// accepts.
//
// Deliberately an allowlist rather than a free-text field. The usage log is the
// evidence base for every retrieval measurement, and #121 has just finished
// closing one route by which non-agent activity got written into it. A general
// "record any event" CLI would reopen the same hole from the other side: any
// script, test, or typo could inject rows that later read as agent behaviour.
//
// A hook that wants a new event name adds it here, in a diff someone reviews.
var RecordableEvents = map[string]string{
	// Fired by the PreCompact write-path trigger. Without it, a window that ends
	// with zero notes banked cannot distinguish "the prompt fired and was
	// ignored" from "the prompt never fired" — which is the did-it-run gate the
	// evidence-loop skill requires and the one place it was missing.
	"write_prompt": "the PreCompact write-path prompt was shown to the agent",
}

// ValidateRecordable reports whether name is an accepted hook event.
func ValidateRecordable(name string) error {
	if _, ok := RecordableEvents[name]; ok {
		return nil
	}
	return fmt.Errorf("unknown hook event %q: hooks record accepts only %s", name, RecordableNames())
}

// RecordableNames lists the accepted names, sorted, for error messages and help.
func RecordableNames() string {
	names := make([]string, 0, len(RecordableEvents))
	for n := range RecordableEvents {
		names = append(names, n)
	}
	sortStrings(names)
	out := ""
	for i, n := range names {
		if i > 0 {
			out += ", "
		}
		out += n
	}
	return out
}
