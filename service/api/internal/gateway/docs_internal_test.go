package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
)

// swaggerDoc is the part of the API description the tests read.
type swaggerDoc struct {
	Swagger string                                 `json:"swagger"`
	Paths   map[string]map[string]swaggerOperation `json:"paths"`
}

type swaggerOperation struct {
	OperationID string `json:"operationId"`
	Parameters  []struct {
		Name string `json:"name"`
		In   string `json:"in"`
	} `json:"parameters"`
	Responses map[string]json.RawMessage `json:"responses"`
}

// get serves GET path on app.
func get(t *testing.T, app *echo.Echo, path string) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))

	return rec
}

// The gateway serves the API description built from the handler annotations (make swagger) and Swagger UI.
func TestServesTheAPIDescription(t *testing.T) {
	t.Parallel()

	app := New(Targets{}, nil)
	checkDocsPage(t, app)

	rec := get(t, app, "/docs/init.js")
	if rec.Code != http.StatusOK ||
		!strings.HasPrefix(rec.Header().Get("Content-Type"), "text/javascript") ||
		!strings.Contains(rec.Body.String(), `url: "/swagger.json"`) {
		t.Fatalf("/docs/init.js: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}

	rec = get(t, app, "/swagger.json")
	if rec.Code != http.StatusOK ||
		!strings.Contains(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("/swagger.json: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}

	var doc swaggerDoc

	err := json.Unmarshal(rec.Body.Bytes(), &doc)
	if err != nil || doc.Swagger != "2.0" {
		t.Fatalf("swagger document: %v %q", err, doc.Swagger)
	}

	checkRoutesDocumented(t, doc)
}

// checkDocsPage: Swagger UI is one pinned release checked by its hashes, and the page runs no inline script (IR371).
func checkDocsPage(t *testing.T, app *echo.Echo) {
	t.Helper()

	rec := get(t, app, "/docs")
	if rec.Code != http.StatusOK ||
		!strings.Contains(rec.Header().Get("Content-Type"), "text/html") ||
		!strings.Contains(rec.Body.String(), `<script src="/docs/init.js">`) {
		t.Fatalf("/docs: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}

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

	checkDocsCSP(t, csp)
}

// checkDocsCSP: the page runs only the pinned Swagger UI file and its own script, and no one frames it.
func checkDocsCSP(t *testing.T, csp string) {
	t.Helper()

	if !strings.Contains(csp, "script-src 'self' "+swaggerUIBundle+";") ||
		strings.Contains(csp, "unsafe-eval") ||
		!strings.Contains(csp, "frame-ancestors 'none'") {
		t.Errorf("/docs CSP: %q", csp)
	}
}

// checkRoutesDocumented: every REST route of the catalog (IR222) is documented with its path parameters, write
// headers and responses, and the document has no other operation.
func checkRoutesDocumented(t *testing.T, doc swaggerDoc) {
	t.Helper()

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

			checkOperationDocumented(t, spec, route, operation)
		}
	}

	operations := 0

	for path, methods := range doc.Paths {
		if strings.HasPrefix(path, "/v1/ops") {
			t.Errorf("%s is documented: the description shows the REST routes only", path)
		}

		operations += len(methods)
	}

	if documented != catalogRoutes || operations != documented {
		t.Errorf(
			"documented %d of the catalog's routes, %d operations in the document",
			documented,
			operations,
		)
	}
}

// checkOperationDocumented: the route's operation ID, the Idempotency-Key of a write, its path parameters and the
// responses.
func checkOperationDocumented(
	t *testing.T,
	spec ops.Spec,
	route ops.Route,
	operation swaggerOperation,
) {
	t.Helper()

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

	checkWriteHeader(t, spec, operation)

	path := map[string]bool{}

	for _, param := range operation.Parameters {
		if param.In == "path" {
			path[param.Name] = true
		}
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

// checkWriteHeader: the Idempotency-Key header is documented for the writes, and only for them.
func checkWriteHeader(t *testing.T, spec ops.Spec, operation swaggerOperation) {
	t.Helper()

	idem := false
	for _, param := range operation.Parameters {
		idem = idem || (param.Name == "Idempotency-Key" && param.In == "header")
	}

	if idem != (spec.Mode == ops.Write) {
		t.Errorf(
			"%s: Idempotency-Key documented %v for a %s operation",
			spec.Name,
			idem,
			spec.Mode,
		)
	}
}
