package locomo

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/azagatti/hydra-db/internal/llm"
	"github.com/azagatti/hydra-db/internal/memory"
	"github.com/azagatti/hydra-db/internal/memory/inmemory"
)

const classifyBatchSize = 15

const classifySystemPrompt = `You are a memory classification engine. Classify each dialogue turn into a memory type with confidence and tags.

Memory types:
- episodic: Time-bounded events, experiences, interactions. "We went to Paris last summer."
- semantic: General knowledge, facts, preferences, relationships. "I'm allergic to peanuts."
- operational: Procedures, instructions, how-to knowledge. "To reset your password, go to settings."
- working: Transient filler, greetings, acknowledgments. "Haha yeah" or "Let me check."

For each turn, respond with a JSON array where each element has:
- "dia_id": the dialog ID from the input
- "memory_type": one of episodic, semantic, operational, working
- "confidence": 0.0 to 1.0 (how strongly this turn contains memorable information; filler/greetings should be 0.1-0.2)
- "tags": 1-5 relevant topic tags from this vocabulary: travel, food, health, family, work, hobby, birthday, preference, location, relationship, event, plan, opinion, music, sports, education, finance, technology, pet, weather
- "summary": one-sentence summary of the memorable content

Respond ONLY with the JSON array, no other text or markdown.`

const querySystemPrompt = `You are a memory retrieval planner. Given a question about a conversation, determine the best search parameters to find relevant memories.

Available memory types: episodic, semantic, operational, working
Available tags: travel, food, health, family, work, hobby, birthday, preference, location, relationship, event, plan, opinion, music, sports, education, finance, technology, pet, weather

Respond with a JSON object:
- "memory_type": which type is most likely to contain the answer (use "" to search all types)
- "tags": array with exactly ONE most-relevant tag (the system requires ALL tags to match, so use only the single best one; use [] to skip tag filtering)
- "min_confidence": minimum confidence threshold (use 0.3 to filter noise, 0.0 to include everything)
- "reasoning": brief explanation of your search strategy

Respond ONLY with the JSON object, no other text or markdown.`

// TurnClassification is the LLM's classification of a conversation turn.
type TurnClassification struct {
	DiaID      string   `json:"dia_id"`
	MemoryType string   `json:"memory_type"`
	Confidence float64  `json:"confidence"`
	Tags       []string `json:"tags"`
	Summary    string   `json:"summary"`
}

// SmartSearchQuery is the LLM's recommended search parameters.
type SmartSearchQuery struct {
	MemoryType    string   `json:"memory_type"`
	Tags          []string `json:"tags"`
	MinConfidence float64  `json:"min_confidence"`
	Reasoning     string   `json:"reasoning"`
}

// LLMStrategy uses an LLM to classify turns and generate targeted queries.
// Not safe for concurrent use.
type LLMStrategy struct {
	client     *llm.Client
	TotalUsage TokenUsage
}

// NewLLMStrategy creates an LLM-powered strategy.
func NewLLMStrategy(client *llm.Client) *LLMStrategy {
	return &LLMStrategy{client: client}
}

// Name returns the strategy identifier.
func (s *LLMStrategy) Name() string { return "llm" }

// Ingest classifies each conversation turn via LLM and stores with
// appropriate type, confidence, and tags.
func (s *LLMStrategy) Ingest(ctx context.Context, sample Sample) (*IngestedSample, error) {
	provider := inmemory.NewProvider()
	plane := memory.NewPlane(provider)

	result := &IngestedSample{
		SampleID:   sample.SampleID,
		Plane:      plane,
		MemToDiaID: make(map[string]string),
	}

	for si, sess := range sample.Sessions {
		fmt.Fprintf(os.Stderr,"  [ingest] Session %d/%d (index %d, %d turns)...\n",
			si+1, len(sample.Sessions), sess.Index, len(sess.Turns))
		sessionTime := parseDateTime(sess.DateTime)

		// Process turns in batches.
		for batchStart := 0; batchStart < len(sess.Turns); batchStart += classifyBatchSize {
			batchEnd := batchStart + classifyBatchSize
			if batchEnd > len(sess.Turns) {
				batchEnd = len(sess.Turns)
			}
			batch := sess.Turns[batchStart:batchEnd]

			classifications, err := s.classifyBatch(ctx, sample, sess, batch)
			if err != nil {
				// Fall back to basic classification on LLM failure.
				fmt.Fprintf(os.Stderr,"  [warn] LLM classification failed for session %d batch %d, falling back to basic: %v\n",
					sess.Index, batchStart/classifyBatchSize, err)
				classifications = fallbackClassify(batch)
			}

			// Build lookup for classifications by dia_id.
			classMap := make(map[string]TurnClassification, len(classifications))
			for _, c := range classifications {
				classMap[c.DiaID] = c
			}

			for i, turn := range batch {
				cls, ok := classMap[turn.DiaID]
				if !ok {
					cls = TurnClassification{
						DiaID:      turn.DiaID,
						MemoryType: "episodic",
						Confidence: 0.5,
						Tags:       []string{},
						Summary:    turn.Text,
					}
				}

				content, err := json.Marshal(map[string]string{
					"speaker": turn.Speaker,
					"text":    turn.Text,
					"dia_id":  turn.DiaID,
					"summary": cls.Summary,
				})
				if err != nil {
					return nil, fmt.Errorf("marshal turn %s: %w", turn.DiaID, err)
				}

				memType := parseMemoryType(cls.MemoryType)
				mem := memory.NewMemory(memType, content, turn.Speaker, sample.SampleID)
				mem.Confidence = clampConfidence(cls.Confidence)
				mem.Tags = append([]string{fmt.Sprintf("session-%d", sess.Index)}, cls.Tags...)
				mem.Metadata = map[string]string{
					"dia_id":  turn.DiaID,
					"session": fmt.Sprintf("%d", sess.Index),
					"summary": cls.Summary,
				}
				mem.CreatedAt = sessionTime.Add(time.Duration(batchStart+i) * time.Second)

				if err := plane.Store(ctx, mem); err != nil {
					return nil, fmt.Errorf("store turn %s: %w", turn.DiaID, err)
				}

				result.MemToDiaID[mem.ID] = turn.DiaID
				result.TurnCount++
			}
		}
	}

	return result, nil
}

// Query uses the LLM to generate targeted search parameters for each question.
func (s *LLMStrategy) Query(ctx context.Context, ingested *IngestedSample, qa []QAItem) ([]QueryResult, error) {
	results := make([]QueryResult, 0, len(qa))

	for i, q := range qa {
		if (i+1)%20 == 0 || i == 0 {
			fmt.Fprintf(os.Stderr,"  [query] Processing question %d/%d...\n", i+1, len(qa))
		}

		smartQuery, err := s.planQuery(ctx, q)
		if err != nil {
			// Fall back to broad search.
			smartQuery = &SmartSearchQuery{MinConfidence: 0.2}
		}

		searchQuery := memory.SearchQuery{
			TenantID:      ingested.SampleID,
			MinConfidence: smartQuery.MinConfidence,
			Limit:         10000,
		}

		if smartQuery.MemoryType != "" {
			searchQuery.Type = parseMemoryType(smartQuery.MemoryType)
		}
		if len(smartQuery.Tags) > 0 {
			searchQuery.Tags = smartQuery.Tags
		}

		memories, err := ingested.Plane.Search(ctx, searchQuery)
		if err != nil {
			return nil, fmt.Errorf("search for question %q: %w", q.Question, err)
		}

		retrieved := extractDiaIDs(memories, ingested.MemToDiaID)

		results = append(results, QueryResult{
			QA:           q,
			RetrievedIDs: retrieved,
		})
	}

	return results, nil
}

func (s *LLMStrategy) classifyBatch(ctx context.Context, sample Sample, sess Session, turns []Turn) ([]TurnClassification, error) {
	var lines []string
	for _, t := range turns {
		lines = append(lines, fmt.Sprintf("%s | %s: %s", t.DiaID, t.Speaker, t.Text))
	}

	userMsg := fmt.Sprintf("Conversation between %s and %s, session %d (%s):\n\n%s",
		sample.SpeakerA, sample.SpeakerB, sess.Index, sess.DateTime,
		strings.Join(lines, "\n"))

	resp, err := s.client.Complete(ctx, llm.CompleteRequest{
		SystemPrompt: classifySystemPrompt,
		UserMessage:  userMsg,
		MaxTokens:    2048,
	})
	if err != nil {
		return nil, fmt.Errorf("llm complete: %w", err)
	}

	s.TotalUsage.InputTokens += resp.Usage.InputTokens
	s.TotalUsage.OutputTokens += resp.Usage.OutputTokens

	text := cleanJSON(resp.Text)
	if text == "" {
		return nil, fmt.Errorf("LLM returned empty response (tokens: %d in / %d out)", resp.Usage.InputTokens, resp.Usage.OutputTokens)
	}

	var classifications []TurnClassification
	if err := json.Unmarshal([]byte(text), &classifications); err != nil {
		return nil, fmt.Errorf("parse classification JSON: %w\nraw response (first 500 chars): %.500s", err, text)
	}

	return classifications, nil
}

func (s *LLMStrategy) planQuery(ctx context.Context, qa QAItem) (*SmartSearchQuery, error) {
	userMsg := fmt.Sprintf("Question: %s\nCategory: %s", qa.Question, qa.Category.String())

	resp, err := s.client.Complete(ctx, llm.CompleteRequest{
		SystemPrompt: querySystemPrompt,
		UserMessage:  userMsg,
		MaxTokens:    256,
	})
	if err != nil {
		return nil, fmt.Errorf("llm complete: %w", err)
	}

	s.TotalUsage.InputTokens += resp.Usage.InputTokens
	s.TotalUsage.OutputTokens += resp.Usage.OutputTokens

	text := cleanJSON(resp.Text)
	if text == "" {
		return nil, fmt.Errorf("LLM returned empty response for query")
	}

	var query SmartSearchQuery
	if err := json.Unmarshal([]byte(text), &query); err != nil {
		return nil, fmt.Errorf("parse query JSON: %w\nraw response (first 500 chars): %.500s", err, text)
	}

	return &query, nil
}

func parseMemoryType(s string) memory.Type {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "episodic":
		return memory.Episodic
	case "semantic":
		return memory.Semantic
	case "operational":
		return memory.Operational
	case "working":
		return memory.Working
	default:
		return memory.Episodic
	}
}

func clampConfidence(c float64) float64 {
	if c < 0 {
		return 0
	}
	if c > 1 {
		return 1
	}
	return c
}

func fallbackClassify(turns []Turn) []TurnClassification {
	result := make([]TurnClassification, len(turns))
	for i, t := range turns {
		result[i] = TurnClassification{
			DiaID:      t.DiaID,
			MemoryType: "episodic",
			Confidence: 0.5,
			Tags:       []string{},
			Summary:    t.Text,
		}
	}
	return result
}

// cleanJSON strips markdown code fences and whitespace from LLM JSON output.
func cleanJSON(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}
