package locomo

import (
	"context"
	"encoding/json"

	"github.com/azagatti/hydra-db/internal/memory"
)

// QueryResult holds the dialog IDs retrieved for a single QA question.
type QueryResult struct {
	QA           QAItem
	RetrievedIDs []string
}

// ExecuteQueries runs all QA questions against the ingested sample's memory plane.
// In the basic strategy, the search is identical for every question (no semantic
// filtering), so we execute it once and reuse the result set.
func ExecuteQueries(ctx context.Context, sample *IngestedSample, qa []QAItem) ([]QueryResult, error) {
	query := memory.SearchQuery{
		Type:     memory.Episodic,
		TenantID: sample.SampleID,
		Limit:    10000, // high limit to retrieve all turns (provider defaults 0 to 50)
	}

	memories, err := sample.Plane.Search(ctx, query)
	if err != nil {
		return nil, err
	}

	retrieved := extractDiaIDs(memories, sample.MemToDiaID)

	results := make([]QueryResult, 0, len(qa))
	for _, q := range qa {
		results = append(results, QueryResult{
			QA:           q,
			RetrievedIDs: retrieved,
		})
	}

	return results, nil
}

func extractDiaIDs(memories []*memory.Memory, memToDiaID map[string]string) []string {
	var ids []string
	for _, mem := range memories {
		// Try metadata first.
		if diaID, ok := mem.Metadata["dia_id"]; ok && diaID != "" {
			ids = append(ids, diaID)
			continue
		}
		// Fall back to memory ID index.
		if diaID, ok := memToDiaID[mem.ID]; ok {
			ids = append(ids, diaID)
			continue
		}
		// Last resort: extract from content.
		var content struct {
			DiaID string `json:"dia_id"`
		}
		if err := json.Unmarshal(mem.Content, &content); err == nil && content.DiaID != "" {
			ids = append(ids, content.DiaID)
		}
	}
	return ids
}
