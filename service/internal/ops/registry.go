package ops

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"

	"github.com/pradita/ac-project/service/internal/platform/apperr"
	"github.com/pradita/ac-project/service/internal/platform/authz"
)

// Principal is the authenticated caller with tenant and membership (nil for anonymous public calls).
type Principal struct {
	authz.Principal
	TenantID     uuid.UUID
	MembershipID uuid.UUID
	UserID       uuid.UUID
	OrgID        uuid.UUID
	ScopeVersion int
	Scopes       map[string][]uuid.UUID // membership_scopes by kind (tenant, organization, property, unit)
	Employment   string                 // internal | external (technicians)
}

// Event is an outbox row recorded by a handler (backend architecture §8).
type Event struct {
	AggregateType string
	AggregateID   uuid.UUID
	Type          string
	Payload       any
}

// AuditEntry is one audit_log row recorded by a handler.
type AuditEntry struct {
	Action          string
	TargetKind      string
	TargetID        string
	PreviousVersion *int
	NextVersion     *int
	Reason          string
}

// Call carries everything a handler needs for one operation.
type Call struct {
	Tx            pgx.Tx
	Principal     *Principal
	Now           time.Time
	CorrelationID string
	// ExpectedVersion is WriteOptions.expectedVersion (X-Expected-Version header), checked against the write
	// version catalog before the handler runs: present when the branch requires it, nil when it must be omitted.
	ExpectedVersion *int
	Candidates      []authz.Alternative
	Events          []Event
	Audits          []AuditEntry
}

// Emit records an outbox event written in the same transaction.
func (c *Call) Emit(e Event) { c.Events = append(c.Events, e) }

// Audit records an audit entry written in the same transaction.
func (c *Call) Audit(a AuditEntry) { c.Audits = append(c.Audits, a) }

// Require checks the catalog predicates for the caller against facts computed from the loaded target.
// Out-of-scope targets are NOT_FOUND (D01), never FORBIDDEN.
func (c *Call) Require(f authz.Facts) error {
	if authz.Allowed(c.Candidates, f) {
		return nil
	}
	return apperr.E(apperr.NotFound, "error.notFound")
}

// Operation is a registered handler bound to its catalog spec.
type Operation struct {
	Spec     Spec
	Expr     authz.Expr
	NewInput func() any
	Handle   func(ctx context.Context, c *Call, in any) (any, error)
}

// Runner runs fn in one transaction with the RLS context of the principal (implemented by platform/db).
type Runner interface {
	Run(ctx context.Context, readOnly bool, p *Principal, fn func(pgx.Tx) error) error
}

// Recorder persists audit and outbox rows inside the transaction before commit.
type Recorder interface {
	Record(ctx context.Context, tx pgx.Tx, c *Call, op string) error
}

// Idempotency stores write results by Idempotency-Key (platform.idempotency_keys).
type Idempotency interface {
	// Begin returns a stored response when the same key and hash completed before, CONFLICT for a reused key
	// with another hash or an in-progress key, or (nil, nil) when the caller may proceed.
	Begin(ctx context.Context, p *Principal, op, key, hash string) (json.RawMessage, error)
	Complete(ctx context.Context, p *Principal, op, key string, resp json.RawMessage) error
	Abort(ctx context.Context, p *Principal, op, key string)
}

// Registry maps operation names to handlers.
type Registry struct {
	ops   map[string]*Operation
	specs map[string]Spec
	DB    Runner
	Rec   Recorder
	Idem  Idempotency
	Clock func() time.Time
}

// NewRegistry creates an empty registry backed by the generated catalog.
func NewRegistry() *Registry {
	return &Registry{ops: map[string]*Operation{}, specs: SpecByName(), Clock: func() time.Time { return time.Now().UTC() }}
}

// Register binds a typed handler to a catalog operation. It panics on unknown or duplicate names so the catalog and
// the code cannot drift.
func Register[I any, O any](r *Registry, name string, h func(context.Context, *Call, *I) (O, error)) {
	spec, ok := r.specs[name]
	if !ok {
		panic("ops: operation not in catalog: " + name)
	}
	if _, dup := r.ops[name]; dup {
		panic("ops: duplicate handler: " + name)
	}
	expr, err := authz.Parse(spec.Authorization)
	if err != nil {
		panic(err)
	}
	r.ops[name] = &Operation{
		Spec: spec, Expr: expr,
		NewInput: func() any { return new(I) },
		Handle:   func(ctx context.Context, c *Call, in any) (any, error) { return h(ctx, c, in.(*I)) },
	}
}

// Registered returns the names with handlers.
func (r *Registry) Registered() map[string]bool {
	m := map[string]bool{}
	for k := range r.ops {
		m[k] = true
	}
	return m
}

// Validator is implemented by inputs with field rules.
type Validator interface{ Validate() map[string]string }

// Result is the ServiceResult envelope.
type Result struct {
	Data any            `json:"data"`
	Meta map[string]any `json:"meta"`
}

type ctxKey struct{}

// WithPrincipal stores the principal (set by the authentication middleware).
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

// PrincipalFrom returns the principal or nil.
func PrincipalFrom(ctx context.Context) *Principal {
	p, _ := ctx.Value(ctxKey{}).(*Principal)
	return p
}

// fail returns err as a DomainError carrying the correlation ID; Echo's HTTPErrorHandler writes it (Echo guide,
// Error Handling: handlers return errors to one central handler).
func fail(_ *echo.Context, err error, corr string) error {
	de := apperr.From(err)
	de.CorrelationID = corr
	return de
}

// branchMatches evaluates a write-version-catalog branch against the decoded input.
func branchMatches(branch string, in map[string]any) bool {
	switch {
	case branch == "all":
		return true
	case strings.HasSuffix(branch, " present"):
		v, ok := in[strings.TrimSuffix(branch, " present")]
		return ok && v != nil
	case strings.HasSuffix(branch, " omitted"):
		v, ok := in[strings.TrimSuffix(branch, " omitted")]
		return !ok || v == nil
	case strings.Contains(branch, "="):
		field, vals, _ := strings.Cut(branch, "=")
		got, _ := in[field].(string)
		for _, v := range strings.Split(vals, "|") {
			if v == got {
				return true
			}
		}
	}
	return false
}

// expectedVersion applies the write version catalog: a required version must be a positive integer header,
// an omitted one must be absent (VALIDATION otherwise, write version catalog conflict_behavior).
func expectedVersion(s Spec, body []byte, header string) (*int, error) {
	var in map[string]any
	_ = json.Unmarshal(body, &in)
	rule := ""
	for _, v := range s.Versions {
		if branchMatches(v.Branch, in) {
			rule = v.Rule
			break
		}
	}
	switch {
	case rule == "required" && header == "":
		return nil, apperr.Fields(map[string]string{"expectedVersion": "error.required"})
	case rule != "required" && header != "":
		return nil, apperr.Fields(map[string]string{"expectedVersion": "error.notAllowed"})
	case header == "":
		return nil, nil
	}
	n, err := strconv.Atoi(header)
	if err != nil || n < 0 { // 0 names an absent row (IR120 item 7: a default rule without a stored setting)
		return nil, apperr.Fields(map[string]string{"expectedVersion": "error.invalid"})
	}
	return &n, nil
}

// Dispatch is the Echo handler for POST /v1/ops/:operation.
func (r *Registry) Dispatch(c *echo.Context) error {
	ctx := c.Request().Context()
	corr := c.Response().Header().Get(echo.HeaderXRequestID)
	if corr == "" {
		corr = uuid.Must(uuid.NewV7()).String()
	}
	name := c.Param("operation")
	op, ok := r.ops[name]
	if !ok {
		return fail(c, apperr.E(apperr.NotFound, "error.unknownOperation"), corr)
	}
	body, err := io.ReadAll(io.LimitReader(c.Request().Body, 1<<20+1))
	if err != nil || len(body) > 1<<20 {
		return fail(c, apperr.E(apperr.Validation, "error.bodyTooLarge"), corr)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		body = []byte("{}")
	}
	in := op.NewInput()
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(in); err != nil {
		return fail(c, apperr.Fields(map[string]string{"_": "error.malformedInput"}), corr)
	}
	if v, ok := in.(Validator); ok {
		if fe := v.Validate(); len(fe) > 0 {
			return fail(c, apperr.Fields(fe), corr)
		}
	}
	p := PrincipalFrom(ctx)
	var ap *authz.Principal
	if p != nil {
		ap = &p.Principal
	}
	cands := op.Expr.Candidates(ap)
	if len(cands) == 0 {
		if p == nil {
			return fail(c, apperr.E(apperr.Unauthenticated, "error.unauthenticated"), corr)
		}
		return fail(c, apperr.E(apperr.Forbidden, "error.forbidden"), corr)
	}
	key := c.Request().Header.Get("Idempotency-Key")
	write := op.Spec.Mode == Write
	if write {
		if len(key) < 8 || len(key) > 128 { // platform.idempotency_keys CHECK (8–128)
			return fail(c, apperr.Fields(map[string]string{"Idempotency-Key": "error.required"}), corr)
		}
		if r.Idem != nil {
			sum := sha256.Sum256(append([]byte(name+"\n"), body...))
			stored, err := r.Idem.Begin(ctx, p, name, key, hex.EncodeToString(sum[:]))
			if err != nil {
				return fail(c, err, corr)
			}
			if stored != nil {
				return c.JSONBlob(http.StatusOK, stored)
			}
		}
	}
	ev, err := expectedVersion(op.Spec, body, c.Request().Header.Get("X-Expected-Version"))
	if err != nil {
		if write && r.Idem != nil {
			r.Idem.Abort(ctx, p, name, key)
		}
		return fail(c, err, corr)
	}
	call := &Call{Principal: p, Now: r.Clock(), CorrelationID: corr, Candidates: cands, ExpectedVersion: ev}
	var out any
	var cursor int64
	err = r.DB.Run(ctx, !write, p, func(tx pgx.Tx) error {
		call.Tx = tx
		// business time of this transaction for SQL defaults and platform.app_now() (IR157)
		if tx != nil {
			if _, err := tx.Exec(ctx, `SELECT set_config('app.now', $1, true)`, call.Now.UTC().Format(time.RFC3339Nano)); err != nil {
				return err
			}
		}
		var herr error
		out, herr = op.Handle(ctx, call, in)
		if herr != nil {
			return herr
		}
		if write && r.Rec != nil {
			if err := r.Rec.Record(ctx, tx, call, name); err != nil {
				return err
			}
		}
		if tx != nil && p != nil { // Meta.eventCursor: the tenant's event position of this snapshot (D07)
			return tx.QueryRow(ctx, `SELECT count(*) FROM platform.outbox`).Scan(&cursor)
		}
		return nil
	})
	if err != nil {
		if write && r.Idem != nil {
			r.Idem.Abort(ctx, p, name, key)
		}
		return fail(c, err, corr)
	}
	resp, err := json.Marshal(Result{Data: out, Meta: map[string]any{"operation": name, "correlationId": corr, "snapshotAt": call.Now.UTC(), "eventCursor": cursor}})
	if err != nil {
		return fail(c, fmt.Errorf("encode: %w", err), corr)
	}
	if write && r.Idem != nil {
		if err := r.Idem.Complete(ctx, p, name, key, resp); err != nil {
			return fail(c, err, corr)
		}
	}
	return c.JSONBlob(http.StatusOK, resp)
}
