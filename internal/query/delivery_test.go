package query

import (
	"testing"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/stretchr/testify/assert"
)

// BodyDecision answers "will the TEXT FORMATTER render bodies". Telemetry needs
// a different question: "did note text actually reach the caller". They diverge
// on the --json path, because --pointers-only gates only the text renderer while
// the JSON envelope serializes bodies regardless.
//
// Measured live: `ask --pointers-only --json` returned a 5,770-character target
// body while the telemetry recorded body_delivered=false. IsActivationSignal
// then discarded a genuine read — reopening, from the other side, the exact loop
// the phantom-read fix closed.
func TestDeliveredTo_JSONDeliversDespitePointersOnly(t *testing.T) {
	r := &AskResult{
		TopHitConfidence: ConfidenceStrong,
		Context: &memory.ContextPackResult{
			TargetID: "t",
			Target:   &memory.ContextPackTarget{ID: "t", Body: "a real body that the JSON envelope carries"},
		},
	}

	delivered, reason := r.DeliveredTo(true /* callerAsked */, true /* jsonOutput */)

	assert.True(t, delivered,
		"the JSON envelope carried the body; recording otherwise discards a genuine read")
	assert.Empty(t, reason)
}

// In text mode the caller's --pointers-only really does withhold, so the answer
// must still be no.
func TestDeliveredTo_TextModeRespectsPointersOnly(t *testing.T) {
	r := &AskResult{
		TopHitConfidence: ConfidenceStrong,
		Context: &memory.ContextPackResult{
			TargetID: "t",
			Target:   &memory.ContextPackTarget{ID: "t", Body: "withheld in text mode"},
		},
	}

	delivered, reason := r.DeliveredTo(true, false)

	assert.False(t, delivered)
	assert.Equal(t, experiment.SuppressedByCaller, reason)
}

// A JSON call that produced no pack delivered nothing, whatever the formatter
// would have done. This is the false-POSITIVE half: on a context-pack error the
// result carries a nil Context, and crediting delivery there reopens the loop.
func TestDeliveredTo_NoPackIsNotADelivery(t *testing.T) {
	r := &AskResult{TopHitConfidence: ConfidenceStrong, Context: nil}

	delivered, _ := r.DeliveredTo(false, true)

	assert.False(t, delivered,
		"the context pack failed, so no text reached the caller — crediting it reopens the phantom loop")
}

// And a pack whose items carry no text is not a delivery either.
func TestDeliveredTo_EmptyPackIsNotADelivery(t *testing.T) {
	r := &AskResult{
		TopHitConfidence: ConfidenceStrong,
		Context:          &memory.ContextPackResult{TargetID: "t", Target: &memory.ContextPackTarget{ID: "t"}},
	}

	delivered, _ := r.DeliveredTo(false, true)

	assert.False(t, delivered)
}
