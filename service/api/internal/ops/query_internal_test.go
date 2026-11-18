package ops

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
)

// TestBusinessNow: the owner of an internal query reads the caller's business time from X-Business-Now (IR157), so
// both sides see the same instant; without a readable header it uses its own clock.
func TestBusinessNow(t *testing.T) {
	t.Parallel()

	own := time.Date(2026, 9, 22, 3, 0, 0, 0, time.UTC)
	reg := NewRegistry()
	reg.Clock = func() time.Time { return own }

	sent := time.Date(2026, 9, 15, 1, 2, 3, 400, time.UTC)
	cases := map[string]time.Time{
		sent.Format(time.RFC3339Nano): sent,
		"":                            own,
		"yesterday":                   own,
	}

	for header, want := range cases {
		req := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodPost,
			"/internal/v1/queries/q",
			http.NoBody,
		)
		if header != "" {
			req.Header.Set(BusinessNowHeader, header)
		}

		got := reg.businessNow(echo.New().NewContext(req, httptest.NewRecorder()))
		if !got.Equal(want) {
			t.Errorf("X-Business-Now %q: %v, want %v", header, got, want)
		}
	}
}
