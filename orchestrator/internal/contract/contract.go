// Package contract carga los contratos de contracts/ (en la raíz del repositorio) y compara cuerpos JSON
// por forma. Lo usan solo las pruebas de contrato de los adaptadores; el formato está en contracts/README.md.
package contract

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// File es un contrato entre un consumidor y un proveedor.
type File struct {
	Consumer     string        `json:"consumer"`
	Provider     string        `json:"provider"`
	Interactions []Interaction `json:"interactions"`
}

// Interaction es una solicitud del consumidor y la respuesta que necesita.
type Interaction struct {
	Name            string   `json:"name"`
	ProviderState   string   `json:"providerState"`
	Request         Request  `json:"request"`
	Response        Response `json:"response"`
	Exact           []string `json:"exact"`
	ConsumerExpects string   `json:"consumerExpects"`
}

// Request es lo que envía el consumidor.
type Request struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Query   map[string]string `json:"query"`
	Headers map[string]string `json:"headers"`
	Body    json.RawMessage   `json:"body"`
	// Dynamic son los campos que el consumidor genera en cada llamada (id de transacción, fecha): se comparan
	// por forma. El resto de la solicitud se compara por valor.
	Dynamic []string `json:"dynamic"`
}

// Response es lo que el consumidor necesita recibir.
type Response struct {
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body"`
}

// Load lee contracts/<name> subiendo desde el directorio de la prueba hasta encontrar la carpeta.
func Load(t testing.TB, name string) File {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		path := filepath.Join(dir, "contracts", name)
		if data, err := os.ReadFile(path); err == nil {
			var f File
			if err := json.Unmarshal(data, &f); err != nil {
				t.Fatalf("contrato %s inválido: %v", name, err)
			}
			if len(f.Interactions) == 0 {
				t.Fatalf("contrato %s sin interacciones", name)
			}
			return f
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no se encontró contracts/%s subiendo desde el directorio de la prueba", name)
		}
		dir = parent
	}
}

// Provider levanta un proveedor falso que responde lo que dice el contrato y registra como error de la prueba
// cualquier diferencia entre la solicitud del consumidor y la del contrato.
func Provider(t testing.TB, it Interaction) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != it.Request.Method || r.URL.Path != it.Request.Path {
			t.Errorf("%s: solicitud %s %s, el contrato dice %s %s", it.Name, r.Method, r.URL.Path, it.Request.Method, it.Request.Path)
		}
		for key, want := range it.Request.Query {
			if got := r.URL.Query().Get(key); got != want {
				t.Errorf("%s: query %s=%q, el contrato dice %q", it.Name, key, got, want)
			}
		}
		for key, want := range it.Request.Headers {
			if got := r.Header.Get(key); got != want {
				t.Errorf("%s: cabecera %s=%q, el contrato dice %q", it.Name, key, got, want)
			}
		}
		if len(it.Request.Body) > 0 {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("%s: leer la solicitud: %v", it.Name, err)
			}
			for _, p := range MatchRequest(it.Request, body) {
				t.Errorf("%s: solicitud fuera del contrato: %s", it.Name, p)
			}
		}
		if len(it.Response.Body) > 0 {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(it.Response.Status)
		_, _ = w.Write(it.Response.Body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// ForPath devuelve las interacciones de una ruta: cada adaptador prueba solo las suyas.
func (f File) ForPath(path string) []Interaction {
	var out []Interaction
	for _, it := range f.Interactions {
		if it.Request.Path == path {
			out = append(out, it)
		}
	}
	return out
}

// Match compara actual contra el ejemplo del contrato: los mismos campos con el mismo tipo JSON (el actual
// puede tener campos de más) y el mismo valor en las rutas de exact. Un arreglo vacío en el ejemplo exige
// un arreglo vacío; uno con elementos exige que cada elemento tenga la forma del primero.
func Match(expected, actual json.RawMessage, exact []string) []string {
	var want, got any
	if err := json.Unmarshal(expected, &want); err != nil {
		return []string{fmt.Sprintf("ejemplo del contrato inválido: %v", err)}
	}
	if err := json.Unmarshal(actual, &got); err != nil {
		return []string{fmt.Sprintf("cuerpo que no es JSON: %v", err)}
	}
	problems := shape("$", want, got)
	for _, path := range exact {
		w, okW := lookup(want, path)
		g, okG := lookup(got, path)
		if !okW {
			problems = append(problems, fmt.Sprintf("%s: está en exact pero no en el ejemplo", path))
			continue
		}
		if !okG || !reflect.DeepEqual(w, g) {
			problems = append(problems, fmt.Sprintf("%s: se esperaba %v, llegó %v", path, w, g))
		}
	}
	return problems
}

// MatchRequest compara la solicitud del consumidor con la del contrato por forma y, salvo los campos
// dinámicos, por valor: el proveedor recibe exactamente esos valores (cambiar la placa por el documento,
// ambos texto, también rompe el contrato).
func MatchRequest(req Request, actual json.RawMessage) []string {
	problems := Match(req.Body, actual, nil)
	if len(problems) > 0 {
		return problems
	}
	var want, got any
	_ = json.Unmarshal(req.Body, &want) // Match ya validó los dos JSON
	_ = json.Unmarshal(actual, &got)
	skip := make(map[string]bool, len(req.Dynamic))
	for _, path := range req.Dynamic {
		skip[path] = true
	}
	return values("", want, got, skip)
}

func values(path string, want, got any, skip map[string]bool) []string {
	if skip[path] {
		return nil
	}
	switch w := want.(type) {
	case map[string]any:
		g, _ := got.(map[string]any)
		var problems []string
		for key, value := range w {
			child := key
			if path != "" {
				child = path + "." + key
			}
			problems = append(problems, values(child, value, g[key], skip)...)
		}
		return problems
	case []any:
		g, _ := got.([]any)
		var problems []string
		for i := range w {
			if i < len(g) {
				problems = append(problems, values(fmt.Sprintf("%s[%d]", path, i), w[i], g[i], skip)...)
			}
		}
		return problems
	default:
		if !reflect.DeepEqual(want, got) {
			return []string{fmt.Sprintf("%s: se esperaba %v, llegó %v", path, want, got)}
		}
		return nil
	}
}

func shape(path string, want, got any) []string {
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			return []string{fmt.Sprintf("%s: se esperaba un objeto, llegó %s", path, kind(got))}
		}
		var problems []string
		for key, value := range w {
			child, present := g[key]
			if !present {
				problems = append(problems, fmt.Sprintf("%s.%s: falta", path, key))
				continue
			}
			problems = append(problems, shape(path+"."+key, value, child)...)
		}
		return problems
	case []any:
		g, ok := got.([]any)
		if !ok {
			return []string{fmt.Sprintf("%s: se esperaba un arreglo, llegó %s", path, kind(got))}
		}
		if len(w) == 0 {
			if len(g) != 0 {
				return []string{fmt.Sprintf("%s: se esperaba un arreglo vacío, llegaron %d elementos", path, len(g))}
			}
			return nil
		}
		var problems []string
		for i, item := range g {
			problems = append(problems, shape(fmt.Sprintf("%s[%d]", path, i), w[0], item)...)
		}
		return problems
	default:
		if kind(want) != kind(got) {
			return []string{fmt.Sprintf("%s: se esperaba %s, llegó %s", path, kind(want), kind(got))}
		}
		return nil
	}
}

func kind(v any) string {
	switch v.(type) {
	case map[string]any:
		return "objeto"
	case []any:
		return "arreglo"
	case string:
		return "texto"
	case float64:
		return "número"
	case bool:
		return "booleano"
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%T", v)
	}
}

func lookup(v any, path string) (any, bool) {
	for _, key := range strings.Split(path, ".") {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, false
		}
		if v, ok = m[key]; !ok {
			return nil, false
		}
	}
	return v, true
}
