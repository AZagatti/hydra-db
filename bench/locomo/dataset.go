package locomo

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

const (
	datasetURL  = "https://raw.githubusercontent.com/snap-research/locomo/main/data/locomo10.json"
	cacheDir    = "bench/locomo/testdata"
	cacheFile   = "locomo10.json"
)

// LoadDataset loads the LoCoMo dataset from a local path or downloads it.
// If dataPath is empty, it downloads from the LoCoMo repo and caches locally.
func LoadDataset(dataPath string) (Dataset, error) {
	if dataPath == "" {
		var err error
		dataPath, err = ensureCached()
		if err != nil {
			return nil, fmt.Errorf("download dataset: %w", err)
		}
	}

	data, err := os.ReadFile(dataPath)
	if err != nil {
		return nil, fmt.Errorf("read dataset: %w", err)
	}

	var dataset Dataset
	if err := json.Unmarshal(data, &dataset); err != nil {
		return nil, fmt.Errorf("parse dataset: %w", err)
	}

	// Assign sample IDs based on array index if not set.
	for i := range dataset {
		if dataset[i].SampleID == "" {
			dataset[i].SampleID = fmt.Sprintf("sample_%d", i)
		}
	}

	return dataset, nil
}

func ensureCached() (string, error) {
	path := filepath.Join(cacheDir, cacheFile)

	if _, err := os.Stat(path); err == nil {
		return path, nil
	}

	fmt.Printf("Downloading LoCoMo dataset from %s ...\n", datasetURL)

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("create cache dir: %w", err)
	}

	resp, err := http.Get(datasetURL)
	if err != nil {
		return "", fmt.Errorf("HTTP GET: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d from %s", resp.StatusCode, datasetURL)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if err := os.WriteFile(path, body, 0o644); err != nil {
		return "", fmt.Errorf("write cache: %w", err)
	}

	fmt.Printf("Cached dataset at %s (%d bytes)\n", path, len(body))
	return path, nil
}
