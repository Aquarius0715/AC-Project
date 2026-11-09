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

// noDB runs the transaction function without a database: session.get reads only the principal.
type noDB struct{}

func (noDB) Run(_ context.Context, _ bool, _ *ops.Principal, fn func(pgx.Tx) error) error {
	return fn(nil)
}

// TestSessionGet: the session names the principal's role, user, organization, scope version and granted permissions,
// and ends SessionTTL after now.
func TestSessionGet(t *testing.T) {
	t.Parallel()

	reg := ops.NewRegistry()
	reg.DB = noDB{}
	now := time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC)
	reg.Clock = func() time.Time { return now }
	Register(reg)

	app := echo.New()

	app.HTTPErrorHandler = ops.HTTPErrorHandler

	err := reg.MountREST(app.Group("/v1"))
	if err != nil {
		t.Fatal(err)
	}

	principal := &ops.Principal{
		Principal: authz.Principal{
			Role:       authz.RoleClient,
			ClientRole: clientRoleOwner,
			Permissions: map[string]bool{
				"control.execute": true,
				"alert.read":      true,
				"unused":          false,
			},
		},
		TenantID:     uuid.New(),
		MembershipID: uuid.New(),
		UserID:       uuid.New(),
		OrgID:        uuid.New(),
		ScopeVersion: 3, Scopes: nil, Employment: "", Timezone: "", SessionsRevokedAt: nil,
		IssuerHost: "",
	}
	req := httptest.NewRequestWithContext(
		ops.WithPrincipal(t.Context(), principal),
		http.MethodGet,
		"/v1/session",
		nil,
	)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	var res struct {
		Data Session `json:"data"`
	}

	err = json.Unmarshal(rec.Body.Bytes(), &res)
	if err != nil || rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}

	session := res.Data
	if session.Role != authz.RoleClient || session.ScopeVersion != 3 ||
		session.UserID != principal.UserID ||
		strings.Join(session.Permissions, ",") != "alert.read,control.execute" ||
		!session.ExpiresAt.Equal(now.Add(SessionTTL)) ||
		session.Generation != 1 ||
		session.OrganizationID != principal.OrgID {
		t.Fatalf("%+v", session)
	}
}
