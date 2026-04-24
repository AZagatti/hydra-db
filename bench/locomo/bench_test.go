package locomo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScoreQuestion_PerfectRecall(t *testing.T) {
	result := QueryResult{
		QA: QAItem{
			Question: "When did X happen?",
			Category: CategorySingleHop,
			Evidence: []string{"D1:3", "D1:4"},
		},
		RetrievedIDs: []string{"D1:1", "D1:2", "D1:3", "D1:4", "D1:5"},
	}

	score := ScoreQuestion(result)

	assert.Equal(t, 1.0, score.Recall)
	assert.InDelta(t, 0.4, score.Precision, 0.001) // 2/5
	assert.Equal(t, 5, score.Retrieved)
	assert.Equal(t, 2, score.Evidence)
}

func TestScoreQuestion_PerfectPrecision(t *testing.T) {
	result := QueryResult{
		QA: QAItem{
			Question: "What did Y say?",
			Category: CategoryMultiHop,
			Evidence: []string{"D1:3", "D1:4", "D2:1"},
		},
		RetrievedIDs: []string{"D1:3"},
	}

	score := ScoreQuestion(result)

	assert.Equal(t, 1.0, score.Precision)
	assert.InDelta(t, 0.333, score.Recall, 0.001) // 1/3
}

func TestScoreQuestion_NoOverlap(t *testing.T) {
	result := QueryResult{
		QA: QAItem{
			Question: "Where is Z?",
			Category: CategoryTemporal,
			Evidence: []string{"D1:3"},
		},
		RetrievedIDs: []string{"D2:1", "D2:2"},
	}

	score := ScoreQuestion(result)

	assert.Equal(t, 0.0, score.Precision)
	assert.Equal(t, 0.0, score.Recall)
	assert.Equal(t, 0.0, score.F1)
}

func TestScoreQuestion_EmptyRetrieved(t *testing.T) {
	result := QueryResult{
		QA: QAItem{
			Question: "What happened?",
			Category: CategorySingleHop,
			Evidence: []string{"D1:1"},
		},
		RetrievedIDs: []string{},
	}

	score := ScoreQuestion(result)

	assert.Equal(t, 0.0, score.Precision)
	assert.Equal(t, 0.0, score.Recall)
	assert.Equal(t, 0.0, score.F1)
}

func TestScoreQuestion_AdversarialEmpty_CorrectRejection(t *testing.T) {
	result := QueryResult{
		QA: QAItem{
			Question: "Did A ever fly to Mars?",
			Category: CategoryAdversarial,
			Evidence: []string{},
		},
		RetrievedIDs: []string{},
	}

	score := ScoreQuestion(result)

	assert.Equal(t, 1.0, score.F1, "correct rejection of adversarial question")
	assert.Equal(t, 1.0, score.Precision)
	assert.Equal(t, 1.0, score.Recall)
}

func TestScoreQuestion_AdversarialEmpty_FalsePositive(t *testing.T) {
	result := QueryResult{
		QA: QAItem{
			Question: "Did A ever fly to Mars?",
			Category: CategoryAdversarial,
			Evidence: []string{},
		},
		RetrievedIDs: []string{"D1:1", "D1:2"},
	}

	score := ScoreQuestion(result)

	assert.Equal(t, 0.0, score.F1, "false positive on adversarial question")
}

func TestScoreQuestion_PerfectMatch(t *testing.T) {
	result := QueryResult{
		QA: QAItem{
			Question: "What did A say?",
			Category: CategorySingleHop,
			Evidence: []string{"D1:3", "D1:4"},
		},
		RetrievedIDs: []string{"D1:3", "D1:4"},
	}

	score := ScoreQuestion(result)

	assert.Equal(t, 1.0, score.Precision)
	assert.Equal(t, 1.0, score.Recall)
	assert.Equal(t, 1.0, score.F1)
}

func TestScoreQuestion_DuplicateRetrieved(t *testing.T) {
	result := QueryResult{
		QA: QAItem{
			Question: "test",
			Category: CategorySingleHop,
			Evidence: []string{"D1:3"},
		},
		RetrievedIDs: []string{"D1:3", "D1:3", "D1:3"},
	}

	score := ScoreQuestion(result)

	// toSet deduplicates, so retrieved set size is 1
	assert.Equal(t, 1.0, score.Precision)
	assert.Equal(t, 1.0, score.Recall)
	assert.Equal(t, 1.0, score.F1)
}

func TestAggregateByCategory(t *testing.T) {
	scores := []QuestionScore{
		{Category: CategorySingleHop, Precision: 0.5, Recall: 1.0, F1: 0.667},
		{Category: CategorySingleHop, Precision: 1.0, Recall: 0.5, F1: 0.667},
		{Category: CategoryTemporal, Precision: 0.25, Recall: 1.0, F1: 0.4},
	}

	cats := AggregateByCategory(scores)

	assert.Len(t, cats, 2)

	assert.Equal(t, "single_hop", cats[0].Category)
	assert.Equal(t, 2, cats[0].Count)
	assert.InDelta(t, 0.75, cats[0].Precision, 0.001)
	assert.InDelta(t, 0.75, cats[0].Recall, 0.001)

	assert.Equal(t, "temporal", cats[1].Category)
	assert.Equal(t, 1, cats[1].Count)
}

func TestAggregateOverall(t *testing.T) {
	scores := []QuestionScore{
		{Precision: 0.5, Recall: 1.0, F1: 0.667},
		{Precision: 1.0, Recall: 0.5, F1: 0.667},
	}

	overall := AggregateOverall(scores)

	assert.Equal(t, "OVERALL", overall.Category)
	assert.Equal(t, 2, overall.Count)
	assert.InDelta(t, 0.75, overall.Precision, 0.001)
	assert.InDelta(t, 0.75, overall.Recall, 0.001)
	assert.InDelta(t, 0.667, overall.F1, 0.001)
}

func TestAggregateOverall_Empty(t *testing.T) {
	overall := AggregateOverall(nil)

	assert.Equal(t, "OVERALL", overall.Category)
	assert.Equal(t, 0, overall.Count)
}

func TestCategoryString(t *testing.T) {
	tests := []struct {
		cat  Category
		want string
	}{
		{CategorySingleHop, "single_hop"},
		{CategoryTemporal, "temporal"},
		{CategoryMultiHop, "multi_hop"},
		{CategoryOpenDomain, "open_domain"},
		{CategoryAdversarial, "adversarial"},
		{Category(99), "unknown(99)"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.cat.String())
	}
}

func TestUnmarshalSample(t *testing.T) {
	raw := `{
		"conversation": {
			"speaker_a": "Alice",
			"speaker_b": "Bob",
			"session_1": [
				{"speaker": "Alice", "dia_id": "D1:1", "text": "Hello"},
				{"speaker": "Bob", "dia_id": "D1:2", "text": "Hi there"}
			],
			"session_1_date_time": "5 January 2023",
			"session_2": [
				{"speaker": "Alice", "dia_id": "D2:1", "text": "How are you?"}
			],
			"session_2_date_time": "12 January 2023"
		},
		"qa": [
			{
				"question": "What did Alice say first?",
				"answer": "Hello",
				"evidence": ["D1:1"],
				"category": 1
			}
		]
	}`

	var sample Sample
	err := sample.UnmarshalJSON([]byte(raw))

	assert.NoError(t, err)
	assert.Equal(t, "Alice", sample.SpeakerA)
	assert.Equal(t, "Bob", sample.SpeakerB)
	assert.Len(t, sample.Sessions, 2)
	assert.Equal(t, 1, sample.Sessions[0].Index)
	assert.Equal(t, 2, sample.Sessions[1].Index)
	assert.Len(t, sample.Sessions[0].Turns, 2)
	assert.Len(t, sample.Sessions[1].Turns, 1)
	assert.Equal(t, "D1:1", sample.Sessions[0].Turns[0].DiaID)
	assert.Len(t, sample.QA, 1)
	assert.Equal(t, CategorySingleHop, sample.QA[0].Category)
	assert.Equal(t, []string{"D1:1"}, sample.QA[0].Evidence)
}
