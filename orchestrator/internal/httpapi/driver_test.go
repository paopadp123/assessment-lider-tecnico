package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
)

const testDeviceToken = "device-token-with-at-least-32-bytes!!"

type fakeDriverReporter struct {
	created bool
	err     error
	got     []app.DriverAlert
}

func (f *fakeDriverReporter) Execute(_ context.Context, a app.DriverAlert) (bool, error) {
	f.got = append(f.got, a)
	return f.created, f.err
}

func alertBody(occurredAt time.Time) string {
	return `{"alertId":"6f1c2a7e-0000-4000-8000-000000000001","plate":"ABC123","tripId":"trip-1","type":"PANIC","occurredAt":"` +
		occurredAt.UTC().Format(time.RFC3339Nano) + `"}`
}

func postAlert(t *testing.T, h *Driver, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	h.Register(mux)
	req := httptest.NewRequest(http.MethodPost, "/driver/alerts", strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestDriverAlerts_FirstIs201AndRetryIs200(t *testing.T) {
	body := alertBody(time.Now().Add(-time.Minute))
	first := &fakeDriverReporter{created: true}
	rec := postAlert(t, NewDriver(first, testDeviceToken, discard()), testDeviceToken, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("primera alerta: status %d, cuerpo %s", rec.Code, rec.Body)
	}
	var res struct {
		AlertID string `json:"alertId"`
		Status  string `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || res.AlertID != "6f1c2a7e-0000-4000-8000-000000000001" || res.Status != "RECEIVED" {
		t.Fatalf("respuesta = %s (err %v)", rec.Body, err)
	}
	got := first.got[0]
	if got.Plate.Raw() != "ABC123" || got.TripID != "trip-1" || got.Type != app.DriverAlertPanic {
		t.Fatalf("alerta enviada al caso de uso = %+v", got)
	}

	// El teléfono reintenta la misma alerta (no recibió la primera respuesta): 200, no un error.
	if rec := postAlert(t, NewDriver(&fakeDriverReporter{created: false}, testDeviceToken, discard()), testDeviceToken, body); rec.Code != http.StatusOK {
		t.Fatalf("reintento: status %d, se esperaba 200", rec.Code)
	}
}

func TestDriverAlerts_RequireTheDeviceToken(t *testing.T) {
	body := alertBody(time.Now())
	cases := []struct {
		name, configured, sent string
	}{
		{"sin token", testDeviceToken, ""},
		{"token equivocado", testDeviceToken, "otro-token-con-al-menos-32-bytes!!!!!"},
		{"sin token configurado en el servidor", "", ""},
		{"servidor sin token aunque el cliente envíe uno vacío", "", " "},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reporter := &fakeDriverReporter{created: true}
			rec := postAlert(t, NewDriver(reporter, c.configured, discard()), c.sent, body)
			if rec.Code != http.StatusUnauthorized || len(reporter.got) != 0 {
				t.Fatalf("status %d y %d alertas procesadas; se esperaba 401 sin procesar", rec.Code, len(reporter.got))
			}
		})
	}
}

func TestDriverAlerts_InvalidBodiesAre400(t *testing.T) {
	now := time.Now().UTC()
	valid := alertBody(now)
	cases := map[string]string{
		"no es JSON":        `{`,
		"alertId no UUID":   strings.Replace(valid, "6f1c2a7e-0000-4000-8000-000000000001", "alerta-1", 1),
		"placa inválida":    strings.Replace(valid, "ABC123", "AB-12", 1),
		"tipo desconocido":  strings.Replace(valid, "PANIC", "FIRE", 1),
		"viaje vacío":       strings.Replace(valid, `"trip-1"`, `""`, 1),
		"fecha inválida":    strings.Replace(valid, now.Format(time.RFC3339Nano), "ayer", 1),
		"fecha absurda":     alertBody(now.Add(48 * time.Hour)),
		"campo desconocido": strings.Replace(valid, `"type"`, `"extra":1,"type"`, 1),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			reporter := &fakeDriverReporter{created: true}
			rec := postAlert(t, NewDriver(reporter, testDeviceToken, discard()), testDeviceToken, body)
			if rec.Code != http.StatusBadRequest || len(reporter.got) != 0 {
				t.Fatalf("status %d y %d alertas procesadas; se esperaba 400 sin procesar", rec.Code, len(reporter.got))
			}
		})
	}
}

// Un teléfono con el reloj adelantado no debe perder una alerta de pánico.
func TestDriverAlerts_PhoneClockAheadIsAccepted(t *testing.T) {
	reporter := &fakeDriverReporter{created: true}
	rec := postAlert(t, NewDriver(reporter, testDeviceToken, discard()), testDeviceToken, alertBody(time.Now().Add(2*time.Hour)))
	if rec.Code != http.StatusCreated || len(reporter.got) != 1 {
		t.Fatalf("reloj adelantado 2 h: status %d, se esperaba 201", rec.Code)
	}
}

func TestDriverAlerts_ConflictIs409AndStoreErrorIs500(t *testing.T) {
	body := alertBody(time.Now())
	if rec := postAlert(t, NewDriver(&fakeDriverReporter{err: app.ErrDriverAlertConflict}, testDeviceToken, discard()), testDeviceToken, body); rec.Code != http.StatusConflict {
		t.Fatalf("mismo id con otro contenido: status %d, se esperaba 409", rec.Code)
	}
	// 500: el teléfono conserva la alerta en su cola y la reintenta.
	if rec := postAlert(t, NewDriver(&fakeDriverReporter{err: errors.New("db down")}, testDeviceToken, discard()), testDeviceToken, body); rec.Code != http.StatusInternalServerError {
		t.Fatalf("error de la base: status %d, se esperaba 500", rec.Code)
	}
}
