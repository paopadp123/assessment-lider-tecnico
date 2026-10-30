package contract

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMatch(t *testing.T) {
	example := json.RawMessage(`{"policyNumber":"POL-1","data":{"items":[{"id":"a"}]},"empty":[]}`)
	tests := []struct {
		name    string
		actual  string
		exact   []string
		problem string // vacío = coincide
	}{
		{"misma forma y campos de más", `{"policyNumber":"POL-9","data":{"items":[{"id":"b","x":1}]},"empty":[],"extra":true}`, nil, ""},
		{"falta un campo", `{"data":{"items":[]},"empty":[]}`, nil, "$.policyNumber: falta"},
		{"otro tipo", `{"policyNumber":7,"data":{"items":[]},"empty":[]}`, nil, "$.policyNumber: se esperaba texto, llegó número"},
		{"elemento de arreglo con otra forma", `{"policyNumber":"P","data":{"items":[{"id":1}]},"empty":[]}`, nil, "$.data.items[0].id"},
		{"arreglo vacío exige vacío", `{"policyNumber":"P","data":{"items":[]},"empty":[1]}`, nil, "$.empty: se esperaba un arreglo vacío"},
		{"valor exacto distinto", `{"policyNumber":"POL-9","data":{"items":[]},"empty":[]}`, []string{"policyNumber"}, "policyNumber: se esperaba POL-1"},
		{"valor exacto igual", `{"policyNumber":"POL-1","data":{"items":[]},"empty":[]}`, []string{"policyNumber"}, ""},
		{"no es JSON", `<html>503</html>`, nil, "cuerpo que no es JSON"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			problems := Match(example, json.RawMessage(tt.actual), tt.exact)
			joined := strings.Join(problems, "; ")
			switch {
			case tt.problem == "" && len(problems) > 0:
				t.Fatalf("no debía haber diferencias: %s", joined)
			case tt.problem != "" && !strings.Contains(joined, tt.problem):
				t.Fatalf("se esperaba %q, llegó %q", tt.problem, joined)
			}
		})
	}
}

func TestMatchRequestComparesValuesExceptDynamicFields(t *testing.T) {
	req := Request{Body: json.RawMessage(`{"plate":"ABC123","doc":"1020","header":{"id":"tx-1"}}`), Dynamic: []string{"header.id"}}
	if p := MatchRequest(req, json.RawMessage(`{"plate":"ABC123","doc":"1020","header":{"id":"tx-999"}}`)); len(p) > 0 {
		t.Fatalf("un campo dinámico distinto no debía fallar: %v", p)
	}
	p := MatchRequest(req, json.RawMessage(`{"plate":"1020","doc":"ABC123","header":{"id":"tx-1"}}`))
	if joined := strings.Join(p, "; "); !strings.Contains(joined, "plate: se esperaba ABC123") || !strings.Contains(joined, "doc:") {
		t.Fatalf("intercambiar placa y documento debía fallar: %v", p)
	}
}

// TestEveryInteractionHasAConsumerTest evita que una interacción nueva quede sin probar en silencio: cada ruta
// del contrato tiene que ser una de las que prueban los adaptadores.
func TestEveryInteractionHasAConsumerTest(t *testing.T) {
	tested := map[string][]string{
		"orchestrator-issuing-service.json": {"/policies"},                  // adapters/issuer
		"orchestrator-simulator.json":       {"/v1/quotes", "/v1/payments"}, // adapters/quoter y adapters/psp
	}
	for name, paths := range tested {
		f := Load(t, name)
		covered := 0
		for _, path := range paths {
			covered += len(f.ForPath(path))
		}
		if covered != len(f.Interactions) {
			t.Errorf("%s: %d interacciones, %d con prueba de consumidor", name, len(f.Interactions), covered)
		}
	}
}

func TestLoadFindsTheRepositoryContracts(t *testing.T) {
	f := Load(t, "orchestrator-issuing-service.json")
	if f.Consumer != "orchestrator" || f.Provider != "issuing-service" {
		t.Fatalf("contrato inesperado: %+v", f)
	}
	if len(f.ForPath("/policies")) == 0 {
		t.Fatal("sin interacciones para /policies")
	}
}
