package ops_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
)

// TestInternalCallerMiddleware covers the gate of IR768: only the exact service token marks a request as an internal
// caller; a wrong or missing token, and a service with no token configured, mark nothing.
func TestInternalCallerMiddleware(t *testing.T) {
	t.Parallel()

	const token = "service-token"

	cases := []struct {
		name   string
		config string
		header string
		want   bool
	}{
		{"matching token", token, token, true},
		{"wrong token", token, "other", false},
		{"missing header", token, "", false},
		{"prefix of the token", token, "service", false},
		{"no token configured, none sent", "", "", false},
		{"no token configured, one sent", "", token, false},
	}

	for _, tcase := range cases {
		t.Run(tcase.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
			if tcase.header != "" {
				req.Header.Set(ops.InternalTokenHeader, tcase.header)
			}

			ectx := echo.New().NewContext(req, httptest.NewRecorder())

			var got bool

			handler := ops.InternalCallerMiddleware(tcase.config)(func(c *echo.Context) error {
				got = ops.InternalCaller(c.Request().Context())

				return nil
			})

			err := handler(ectx)
			if err != nil {
				t.Fatalf("handler: %v", err)
			}

			if got != tcase.want {
				t.Fatalf("InternalCaller = %v, want %v", got, tcase.want)
			}
		})
	}
}
