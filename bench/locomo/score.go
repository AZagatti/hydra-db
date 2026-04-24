package locomo

// ScoreQuestion computes precision, recall, and F1 for a single QA question
// by comparing retrieved dialog IDs against ground truth evidence.
func ScoreQuestion(result QueryResult) QuestionScore {
	evidence := toSet(result.QA.Evidence)
	retrieved := toSet(result.RetrievedIDs)

	// Adversarial questions with no evidence: correct if nothing relevant retrieved.
	if len(evidence) == 0 {
		f1 := 0.0
		if len(retrieved) == 0 {
			f1 = 1.0
		}
		return QuestionScore{
			Question:  result.QA.Question,
			Category:  result.QA.Category,
			Precision: f1,
			Recall:    1.0, // nothing to recall
			F1:        f1,
			Retrieved: len(retrieved),
			Evidence:  0,
		}
	}

	tp := intersectionSize(retrieved, evidence)

	precision := 0.0
	if len(retrieved) > 0 {
		precision = float64(tp) / float64(len(retrieved))
	}

	recall := float64(tp) / float64(len(evidence))

	f1 := 0.0
	if precision+recall > 0 {
		f1 = 2 * precision * recall / (precision + recall)
	}

	return QuestionScore{
		Question:  result.QA.Question,
		Category:  result.QA.Category,
		Precision: precision,
		Recall:    recall,
		F1:        f1,
		Retrieved: len(retrieved),
		Evidence:  len(evidence),
	}
}

// AggregateByCategory computes macro-averaged scores per category.
func AggregateByCategory(scores []QuestionScore) []CategoryScore {
	type accumulator struct {
		precision float64
		recall    float64
		f1        float64
		count     int
	}

	buckets := make(map[Category]*accumulator)
	for _, s := range scores {
		acc, ok := buckets[s.Category]
		if !ok {
			acc = &accumulator{}
			buckets[s.Category] = acc
		}
		acc.precision += s.Precision
		acc.recall += s.Recall
		acc.f1 += s.F1
		acc.count++
	}

	var result []CategoryScore
	for _, cat := range AllCategories() {
		acc, ok := buckets[cat]
		if !ok {
			continue
		}
		n := float64(acc.count)
		result = append(result, CategoryScore{
			Category:  cat.String(),
			Count:     acc.count,
			Precision: acc.precision / n,
			Recall:    acc.recall / n,
			F1:        acc.f1 / n,
		})
	}

	return result
}

// AggregateOverall computes macro-averaged scores across all questions.
func AggregateOverall(scores []QuestionScore) CategoryScore {
	if len(scores) == 0 {
		return CategoryScore{Category: "OVERALL"}
	}

	var precision, recall, f1 float64
	for _, s := range scores {
		precision += s.Precision
		recall += s.Recall
		f1 += s.F1
	}

	n := float64(len(scores))
	return CategoryScore{
		Category:  "OVERALL",
		Count:     len(scores),
		Precision: precision / n,
		Recall:    recall / n,
		F1:        f1 / n,
	}
}

func toSet(items []string) map[string]struct{} {
	s := make(map[string]struct{}, len(items))
	for _, item := range items {
		s[item] = struct{}{}
	}
	return s
}

func intersectionSize(a, b map[string]struct{}) int {
	count := 0
	for k := range a {
		if _, ok := b[k]; ok {
			count++
		}
	}
	return count
}
