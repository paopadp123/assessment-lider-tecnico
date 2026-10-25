package order

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

func TestNewPlate(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "ABC123", want: "ABC123"},
		{in: " abc-123 ", want: "ABC123"},
		{in: "abc12d", want: "ABC12D"}, // motocicleta
		{in: "AB1234", wantErr: true},
		{in: "ABC1234", wantErr: true},
		{in: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			p, err := NewPlate(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && p.Raw() != tt.want {
				t.Fatalf("Raw() = %q, want %q", p.Raw(), tt.want)
			}
		})
	}
}

func TestNewDocumentID(t *testing.T) {
	tests := []struct {
		in      string
		wantErr bool
	}{
		{in: "1020304050"},
		{in: "12345"},
		{in: "1234", wantErr: true},
		{in: "12345678901", wantErr: true},
		{in: "10203A4050", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if _, err := NewDocumentID(tt.in); (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// Regla crítica: ningún dato sensible en claro en los logs, aunque alguien loguee el valor completo.
func TestSensitiveValuesAreMaskedInLogsAndFormatting(t *testing.T) {
	plate, _ := NewPlate("ABC123")
	doc, _ := NewDocumentID("1020304050")

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	logger.Info("order", "plate", plate, "document", doc)
	printed := fmt.Sprintf("%v %s %+v %#v %#v", plate, doc, struct{ P Plate }{plate}, plate, doc)
	serialized, err := json.Marshal(struct {
		P Plate
		D DocumentID
	}{plate, doc})
	if err != nil {
		t.Fatal(err)
	}

	for _, out := range []string{buf.String(), printed, string(serialized)} {
		if strings.Contains(out, "ABC123") || strings.Contains(out, "1020304050") {
			t.Fatalf("dato sensible en claro: %s", out)
		}
	}
	if !strings.Contains(buf.String(), "ABC***") || !strings.Contains(buf.String(), "****4050") {
		t.Fatalf("enmascarado inesperado: %s", buf.String())
	}
}

func TestNewOrder_ValidatesAllFields(t *testing.T) {
	_, err := NewOrder(NewOrderInput{Plate: "X", DocumentID: "1", VehicleYear: 1900, CityCode: "BOG"})

	var verr ValidationErrors
	if !errors.As(err, &verr) {
		t.Fatalf("err = %v, want ValidationErrors", err)
	}
	fields := map[string]bool{}
	for _, e := range verr {
		fields[e.Field] = true
	}
	for _, f := range []string{"plate", "documentId", "vehicleYear", "cityCode"} {
		if !fields[f] {
			t.Errorf("falta el error del campo %s (errores: %v)", f, verr)
		}
	}
}

func TestNewOrder_ValidInputStartsCreated(t *testing.T) {
	o, err := NewOrder(NewOrderInput{Plate: "abc123", DocumentID: "1020304050", VehicleYear: 2020, CityCode: "11001"})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if o.Status != Created || o.Plate.Raw() != "ABC123" {
		t.Fatalf("orden inesperada: %+v", o)
	}
}
