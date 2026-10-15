package propose

import (
	"context"
	"testing"

	"github.com/peiman/vaultmind/internal/noisefloor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBandIsClose_AtLeastTheNamedBand(t *testing.T) {
	assert.True(t, bandIsClose(noisefloor.ConfidenceStrong, noisefloor.ConfidenceModerate))
	assert.True(t, bandIsClose(noisefloor.ConfidenceStrong, noisefloor.ConfidenceStrong))
	assert.True(t, bandIsClose(noisefloor.ConfidenceModerate, noisefloor.ConfidenceModerate))
	assert.True(t, bandIsClose(noisefloor.ConfidenceModerate, noisefloor.ConfidenceWeak))
	assert.True(t, bandIsClose(noisefloor.ConfidenceWeak, noisefloor.ConfidenceNoMatch))
	assert.False(t, bandIsClose(noisefloor.ConfidenceModerate, noisefloor.ConfidenceStrong), "a weaker hit stays a new note")
	assert.False(t, bandIsClose(noisefloor.ConfidenceWeak, noisefloor.ConfidenceModerate))
	assert.False(t, bandIsClose("", noisefloor.ConfidenceStrong))
	assert.False(t, bandIsClose(noisefloor.ConfidenceStrong, ""))
	assert.False(t, bandIsClose(noisefloor.ConfidenceStrong, "not-a-real-band"))
	assert.False(t, bandIsClose("not-a-real-band", noisefloor.ConfidenceWeak))
}

// A question-only rank of strong used to be printed as the reason for the
// mode. The mode is the band of the question and the finding. Those two
// texts can disagree, and the line has to say which one decided.
func TestPropose_ExplainsANewNoteWhenTheQuestionIsStrongerThanTheFinding(t *testing.T) {
	v := testVault(t)
	const (
		question  = "How does quartz work?"
		finding   = "The kiln schedule is unrelated."
		targetID  = "concept-memory"
		targetRel = "concepts/memory.md"
	)
	writeTarget(t, v, targetRel, targetID, "concept", "Memory", "accepted", "The memory note.")
	ret := &stubClosest{hit: Hit{
		ID: targetID, Path: targetRel, Type: "concept", Title: "Memory",
		Band: noisefloor.ConfidenceModerate,
	}, ok: true}
	ranker := &fixedRanker{rank: QuestionRank{
		Mode: "hybrid", Window: 5,
		Hits: []RankedHit{{ID: targetID, Band: noisefloor.ConfidenceStrong}},
	}}
	req, out, _, _ := reqOf(question, finding)
	req.Ranker = ranker
	require.NoError(t, Run(context.Background(), v, req, ret))

	assert.Contains(t, out.String(), "mode: new\n")
	assert.Contains(t, out.String(), "path: concepts/how-does-quartz-work.md\n")
	assert.Contains(t, out.String(), "best answer "+targetID+" (moderate) on the question and the finding")
	assert.Contains(t, out.String(), "which is not close enough for strong, so this is a new note")
	assert.Contains(t, out.String(), "question alone — best answer "+targetID+" at #1 (strong)")
	assert.Equal(t, []string{question, question}, ranker.questions)
	assert.Equal(t, question+"\n"+finding, ret.text)
	assert.NotContains(t, ranker.questions[0], finding)
}

func TestPropose_CloseBandModerateIncludesAStrongHit(t *testing.T) {
	v := testVault(t)
	const (
		targetID  = "concept-memory"
		targetRel = "concepts/memory.md"
	)
	writeTarget(t, v, targetRel, targetID, "concept", "Memory", "accepted", "The memory note.")
	ret := &stubClosest{hit: Hit{
		ID: targetID, Path: targetRel, Type: "concept", Title: "Memory",
		Band: noisefloor.ConfidenceStrong,
	}, ok: true}
	req, out, _, _ := reqOf("How does quartz work?", "Quartz keeps one timeline.")
	req.CloseBand = noisefloor.ConfidenceModerate
	require.NoError(t, Run(context.Background(), v, req, ret))

	assert.Contains(t, out.String(), "mode: patch\n")
	assert.Contains(t, out.String(), proposalPath(targetRel, targetID, "Quartz keeps one timeline.", proposeNow))
	assert.NotContains(t, out.String(), "not close enough")
}

func TestPropose_CloseBandModerateRejectsAWeakHit(t *testing.T) {
	v := testVault(t)
	writeTarget(t, v, "concepts/memory.md", "concept-memory", "concept", "Memory", "accepted", "The memory note.")
	ret := &stubClosest{hit: Hit{
		ID: "concept-memory", Path: "concepts/memory.md", Type: "concept",
		Band: noisefloor.ConfidenceWeak,
	}, ok: true}
	req, out, _, _ := reqOf("How does quartz work?", "A weak finding.")
	req.CloseBand = noisefloor.ConfidenceModerate
	require.NoError(t, Run(context.Background(), v, req, ret))
	assert.Contains(t, out.String(), "mode: new\n")
}

type fixedRanker struct {
	rank      QuestionRank
	questions []string
}

func (f *fixedRanker) RankQuestion(_ context.Context, question string, _ bool) (QuestionRank, error) {
	f.questions = append(f.questions, question)
	return f.rank, nil
}

func (f *fixedRanker) Close() error { return nil }
