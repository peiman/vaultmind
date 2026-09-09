// ckeletin:allow-custom-command
//
// Not a command — a pflag Value wrapper for ask.go's --vault flag. Lives in
// cmd/ because it wraps a flag registered there; the whitelist comment opts
// out of the ultra-thin-command validator (ADR-001).
package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// repeatedFlagValue remembers EVERY value a flag was given, not just the last.
//
// pflag overwrites on repeat, which is how `--vault A --vault B` came to
// search B alone and answer "nothing relevant" without ever opening A — a
// confident wrong answer, and the defect that motivated federated search.
// Shipping `--vaults` gave people a way to do the right thing; it did not
// close the trap, so the next person to try the obvious thing would still get
// the silent wrong answer. An alternative is not a fix.
type repeatedFlagValue struct {
	pflag.Value
	given []string
}

// Set records the occurrence and then behaves exactly as the wrapped flag.
func (r *repeatedFlagValue) Set(s string) error {
	r.given = append(r.given, s)
	return r.Value.Set(s)
}

// Reset forgets the recorded occurrences. A one-shot CLI parses once per
// process and never needs this; the test harness, which reuses one command
// across runs to simulate fresh processes, does.
func (r *repeatedFlagValue) Reset() { r.given = nil }

// trackRepeats makes a flag remember its occurrences. Safe to call once per
// command construction; a flag that is absent is left alone.
func trackRepeats(cmd *cobra.Command, name string) {
	f := cmd.Flags().Lookup(name)
	if f == nil {
		return
	}
	if _, already := f.Value.(*repeatedFlagValue); already {
		return
	}
	f.Value = &repeatedFlagValue{Value: f.Value}
}

// repeatedValues returns every value given for a tracked flag.
func repeatedValues(cmd *cobra.Command, name string) []string {
	f := cmd.Flags().Lookup(name)
	if f == nil {
		return nil
	}
	if rv, ok := f.Value.(*repeatedFlagValue); ok {
		return rv.given
	}
	return nil
}

// errRepeatedVaultFlag explains what would have been dropped and points at the
// flag that does what the user meant.
//
// It names BOTH paths deliberately. "--vault given twice" tells you that you
// made a mistake; naming the path that was about to be silently discarded
// tells you what the mistake would have cost, which is the part that was
// invisible before.
func errRepeatedVaultFlag(given []string) error {
	return fmt.Errorf(
		"--vault was given %d times (%s) but it takes ONE vault, so all but the last would be silently ignored.\n"+
			"  To search them together:  --vaults %s",
		len(given), strings.Join(given, ", "), strings.Join(given, ","))
}
