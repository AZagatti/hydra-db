package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/azagatti/hydra-db/bench/locomo"
	"github.com/azagatti/hydra-db/internal/llm"
)

func main() {
	dataPath := flag.String("data", "", "path to locomo10.json (downloads if empty)")
	jsonOut := flag.Bool("json", false, "output results as JSON")
	limit := flag.Int("limit", 0, "limit number of samples to process (0 = all)")
	strategy := flag.String("strategy", "basic", "ingestion/query strategy: basic|llm")
	sidecarURL := flag.String("sidecar-url", "http://localhost:3100", "LLM sidecar URL (for llm strategy)")
	flag.Parse()

	dataset, err := locomo.LoadDataset(*dataPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading dataset: %v\n", err)
		os.Exit(1)
	}

	if *limit > 0 && *limit < len(dataset) {
		dataset = dataset[:*limit]
	}

	ctx := context.Background()

	var strat locomo.Strategy
	switch *strategy {
	case "basic":
		strat = &locomo.BasicStrategy{}
	case "llm":
		client := llm.NewClient(llm.WithBaseURL(*sidecarURL))
		if err := client.Health(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "LLM sidecar not reachable at %s: %v\n", *sidecarURL, err)
			fmt.Fprintf(os.Stderr, "Start the sidecar with: make sidecar-start\n")
			os.Exit(1)
		}
		strat = locomo.NewLLMStrategy(client)
	default:
		fmt.Fprintf(os.Stderr, "Unknown strategy: %s (use basic or llm)\n", *strategy)
		os.Exit(1)
	}

	result, err := runBenchmark(ctx, dataset, strat)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running benchmark: %v\n", err)
		os.Exit(1)
	}

	if *jsonOut {
		if err := locomo.ReportJSON(os.Stdout, result); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing JSON: %v\n", err)
			os.Exit(1)
		}
	} else {
		locomo.ReportTable(os.Stdout, result)
	}
}

func runBenchmark(ctx context.Context, dataset locomo.Dataset, strat locomo.Strategy) (locomo.BenchResult, error) {
	var allScores []locomo.QuestionScore
	totalQuestions := 0

	for i, sample := range dataset {
		fmt.Printf("[%s] Processing sample %d/%d (%d sessions, %d QA items)...\n",
			strat.Name(), i+1, len(dataset), len(sample.Sessions), len(sample.QA))

		ingested, err := strat.Ingest(ctx, sample)
		if err != nil {
			return locomo.BenchResult{}, fmt.Errorf("ingest sample %s: %w", sample.SampleID, err)
		}

		results, err := strat.Query(ctx, ingested, sample.QA)
		if err != nil {
			return locomo.BenchResult{}, fmt.Errorf("query sample %s: %w", sample.SampleID, err)
		}

		for _, r := range results {
			score := locomo.ScoreQuestion(r)
			allScores = append(allScores, score)
		}

		totalQuestions += len(sample.QA)
	}

	categories := locomo.AggregateByCategory(allScores)
	overall := locomo.AggregateOverall(allScores)

	result := locomo.BenchResult{
		Provider:   strat.Name(),
		Samples:    len(dataset),
		Questions:  totalQuestions,
		Categories: categories,
		Overall:    overall,
	}

	// Attach token usage if LLM strategy was used.
	if llmStrat, ok := strat.(*locomo.LLMStrategy); ok {
		result.Tokens = &locomo.TokenUsage{
			InputTokens:  llmStrat.TotalUsage.InputTokens,
			OutputTokens: llmStrat.TotalUsage.OutputTokens,
		}
	}

	return result, nil
}
