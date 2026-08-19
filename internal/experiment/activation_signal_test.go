package experiment

import "testing"

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
		bodyDelivered bool
		want          bool
		why           string
	}{
		{
			name:   "a deliberate note_get is the strongest signal",
			source: AccessSourceRead, bodyDelivered: false, want: true,
			why: "the agent typed the id — content was fetched regardless of any earlier render",
		},
		// Changed after review. The constant's comment claimed "recall renders
		// the note", but source="recall" was written by `memory recall` — which
		// 835472a renamed to `memory neighbors`, same code, same formatter,
		// printing `id [type] "title" depth N` and nothing else. It is the
		// literal ancestor of the command classified two cases below as "the
		// agent sees a title at most", and it was being trusted unconditionally
		// while its own descendant is default-denied. 63 such events pass as
		// genuine reads in a live log.
		//
		// No production code writes it any more, so it is gated on delivery
		// rather than deleted: if a recall path is ever wired again, it earns
		// activation the same way `ask` does instead of inheriting a whitelist.
		{
			name:   "legacy recall without a delivered body does not count",
			source: AccessSourceRecall, bodyDelivered: false, want: false,
			why: "it rendered titles; the rename is what the old comment missed",
		},
		{
			name:   "a recall that did deliver a body counts",
			source: AccessSourceRecall, bodyDelivered: true, want: true,
			why: "delivery decides, so a future recall path is not pre-trusted",
		},
		{
			name:   "an ask that delivered its body counts",
			source: AccessSourceAsk, bodyDelivered: true, want: true,
			why: "the body reached the agent — this is a real retrieval",
		},
		{
			name:   "an ask that withheld its body does NOT count",
			source: AccessSourceAsk, bodyDelivered: false, want: false,
			why: "THE PHANTOM: ranking first is not being read. Counting it makes ranking self-reinforcing",
		},
		{
			name:   "neighbour expansion never delivers a body",
			source: AccessSourceNeighbors, bodyDelivered: false, want: false,
			why: "the pack SELECTED the note; the agent saw a title at most",
		},
		{
			name:   "an unknown future source is not trusted",
			source: AccessSource("some-new-path"), bodyDelivered: true, want: false,
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

// Historical events predate the body_delivered field, so it reads as absent
// rather than false. Absent must mean "not proven delivered" — the whole reason
// the loop went unnoticed is that a missing value was read as a benign one.
func TestIsActivationSignal_MissingDeliveryIsNotDelivery(t *testing.T) {
	if IsActivationSignal(AccessSourceAsk, false) {
		t.Fatal("an ask with no recorded delivery must not feed activation; " +
			"every pre-fix event looks exactly like this, and treating absence as " +
			"delivery would keep the historical phantoms in the ranking forever")
	}
}
