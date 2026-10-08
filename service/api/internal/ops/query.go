package ops

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// Internal queries (IR190) are reads one domain exposes to the read models of another (API composition). They are
// not catalog operations: the gateway never routes them, they take the caller's principal and business time, and the
// owner applies its own scope rules. A query of a domain served by this process runs in the caller's transaction;
// otherwise it is sent to the owning service.
type query struct {
	domain string
	handle func(ctx context.Context, c *Call, raw json.RawMessage) (any, error)
}

// QueryPath is the internal route of the queries; callers add X-Internal-Token and the user's Authorization.
const QueryPath = "/internal/v1/queries/:query"

// SystemQueryPath is the internal route for calls without a user (scheduler workers, IR195): X-Internal-Token and
// X-Tenant-Id only; the query runs with a tenant-only principal.
const SystemQueryPath = "/internal/v1/system/queries/:query"

// InternalTokenHeader carries INTERNAL_API_TOKEN on service-to-service calls (Authorization keeps the user's token).
const InternalTokenHeader = "X-Internal-Token"

// BusinessNowHeader carries the caller's business time (IR157) so both sides read the same instant.
const BusinessNowHeader = "X-Business-Now"

// RegisterQuery binds an internal query owned by domain.
func RegisterQuery[I any, O any](r *Registry, domain, name string, h func(context.Context, *Call, *I) (O, error)) {
	if r.queries == nil {
		r.queries = map[string]query{}
	}
	r.queries[name] = query{domain: domain, handle: func(ctx context.Context, c *Call, raw json.RawMessage) (any, error) {
		in := new(I)
		if err := json.Unmarshal(raw, in); err != nil {
			return nil, apperr.Fields(map[string]string{"_": "error.malformedInput"})
		}
		return h(ctx, c, in)
	}}
}

// Delegate is the body of a provider method other domains call (IR192): it asks the owner through the call's
// registry — in the caller's transaction when this process serves the owner's domain, otherwise over HTTP — and runs
// local directly for calls without a registry (the scheduler worker). local must also be registered as the query
// name of the owner's domain.
func Delegate[I any, O any](ctx context.Context, c *Call, name string, in I, local func(context.Context, *Call, *I) (O, error)) (O, error) {
	if c.Queries == nil {
		return local(ctx, c, &in)
	}
	return Ask[O](ctx, c.Queries, c, name, in)
}

// Query runs the internal query name for the call (modules use it; the registry comes with the call).
func Query[O any](ctx context.Context, c *Call, name string, in any) (O, error) {
	return Ask[O](ctx, c.Queries, c, name, in)
}

// Ask runs the internal query name for the caller and decodes its result.
func Ask[O any](ctx context.Context, r *Registry, c *Call, name string, in any) (O, error) {
	var out O
	if r == nil {
		return out, fmt.Errorf("ops: query %s outside a request", name)
	}
	q, ok := r.queries[name]
	if !ok {
		return out, fmt.Errorf("ops: unknown query %s", name)
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return out, err
	}
	var res any
	if r.domains == nil || r.domains[q.domain] {
		if res, err = q.handle(ctx, c, raw); err != nil {
			return out, err
		}
		if v, ok := res.(O); ok {
			return v, nil
		}
		b, err := json.Marshal(res)
		if err != nil {
			return out, err
		}
		return out, json.Unmarshal(b, &out)
	}
	if r.Remote == nil {
		return out, apperr.E(apperr.Unavailable, "error.unavailable")
	}
	body, err := r.Remote.Query(ctx, c, q.domain, name, raw)
	if err != nil {
		return out, err
	}
	return out, json.Unmarshal(body, &out)
}

// QueryTransport sends a query to the service of another domain.
type QueryTransport interface {
	Query(ctx context.Context, c *Call, domain, name string, in json.RawMessage) (json.RawMessage, error)
}

// HTTPQueries is the QueryTransport between the domain services: POST <domain URL>/internal/v1/queries/<name>.
type HTTPQueries struct {
	Targets map[string]*url.URL // by domain (<DOMAIN>_API_URL)
	Token   string              // INTERNAL_API_TOKEN
	Client  *http.Client
}

// Query implements QueryTransport.
func (h *HTTPQueries) Query(ctx context.Context, c *Call, domain, name string, in json.RawMessage) (json.RawMessage, error) {
	base := h.Targets[domain]
	if base == nil || c.Principal == nil {
		return nil, apperr.E(apperr.Unavailable, "error.unavailable")
	}
	path := "/internal/v1/queries/" // the caller's user and membership
	if c.Bearer == "" {
		path = "/internal/v1/system/queries/" // no user: a worker of the tenant (IR195)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(base.String(), "/")+path+url.PathEscape(name), bytes.NewReader(in))
	if err != nil {
		return nil, err
	}
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set("X-Tenant-Id", c.Principal.TenantID.String()) // the same tenant (and membership) context as the caller
	if c.Bearer != "" {
		req.Header.Set(echo.HeaderAuthorization, "Bearer "+c.Bearer)
		req.Header.Set("X-Membership-Id", c.Principal.MembershipID.String())
	}
	req.Header.Set(InternalTokenHeader, h.Token)
	req.Header.Set(echo.HeaderXRequestID, c.CorrelationID)
	req.Header.Set(BusinessNowHeader, c.Now.UTC().Format(time.RFC3339Nano))
	client := h.Client
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, apperr.E(apperr.Unavailable, "error.unavailable")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, apperr.E(apperr.Unavailable, "error.unavailable")
	}
	if resp.StatusCode != http.StatusOK {
		de := &apperr.DomainError{}
		if json.Unmarshal(body, de) == nil && de.Code != "" {
			return nil, de
		}
		return nil, apperr.E(apperr.Unavailable, "error.unavailable")
	}
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, err
	}
	return env.Data, nil
}

// ServeQuery is the handler of QueryPath (behind the authentication middleware): it checks the internal token and
// runs a query of a served domain in a read transaction with the caller's principal and business time.
func (r *Registry) ServeQuery(token string) echo.HandlerFunc {
	return func(c *echo.Context) error {
		bearer, _ := strings.CutPrefix(c.Request().Header.Get(echo.HeaderAuthorization), "Bearer ")
		return r.serveQuery(c, token, PrincipalFrom(c.Request().Context()), bearer)
	}
}

// ServeSystemQuery is the handler of SystemQueryPath (no user authentication): the internal token admits the call
// and the query runs with a principal of the X-Tenant-Id tenant only (IR195).
func (r *Registry) ServeSystemQuery(token string) echo.HandlerFunc {
	return func(c *echo.Context) error {
		tenant, err := uuid.Parse(c.Request().Header.Get("X-Tenant-Id"))
		if err != nil {
			return apperr.E(apperr.Unauthenticated, "error.unauthenticated")
		}
		return r.serveQuery(c, token, &Principal{TenantID: tenant}, "")
	}
}

func (r *Registry) serveQuery(c *echo.Context, token string, p *Principal, bearer string) error {
	if token == "" || subtle.ConstantTimeCompare([]byte(c.Request().Header.Get(InternalTokenHeader)), []byte(token)) != 1 {
		return apperr.E(apperr.Unauthenticated, "error.unauthenticated")
	}
	name := c.Param("query")
	q, ok := r.queries[name]
	if !ok || !(r.domains == nil || r.domains[q.domain]) {
		return apperr.E(apperr.NotFound, "error.unknownOperation")
	}
	ctx := c.Request().Context()
	raw, err := io.ReadAll(io.LimitReader(c.Request().Body, 1<<20))
	if err != nil {
		return apperr.E(apperr.Validation, "error.bodyTooLarge")
	}
	now := r.Clock()
	if t, err := time.Parse(time.RFC3339Nano, c.Request().Header.Get(BusinessNowHeader)); err == nil {
		now = t
	}
	call := &Call{Principal: p, Now: now, CorrelationID: c.Response().Header().Get(echo.HeaderXRequestID), Bearer: bearer, Queries: r}
	if r.BeforeDispatch != nil {
		r.BeforeDispatch(ctx)
	}
	var out any
	err = r.DB.Run(ctx, true, p, func(tx pgx.Tx) error {
		call.Tx = tx
		if _, err := tx.Exec(ctx, `SELECT set_config('app.now', $1, true)`, now.UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
		var herr error
		out, herr = q.handle(ctx, call, raw)
		return herr
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"data": out})
}
