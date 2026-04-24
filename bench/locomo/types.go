package locomo

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Category represents a LoCoMo question category.
type Category int

const (
	CategorySingleHop Category = 1
	CategoryTemporal  Category = 2
	CategoryMultiHop  Category = 3
	CategoryOpenDomain Category = 4
	CategoryAdversarial Category = 5
)

func (c Category) String() string {
	switch c {
	case CategorySingleHop:
		return "single_hop"
	case CategoryTemporal:
		return "temporal"
	case CategoryMultiHop:
		return "multi_hop"
	case CategoryOpenDomain:
		return "open_domain"
	case CategoryAdversarial:
		return "adversarial"
	default:
		return fmt.Sprintf("unknown(%d)", int(c))
	}
}

// AllCategories returns all LoCoMo question categories in order.
func AllCategories() []Category {
	return []Category{
		CategorySingleHop,
		CategoryTemporal,
		CategoryMultiHop,
		CategoryOpenDomain,
		CategoryAdversarial,
	}
}

// Turn represents a single dialogue turn in a conversation.
type Turn struct {
	Speaker string `json:"speaker"`
	DiaID   string `json:"dia_id"`
	Text    string `json:"text"`
}

// Session represents a conversation session with its turns and timestamp.
type Session struct {
	Index    int
	DateTime string
	Turns    []Turn
}

// QAItem represents a question-answer pair with ground truth evidence.
type QAItem struct {
	Question          string          `json:"question"`
	Answer            json.RawMessage `json:"answer"`
	Evidence          []string        `json:"evidence"`
	Category          Category        `json:"category"`
	AdversarialAnswer string          `json:"adversarial_answer,omitempty"`
}

// AnswerText returns the answer as a string, handling both string and number JSON values.
func (q QAItem) AnswerText() string {
	var s string
	if err := json.Unmarshal(q.Answer, &s); err == nil {
		return s
	}
	// Fall back to raw representation (numbers, etc.)
	return string(q.Answer)
}

// Sample represents a single LoCoMo conversation with its QA pairs.
type Sample struct {
	SampleID     string
	SpeakerA     string
	SpeakerB     string
	Sessions     []Session
	QA           []QAItem
}

// UnmarshalJSON handles the dynamic session_N keys in the LoCoMo JSON format.
func (s *Sample) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("unmarshal sample: %w", err)
	}

	if qa, ok := raw["qa"]; ok {
		if err := json.Unmarshal(qa, &s.QA); err != nil {
			return fmt.Errorf("unmarshal qa: %w", err)
		}
	}

	if conv, ok := raw["conversation"]; ok {
		var convMap map[string]json.RawMessage
		if err := json.Unmarshal(conv, &convMap); err != nil {
			return fmt.Errorf("unmarshal conversation: %w", err)
		}

		if sa, ok := convMap["speaker_a"]; ok {
			_ = json.Unmarshal(sa, &s.SpeakerA)
		}
		if sb, ok := convMap["speaker_b"]; ok {
			_ = json.Unmarshal(sb, &s.SpeakerB)
		}

		// Collect session indices.
		var indices []int
		for key := range convMap {
			if strings.HasPrefix(key, "session_") && !strings.Contains(key, "date_time") {
				idx, err := strconv.Atoi(strings.TrimPrefix(key, "session_"))
				if err == nil {
					indices = append(indices, idx)
				}
			}
		}
		sort.Ints(indices)

		for _, idx := range indices {
			sessionKey := fmt.Sprintf("session_%d", idx)
			dateKey := fmt.Sprintf("session_%d_date_time", idx)

			sess := Session{Index: idx}

			if dt, ok := convMap[dateKey]; ok {
				_ = json.Unmarshal(dt, &sess.DateTime)
			}

			if turns, ok := convMap[sessionKey]; ok {
				if err := json.Unmarshal(turns, &sess.Turns); err != nil {
					return fmt.Errorf("unmarshal session_%d: %w", idx, err)
				}
			}

			s.Sessions = append(s.Sessions, sess)
		}
	}

	return nil
}

// Dataset is a collection of LoCoMo samples.
type Dataset []Sample

// QuestionScore holds the retrieval score for a single QA question.
type QuestionScore struct {
	Question  string
	Category  Category
	Precision float64
	Recall    float64
	F1        float64
	Retrieved int
	Evidence  int
}

// CategoryScore holds aggregated scores for a question category.
type CategoryScore struct {
	Category  string
	Count     int
	Precision float64
	Recall    float64
	F1        float64
}

// TokenUsage tracks LLM token consumption across a benchmark run.
type TokenUsage struct {
	InputTokens  int `json:"inputTokens"`
	OutputTokens int `json:"outputTokens"`
}

// BenchResult holds the full benchmark results.
type BenchResult struct {
	Provider   string
	Samples    int
	Questions  int
	Categories []CategoryScore
	Overall    CategoryScore
	Tokens     *TokenUsage `json:"tokens,omitempty"`
}
