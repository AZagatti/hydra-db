package locomo

import "context"

// Strategy defines how conversation turns are ingested into memory
// and how QA questions are turned into memory queries.
type Strategy interface {
	// Name returns a human-readable identifier for the strategy.
	Name() string

	// Ingest processes a sample's conversation into a populated IngestedSample.
	Ingest(ctx context.Context, sample Sample) (*IngestedSample, error)

	// Query executes all QA items against the ingested sample, returning
	// retrieved dialog IDs per question.
	Query(ctx context.Context, ingested *IngestedSample, qa []QAItem) ([]QueryResult, error)
}
