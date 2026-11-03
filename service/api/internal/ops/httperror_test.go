package ops

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

func TestHTTPErrorHandler(t *testing.T) {
	app := echo.New()
	app.HTTPErrorHandler = HTTPErrorHandler
	app.GET(
		"/domain",
		func(*echo.Context) error { return apperr.E(apperr.Conflict, apperr.KeyVersionConflict) },
	)
	app.HEAD(
		"/domain",
		func(*echo.Context) error { return apperr.E(apperr.Conflict, apperr.KeyVersionConflict) },
	)
	app.GET("/plain", func(*echo.Context) error { return errors.New("database exploded") })
	app.GET("/large", func(*echo.Context) error { return echo.ErrStatusRequestEntityTooLarge })
	app.GET("/timeout", func(*echo.Context) error { return echo.ErrServiceUnavailable })
	app.GET("/committed", func(c *echo.Context) error {
		_ = c.NoContent(http.StatusAccepted)

		return apperr.E(apperr.Conflict, "late")
	})

	for _, tc := range []struct {
		method, path string
		status       int
		code         string
	}{
		{http.MethodGet, "/domain", 409, "CONFLICT"},
		{http.MethodGet, "/plain", 503, "UNAVAILABLE"},
		{http.MethodGet, "/large", 422, "VALIDATION"},
		{http.MethodGet, "/timeout", 504, "TIMEOUT"},
		{http.MethodGet, "/missing", 404, "NOT_FOUND"},
		{http.MethodPost, "/domain", 404, "NOT_FOUND"},
		{http.MethodHead, "/domain", 409, ""},
		{http.MethodGet, "/committed", 202, ""},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		req.Header.Set(echo.HeaderXRequestID, "corr-1")

		rec := httptest.NewRecorder()
		rec.Header().Set(echo.HeaderXRequestID, "corr-1")
		app.ServeHTTP(rec, req)

		if rec.Code != tc.status {
			t.Errorf("%s %s: status %d want %d", tc.method, tc.path, rec.Code, tc.status)

			continue
		}

		if tc.code == "" {
			if rec.Body.Len() != 0 {
				t.Errorf("%s %s: unexpected body %s", tc.method, tc.path, rec.Body)
			}

			continue
		}

		var domainError apperr.DomainError

		err := json.Unmarshal(
			rec.Body.Bytes(),
			&domainError,
		)
		if err != nil || string(domainError.Code) != tc.code ||
			domainError.CorrelationID != "corr-1" {
			t.Errorf("%s %s: body %s", tc.method, tc.path, rec.Body)
		}

		if tc.path == "/plain" && domainError.MessageKey == "database exploded" {
			t.Error("internal error text leaked")
		}
	}
}
