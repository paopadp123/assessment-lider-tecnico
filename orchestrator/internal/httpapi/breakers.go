package httpapi

import (
	"net/http"
)

// BreakerStatus es el estado de un circuit breaker, visible para operación.
type BreakerStatus struct {
	Name                string `json:"name"`
	State               string `json:"state"` // closed, open o half-open
	ConsecutiveFailures uint32 `json:"consecutiveFailures"`
}

// Breakers expone el estado de los circuit breakers en GET /ops/breakers.
func Breakers(sources ...func() BreakerStatus) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {}
}
