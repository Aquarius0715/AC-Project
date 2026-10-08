package identity

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/authz"
)

type noDB struct{}

func (noDB) Run(ctx context.Context, ro bool, p *ops.Principal, fn func(pgx.Tx) error) error {
	return fn(nil)
}

func TestSessionGet(t *testing.T) {
	r := ops.NewRegistry()
	r.DB = noDB{}
	now := time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC)
	r.Clock = func() time.Time { return now }
	Register(r)
	e := echo.New()
	e.HTTPErrorHandler = ops.HTTPErrorHandler
	e.POST("/v1/ops/:operation", r.Dispatch)
	p := &ops.Principal{Principal: authz.Principal{Role: "client", ClientRole: "owner",
		Permissions: map[string]bool{"control.execute": true, "alert.read": true, "unused": false}},
		TenantID: uuid.New(), MembershipID: uuid.New(), UserID: uuid.New(), ScopeVersion: 3}
	req := httptest.NewRequest(http.MethodPost, "/v1/ops/session.get", strings.NewReader(`{}`))
	req = req.WithContext(ops.WithPrincipal(req.Context(), p))
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	var res struct{ Data Session }
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil || w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	s := res.Data
	if s.Role != "client" || s.ScopeVersion != 3 || s.UserID != p.UserID || strings.Join(s.Permissions, ",") != "alert.read,control.execute" ||
		!s.ExpiresAt.Equal(now.Add(SessionTTL)) || s.Generation != 1 {
		t.Fatalf("%+v", s)
	}
}
