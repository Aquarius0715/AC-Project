package ops

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
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

// The limits of an internal query: how long the caller waits for the owning service, and the size of the answer it
// reads and of the input the owner reads.
const (
	queryTimeout   = 20 * time.Second
	maxQueryResult = 8 << 20
	maxQueryInput  = 1 << 20
)

var (
	errQueryOutsideRequest = errors.New("ops: query outside a request")
	errUnknownQuery        = errors.New("ops: unknown query")
)

// RegisterQuery binds an internal query owned by domain.
func RegisterQuery[I any, O any](
	reg *Registry,
	domain, name string,
	handler func(context.Context, *Call, *I) (O, error),
) {
	if reg.queries == nil {
		reg.queries = map[string]query{}
	}

	reg.queries[name] = query{
		domain: domain,
		handle: func(ctx context.Context, call *Call, raw json.RawMessage) (any, error) {
			input := new(I)

			err := json.Unmarshal(raw, input)
			if err != nil {
				return nil, apperr.Fields(map[string]string{"_": apperr.KeyMalformedInput})
			}
			// an input that validates itself is checked here too, as the operations' inputs are (IR431)
			if v, ok := any(input).(Validator); ok {
				if fe := v.Validate(); len(fe) > 0 {
					return nil, apperr.Fields(fe)
				}
			}

			return handler(ctx, call, input)
		},
	}
}

// Delegate is the body of a provider method other domains call (IR192): it asks the owner through the call's
// registry — in the caller's transaction when this process serves the owner's domain, otherwise over HTTP — and runs
// local directly for calls without a registry (the scheduler worker). local must also be registered as the query
// name of the owner's domain.
func Delegate[I any, O any]( //nolint:ireturn // O is the query's own result type, chosen by the caller
	ctx context.Context,
	call *Call,
	name string,
	input I,
	local func(context.Context, *Call, *I) (O, error),
) (O, error) {
	if call.Queries == nil {
		return local(ctx, call, &input)
	}

	return Ask[O](ctx, call.Queries, call, name, input)
}

// Query runs the internal query name for the call (modules use it; the registry comes with the call).
func Query[O any]( //nolint:ireturn // O is the query's own result type, chosen by the caller
	ctx context.Context,
	call *Call,
	name string,
	in any,
) (O, error) {
	return Ask[O](ctx, call.Queries, call, name, in)
}

// Ask runs the internal query name for the caller and decodes its result.
func Ask[O any]( //nolint:ireturn // O is the query's own result type, chosen by the caller
	ctx context.Context,
	reg *Registry,
	call *Call,
	name string,
	in any,
) (O, error) {
	var out O

	registered, raw, err := findQuery(reg, name, in)
	if err != nil {
		return out, err
	}

	if reg.domains == nil || reg.domains[registered.domain] {
		res, err := registered.handle(ctx, call, raw)
		if err != nil {
			return out, err
		}

		if typed, ok := res.(O); ok {
			return typed, nil
		}

		encoded, err := json.Marshal(res)
		if err != nil {
			return out, apperr.Wrap(err, "json.Marshal")
		}

		return out, apperr.Wrap(json.Unmarshal(encoded, &out), "json.Unmarshal")
	}

	if reg.Remote == nil {
		return out, apperr.E(apperr.Unavailable, apperr.KeyUnavailable)
	}

	body, err := reg.Remote.Query(ctx, call, registered.domain, name, raw)
	if err != nil {
		return out, apperr.Wrap(err, "ops.QueryTransport.Query")
	}

	return out, apperr.Wrap(json.Unmarshal(body, &out), "json.Unmarshal")
}

// findQuery returns the registered query name of reg and its input as JSON.
func findQuery(reg *Registry, name string, input any) (query, json.RawMessage, error) {
	var none query
	if reg == nil {
		return none, nil, fmt.Errorf("%w: %s", errQueryOutsideRequest, name)
	}

	registered, ok := reg.queries[name]
	if !ok {
		return none, nil, fmt.Errorf("%w: %s", errUnknownQuery, name)
	}

	raw, err := json.Marshal(input)
	if err != nil {
		return none, nil, apperr.Wrap(err, "json.Marshal")
	}

	return registered, raw, nil
}

// QueryTransport sends a query to the service of another domain.
type QueryTransport interface {
	Query(
		ctx context.Context,
		c *Call,
		domain, name string,
		in json.RawMessage,
	) (json.RawMessage, error)
}

// HTTPQueries is the QueryTransport between the domain services: POST <domain URL>/internal/v1/queries/<name>.
type HTTPQueries struct {
	Targets map[string]*url.URL // by domain (<DOMAIN>_API_URL)
	Token   string              // INTERNAL_API_TOKEN
	Client  *http.Client
}

// Query implements QueryTransport.
func (h *HTTPQueries) Query(
	ctx context.Context,
	call *Call,
	domain, name string,
	input json.RawMessage,
) (json.RawMessage, error) {
	base := h.Targets[domain]
	if base == nil || call.Principal == nil {
		return nil, apperr.E(apperr.Unavailable, apperr.KeyUnavailable)
	}

	req, err := newQueryRequest(ctx, call, base, name, input, h.Token)
	if err != nil {
		return nil, err
	}

	client := h.Client
	if client == nil {
		client = &http.Client{
			Timeout:       queryTimeout,
			Transport:     nil,
			CheckRedirect: nil,
			Jar:           nil,
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, apperr.E(apperr.Unavailable, apperr.KeyUnavailable)
	}

	defer func() { _ = resp.Body.Close() }()

	return queryResult(resp)
}

// newQueryRequest is the POST of a query to the owning service at base: the caller's tenant, and its membership and
// token unless a worker asks, the internal token, the correlation ID and the business time.
func newQueryRequest(
	ctx context.Context,
	call *Call,
	base *url.URL,
	name string,
	input json.RawMessage,
	token string,
) (*http.Request, error) {
	path := "/internal/v1/queries/" // the caller's user and membership
	if call.Bearer == "" {
		path = "/internal/v1/system/queries/" // no user: a worker of the tenant (IR195)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		strings.TrimSuffix(base.String(), "/")+path+url.PathEscape(name),
		bytes.NewReader(input),
	)
	if err != nil {
		return nil, apperr.Wrap(err, "http.NewRequestWithContext")
	}

	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set(
		"X-Tenant-Id",
		call.Principal.TenantID.String(),
	) // the same tenant (and membership) context as the caller

	if call.Bearer != "" {
		req.Header.Set(echo.HeaderAuthorization, "Bearer "+call.Bearer)
		req.Header.Set("X-Membership-Id", call.Principal.MembershipID.String())
	}

	req.Header.Set(InternalTokenHeader, token)
	req.Header.Set(echo.HeaderXRequestID, call.CorrelationID)
	req.Header.Set(BusinessNowHeader, call.Now.UTC().Format(time.RFC3339Nano))

	return req, nil
}

// queryResult reads the owning service's answer: the data of a success, a DomainError it refused with, anything else
// UNAVAILABLE.
func queryResult(resp *http.Response) (json.RawMessage, error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxQueryResult))
	if err != nil {
		return nil, apperr.E(apperr.Unavailable, apperr.KeyUnavailable)
	}

	if resp.StatusCode != http.StatusOK {
		domainErr := &apperr.DomainError{
			Code:              "",
			MessageKey:        "",
			FieldErrors:       nil,
			CorrelationID:     "",
			RetryAfterSeconds: nil,
		}
		if json.Unmarshal(body, domainErr) == nil && domainErr.Code != "" {
			return nil, domainErr
		}

		return nil, apperr.E(apperr.Unavailable, apperr.KeyUnavailable)
	}

	var env struct {
		Data json.RawMessage `json:"data" swaggertype:"object"`
	}

	unmarshalErr := json.Unmarshal(body, &env)
	if unmarshalErr != nil {
		return nil, apperr.Wrap(unmarshalErr, "json.Unmarshal")
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
// and the query runs with the system principal of the X-Tenant-Id tenant (IR195; the domain schedulers' jobs read the
// tenant's units, IR239).
func (r *Registry) ServeSystemQuery(token string) echo.HandlerFunc {
	return func(ectx *echo.Context) error {
		tenant, err := uuid.Parse(ectx.Request().Header.Get("X-Tenant-Id"))
		if err != nil {
			return apperr.E(apperr.Unauthenticated, apperr.KeyUnauthenticated)
		}

		return r.serveQuery(
			ectx,
			token,
			SystemPrincipal(tenant, uuid.Nil),
			"",
		)
	}
}

func (r *Registry) serveQuery(
	ectx *echo.Context,
	token string,
	principal *Principal,
	bearer string,
) error {
	if !internalTokenValid(ectx, token) {
		return apperr.E(apperr.Unauthenticated, apperr.KeyUnauthenticated)
	}

	name := ectx.Param("query")

	registered, ok := r.queries[name]
	if !ok || (r.domains != nil && !r.domains[registered.domain]) {
		return apperr.E(apperr.NotFound, apperr.KeyUnknownOperation)
	}

	ctx := ectx.Request().Context()

	raw, err := io.ReadAll(io.LimitReader(ectx.Request().Body, maxQueryInput))
	if err != nil {
		return apperr.E(apperr.Validation, apperr.KeyBodyTooLarge)
	}

	now := r.businessNow(ectx)
	call := &Call{
		Principal:     principal,
		Now:           now,
		CorrelationID: ectx.Response().Header().Get(echo.HeaderXRequestID),
		Bearer:        bearer,
		Queries:       r, Tx: nil, ExpectedVersion: nil, Candidates: nil, Events: nil, Audits: nil,
	}

	if r.BeforeDispatch != nil {
		r.BeforeDispatch(ctx)
	}

	var out any

	err = r.DB.Run(ctx, true, principal, func(tx pgx.Tx) error {
		call.Tx = r.txFor(tx)

		err := setAppNow(ctx, tx, now)
		if err != nil {
			return err
		}

		var herr error

		out, herr = registered.handle(ctx, call, raw)

		return herr
	})
	if err != nil {
		return apperr.Wrap(err, "ops.Runner.Run")
	}

	return apperr.Wrap(ectx.JSON(http.StatusOK, map[string]any{"data": out}), "echo.Context.JSON")
}

// internalTokenValid compares X-Internal-Token with the internal token in constant time; without a token nobody is
// admitted.
func internalTokenValid(ectx *echo.Context, token string) bool {
	return token != "" &&
		subtle.ConstantTimeCompare(
			[]byte(ectx.Request().Header.Get(InternalTokenHeader)),
			[]byte(token),
		) == 1
}

// businessNow is the caller's business time (X-Business-Now), else this process's clock.
func (r *Registry) businessNow(ectx *echo.Context) time.Time {
	now := r.Clock()

	sent, err := time.Parse(time.RFC3339Nano, ectx.Request().Header.Get(BusinessNowHeader))
	if err == nil {
		return sent
	}

	return now
}
