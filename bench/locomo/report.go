package locomo

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"
)

// ReportTable writes a human-readable results table to w.
func ReportTable(w io.Writer, result BenchResult) {
	//nolint:errcheck
	fmt.Fprintln(w)
	//nolint:errcheck
	fmt.Fprintln(w, "LoCoMo Benchmark Results")
	//nolint:errcheck
	fmt.Fprintln(w, "========================")
	//nolint:errcheck
	fmt.Fprintf(w, "Provider:   %s\n", result.Provider)
	//nolint:errcheck
	fmt.Fprintf(w, "Samples:    %d\n", result.Samples)
	//nolint:errcheck
	fmt.Fprintf(w, "Questions:  %d\n", result.Questions)
	//nolint:errcheck
	fmt.Fprintln(w)

	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	//nolint:errcheck
	fmt.Fprintf(tw, "Category\tCount\tPrecision\tRecall\tF1\n")
	//nolint:errcheck
	fmt.Fprintf(tw, "--------\t-----\t---------\t------\t--\n")

	for _, cat := range result.Categories {
		//nolint:errcheck
		fmt.Fprintf(tw, "%s\t%d\t%.4f\t%.4f\t%.4f\n",
			cat.Category, cat.Count, cat.Precision, cat.Recall, cat.F1)
	}

	//nolint:errcheck
	fmt.Fprintf(tw, "--------\t-----\t---------\t------\t--\n")
	//nolint:errcheck
	fmt.Fprintf(tw, "%s\t%d\t%.4f\t%.4f\t%.4f\n",
		result.Overall.Category, result.Overall.Count,
		result.Overall.Precision, result.Overall.Recall, result.Overall.F1)

	//nolint:errcheck
	tw.Flush()

	if result.Tokens != nil {
		//nolint:errcheck
		fmt.Fprintf(w, "\nToken Usage: %d input, %d output (%d total)\n",
			result.Tokens.InputTokens, result.Tokens.OutputTokens,
			result.Tokens.InputTokens+result.Tokens.OutputTokens)
	}

	//nolint:errcheck
	fmt.Fprintln(w)
}

// ReportJSON writes machine-readable JSON results to w.
func ReportJSON(w io.Writer, result BenchResult) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}
