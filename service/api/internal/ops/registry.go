package ops

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/authz"
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
	Timezone     string                 // the user's preferences timezone ("" when unset), IR189
	// SessionsRevokedAt ends every sign-in before it (members.revokeSessions, IR384): a token whose sign-in time is
	// earlier is refused. Wall clock, as the identity provider's auth_time is; nil when never revoked.
	SessionsRevokedAt *time.Time
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
	// Result is success unless set: pending for a request whose result comes later under the same correlation ID
	// (IR90; a device command, IR604).
	Result string
	// Before / After hold the changed fields only (see Changes); the recorder masks secrets and contacts.
	Before, After map[string]*string
}

// Call carries everything a handler needs for one operation.
type Call struct {
	Tx            pgx.Tx
	Principal     *Principal
	Now           time.Time
	CorrelationID string
	// Bearer is the caller's access token, forwarded on internal queries to other services (IR190).
	Bearer string
	// Queries runs internal queries of other domains for this call (see Query; nil outside requests).
	Queries *Registry
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

	return apperr.E(apperr.NotFound, apperr.KeyNotFound)
}

// Operation is a registered handler bound to its catalog spec.
type Operation struct {
	Spec     Spec
	Expr     authz.Expr
	NewInput func() any
	Result   reflect.Type // the handler's result type, held to the contract's result by TestResultsMatchContract (IR392)
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

// DeniedRecorder writes the audit entry of a write refused for permission or scope, in a transaction of its own after
// the refused one rolled back (IR603).
type DeniedRecorder interface {
	RecordDenied(ctx context.Context, tx pgx.Tx, c *Call, op string, d Denial) error
}

// Denial is a refused write: the target it asked for and the refusal's message key.
type Denial struct {
	TargetKind, TargetID, Reason string
}

// deniedKinds names the audit target kind of an operation's resource, as the entries of its successful writes do; an
// operation's own entry wins over its resource's (members.clearUnavailability names an unavailability, IR612).
var deniedKinds = map[string]string{
	"alerts":                      "alert",
	"automations":                 "automation",
	"commands":                    "command",
	"contracts":                   "contract",
	"devices":                     "device",
	"invoices":                    "invoice",
	"jobs":                        "job",
	"members":                     "membership",
	"payments":                    "payment",
	"policies":                    "policy",
	"restrictions":                "restriction",
	"units":                       "unit",
	"spaces":                      "space",
	"properties":                  "property",
	"customers":                   "customer",
	"certificates":                "certificate",
	"clientUsers":                 "user",
	"partnerUsers":                "user",
	"members.clearUnavailability": "unavailability",
}

// deniedTarget is the target a refused write asked for: the input's id under the kind of the operation's resource,
// else its <kind>Id, else the first other …Id field that holds a UUID, named by that field (propertyId: property).
// An input without one records the resource's kind and "—". No other input value is kept.
func deniedTarget(op string, input any) (string, string) {
	res, _, _ := strings.Cut(op, ".")

	kind := deniedKinds[op]
	if kind == "" {
		kind = deniedKinds[res]
	}

	if kind == "" {
		kind = res
	}

	raw, _ := json.Marshal(input)

	var fields map[string]any

	_ = json.Unmarshal(raw, &fields)

	uuidOf := func(k string) (string, bool) {
		value, ok := fields[k].(string)
		if !ok {
			return "", false
		}

		_, err := uuid.Parse(value)

		return value, err == nil
	}
	if id, ok := uuidOf("id"); ok {
		return kind, id
	}

	rest := []string{}

	for k := range fields {
		if strings.HasSuffix(k, "Id") && k != kind+"Id" {
			rest = append(rest, k)
		}
	}

	slices.Sort(rest)

	for _, k := range append([]string{kind + "Id"}, rest...) {
		if id, ok := uuidOf(k); ok {
			return strings.TrimSuffix(k, "Id"), id
		}
	}

	return kind, "—"
}

// recordDenied writes the audit entry of a write refused for permission (FORBIDDEN) or scope (NOT_FOUND), at most one
// per actor and operation a minute (DenyLimit), so a flood of attempts cannot fill the log (IR603). A failure here is
// logged and never answered: the caller gets the refusal.
func (r *Registry) recordDenied(
	ctx context.Context,
	call *Call,
	operation string,
	in any,
	err error,
) {
	deniedRecorder, ok := r.Rec.(DeniedRecorder)

	domainErr := apperr.From(err)
	if !ok || r.DB == nil || call.Principal == nil ||
		(domainErr.Code != apperr.Forbidden && domainErr.Code != apperr.NotFound) {
		return
	}

	if r.DenyLimit != nil {
		wait, lerr := r.DenyLimit.Allow(
			ctx,
			call.Principal.TenantID.String()+":"+call.Principal.UserID.String()+":"+operation,
		)
		if lerr == nil &&
			wait > 0 {
			return
		}
	}

	kind, targetID := deniedTarget(operation, in)

	rerr := r.DB.Run(ctx, false, call.Principal, func(txn pgx.Tx) error {
		if txn != nil {
			_, err := txn.Exec(
				ctx,
				`SELECT set_config('app.now', $1, true)`,
				call.Now.UTC().Format(time.RFC3339Nano),
			)
			if err != nil {
				return apperr.Wrap(err, "select set_config")
			}
		}

		return deniedRecorder.RecordDenied(
			ctx,
			txn,
			call,
			operation,
			Denial{TargetKind: kind, TargetID: targetID, Reason: domainErr.MessageKey},
		)
	})
	if rerr != nil {
		slog.WarnContext(ctx, "ops: denied write not recorded", "op", operation, "err", rerr)
	}
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
	// DenyLimit bounds the denied writes recorded in the audit log: one per actor and operation a minute (IR603);
	// nil records every one.
	DenyLimit WriteLimiter

	domains map[string]bool // ServeDomains; nil = all

	// AfterCommit runs after a write committed and before the response (IR183 inline mode: a single process serving
	// every domain applies the resulting events synchronously, so callers see their effects as before).
	AfterCommit func(ctx context.Context)
	// BeforeDispatch runs before every operation (inline mode: events from direct writes such as the change-capture
	// triggers of IR186, the worker or device paths are applied before the operation reads their projections).
	BeforeDispatch func(ctx context.Context)
	// WrapTx, when set, wraps the transaction each operation and query runs in. Tests use it to make chosen statements
	// fail and reach the database error paths; nil in production.
	WrapTx func(pgx.Tx) pgx.Tx
	// Limit bounds the writes of one membership (nil: no limit, IR375).
	Limit WriteLimiter

	queries map[string]query // internal queries (IR190)
	// Remote sends queries of domains this process does not serve (nil: such queries are UNAVAILABLE).
	Remote QueryTransport

	// Jobs are clock-driven transitions a module adds to its domain's scheduler (IR54 schedule automations): run
	// once per tick in each tenant's transaction, their events and audits recorded like a write's.
	Jobs map[string][]Job
}

// Job is one scheduler job; it returns how many changes it made.
type Job func(ctx context.Context, c *Call) (int, error)

// AddJob adds a scheduler job of the domain.
func (r *Registry) AddJob(domain string, j Job) {
	if r.Jobs == nil {
		r.Jobs = map[string][]Job{}
	}

	r.Jobs[domain] = append(r.Jobs[domain], j)
}

// NewRegistry creates an empty registry backed by the generated catalog.
func NewRegistry() *Registry {
	return &Registry{
		ops:            map[string]*Operation{},
		specs:          SpecByName(),
		Clock:          func() time.Time { return time.Now().UTC() },
		DB:             nil,
		Rec:            nil,
		Idem:           nil,
		DenyLimit:      nil,
		domains:        nil,
		AfterCommit:    nil,
		BeforeDispatch: nil,
		WrapTx:         nil,
		Limit:          nil,
		queries:        nil,
		Remote:         nil,
		Jobs:           nil,
	}
}

// Register binds a typed handler to a catalog operation. It panics on unknown or duplicate names so the catalog and
// the code cannot drift.
func Register[I any, O any](
	reg *Registry,
	name string,
	handler func(context.Context, *Call, *I) (O, error),
) {
	spec, ok := reg.specs[name]
	if !ok {
		panic("ops: operation not in catalog: " + name)
	}

	if _, dup := reg.ops[name]; dup {
		panic("ops: duplicate handler: " + name)
	}

	expr, err := authz.Parse(spec.Authorization)
	if err != nil {
		panic(err)
	}

	reg.ops[name] = &Operation{
		Spec:     spec,
		Expr:     expr,
		NewInput: func() any { return new(I) },
		Result:   reflect.TypeOf((*O)(nil)).Elem(),
		Handle:   func(ctx context.Context, c *Call, in any) (any, error) { return handler(ctx, c, in.(*I)) },
	}
}

// Operations returns the registered operations sorted by name.
func (r *Registry) Operations() []*Operation {
	out := make([]*Operation, 0, len(r.ops))
	for _, o := range r.ops {
		out = append(out, o)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Spec.Name < out[j].Spec.Name })

	return out
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
func branchMatches(branch string, input map[string]any) bool {
	switch {
	case branch == "all":
		return true
	case strings.HasSuffix(branch, " present"):
		v, ok := input[strings.TrimSuffix(branch, " present")]

		return ok && v != nil
	case strings.HasSuffix(branch, " omitted"):
		v, ok := input[strings.TrimSuffix(branch, " omitted")]

		return !ok || v == nil
	case strings.Contains(branch, "="):
		field, vals, _ := strings.Cut(branch, "=")

		got, _ := input[field].(string)
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
func expectedVersion(spec Spec, body []byte, header string) (*int, error) {
	var input map[string]any

	_ = json.Unmarshal(body, &input)
	rule := ""

	for _, v := range spec.Versions {
		if branchMatches(v.Branch, input) {
			rule = v.Rule

			break
		}
	}

	switch {
	case rule == "required" && header == "":
		return nil, apperr.Fields(map[string]string{"expectedVersion": apperr.KeyRequired})
	case rule != "required" && header != "":
		return nil, apperr.Fields(map[string]string{"expectedVersion": apperr.KeyNotAllowed})
	case header == "":
		return nil, nil
	}

	version, err := strconv.Atoi(header)
	if err != nil ||
		version < 0 { // 0 names an absent row (IR120 item 7: a default rule without a stored setting)
		return nil, apperr.Fields(map[string]string{"expectedVersion": apperr.KeyInvalid})
	}

	return &version, nil
}

// WriteLimiter bounds the writes of one caller (network architecture §6, IR375).
type WriteLimiter interface {
	// Allow counts one write of key and returns the whole seconds to wait when key is over its limit (0: allowed).
	Allow(ctx context.Context, key string) (wait int, err error)
}

// fileBodies are the operations whose input carries BlobInput files (JPEG/PNG up to 5 MiB, sent base64 in JSON, so
// about 6.7 MiB each): attachments.add and jobs.reportProblem one file, reports.signOff a signature and a site photo
// (IR221), certificates.submit one PDF/JPEG/PNG of up to 10 MB, about 13.4 MB in JSON (IR133, IR277). Every other
// operation keeps the 1 MiB body limit.
var fileBodies = map[string]int64{
	"attachments.add":     8 << 20,
	"jobs.reportProblem":  8 << 20,
	"reports.signOff":     16 << 20,
	"certificates.submit": 16 << 20,
}

func bodyLimit(operation string) int64 {
	if n, ok := fileBodies[operation]; ok {
		return n
	}

	return 1 << 20
}

func correlationID(c *echo.Context) string {
	if corr := c.Response().Header().Get(echo.HeaderXRequestID); corr != "" {
		return corr
	}

	return uuid.Must(uuid.NewV7()).String()
}

// readBody reads the request body within the operation's limit.
func readBody(c *echo.Context, operation string) ([]byte, error) {
	limit := bodyLimit(operation)

	body, err := io.ReadAll(io.LimitReader(c.Request().Body, limit+1))
	if err != nil || int64(len(body)) > limit {
		return nil, apperr.E(apperr.Validation, apperr.KeyBodyTooLarge)
	}

	return body, nil
}

// run is the pipeline every route of an operation shares: decode and validate the JSON input, authorize, claim the
// Idempotency-Key, check the expected version, run the handler in one transaction and write the ServiceResult.
func (r *Registry) run(ectx *echo.Context, operation *Operation, body []byte, corr string) error {
	ctx := ectx.Request().Context()
	name := operation.Spec.Name

	if r.BeforeDispatch != nil {
		r.BeforeDispatch(ctx)
	}

	if len(bytes.TrimSpace(body)) == 0 {
		body = []byte("{}")
	}

	input := operation.NewInput()
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()

	err := dec.Decode(input)
	if err != nil {
		return fail(ectx, apperr.Fields(map[string]string{"_": apperr.KeyMalformedInput}), corr)
	}

	if v, ok := input.(Validator); ok {
		if fe := v.Validate(); len(fe) > 0 {
			return fail(ectx, apperr.Fields(fe), corr)
		}
	}

	principal := PrincipalFrom(ctx)

	var ap *authz.Principal
	if principal != nil {
		ap = &principal.Principal
	}

	cands := operation.Expr.Candidates(ap)
	if len(cands) == 0 {
		if principal == nil {
			return fail(ectx, apperr.E(apperr.Unauthenticated, apperr.KeyUnauthenticated), corr)
		}

		denied := apperr.E(apperr.Forbidden, apperr.KeyForbidden)
		if operation.Spec.Mode == Write {
			r.recordDenied(
				ctx,
				&Call{
					Principal:       principal,
					Now:             r.Clock(),
					CorrelationID:   corr,
					Tx:              nil,
					Bearer:          "",
					Queries:         nil,
					ExpectedVersion: nil,
					Candidates:      nil,
					Events:          nil,
					Audits:          nil,
				},
				name,
				input,
				denied,
			)
		}

		return fail(ectx, denied, corr)
	}

	key := ectx.Request().Header.Get("Idempotency-Key")

	write := operation.Spec.Mode == Write
	if write {
		if len(key) < 8 || len(key) > 128 { // platform.idempotency_keys CHECK (8–128)
			return fail(
				ectx,
				apperr.Fields(map[string]string{"Idempotency-Key": apperr.KeyRequired}),
				corr,
			)
		}
		// the writes of one membership per minute, counted across instances and services (IR375). A store that does not
		// answer lets the write through: the edge limits still hold, and a sign-in session is never blocked by the cache.
		if r.Limit != nil && principal != nil {
			wait, err := r.Limit.Allow(
				ctx,
				principal.TenantID.String()+":"+principal.MembershipID.String(),
			)
			if err != nil {
				slog.WarnContext(ctx, "ops: write limit not checked", "err", err)
			} else if wait > 0 {
				ectx.Response().Header().Set("Retry-After", strconv.Itoa(wait))

				return fail(
					ectx,
					apperr.E(apperr.RateLimited, "error.rateLimited").RetryAfter(wait),
					corr,
				)
			}
		}

		if r.Idem != nil {
			sum := sha256.Sum256(append([]byte(name+"\n"), body...))

			stored, err := r.Idem.Begin(ctx, principal, name, key, hex.EncodeToString(sum[:]))
			if err != nil {
				return fail(ectx, err, corr)
			}

			if stored != nil {
				return apperr.Wrap(ectx.JSONBlob(http.StatusOK, stored), "echo.Context.JSONBlob")
			}
		}
	}

	expected, err := expectedVersion(
		operation.Spec,
		body,
		ectx.Request().Header.Get("X-Expected-Version"),
	)
	if err != nil {
		if write && r.Idem != nil {
			r.Idem.Abort(ctx, principal, name, key)
		}

		return fail(ectx, err, corr)
	}

	bearer, _ := strings.CutPrefix(ectx.Request().Header.Get(echo.HeaderAuthorization), "Bearer ")
	call := &Call{
		Principal:       principal,
		Now:             r.Clock(),
		CorrelationID:   corr,
		Candidates:      cands,
		ExpectedVersion: expected,
		Bearer:          bearer,
		Queries:         r, Tx: nil, Events: nil, Audits: nil,
	}

	var (
		out    any
		cursor int64
	)

	err = r.DB.Run(ctx, !write, principal, func(txn pgx.Tx) error {
		call.Tx = r.txFor(txn)
		// business time of this transaction for SQL defaults and platform.app_now() (IR157)
		if txn != nil {
			_, err := txn.Exec(
				ctx,
				`SELECT set_config('app.now', $1, true)`,
				call.Now.UTC().Format(time.RFC3339Nano),
			)
			if err != nil {
				return apperr.Wrap(err, "select set_config")
			}
		}

		var herr error

		out, herr = operation.Handle(ctx, call, input)
		if herr != nil {
			return herr
		}

		if write && r.Rec != nil {
			err := r.Rec.Record(ctx, txn, call, name)
			if err != nil {
				return apperr.Wrap(err, "ops.Recorder.Record")
			}
		}

		if txn != nil &&
			principal != nil { // Meta.eventCursor: the tenant's event position of this snapshot (D07)
			return txn.QueryRow(ctx, `SELECT count(*) FROM platform.outbox`).Scan(&cursor)
		}

		return nil
	})
	if err != nil {
		if write && r.Idem != nil {
			r.Idem.Abort(ctx, principal, name, key)
		}

		if write {
			r.recordDenied(ctx, call, name, input, err)
		}

		return fail(ectx, err, corr)
	}

	resp, err := json.Marshal(
		Result{
			Data: out,
			Meta: map[string]any{
				"operation":     name,
				"correlationId": corr,
				"snapshotAt":    call.Now.UTC(),
				"eventCursor":   cursor,
			},
		},
	)
	if err != nil {
		return fail(ectx, fmt.Errorf("encode: %w", err), corr)
	}

	if write && r.Idem != nil {
		err := r.Idem.Complete(ctx, principal, name, key, resp)
		if err != nil {
			return fail(ectx, err, corr)
		}
	}

	if write && r.AfterCommit != nil {
		r.AfterCommit(ctx)
	}

	return apperr.Wrap(ectx.JSONBlob(http.StatusOK, resp), "echo.Context.JSONBlob")
}

// txFor is the transaction a call runs in: tx itself unless WrapTx is set.
func (r *Registry) txFor(tx pgx.Tx) pgx.Tx {
	if r.WrapTx == nil || tx == nil {
		return tx
	}

	return r.WrapTx(tx)
}
