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
	// every operation of the catalog is documented as POST /v1/ops/<operation> with its write headers
	for _, s := range ops.Catalog {
		p, ok := doc.Paths["/v1/ops/"+s.Name]["post"]
		if !ok {
			t.Errorf("%s has no annotated handler (run make swagger)", s.Name)
			continue
		}
		idem := false
		for _, x := range p.Parameters {
			idem = idem || (x.Name == "Idempotency-Key" && x.In == "header")
		}
		if idem != (s.Mode == ops.Write) {
			t.Errorf("%s: Idempotency-Key documented %v for a %s operation", s.Name, idem, s.Mode)
		}
		for _, code := range []string{"200", "401", "403", "404", "409", "422"} {
			if _, ok := p.Responses[code]; !ok {
				t.Errorf("%s: response %s missing", s.Name, code)
			}
		}
	}
	if len(doc.Paths) != len(ops.Catalog) {
		t.Errorf("documented %d paths for %d operations", len(doc.Paths), len(ops.Catalog))
	}
}
