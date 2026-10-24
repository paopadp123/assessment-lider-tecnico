package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBreakers_ReportsEachBreakerState(t *testing.T) {
	h := Breakers(func() BreakerStatus {
		return BreakerStatus{Name: "policy-issuer", State: "open", ConsecutiveFailures: 5}
	})
	rec := httptest.NewRecorder()

	h(rec, httptest.NewRequest(http.MethodGet, "/ops/breakers", nil))

	var got []BreakerStatus
	if rec.Code != http.StatusOK || json.NewDecoder(rec.Body).Decode(&got) != nil {
		t.Fatalf("status = %d", rec.Code)
	}
	if len(got) != 1 || got[0] != (BreakerStatus{Name: "policy-issuer", State: "open", ConsecutiveFailures: 5}) {
		t.Fatalf("cuerpo = %+v", got)
	}
}
