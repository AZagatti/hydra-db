package locomo

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"
)

// ReportTable writes a human-readable results table to w.
func ReportTable(w io.Writer, result BenchResult) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "LoCoMo Benchmark Results")
	fmt.Fprintln(w, "========================")
	fmt.Fprintf(w, "Provider:   %s\n", result.Provider)
	fmt.Fprintf(w, "Samples:    %d\n", result.Samples)
	fmt.Fprintf(w, "Questions:  %d\n", result.Questions)
	fmt.Fprintln(w)

	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	fmt.Fprintf(tw, "Category\tCount\tPrecision\tRecall\tF1\n")
	fmt.Fprintf(tw, "--------\t-----\t---------\t------\t--\n")

	for _, cat := range result.Categories {
		fmt.Fprintf(tw, "%s\t%d\t%.4f\t%.4f\t%.4f\n",
			cat.Category, cat.Count, cat.Precision, cat.Recall, cat.F1)
	}

	fmt.Fprintf(tw, "--------\t-----\t---------\t------\t--\n")
	fmt.Fprintf(tw, "%s\t%d\t%.4f\t%.4f\t%.4f\n",
		result.Overall.Category, result.Overall.Count,
		result.Overall.Precision, result.Overall.Recall, result.Overall.F1)

	tw.Flush()

	if result.Tokens != nil {
		fmt.Fprintf(w, "\nToken Usage: %d input, %d output (%d total)\n",
			result.Tokens.InputTokens, result.Tokens.OutputTokens,
			result.Tokens.InputTokens+result.Tokens.OutputTokens)
	}

	fmt.Fprintln(w)
}

// ReportJSON writes machine-readable JSON results to w.
func ReportJSON(w io.Writer, result BenchResult) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}
