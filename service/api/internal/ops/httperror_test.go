package ops_test

import (
	"encoding/json"
	"errors"
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
