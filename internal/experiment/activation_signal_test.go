package experiment

import "testing"

func delivered() *bool  { b := true; return &b }
func withheld() *bool   { b := false; return &b }
func unrecorded() *bool { return nil }

// Activation is ACT-R base-level: a note becomes more retrievable the more it
// has been retrieved. That model only holds if "retrieved" means the agent
// actually encountered the note's CONTENT.
//
// It did not. `note_access` is logged for a query's top hit whenever the hit
// clears the relevance floor — a check on relevance, not on delivery — and under
// pointers-only no body is ever handed over. So a note could rank first, record
// a read it never got, gain activation weight, and rank first more reliably.
//
// Measured on a live log: `principle-how-to-write-arcs` carries 222 such
// accesses against 181 rank-1 injections. The 98% repeat rate in the mid-task
// hook is this loop closing on itself — which is why deduplicating the hook
// would have hidden the cause rather than fixed it.
func TestIsActivationSignal(t *testing.T) {
	cases := []struct {
		name          string
		source        AccessSource
		bodyDelivered *bool
		want          bool
		why           string
	}{
		{
			name:   "note_get that rendered a body is the strongest signal",
			source: AccessSourceRead, bodyDelivered: delivered(), want: true,
			why: "the agent typed the id and got the note back",
		},
		{
			// This case used to return true unconditionally. `note get
			// --frontmatter-only` prints type, title and fields — no body — and
			// a miss prints nothing at all. Both recorded an activation boost
			// for a note whose text never arrived, which is the same phantom the
			// ask path was fixed for. #127 made the recorder honest about this;
			// the reader ignored it, so the fix stopped at the writer. That is
			// the third time this session that a writer was corrected and its
			// reader was not.
			name:   "note_get with --frontmatter-only did NOT deliver",
			source: AccessSourceRead, bodyDelivered: withheld(), want: false,
			why: "naming an id is intent; without text it is intent without content",
		},
		{
			// The asymmetry with ask below is historical fact, not preference:
			// before body_delivered existed, `note get` ALWAYS rendered a body
			// (--frontmatter-only came later and is recorded), while no hook path
			// ever delivered one at all — that is issue #122. So an unrecorded
			// note_get almost certainly delivered, and an unrecorded ask almost
			// certainly did not.
			name:   "note_get predating the field counts",
			source: AccessSourceRead, bodyDelivered: unrecorded(), want: true,
			why: "retiring every historical note_get would delete the strongest signal the tool has",
		},
		{
			name:   "legacy recall without a delivered body does not count",
			source: AccessSourceRecall, bodyDelivered: withheld(), want: false,
			why: "it rendered titles; `memory recall` became `memory neighbors`, same formatter",
		},
		{
			name:   "a recall that did deliver a body counts",
			source: AccessSourceRecall, bodyDelivered: delivered(), want: true,
			why: "delivery decides, so a future recall path is not pre-trusted",
		},
		{
			name:   "an ask that delivered its body counts",
			source: AccessSourceAsk, bodyDelivered: delivered(), want: true,
			why: "the body reached the agent — this is a real retrieval",
		},
		{
			name:   "an ask that withheld its body does NOT count",
			source: AccessSourceAsk, bodyDelivered: withheld(), want: false,
			why: "THE PHANTOM: ranking first is not being read. Counting it makes ranking self-reinforcing",
		},
		{
			name:   "an ask predating the field does NOT count",
			source: AccessSourceAsk, bodyDelivered: unrecorded(), want: false,
			why: "in that era no hook delivered a body at all (#122), so absence is evidence of absence here",
		},
		{
			name:   "neighbour expansion never delivers a body",
			source: AccessSourceNeighbors, bodyDelivered: withheld(), want: false,
			why: "the pack SELECTED the note; the agent saw a title at most",
		},
		{
			name:   "an unknown future source is not trusted",
			source: AccessSource("some-new-path"), bodyDelivered: delivered(), want: false,
			why: "default-deny, so a new logging site cannot silently reopen the loop",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsActivationSignal(tc.source, tc.bodyDelivered); got != tc.want {
				t.Errorf("IsActivationSignal(%q, %v) = %v, want %v — %s",
					tc.source, tc.bodyDelivered, got, tc.want, tc.why)
			}
		})
	}
}

// The three states must be genuinely distinguishable, not two states with a
// default.
//
// The previous version of this test was named MissingDeliveryIsNotDelivery and
// asserted IsActivationSignal(AccessSourceAsk, false) — because the signature
// was a plain bool and could not express "missing" at all. It tested the false
// case twice and called one of them absent. A test cannot check a distinction
// its own types collapse.
func TestIsActivationSignal_AbsentIsNotTheSameAsFalse(t *testing.T) {
	if IsActivationSignal(AccessSourceRead, unrecorded()) == IsActivationSignal(AccessSourceRead, withheld()) {
		t.Fatal("note_get treats an unrecorded event and an explicit non-delivery alike; " +
			"one is a pre-field read that did deliver, the other is --frontmatter-only")
	}
	if IsActivationSignal(AccessSourceAsk, unrecorded()) != IsActivationSignal(AccessSourceAsk, withheld()) {
		t.Fatal("for ask the two DO coincide, and for a reason: no pre-field ask ever " +
			"delivered a body. If that ever stops being true this must be revisited")
	}
}

// activationSignalFrom is where the tri-state is actually decided, because it
// is the only place that sees whether the key was present in the event at all.
// A `data["body_delivered"].(bool)` two-value assertion silently reports a
// MISSING key as false — the exact NULL-read-as-zero shape this session has
// been closing, and it was sitting in the filter written to close it.
func TestActivationSignalFrom_TellsAbsentFromFalse(t *testing.T) {
	cases := []struct {
		name string
		data map[string]any
		want bool
	}{
		{"note_get with no delivery field (pre-migration)", map[string]any{"source": "note_get"}, true},
		{"note_get recorded as not delivering", map[string]any{"source": "note_get", "body_delivered": false}, false},
		{"note_get recorded as delivering", map[string]any{"source": "note_get", "body_delivered": true}, true},
		{"ask with no delivery field", map[string]any{"source": "ask"}, false},
		{"ask recorded as delivering", map[string]any{"source": "ask", "body_delivered": true}, true},
		{"no source at all", map[string]any{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := activationSignalFrom(tc.data); got != tc.want {
				t.Errorf("activationSignalFrom(%v) = %v, want %v", tc.data, got, tc.want)
			}
		})
	}
}
