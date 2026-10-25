package order

import (
	"encoding/json"
	"errors"
	"log/slog"
	"regexp"
	"strings"
)

// OrderID identifica una orden. Es un tipo propio para que el compilador no deje confundirlo
// con otros identificadores (cotización, pago) que también son texto.
type OrderID string

var (
	// Placa colombiana: carro ABC123 o moto ABC12D.
	plateFormat    = regexp.MustCompile(`^[A-Z]{3}[0-9]{2}[0-9A-Z]$`)
	documentFormat = regexp.MustCompile(`^[0-9]{5,10}$`)
)

// Plate es la placa de un vehículo, validada y normalizada.
// String, GoString, LogValue y MarshalJSON la enmascaran: no se puede imprimir ni serializar en claro por accidente.
type Plate struct{ raw string }

// NewPlate valida y normaliza una placa (mayúsculas, sin espacios ni guiones).
func NewPlate(s string) (Plate, error) {
	norm := strings.ToUpper(strings.NewReplacer(" ", "", "-", "").Replace(s))
	if !plateFormat.MatchString(norm) {
		return Plate{}, errors.New("must be a Colombian plate like ABC123 or ABC12D")
	}
	return Plate{raw: norm}, nil
}

// Raw devuelve la placa completa. Solo para persistir o enviar a terceros, nunca para loguear.
func (p Plate) Raw() string { return p.raw }

func (p Plate) String() string               { return p.masked() }
func (p Plate) GoString() string             { return p.masked() } // %#v ignora String y mostraría el campo interno
func (p Plate) LogValue() slog.Value         { return slog.StringValue(p.masked()) }
func (p Plate) MarshalJSON() ([]byte, error) { return json.Marshal(p.masked()) }

func (p Plate) masked() string {
	if len(p.raw) < 3 {
		return "***"
	}
	return p.raw[:3] + "***"
}

// DocumentID es el documento de identidad del tomador, validado.
// String, GoString, LogValue y MarshalJSON lo enmascaran: no se puede imprimir ni serializar en claro por accidente.
type DocumentID struct{ raw string }

// NewDocumentID valida un documento de identidad numérico de 5 a 10 dígitos.
func NewDocumentID(s string) (DocumentID, error) {
	norm := strings.TrimSpace(s)
	if !documentFormat.MatchString(norm) {
		return DocumentID{}, errors.New("must have 5 to 10 digits")
	}
	return DocumentID{raw: norm}, nil
}

// Raw devuelve el documento completo. Solo para persistir o enviar a terceros, nunca para loguear.
func (d DocumentID) Raw() string { return d.raw }

func (d DocumentID) String() string               { return d.masked() }
func (d DocumentID) GoString() string             { return d.masked() }
func (d DocumentID) LogValue() slog.Value         { return slog.StringValue(d.masked()) }
func (d DocumentID) MarshalJSON() ([]byte, error) { return json.Marshal(d.masked()) }

func (d DocumentID) masked() string {
	if len(d.raw) < 4 {
		return "****"
	}
	return "****" + d.raw[len(d.raw)-4:]
}
