package ops

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/pradita/ac-project/service/internal/platform/apperr"
)

func TestHTTPErrorHandler(t *testing.T) {
	e := echo.New()
	e.HTTPErrorHandler = HTTPErrorHandler
	e.GET("/domain", func(*echo.Context) error { return apperr.E(apperr.Conflict, "error.versionConflict") })
	e.HEAD("/domain", func(*echo.Context) error { return apperr.E(apperr.Conflict, "error.versionConflict") })
	e.GET("/plain", func(*echo.Context) error { return errors.New("database exploded") })
	e.GET("/large", func(*echo.Context) error { return echo.ErrStatusRequestEntityTooLarge })
	e.GET("/timeout", func(*echo.Context) error { return echo.ErrServiceUnavailable })
	e.GET("/committed", func(c *echo.Context) error {
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
		w := httptest.NewRecorder()
		w.Header().Set(echo.HeaderXRequestID, "corr-1")
		e.ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Errorf("%s %s: status %d want %d", tc.method, tc.path, w.Code, tc.status)
			continue
		}
		if tc.code == "" {
			if w.Body.Len() != 0 {
				t.Errorf("%s %s: unexpected body %s", tc.method, tc.path, w.Body)
			}
			continue
		}
		var de apperr.DomainError
		if err := json.Unmarshal(w.Body.Bytes(), &de); err != nil || string(de.Code) != tc.code || de.CorrelationID != "corr-1" {
			t.Errorf("%s %s: body %s", tc.method, tc.path, w.Body)
		}
		if tc.path == "/plain" && de.MessageKey == "database exploded" {
			t.Error("internal error text leaked")
		}
	}
}
