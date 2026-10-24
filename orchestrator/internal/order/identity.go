package order

import "log/slog"

// Plate es la placa de un vehículo, validada y normalizada.
type Plate struct{ raw string }

// NewPlate valida y normaliza una placa.
func NewPlate(s string) (Plate, error) { return Plate{raw: s}, nil }

// Raw devuelve la placa completa. Solo para persistir o enviar a terceros, nunca para loguear.
func (p Plate) Raw() string { return p.raw }

func (p Plate) String() string       { return p.raw }
func (p Plate) LogValue() slog.Value { return slog.StringValue(p.raw) }

// DocumentID es el documento de identidad del tomador, validado.
type DocumentID struct{ raw string }

// NewDocumentID valida un documento de identidad.
func NewDocumentID(s string) (DocumentID, error) { return DocumentID{raw: s}, nil }

// Raw devuelve el documento completo. Solo para persistir o enviar a terceros, nunca para loguear.
func (d DocumentID) Raw() string { return d.raw }

func (d DocumentID) String() string       { return d.raw }
func (d DocumentID) LogValue() slog.Value { return slog.StringValue(d.raw) }
