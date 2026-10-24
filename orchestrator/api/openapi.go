// Package api publica el contrato HTTP del orquestador (OpenAPI 3.0). Es la fuente de los tipos de la consola.
package api

import _ "embed"

// Spec es el contrato en YAML, embebido en el binario para servirlo sin depender de archivos externos.
//
//go:embed openapi.yaml
var Spec []byte
