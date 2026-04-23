package body

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHealthReport_Fields(t *testing.T) {
	report := HealthReport{
		Head:   "api",
		Status: HealthHealthy,
		Detail: "all good",
	}

	assert.Equal(t, "api", report.Head)
	assert.Equal(t, HealthHealthy, report.Status)
	assert.Equal(t, "all good", report.Detail)
}

func TestHealthStatus_Values(t *testing.T) {
	assert.Equal(t, HealthStatus("healthy"), HealthHealthy)
	assert.Equal(t, HealthStatus("degraded"), HealthDegraded)
	assert.Equal(t, HealthStatus("unhealthy"), HealthUnhealthy)
}
