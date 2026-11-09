package ops

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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

// TenantPrincipal is the principal of work that acts for no one in tenant, such as a scheduler transaction or a demo
// trigger: it carries only the tenant that row-level security needs.
func TenantPrincipal(tenant uuid.UUID) *Principal {
	return &Principal{
		Principal:         authz.Principal{Role: "", Permissions: nil, ClientRole: ""},
		TenantID:          tenant,
		MembershipID:      uuid.UUID{},
		UserID:            uuid.UUID{},
		OrgID:             uuid.UUID{},
		ScopeVersion:      0,
		Scopes:            nil,
		Employment:        "",
		Timezone:          "",
		SessionsRevokedAt: nil,
	}
}

// SystemPrincipal is the system acting in tenant as user, the actor its audit entries name: a scheduler job, or the
// demo's gateway facts.
func SystemPrincipal(tenant, user uuid.UUID) *Principal {
	principal := TenantPrincipal(tenant)
	principal.Role = authz.RoleSystem
	principal.UserID = user

	return principal
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
	// OwnTx is an operation whose handler opens its own transactions (RegisterOwnTx): the pipeline opens none around it.
	OwnTx bool
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
func deniedKinds() map[string]string {
	return map[string]string{
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
}

// deniedKind is the audit target kind of the operation, else of its resource (deniedKinds), else the resource's name.
func deniedKind(op string) string {
	res, _, _ := strings.Cut(op, ".")
	kinds := deniedKinds()

	if kind := kinds[op]; kind != "" {
		return kind
	}

	if kind := kinds[res]; kind != "" {
		return kind
	}

	return res
}

// noTargetID is the target ID of a refused write whose input names no target.
const noTargetID = "—"

// deniedTarget is the target a refused write asked for: the input's id under the kind of the operation's resource,
// else its <kind>Id, else the first other …Id field that holds a UUID, named by that field (propertyId: property).
// An input without one records the resource's kind and "—". No other input value is kept.
func deniedTarget(op string, input any) (string, string) {
	kind := deniedKind(op)

	raw, err := json.Marshal(input)
	if err != nil {
		return kind, noTargetID
	}

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

	return kind, noTargetID
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

// Job is one scheduler job; it returns how many changes it made.
type Job func(ctx context.Context, c *Call) (int, error)

// AddJob adds a scheduler job of the domain.
func (r *Registry) AddJob(domain string, j Job) {
	if r.Jobs == nil {
		r.Jobs = map[string][]Job{}
	}

	r.Jobs[domain] = append(r.Jobs[domain], j)
}

var errInputType = errors.New("ops: input of another type")

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
		Result:   reflect.TypeFor[O](),
		Handle: func(ctx context.Context, call *Call, in any) (any, error) {
			input, ok := in.(*I)
			if !ok { // NewInput made it: never another type
				return nil, fmt.Errorf("%w: %s", errInputType, name)
			}

			return handler(ctx, call, input)
		},
		OwnTx: false,
	}
}

// RegisterOwnTx registers an operation whose handler opens its own transactions, because it does not know its tenant
// until it has looked (demo.trigger). The pipeline opens none around it: a handler that asked the pool for a connection
// while its operation's transaction held one would wait for itself once as many calls ran as the pool has connections.
// call.Tx is nil in it, and what it records goes in its own transactions.
func RegisterOwnTx[I any, O any](
	reg *Registry,
	name string,
	handler func(context.Context, *Call, *I) (O, error),
) {
	Register(reg, name, handler)

	reg.ops[name].OwnTx = true
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

// The write-version-catalog values the pipeline reads (VersionRule), and the field its refusals name.
const (
	branchAll            = "all"
	ruleRequired         = "required"
	fieldExpectedVersion = "expectedVersion"
)

// branchMatches evaluates a write-version-catalog branch against the decoded input.
func branchMatches(branch string, input map[string]any) bool {
	switch {
	case branch == branchAll:
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

		return slices.Contains(strings.Split(vals, "|"), got)
	}

	return false
}

// expectedVersion applies the write version catalog: a required version must be a positive integer header,
// an omitted one must be absent (VALIDATION otherwise, write version catalog conflict_behavior). present is false
// when the write carries no version.
func expectedVersion(spec Spec, body []byte, header string) (int, bool, error) {
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
	case rule == ruleRequired && header == "":
		return 0, false, apperr.Fields(map[string]string{fieldExpectedVersion: apperr.KeyRequired})
	case rule != ruleRequired && header != "":
		return 0, false, apperr.Fields(
			map[string]string{fieldExpectedVersion: apperr.KeyNotAllowed},
		)
	case header == "":
		return 0, false, nil
	}

	version, err := strconv.Atoi(header)
	if err != nil ||
		version < 0 { // 0 names an absent row (IR120 item 7: a default rule without a stored setting)
		return 0, false, apperr.Fields(map[string]string{fieldExpectedVersion: apperr.KeyInvalid})
	}

	return version, true, nil
}

// WriteLimiter bounds the writes of one caller (network architecture §6, IR375).
type WriteLimiter interface {
	// Allow counts one write of key and returns the whole seconds to wait when key is over its limit (0: allowed).
	Allow(ctx context.Context, key string) (wait int, err error)
}

// The request body limits. Operations whose input carries BlobInput files (JPEG/PNG up to 5 MiB, sent base64 in
// JSON, so about 6.7 MiB each) take more: attachments.add and jobs.reportProblem one file, reports.signOff a signature
// and a site photo (IR221), certificates.submit one PDF/JPEG/PNG of up to 10 MB, about 13.4 MB in JSON (IR133, IR277).
// Every other operation keeps the 1 MiB body limit.
const (
	defaultBodyLimit = 1 << 20
	oneFileBodyLimit = 8 << 20
	twoFileBodyLimit = 16 << 20
)

func bodyLimit(operation string) int64 {
	switch operation {
	case "attachments.add", "jobs.reportProblem":
		return oneFileBodyLimit
	case "reports.signOff", "certificates.submit":
		return twoFileBodyLimit
	}

	return defaultBodyLimit
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

// recordDenied writes the audit entry of a write refused for permission (FORBIDDEN) or scope (NOT_FOUND), at most one
// per actor and operation a minute (DenyLimit), so a flood of attempts cannot fill the log (IR603). A failure here is
// logged and never answered: the caller gets the refusal.
func (r *Registry) recordDenied(
	ctx context.Context,
	call *Call,
	operation string,
	input any,
	err error,
) {
	deniedRecorder, ok := r.Rec.(DeniedRecorder)

	domainErr := apperr.From(err)
	if !ok || r.DB == nil || call.Principal == nil ||
		(domainErr.Code != apperr.Forbidden && domainErr.Code != apperr.NotFound) {
		return
	}

	if r.deniedRecently(ctx, call.Principal, operation) {
		return
	}

	kind, targetID := deniedTarget(operation, input)

	rerr := r.DB.Run(ctx, false, call.Principal, func(txn pgx.Tx) error {
		if txn != nil {
			err := setAppNow(ctx, txn, call.Now)
			if err != nil {
				return err
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

// deniedRecently says whether DenyLimit already counted a refusal of the operation for this actor within its minute.
func (r *Registry) deniedRecently(
	ctx context.Context,
	principal *Principal,
	operation string,
) bool {
	if r.DenyLimit == nil {
		return false
	}

	wait, err := r.DenyLimit.Allow(
		ctx,
		principal.TenantID.String()+":"+principal.UserID.String()+":"+operation,
	)

	return err == nil && wait > 0
}

// setAppNow sets the business time of the transaction for SQL defaults and platform.app_now() (IR157).
func setAppNow(ctx context.Context, txn pgx.Tx, now time.Time) error {
	_, err := txn.Exec(
		ctx,
		`SELECT set_config('app.now', $1, true)`,
		now.UTC().Format(time.RFC3339Nano),
	)

	return apperr.Wrap(err, "select set_config")
}

// metaCorrelationID is the ServiceResult meta field of the correlation ID.
const metaCorrelationID = "correlationId"

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

	input, err := decodeInput(operation, body)
	if err != nil {
		return fail(ectx, err, corr)
	}

	principal := PrincipalFrom(ctx)

	cands, err := r.authorize(ctx, operation, principal, input, corr)
	if err != nil {
		return fail(ectx, err, corr)
	}

	key := ectx.Request().Header.Get("Idempotency-Key")
	write := operation.Spec.Mode == Write

	stored, err := r.claim(ectx, write, principal, name, key, body)
	if err != nil {
		return fail(ectx, err, corr)
	}

	if stored != nil {
		return apperr.Wrap(ectx.JSONBlob(http.StatusOK, stored), "echo.Context.JSONBlob")
	}

	call, err := r.newCall(ectx, operation.Spec, principal, cands, body, corr)
	if err != nil {
		r.abort(ctx, write, principal, name, key)

		return fail(ectx, err, corr)
	}

	out, cursor, err := r.execute(ctx, operation, call, input, write)
	if err != nil {
		r.abort(ctx, write, principal, name, key)

		if write {
			r.recordDenied(ctx, call, name, input, err)
		}

		return fail(ectx, err, corr)
	}

	return r.respond(ectx, call, write, name, key, out, cursor)
}

// decodeInput decodes the JSON input strictly (an unknown field is malformed input) and applies the input's own
// field rules.
func decodeInput(operation *Operation, body []byte) (any, error) {
	input := operation.NewInput()
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()

	err := dec.Decode(input)
	if err != nil {
		return nil, apperr.Fields(map[string]string{"_": apperr.KeyMalformedInput})
	}

	if validator, ok := input.(Validator); ok {
		if fieldErrs := validator.Validate(); len(fieldErrs) > 0 {
			return nil, apperr.Fields(fieldErrs)
		}
	}

	return input, nil
}

// authorize returns the caller's alternatives of the operation's predicates: none is UNAUTHENTICATED for an anonymous
// caller, else FORBIDDEN, and a refused write is recorded (IR603).
func (r *Registry) authorize(
	ctx context.Context,
	operation *Operation,
	principal *Principal,
	input any,
	corr string,
) ([]authz.Alternative, error) {
	var ap *authz.Principal
	if principal != nil {
		ap = &principal.Principal
	}

	cands := operation.Expr.Candidates(ap)
	if len(cands) > 0 {
		return cands, nil
	}

	if principal == nil {
		return nil, apperr.E(apperr.Unauthenticated, apperr.KeyUnauthenticated)
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
			operation.Spec.Name,
			input,
			denied,
		)
	}

	return nil, denied
}

// claim admits a write: it needs an Idempotency-Key, stays within its membership's write limit (IR375) and gets the
// stored response when the same write completed before (D04). Reads pass.
func (r *Registry) claim(
	ectx *echo.Context,
	write bool,
	principal *Principal,
	name, key string,
	body []byte,
) (json.RawMessage, error) {
	if !write {
		return nil, nil
	}

	if len(key) < 8 || len(key) > 128 { // platform.idempotency_keys CHECK (8–128)
		return nil, apperr.Fields(map[string]string{"Idempotency-Key": apperr.KeyRequired})
	}

	ctx := ectx.Request().Context()
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

			return nil, apperr.E(apperr.RateLimited, "error.rateLimited").RetryAfter(wait)
		}
	}

	if r.Idem == nil {
		return nil, nil
	}

	sum := sha256.Sum256(append([]byte(name+"\n"), body...))

	stored, err := r.Idem.Begin(ctx, principal, name, key, hex.EncodeToString(sum[:]))

	return stored, apperr.Wrap(err, "ops.Idempotency.Begin")
}

// abort releases the Idempotency-Key of a write that did not commit, so the same intent can be sent again (D04).
func (r *Registry) abort(ctx context.Context, write bool, principal *Principal, name, key string) {
	if write && r.Idem != nil {
		r.Idem.Abort(ctx, principal, name, key)
	}
}

// newCall is the call of an admitted request: the caller, its token, the business time, the predicate alternatives
// and the expected version the write version catalog asks for.
func (r *Registry) newCall(
	ectx *echo.Context,
	spec Spec,
	principal *Principal,
	cands []authz.Alternative,
	body []byte,
	corr string,
) (*Call, error) {
	version, present, err := expectedVersion(
		spec,
		body,
		ectx.Request().Header.Get("X-Expected-Version"),
	)
	if err != nil {
		return nil, err
	}

	bearer, _ := strings.CutPrefix(ectx.Request().Header.Get(echo.HeaderAuthorization), "Bearer ")
	call := &Call{
		Principal:       principal,
		Now:             r.Clock(),
		CorrelationID:   corr,
		Candidates:      cands,
		ExpectedVersion: nil,
		Bearer:          bearer,
		Queries:         r, Tx: nil, Events: nil, Audits: nil,
	}

	if present {
		call.ExpectedVersion = &version
	}

	return call, nil
}

// execute runs the handler in one transaction at the call's business time, records a write's audits and events, and
// returns the result with the tenant's event position (Meta.eventCursor, D07).
func (r *Registry) execute(
	ctx context.Context,
	operation *Operation,
	call *Call,
	input any,
	write bool,
) (any, int64, error) {
	if operation.OwnTx {
		out, err := operation.Handle(ctx, call, input)

		return out, 0, apperr.Wrap(err, "ops.Operation.Handle")
	}

	var (
		out    any
		cursor int64
	)

	err := r.DB.Run(ctx, !write, call.Principal, func(txn pgx.Tx) error {
		// the database runs this callback again after a deadlock or a serialization failure: what an attempt that was
		// rolled back emitted and audited is not recorded with the one that commits (IR663)
		call.Events, call.Audits = nil, nil
		call.Tx = r.txFor(txn)
		// business time of this transaction for SQL defaults and platform.app_now() (IR157)
		if txn != nil {
			err := setAppNow(ctx, txn, call.Now)
			if err != nil {
				return err
			}
		}

		var herr error

		out, herr = operation.Handle(ctx, call, input)
		if herr != nil {
			return herr
		}

		if write && r.Rec != nil {
			err := r.Rec.Record(ctx, txn, call, operation.Spec.Name)
			if err != nil {
				return apperr.Wrap(err, "ops.Recorder.Record")
			}
		}

		if txn != nil &&
			call.Principal != nil { // Meta.eventCursor: the tenant's event position of this snapshot (D07)
			return txn.QueryRow(ctx, `SELECT count(*) FROM platform.outbox`).Scan(&cursor)
		}

		return nil
	})

	return out, cursor, apperr.Wrap(err, "ops.Runner.Run")
}

// respond writes the ServiceResult; a write stores it under its Idempotency-Key and then lets its committed events
// apply (inline mode).
func (r *Registry) respond(
	ectx *echo.Context,
	call *Call,
	write bool,
	name, key string,
	out any,
	cursor int64,
) error {
	ctx := ectx.Request().Context()
	corr := call.CorrelationID

	resp, err := json.Marshal(
		Result{
			Data: out,
			Meta: map[string]any{
				"operation":       name,
				metaCorrelationID: corr,
				"snapshotAt":      call.Now.UTC(),
				"eventCursor":     cursor,
			},
		},
	)
	if err != nil {
		return fail(ectx, fmt.Errorf("encode: %w", err), corr)
	}

	if write && r.Idem != nil {
		err := r.Idem.Complete(ctx, call.Principal, name, key, resp)
		if err != nil {
			return fail(ectx, err, corr)
		}
	}

	if write && r.AfterCommit != nil {
		r.AfterCommit(ctx)
	}

	return apperr.Wrap(ectx.JSONBlob(http.StatusOK, resp), "echo.Context.JSONBlob")
}

// txFor is the transaction a call runs in: txn itself unless WrapTx is set.
func (r *Registry) txFor( //nolint:ireturn // pgx offers a transaction only as the pgx.Tx interface
	txn pgx.Tx,
) pgx.Tx {
	if r.WrapTx == nil || txn == nil {
		return txn
	}

	return r.WrapTx(txn)
}
