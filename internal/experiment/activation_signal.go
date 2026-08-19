package experiment

// AccessSource names what kind of event produced a note_access row.
//
// A defined type rather than a bare string because IsActivationSignal
// default-denies anything it does not recognise: a typo does not fail, it
// silently makes the event invisible to activation. That already happened — a
// fixture seeding "search" asserted against an empty score map while appearing
// to exercise the path, and nothing reported it. The compiler catches that now.
//
// The values are on disk in every existing log, so they are pinned by a test.
type AccessSource string

const (
	// AccessSourceRead is written by `note get`: the agent named a note and its
	// text was rendered. The single strongest signal the tool emits, and the
	// only source trusted without also proving delivery.
	AccessSourceRead AccessSource = "note_get"

	// AccessSourceAsk is written for a query's top hit. It means "this note
	// ranked first and cleared the relevance floor" — NOT "the agent read it".
	// Under pointers-only no body is handed over at all.
	AccessSourceAsk AccessSource = "ask"

	// AccessSourceNeighbors is written when the context pack selects a
	// neighbour. The agent sees a title at most.
	AccessSourceNeighbors AccessSource = "neighbors"

	// AccessSourceRecall was written by `memory recall`, which 835472a renamed
	// to `memory neighbors` — same code, same formatter, printing
	// `id [type] "title" depth N` and no body at all. It is the literal ancestor
	// of AccessSourceNeighbors above.
	//
	// Nothing writes it today. It is kept, and gated on delivery like ask,
	// because deleting it would silently reclassify the events already in
	// people's logs, and pre-trusting it would let a future recall path reopen
	// the phantom loop the moment someone rewires it.
	AccessSourceRecall AccessSource = "recall"
)

// IsActivationSignal reports whether a note_access event is evidence that the
// agent actually encountered a note's content, and may therefore raise its
// activation.
//
// WHY THIS EXISTS — the feedback loop it closes:
//
//	a note ranks first
//	  → an access is logged (gated on the relevance floor, not on delivery)
//	  → the activation scorer counts it as a retrieval
//	  → the note is boosted
//	  → it ranks first more reliably
//
// Under pointers-only, no body was ever delivered, so every turn of that loop
// was driven by a read that did not happen. On a live log
// `principle-how-to-write-arcs` shows 222 such accesses against 181 rank-1
// injections; the mid-task hook's 98% repeat rate is this loop, not a
// deduplication bug — which is why deduplicating would have hidden it.
//
// The activation scorer reads the whole history with no time window, so the
// pre-fix events cannot simply age out. Requiring positive proof of delivery
// rather than the absence of a denial is what makes the historical phantoms
// stop counting: they carry no body_delivered at all, so they read as
// not-delivered and drop out on their own.
//
// Default-deny, so a future logging site cannot quietly reopen the loop by
// inventing a source nobody added here.
//
// bodyDelivered is TRI-state and nil means "this event predates the field",
// which is not the same as "no body was delivered". Collapsing the two is what
// the plain-bool version did, and it is the NULL-read-as-zero shape that
// produced the bug this function exists to fix.
func IsActivationSignal(source AccessSource, bodyDelivered *bool) bool {
	switch source {
	case AccessSourceRead:
		// note get names an id and renders the note — the most deliberate signal
		// there is. But --frontmatter-only prints fields and no body, and a miss
		// prints nothing; both are intent without content and must not boost.
		// This used to return true unconditionally while its own comment claimed
		// the recorder had been made honest: the writer was fixed in #127 and the
		// reader kept ignoring it.
		//
		// nil counts, because before the field existed note get ALWAYS rendered a
		// body — --frontmatter-only tracking came with it. Denying those would
		// silently retire the strongest signal in the log.
		return bodyDelivered == nil || *bodyDelivered
	case AccessSourceAsk, AccessSourceRecall:
		// Only when a body actually reached the agent. Recall is here rather
		// than in the always-true case because its command rendered titles;
		// see the constant.
		//
		// nil does NOT count, and the asymmetry with note get above is historical
		// fact rather than caution: in the pre-field era no hook path delivered a
		// body at all (#122), so an unrecorded ask is affirmatively a phantom.
		// The scorer reads the whole history with no time window, so these cannot
		// age out on their own.
		return bodyDelivered != nil && *bodyDelivered
	default:
		return false
	}
}

// activationSignalFrom applies IsActivationSignal to a decoded event_data map.
//
// The two-value type assertion is load-bearing: `data["body_delivered"].(bool)`
// discarding its second return reports a MISSING key as false, which is exactly
// the absence-rendered-as-zero defect this filter was written to close. It was
// sitting inside the fix.
func activationSignalFrom(data map[string]any) bool {
	raw, _ := data["source"].(string)
	var delivered *bool
	if b, present := data["body_delivered"].(bool); present {
		delivered = &b
	}
	return IsActivationSignal(AccessSource(raw), delivered)
}
