package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pradita/ac-project/service/api/internal/ops"
)

// The gateway serves the API description built from the handler annotations (make swagger) and Swagger UI.
func TestServesTheAPIDescription(t *testing.T) {
	e := New(Targets{}, nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/docs", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Header().Get("Content-Type"), "text/html") || !strings.Contains(w.Body.String(), `url: "/swagger.json"`) {
		t.Fatalf("/docs: %d %s", w.Code, w.Header().Get("Content-Type"))
	}
	w = httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/swagger.json", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("/swagger.json: %d %s", w.Code, w.Header().Get("Content-Type"))
	}
	var doc struct {
		Swagger string `json:"swagger"`
		Paths   map[string]map[string]struct {
			OperationID string                      `json:"operationId"`
			Parameters  []struct{ Name, In string } `json:"parameters"`
			Responses   map[string]json.RawMessage  `json:"responses"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil || doc.Swagger != "2.0" {
		t.Fatalf("swagger document: %v %q", err, doc.Swagger)
	}
	// every REST route of the catalog (IR222) is documented with its path parameters, write headers and responses
	documented := 0
	for _, s := range ops.Catalog {
		for _, rt := range s.Routes {
			p, ok := doc.Paths[rt.Path][strings.ToLower(rt.Method)]
			if !ok {
				t.Errorf("%s %s (%s) is not documented (run make swagger)", rt.Method, rt.Path, s.Name)
				continue
			}
			documented++
			if p.OperationID != s.Name && !strings.HasPrefix(p.OperationID, s.Name+".") {
				t.Errorf("%s %s: operationId %s for %s", rt.Method, rt.Path, p.OperationID, s.Name)
			}
			idem, path := false, map[string]bool{}
			for _, x := range p.Parameters {
				idem = idem || (x.Name == "Idempotency-Key" && x.In == "header")
				if x.In == "path" {
					path[x.Name] = true
				}
			}
			if idem != (s.Mode == ops.Write) {
				t.Errorf("%s: Idempotency-Key documented %v for a %s operation", s.Name, idem, s.Mode)
			}
			for _, name := range ops.PathParams(rt.Path) {
				if !path[name] {
					t.Errorf("%s %s: path parameter %s undocumented", rt.Method, rt.Path, name)
				}
			}
			for _, code := range []string{"200", "401", "403", "404", "409", "422"} {
				if _, ok := p.Responses[code]; !ok {
					t.Errorf("%s: response %s missing", s.Name, code)
				}
			}
		}
	}
	operations := 0
	for path, methods := range doc.Paths {
		if strings.HasPrefix(path, "/v1/ops") {
			t.Errorf("%s is documented: the description shows the REST routes only", path)
		}
		operations += len(methods)
	}
	if documented != 221 || operations != documented {
		t.Errorf("documented %d of the catalog's routes, %d operations in the document", documented, operations)
	}
}
