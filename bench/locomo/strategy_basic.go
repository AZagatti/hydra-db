package locomo

import "context"

// BasicStrategy is the brute-force approach: all turns stored as episodic
// with confidence 1.0, all turns retrieved for every question.
type BasicStrategy struct{}

// Name returns the strategy identifier.
func (s *BasicStrategy) Name() string { return "basic" }

// Ingest delegates to the existing Ingest function.
func (s *BasicStrategy) Ingest(ctx context.Context, sample Sample) (*IngestedSample, error) {
	return Ingest(ctx, sample)
}

// Query delegates to the existing ExecuteQueries function.
func (s *BasicStrategy) Query(ctx context.Context, ingested *IngestedSample, qa []QAItem) ([]QueryResult, error) {
	return ExecuteQueries(ctx, ingested, qa)
}
