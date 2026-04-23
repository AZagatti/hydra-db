package body

// HealthStatus represents the operational state of a head.
type HealthStatus string

const (
	// HealthHealthy means the head is fully operational.
	HealthHealthy HealthStatus = "healthy"
	// HealthDegraded means the head is running but with reduced capability.
	HealthDegraded HealthStatus = "degraded"
	// HealthUnhealthy means the head is not functioning correctly.
	HealthUnhealthy HealthStatus = "unhealthy"
)

// HealthReport is the health check result for a single head.
type HealthReport struct {
	Head   string       `json:"head"`
	Status HealthStatus `json:"status"`
	Detail string       `json:"detail,omitempty"`
}
