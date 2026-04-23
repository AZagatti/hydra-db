package adapter

import (
	"testing"

	"github.com/azagatti/hydra-db/internal/adapter/cli"
	"github.com/azagatti/hydra-db/internal/adapter/http"
	"github.com/azagatti/hydra-db/internal/adapter/slack"
)

func TestAdapter_Interface(t *testing.T) {
	t.Parallel()

	var _ Adapter = (*http.Adapter)(nil)
	var _ Adapter = (*cli.Adapter)(nil)
	var _ Adapter = (*slack.Adapter)(nil)
}
