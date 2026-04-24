package locomo

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/azagatti/hydra-db/internal/memory"
	"github.com/azagatti/hydra-db/internal/memory/inmemory"
)

// IngestedSample holds a memory plane loaded with conversation turns
// and an index mapping memory IDs to dialog IDs for scoring.
type IngestedSample struct {
	SampleID   string
	Plane      *memory.Plane
	MemToDiaID map[string]string // memory ID → dialog ID
	TurnCount  int
}

// Ingest converts a LoCoMo sample's conversation turns into Memory records
// stored in a fresh in-memory provider.
func Ingest(ctx context.Context, sample Sample) (*IngestedSample, error) {
	provider := inmemory.NewProvider()
	plane := memory.NewPlane(provider)

	result := &IngestedSample{
		SampleID:   sample.SampleID,
		Plane:      plane,
		MemToDiaID: make(map[string]string),
	}

	for _, sess := range sample.Sessions {
		sessionTime := parseDateTime(sess.DateTime)

		for i, turn := range sess.Turns {
			content, err := json.Marshal(map[string]string{
				"speaker": turn.Speaker,
				"text":    turn.Text,
				"dia_id":  turn.DiaID,
			})
			if err != nil {
				return nil, fmt.Errorf("marshal turn %s: %w", turn.DiaID, err)
			}

			mem := memory.NewMemory(memory.Episodic, content, turn.Speaker, sample.SampleID)
			mem.Tags = []string{fmt.Sprintf("session-%d", sess.Index)}
			mem.Metadata = map[string]string{
				"dia_id":  turn.DiaID,
				"session": fmt.Sprintf("%d", sess.Index),
			}
			// Offset each turn by 1 second to preserve ordering within a session.
			mem.CreatedAt = sessionTime.Add(time.Duration(i) * time.Second)

			if err := plane.Store(ctx, mem); err != nil {
				return nil, fmt.Errorf("store turn %s: %w", turn.DiaID, err)
			}

			result.MemToDiaID[mem.ID] = turn.DiaID
			result.TurnCount++
		}
	}

	return result, nil
}

func parseDateTime(s string) time.Time {
	formats := []string{
		"2 January 2006",
		"January 2, 2006",
		"2006-01-02",
		"2006-01-02T15:04:05",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	fmt.Fprintf(os.Stderr, "  [warn] unparseable date %q, using current time\n", s)
	return time.Now()
}
