package ops_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// correlation is the request ID the tests send and expect back.
const correlation = "corr-1"

// pathDomain answers with a DomainError.
const pathDomain = "/domain"

// pathUpstream answers with a DomainError that already carries the correlation ID of the service it came from.
const pathUpstream = "/upstream"

// upstreamCorrelation is the correlation ID the error carries before the handler sees it.
const upstreamCorrelation = "upstream-1"

var errDatabaseExploded = errors.New("database exploded")

// errorCase is a request and the status and error code it gets ("" for no body).
type errorCase struct {
	method, path string
	status       int
	code         apperr.Code
}

// errorApp answers each path with one kind of error.
func errorApp() *echo.Echo {
	app := echo.New()
	app.HTTPErrorHandler = ops.HTTPErrorHandler
	app.GET(
		pathDomain,
		func(*echo.Context) error { return apperr.E(apperr.Conflict, apperr.KeyVersionConflict) },
	)
	app.HEAD(
		pathDomain,
		func(*echo.Context) error { return apperr.E(apperr.Conflict, apperr.KeyVersionConflict) },
	)
	app.GET("/plain", func(*echo.Context) error { return errDatabaseExploded })
	app.GET("/large", func(*echo.Context) error { return echo.ErrStatusRequestEntityTooLarge })
	app.GET("/timeout", func(*echo.Context) error { return echo.ErrServiceUnavailable })
	app.GET("/media", func(*echo.Context) error { return echo.ErrUnsupportedMediaType })
	app.GET(pathUpstream, func(*echo.Context) error {
		relayed := apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
		relayed.CorrelationID = upstreamCorrelation

		return relayed
	})
	app.GET("/committed", func(c *echo.Context) error {
		_ = c.NoContent(http.StatusAccepted)

		return apperr.E(apperr.Conflict, "late")
	})

	return app
}

func TestHTTPErrorHandler(t *testing.T) {
	t.Parallel()

	app := errorApp()

	for _, testCase := range []errorCase{
		{http.MethodGet, pathDomain, 409, apperr.Conflict},
		{http.MethodGet, "/plain", 503, apperr.Unavailable},
		{http.MethodGet, "/large", 422, apperr.Validation},
		{http.MethodGet, "/timeout", 504, apperr.Timeout},
		{http.MethodGet, "/media", 503, apperr.Unavailable}, // an Echo status with no DomainError of its own
		{http.MethodGet, "/missing", 404, apperr.NotFound},
		{http.MethodPost, pathDomain, 404, apperr.NotFound},
		{http.MethodHead, pathDomain, 409, ""},
		{http.MethodGet, "/committed", 202, ""},
	} {
		req := httptest.NewRequestWithContext(t.Context(), testCase.method, testCase.path, nil)
		req.Header.Set(echo.HeaderXRequestID, correlation)

		rec := httptest.NewRecorder()
		rec.Header().Set(echo.HeaderXRequestID, correlation)
		app.ServeHTTP(rec, req)

		checkErrorResponse(t, testCase, rec)
	}
}

// checkErrorResponse: the status, and a DomainError with the code and the request's correlation ID unless no body is
// wanted; an internal error's text never leaks.
func checkErrorResponse(t *testing.T, testCase errorCase, rec *httptest.ResponseRecorder) {
	t.Helper()

	if rec.Code != testCase.status {
		t.Errorf(
			"%s %s: status %d want %d",
			testCase.method,
			testCase.path,
			rec.Code,
			testCase.status,
		)

		return
	}

	if testCase.code == "" {
		if rec.Body.Len() != 0 {
			t.Errorf("%s %s: unexpected body %s", testCase.method, testCase.path, rec.Body)
		}

		return
	}

	var domainError apperr.DomainError

	err := json.Unmarshal(
		rec.Body.Bytes(),
		&domainError,
	)
	if err != nil || domainError.Code != testCase.code ||
		domainError.CorrelationID != correlation {
		t.Errorf("%s %s: body %s", testCase.method, testCase.path, rec.Body)
	}

	if testCase.path == "/plain" && domainError.MessageKey == errDatabaseExploded.Error() {
		t.Error("internal error text leaked")
	}
}

// serve sends one GET with the test's request ID to app.
func serve(t *testing.T, app *echo.Echo, path string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	rec.Header().Set(echo.HeaderXRequestID, correlation)
	app.ServeHTTP(rec, req)

	return rec
}

// A server error is logged with its cause and the request ID, which is all an operator has to find it, since the body
// says only UNAVAILABLE; a client error is not logged.
func TestHTTPErrorHandlerLogsServerErrors(t *testing.T) {
	t.Parallel()

	logs := new(bytes.Buffer)
	app := errorApp()
	app.Logger = slog.New(slog.NewJSONHandler(logs, nil))

	serve(t, app, pathDomain)

	if logs.Len() != 0 {
		t.Errorf("a 409 was logged: %s", logs)
	}

	serve(t, app, "/plain")

	entry := map[string]string{} // slog's keys: time, level, msg and the handler's attributes

	err := json.Unmarshal(logs.Bytes(), &entry)
	if err != nil {
		t.Fatalf("log %q: %v", logs, err)
	}

	if entry["level"] != "ERROR" || entry["error"] != errDatabaseExploded.Error() ||
		entry["request_id"] != correlation {
		t.Errorf("log entry %v", entry)
	}
}

// An error that already carries a correlation ID (one relayed from another service) keeps it; only an error without
// one gets the request's (D07).
func TestHTTPErrorHandlerKeepsUpstreamCorrelation(t *testing.T) {
	t.Parallel()

	rec := serve(t, errorApp(), pathUpstream)

	var domainError apperr.DomainError

	err := json.Unmarshal(rec.Body.Bytes(), &domainError)
	if err != nil || rec.Code != http.StatusConflict ||
		domainError.CorrelationID != upstreamCorrelation {
		t.Errorf("status %d body %s", rec.Code, rec.Body)
	}
}
