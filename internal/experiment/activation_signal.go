package experiment

// Access sources recorded on a note_access event. They are not
// interchangeable: they describe very different things happening to a note, and
// only some of them mean the agent encountered its content.
const (
	// AccessSourceAsk is written for a query's top hit. It means "this note
	// ranked first and cleared the relevance floor" — NOT "the agent read it".
	// Under pointers-only no body is handed over at all.
	AccessSourceAsk = "ask"

	// AccessSourceNeighbors is written when the context pack selects a
	// neighbour. The agent sees a title at most.
	AccessSourceNeighbors = "neighbors"

	// AccessSourceRecall was written by `memory recall`, which 835472a renamed
	// to `memory neighbors` — same code, same formatter, printing
	// `id [type] "title" depth N` and no body at all. It is the literal ancestor
	// of AccessSourceNeighbors above.
	//
	// Nothing writes it today. It is kept, and gated on delivery like ask,
	// because deleting it would silently reclassify the events already in
	// people's logs, and pre-trusting it would let a future recall path reopen
	// the phantom loop the moment someone rewires it.
	AccessSourceRecall = "recall"
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
// activationSignalFrom applies IsActivationSignal to a decoded event_data map.
//
// A missing body_delivered reads as false, which is the point: every event
// written before the field existed lacks it, and treating absence as delivery
// would leave the historical phantoms in the ranking permanently.
func activationSignalFrom(data map[string]any) bool {
	source, _ := data["source"].(string)
	delivered, _ := data["body_delivered"].(bool)
	return IsActivationSignal(source, delivered)
}

func IsActivationSignal(source string, bodyDelivered bool) bool {
	switch source {
	case AccessSourceRead:
		// note get fetches and renders the note by name — the most deliberate
		// signal there is, and it now records its own delivery honestly
		// (--frontmatter-only and misses no longer claim one).
		return true
	case AccessSourceAsk, AccessSourceRecall:
		// Only when a body actually reached the agent. Recall is here rather
		// than in the always-true case because its command rendered titles;
		// see the constant.
		return bodyDelivered
	default:
		return false
	}
}
