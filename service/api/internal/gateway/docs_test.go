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
	app := New(Targets{}, nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/docs", nil))

	if rec.Code != http.StatusOK ||
		!strings.Contains(rec.Header().Get("Content-Type"), "text/html") ||
		!strings.Contains(rec.Body.String(), `<script src="/docs/init.js">`) {
		t.Fatalf("/docs: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	// IR371: the Swagger UI files are one pinned release, checked by their hashes, and the page runs no inline script
	page, csp := rec.Body.String(), rec.Header().Get("Content-Security-Policy")
	for _, want := range []string{`swagger-ui-dist@` + swaggerUIVersion + `/swagger-ui-bundle.js" ` +
		`integrity="sha384-`, `swagger-ui-dist@` + swaggerUIVersion + `/swagger-ui.css" integrity="sha384-`} {
		if !strings.Contains(page, want) {
			t.Errorf("/docs lacks %s", want)
		}
	}

	if strings.Contains(page, "swagger-ui-dist@5/") || strings.Contains(page, "<script>") {
		t.Error("/docs loads a floating version or runs an inline script")
	}

	if !strings.Contains(csp, "script-src 'self' "+swaggerUIBundle+";") ||
		strings.Contains(csp, "unsafe-eval") ||
		!strings.Contains(csp, "frame-ancestors 'none'") {
		t.Errorf("/docs CSP: %q", csp)
	}

	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/docs/init.js", nil))

	if rec.Code != http.StatusOK ||
		!strings.HasPrefix(rec.Header().Get("Content-Type"), "text/javascript") ||
		!strings.Contains(rec.Body.String(), `url: "/swagger.json"`) {
		t.Fatalf("/docs/init.js: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}

	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/swagger.json", nil))

	if rec.Code != http.StatusOK ||
		!strings.Contains(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("/swagger.json: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}

	var doc struct {
		Swagger string `json:"swagger"`
		Paths   map[string]map[string]struct {
			OperationID string                      `json:"operationId"`
			Parameters  []struct{ Name, In string } `json:"parameters"`
			Responses   map[string]json.RawMessage  `json:"responses"`
		} `json:"paths"`
	}

	err := json.Unmarshal(rec.Body.Bytes(), &doc)
	if err != nil || doc.Swagger != "2.0" {
		t.Fatalf("swagger document: %v %q", err, doc.Swagger)
	}
	// every REST route of the catalog (IR222) is documented with its path parameters, write headers and responses
	documented := 0

	for _, spec := range ops.Catalog {
		for _, route := range spec.Routes {
			operation, ok := doc.Paths[route.Path][strings.ToLower(route.Method)]
			if !ok {
				t.Errorf(
					"%s %s (%s) is not documented (run make swagger)",
					route.Method,
					route.Path,
					spec.Name,
				)

				continue
			}

			documented++

			if operation.OperationID != spec.Name &&
				!strings.HasPrefix(operation.OperationID, spec.Name+".") {
				t.Errorf(
					"%s %s: operationId %s for %s",
					route.Method,
					route.Path,
					operation.OperationID,
					spec.Name,
				)
			}

			idem, path := false, map[string]bool{}
			for _, x := range operation.Parameters {
				idem = idem || (x.Name == "Idempotency-Key" && x.In == "header")
				if x.In == "path" {
					path[x.Name] = true
				}
			}

			if idem != (spec.Mode == ops.Write) {
				t.Errorf(
					"%s: Idempotency-Key documented %v for a %s operation",
					spec.Name,
					idem,
					spec.Mode,
				)
			}

			for _, name := range ops.PathParams(route.Path) {
				if !path[name] {
					t.Errorf(
						"%s %s: path parameter %s undocumented",
						route.Method,
						route.Path,
						name,
					)
				}
			}

			for _, code := range []string{"200", "401", "403", "404", "409", "422"} {
				if _, ok := operation.Responses[code]; !ok {
					t.Errorf("%s: response %s missing", spec.Name, code)
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

	if documented != 233 || operations != documented {
		t.Errorf(
			"documented %d of the catalog's routes, %d operations in the document",
			documented,
			operations,
		)
	}
}
