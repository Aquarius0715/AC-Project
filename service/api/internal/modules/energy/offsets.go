package energy

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/audit"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

// QuoteTTL is the offset quote lifetime (D-contract: 15 minutes).
const QuoteTTL = 15 * time.Minute

// MarketConcept is the fixed future-concept label of OffsetQuote.
type MarketConcept struct {
	Stage              string `json:"stage"`
	ProviderLabel      string `json:"providerLabel"`
	VerificationStatus string `json:"verificationStatus"`
	LedgerStatus       string `json:"ledgerStatus"`
}

// Quote is OffsetQuote.
type Quote struct {
	ID                   uuid.UUID     `json:"id"`
	TenantID             uuid.UUID     `json:"tenantId"`
	Version              int           `json:"version"`
	CreatedAt            time.Time     `json:"createdAt"`
	UpdatedAt            time.Time     `json:"updatedAt"`
	CustomerID           uuid.UUID     `json:"customerId"`
	Purpose              string        `json:"purpose"`
	AmountKg             float64       `json:"amountKg"`
	Period               Range         `json:"period"`
	UnitIDs              []uuid.UUID   `json:"unitIds"`
	EstimatedAmountMinor *int64        `json:"estimatedAmountMinor"`
	Currency             *string       `json:"currency"`
	ExpiresAt            time.Time     `json:"expiresAt"`
	Provider             string        `json:"provider"`
	Scheme               string        `json:"scheme"`
	IsDemo               bool          `json:"isDemo"`
	MarketConcept        MarketConcept `json:"marketConcept"`
}

// QuoteInput is offsets.preview input.
type QuoteInput struct {
	CustomerID *uuid.UUID  `json:"customerId,omitempty"`
	Purpose    string      `json:"purpose"`
	AmountKg   float64     `json:"amountKg"`
	Period     Range       `json:"period"`
	UnitIDs    []uuid.UUID `json:"unitIds"`
}

// Validate implements ops.Validator (DD-C13 / DD-A15).
func (in *QuoteInput) Validate() map[string]string {
	fe := map[string]string{}
	in.Purpose = strings.TrimSpace(in.Purpose)
	if n := utf8.RuneCountInString(in.Purpose); n < 1 || n > 1000 {
		fe["purpose"] = "error.length"
	}
	if math.IsNaN(in.AmountKg) || in.AmountKg <= 0 || in.AmountKg > 100000 || math.Abs(in.AmountKg*1000-math.Round(in.AmountKg*1000)) > 1e-6 {
		fe["amountKg"] = "error.range"
	}
	if in.Period.From.IsZero() || !in.Period.From.Before(in.Period.To) || in.Period.To.Sub(in.Period.From) > 366*24*time.Hour {
		fe["period"] = "error.range"
	}
	seen := map[uuid.UUID]bool{}
	for _, u := range in.UnitIDs {
		if u == uuid.Nil || seen[u] {
			fe["unitIds"] = "error.invalid"
		}
		seen[u] = true
	}
	if len(in.UnitIDs) == 0 || len(in.UnitIDs) > 100 {
		fe["unitIds"] = "error.invalid"
	}
	return fe
}

const quoteCols = `q.id, q.tenant_id, q.version, q.created_at, q.customer_id, q.purpose, q.amount_kg::float8, lower(q.period), upper(q.period), q.unit_ids,
	q.estimated_amount_minor, q.currency, q.expires_at, q.provider`

func scanQuote(r pgx.Row) (Quote, error) {
	var x Quote
	err := r.Scan(&x.ID, &x.TenantID, &x.Version, &x.CreatedAt, &x.CustomerID, &x.Purpose, &x.AmountKg, &x.Period.From, &x.Period.To, &x.UnitIDs,
		&x.EstimatedAmountMinor, &x.Currency, &x.ExpiresAt, &x.Provider)
	x.UpdatedAt, x.Scheme, x.IsDemo = x.CreatedAt, "demo", true
	x.MarketConcept = MarketConcept{Stage: "future_concept", ProviderLabel: "unselected", VerificationStatus: "unverified", LedgerStatus: "not_connected"}
	return x, err
}

// customerOf resolves the target customer: clients use their own (a different customerId is NOT_FOUND); HQ must name it.
func customerOf(ctx context.Context, c *ops.Call, given *uuid.UUID) (uuid.UUID, uuid.UUID, error) {
	var id, org uuid.UUID
	var err error
	switch {
	case c.Principal.Role == "client":
		err = c.Tx.QueryRow(ctx, `SELECT id, organization_id FROM energy.ref_customers WHERE organization_id = $1`, c.Principal.OrgID).Scan(&id, &org)
		if err == nil && given != nil && *given != id {
			return id, org, apperr.E(apperr.NotFound, "error.notFound")
		}
	case given == nil:
		return id, org, apperr.Fields(map[string]string{"customerId": "error.required"})
	default:
		err = c.Tx.QueryRow(ctx, `SELECT id, organization_id FROM energy.ref_customers WHERE id = $1`, *given).Scan(&id, &org)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return id, org, apperr.E(apperr.NotFound, "error.notFound")
	}
	return id, org, err
}

// previewQuote saves a quote snapshot that expires in 15 minutes (provider unselected, no price).
//
//	@Summary		offsets.preview (write)
//	@ID				offsets.preview
//	@Description	Authorization: client:self | admin:offset.write
//	@Description	Validation: D01; input constraints in the corresponding DD; new quote save needs no expectedVersion (write)
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A15, DD-C13
//	@Tags			offsets
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		QuoteInput	true	"input"
//	@Success		200				{object}	ops.Envelope{data=Quote}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/offsets/preview [post]
func previewQuote(ctx context.Context, c *ops.Call, in *QuoteInput) (Quote, error) {
	customer, org, err := customerOf(ctx, c, in.CustomerID)
	if err != nil {
		return Quote{}, err
	}
	var inOrg int
	if err := c.Tx.QueryRow(ctx, `SELECT count(*) FROM energy.ref_units WHERE id = ANY($1) AND customer_org_id = $2 AND NOT archived`, in.UnitIDs, org).Scan(&inOrg); err != nil {
		return Quote{}, err
	}
	if inOrg != len(in.UnitIDs) {
		if c.Principal.Role == "client" {
			return Quote{}, apperr.E(apperr.NotFound, "error.notFound")
		}
		return Quote{}, apperr.Fields(map[string]string{"unitIds": "errors.units_outside_customer"})
	}
	var id uuid.UUID
	if err := c.Tx.QueryRow(ctx, `INSERT INTO energy.offset_quotes (tenant_id, customer_id, purpose, amount_kg, period, unit_ids, expires_at, created_at)
		VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, tstzrange($4, $5), $6, $7, $8) RETURNING id`,
		customer, in.Purpose, in.AmountKg, in.Period.From, in.Period.To, in.UnitIDs, c.Now.Add(QuoteTTL), c.Now).Scan(&id); err != nil {
		return Quote{}, err
	}
	return scanQuote(c.Tx.QueryRow(ctx, "SELECT "+quoteCols+" FROM energy.offset_quotes q WHERE q.id = $1", id))
}

// Attempt is OffsetAttempt.
type Attempt struct {
	ID          uuid.UUID  `json:"id"`
	Stage       string     `json:"stage"`
	Status      string     `json:"status"`
	StartedAt   time.Time  `json:"startedAt"`
	CompletedAt *time.Time `json:"completedAt"`
}

// Record is OffsetRecord.
type Record struct {
	ID                 uuid.UUID  `json:"id"`
	TenantID           uuid.UUID  `json:"tenantId"`
	Version            int        `json:"version"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
	QuoteID            uuid.UUID  `json:"quoteId"`
	CustomerID         uuid.UUID  `json:"customerId"`
	AmountKg           float64    `json:"amountKg"`
	Attempts           []Attempt  `json:"attempts"`
	CurrentAttemptID   *uuid.UUID `json:"currentAttemptId"`
	State              string     `json:"state"`
	PreviousState      *string    `json:"previousState"`
	PurchaseRef        *string    `json:"purchaseRef"`
	RetirementRef      *string    `json:"retirementRef"`
	DemoCertificateRef *string    `json:"demoCertificateRef"`
	EventHistory       []Event    `json:"eventHistory"`
	IsDemo             bool       `json:"isDemo"`
	customerOrg        uuid.UUID
}

// Event is AuditView (offset record history).
type Event struct {
	ID              uuid.UUID          `json:"id"`
	TenantID        uuid.UUID          `json:"tenantId"`
	Version         int                `json:"version"`
	CreatedAt       time.Time          `json:"createdAt"`
	UpdatedAt       time.Time          `json:"updatedAt"`
	ActorID         string             `json:"actorId"`
	ActorName       *string            `json:"actorName"`
	ActorRoleAtTime string             `json:"actorRoleAtTime"`
	Action          string             `json:"action"`
	TargetRef       map[string]string  `json:"targetRef"`
	PreviousVersion *int               `json:"previousVersion"`
	NextVersion     *int               `json:"nextVersion"`
	OccurredAt      time.Time          `json:"occurredAt"`
	CorrelationID   string             `json:"correlationId"`
	Result          string             `json:"result"`
	MaskedBefore    map[string]*string `json:"maskedBefore"`
	MaskedAfter     map[string]*string `json:"maskedAfter"`
	Reason          *string            `json:"reason"`
}

const recordCols = `r.id, r.tenant_id, r.version, r.created_at, r.updated_at, r.quote_id, r.customer_id, r.amount_kg::float8, r.state, r.previous_state, r.purchase_ref,
	r.retirement_ref, r.certificate_ref, cu.organization_id`

const recordFrom = `energy.offset_records r JOIN energy.ref_customers cu ON cu.id = r.customer_id`

func scanRecord(row pgx.Row) (Record, error) {
	var x Record
	err := row.Scan(&x.ID, &x.TenantID, &x.Version, &x.CreatedAt, &x.UpdatedAt, &x.QuoteID, &x.CustomerID, &x.AmountKg, &x.State, &x.PreviousState, &x.PurchaseRef,
		&x.RetirementRef, &x.DemoCertificateRef, &x.customerOrg)
	x.IsDemo = true
	return x, err
}

func decorateRecord(ctx context.Context, c *ops.Call, x *Record) error {
	rows, err := c.Tx.Query(ctx, `SELECT id, stage, status, started_at, completed_at FROM energy.offset_attempts WHERE record_id = $1 ORDER BY started_at, id`, x.ID)
	if err != nil {
		return err
	}
	x.Attempts = []Attempt{}
	for rows.Next() {
		var a Attempt
		if err := rows.Scan(&a.ID, &a.Stage, &a.Status, &a.StartedAt, &a.CompletedAt); err != nil {
			rows.Close()
			return err
		}
		x.Attempts = append(x.Attempts, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if x.State != "demo_retired" && len(x.Attempts) > 0 {
		id := x.Attempts[len(x.Attempts)-1].ID
		x.CurrentAttemptID = &id
	}
	hist, err := audit.History(ctx, c, "offset_record", x.ID.String()) // identity's audit log (IR196)
	if err != nil {
		return err
	}
	x.EventHistory = []Event{}
	for _, v := range hist {
		e := Event(v)
		if c.Principal.Role == "client" { // a customer never sees an actor's name (IR305)
			e.ActorName = nil
		}
		x.EventHistory = append(x.EventHistory, e)
	}
	return nil
}

func loadRecord(ctx context.Context, c *ops.Call, id uuid.UUID, lock bool) (Record, error) {
	q := "SELECT " + recordCols + " FROM " + recordFrom + " WHERE r.id = $1"
	if lock {
		q += " FOR UPDATE OF r"
	}
	x, err := scanRecord(c.Tx.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && c.Principal.Role == "client" && x.customerOrg != c.Principal.OrgID) {
		return Record{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return x, err
	}
	return x, decorateRecord(ctx, c, &x)
}

// @Summary		offsets.list (read)
// @ID				offsets.list
// @Description	Authorization: client:self | admin:offset.read; IR90 client only when all unitIds in scope
// @Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
// @Description	Recovery: D04: retry only UNAVAILABLE, at most twice
// @Description	Design: DD-A15, DD-C13 · Query: filters customerId,status,from,to · sort id,createdAt,updatedAt (default createdAt desc;id desc)
// @Tags			offsets
// @Accept			json
// @Produce		json
// @Param			cursor		query		string	false	"page cursor: nextCursor of the previous page (D12)"
// @Param			limit		query		integer	false	"page size 1–100, default 25"
// @Param			sort		query		string	false	"field:direction — fields id,createdAt,updatedAt; default createdAt desc;id desc"
// @Param			customerId	query		string	false	"filter → OffsetQuote.customerId via quoteId"
// @Param			status		query		string	false	"filter → state"
// @Param			from		query		string	false	"filter → [from,to) on createdAt"
// @Param			to			query		string	false	"filter → [from,to) on createdAt"
// @Success		200			{object}	ops.Envelope{data=RecordPage}
// @Failure		401			{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403			{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404			{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409			{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422			{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429			{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503			{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504			{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/offsets [get]
func listRecords(ctx context.Context, c *ops.Call, in *paging.Query) (paging.Page[Record], error) {
	var f struct {
		CustomerID *uuid.UUID `json:"customerId,omitempty"`
		Status     *string    `json:"status,omitempty"` // => state
		From       *time.Time `json:"from,omitempty"`   // [from, to) on createdAt
		To         *time.Time `json:"to,omitempty"`
	}
	if err := decodeStrict(in.Filters, &f); err != nil || (f.Status != nil && !strings.Contains(" demo_requested demo_purchased demo_retired failed ", " "+*f.Status+" ")) {
		return paging.Page[Record]{}, apperr.Fields(map[string]string{"filters": "error.invalid"})
	}
	if f.From != nil && f.To != nil && !f.From.Before(*f.To) {
		return paging.Page[Record]{}, apperr.Fields(map[string]string{"filters.to": "error.range"})
	}
	order, err := paging.OrderBy(in.Sort, map[string]string{"id": "r.id", "createdAt": "r.created_at", "updatedAt": "r.updated_at"}, "r.created_at DESC, r.id DESC")
	if err != nil {
		return paging.Page[Record]{}, err
	}
	w, err := paging.Resolve(*in, f, c.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Record]{}, err
	}
	var args []any
	add := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	conds := []string{"TRUE"}
	if c.Principal.Role == "client" { // IR90 / REV19-030: every quote unit must be the client's
		conds = append(conds, "cu.organization_id = "+add(c.Principal.OrgID),
			"NOT EXISTS (SELECT 1 FROM energy.offset_quotes q, unnest(q.unit_ids) uid LEFT JOIN energy.ref_units u ON u.id = uid WHERE q.id = r.quote_id AND u.customer_org_id IS DISTINCT FROM cu.organization_id)")
	}
	if f.CustomerID != nil {
		conds = append(conds, "r.customer_id = "+add(*f.CustomerID))
	}
	if f.Status != nil {
		conds = append(conds, "r.state = "+add(*f.Status))
	}
	if f.From != nil {
		conds = append(conds, "r.created_at >= "+add(*f.From))
	}
	if f.To != nil {
		conds = append(conds, "r.created_at < "+add(*f.To))
	}
	where := strings.Join(conds, " AND ")
	var total int
	if err := c.Tx.QueryRow(ctx, "SELECT count(*) FROM "+recordFrom+" WHERE "+where, args...).Scan(&total); err != nil {
		return paging.Page[Record]{}, err
	}
	rows, err := c.Tx.Query(ctx, fmt.Sprintf("SELECT %s FROM %s WHERE %s ORDER BY %s LIMIT %d OFFSET %d", recordCols, recordFrom, where, order, w.Limit, w.Offset), args...)
	if err != nil {
		return paging.Page[Record]{}, err
	}
	items := []Record{}
	for rows.Next() {
		x, err := scanRecord(rows)
		if err != nil {
			rows.Close()
			return paging.Page[Record]{}, err
		}
		items = append(items, x)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return paging.Page[Record]{}, err
	}
	for i := range items {
		if err := decorateRecord(ctx, c, &items[i]); err != nil {
			return paging.Page[Record]{}, err
		}
	}
	return paging.Page[Record]{Items: items, NextCursor: w.Next(total), Total: total, SnapshotVersion: w.Snapshot}, nil
}

// SimulateInput is offsets.simulate input (the branch union of service-contracts.ts).
type SimulateInput struct {
	Event         string     `json:"event"`
	QuoteID       *uuid.UUID `json:"quoteId,omitempty"`
	QuoteVersion  *int       `json:"quoteVersion,omitempty"`
	RecordID      *uuid.UUID `json:"recordId,omitempty"`
	AttemptID     *uuid.UUID `json:"attemptId,omitempty"`
	EventID       *uuid.UUID `json:"eventId,omitempty"`
	DemoConfirmed bool       `json:"demoConfirmed"`
}

// Validate implements ops.Validator: exactly the fields of the branch.
func (in *SimulateInput) Validate() map[string]string {
	fe := map[string]string{}
	if !in.DemoConfirmed {
		fe["demoConfirmed"] = "error.required"
	}
	switch in.Event {
	case "request":
		if in.QuoteID == nil || in.QuoteVersion == nil || in.RecordID != nil || in.AttemptID != nil || in.EventID != nil {
			fe["quoteId"] = "error.invalid"
		}
	case "purchase_confirm", "retire", "fail":
		if in.RecordID == nil || in.AttemptID == nil || in.EventID == nil || in.QuoteID != nil || in.QuoteVersion != nil {
			fe["recordId"] = "error.invalid"
		}
	case "retry":
		if in.RecordID == nil || in.AttemptID == nil || in.EventID != nil || in.QuoteID != nil || in.QuoteVersion != nil {
			fe["recordId"] = "error.invalid"
		}
	default:
		fe["event"] = "error.invalid"
	}
	return fe
}

// newAttempt starts an attempt; UUIDv7 IDs keep attempts started at the same instant in creation order.
func newAttempt(ctx context.Context, c *ops.Call, record uuid.UUID, stage string) error {
	_, err := c.Tx.Exec(ctx, `INSERT INTO energy.offset_attempts (id, tenant_id, record_id, stage, status, started_at) VALUES ($4, current_setting('app.tenant_id')::uuid, $1, $2, 'pending', $3)`,
		record, stage, c.Now, uuid.Must(uuid.NewV7()))
	return err
}

func demoRef(prefix string) *string {
	s := prefix + "-" + strings.ToUpper(uuid.NewString()[:8])
	return &s
}

// simulate applies the SR18 offset state table with SR22 authorization.
//
//	@Summary		offsets.simulate (write)
//	@ID				offsets.simulate
//	@Description	Authorization: client:self:event=request-or-retry | admin:offset.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A15, DD-C13 · Input versions: quoteVersion=OffsetQuote.version
//	@Tags			offsets
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer			false	"event=purchase_confirm|retire|fail: required (target offset record, read offsets.list); event=request: omit (target none, read offsets.preview); event=retry: required (target offset record, read offsets.list)"
//	@Param			request				body		SimulateInput	true	"input"
//	@Success		200					{object}	ops.Envelope{data=Record}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/offsets/simulate [post]
func simulate(ctx context.Context, c *ops.Call, in *SimulateInput) (Record, error) {
	client := c.Principal.Role == "client"
	if client && in.Event != "request" && in.Event != "retry" {
		return Record{}, apperr.E(apperr.Forbidden, "error.forbidden")
	}
	conflict := apperr.E(apperr.Conflict, "errors.offset_state")
	if in.Event == "request" {
		q, err := scanQuote(c.Tx.QueryRow(ctx, "SELECT "+quoteCols+" FROM energy.offset_quotes q WHERE q.id = $1 FOR UPDATE", *in.QuoteID))
		if errors.Is(err, pgx.ErrNoRows) {
			return Record{}, apperr.E(apperr.NotFound, "error.notFound")
		}
		if err != nil {
			return Record{}, err
		}
		if client {
			if _, _, err := customerOf(ctx, c, &q.CustomerID); err != nil {
				return Record{}, err
			}
		}
		if q.Version != *in.QuoteVersion {
			return Record{}, apperr.E(apperr.Conflict, "error.versionConflict")
		}
		var used bool
		if err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM energy.offset_records WHERE quote_id = $1)`, q.ID).Scan(&used); err != nil {
			return Record{}, err
		}
		if used || !c.Now.Before(q.ExpiresAt) {
			return Record{}, apperr.E(apperr.Conflict, "errors.quote_unavailable")
		}
		var id uuid.UUID
		if err := c.Tx.QueryRow(ctx, `INSERT INTO energy.offset_records (tenant_id, quote_id, customer_id, amount_kg, state, created_at, updated_at)
			VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, 'demo_requested', $4, $4) RETURNING id`, q.ID, q.CustomerID, q.AmountKg, c.Now).Scan(&id); err != nil {
			return Record{}, err
		}
		if err := newAttempt(ctx, c, id, "purchase"); err != nil {
			return Record{}, err
		}
		v := 1
		c.Audit(ops.AuditEntry{Action: "offsets.request", TargetKind: "offset_record", TargetID: id.String(), NextVersion: &v})
		return loadRecord(ctx, c, id, false)
	}
	x, err := loadRecord(ctx, c, *in.RecordID, true)
	if err != nil {
		return x, err
	}
	if in.EventID != nil { // same eventId: stored result when identical, CONFLICT otherwise (SR18)
		var att uuid.UUID
		var kind string
		err := c.Tx.QueryRow(ctx, `SELECT id, completed_event FROM energy.offset_attempts WHERE completed_event_id = $1`, *in.EventID).Scan(&att, &kind)
		if err == nil {
			if att == *in.AttemptID && kind == in.Event {
				return x, nil
			}
			return x, conflict
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return x, err
		}
	}
	if x.Version != *c.ExpectedVersion {
		return x, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	if x.CurrentAttemptID == nil || *x.CurrentAttemptID != *in.AttemptID {
		return x, conflict
	}
	cur := x.Attempts[len(x.Attempts)-1]
	finish := func(status string) error {
		_, err := c.Tx.Exec(ctx, `UPDATE energy.offset_attempts SET status = $2, completed_at = $3, completed_event_id = $4, completed_event = $5 WHERE id = $1`,
			cur.ID, status, c.Now, in.EventID, in.Event)
		return err
	}
	setState := func(state string, purchase, retirement, cert *string) error {
		_, err := c.Tx.Exec(ctx, `UPDATE energy.offset_records SET previous_state = state, state = $2, purchase_ref = COALESCE($3, purchase_ref),
			retirement_ref = COALESCE($4, retirement_ref), certificate_ref = COALESCE($5, certificate_ref), version = version + 1, updated_at = $6 WHERE id = $1`,
			x.ID, state, purchase, retirement, cert, c.Now)
		return err
	}
	switch {
	case in.Event == "purchase_confirm" && x.State == "demo_requested" && cur.Stage == "purchase" && cur.Status == "pending":
		if err := finish("succeeded"); err != nil {
			return x, err
		}
		var ref *string
		if x.PurchaseRef == nil {
			ref = demoRef("DEMO-PUR")
		}
		if err := setState("demo_purchased", ref, nil, nil); err != nil {
			return x, err
		}
		err = newAttempt(ctx, c, x.ID, "retirement")
	case in.Event == "retire" && x.State == "demo_purchased" && cur.Stage == "retirement" && cur.Status == "pending":
		if err := finish("succeeded"); err != nil {
			return x, err
		}
		err = setState("demo_retired", nil, demoRef("DEMO-RET"), demoRef("DEMO-CERT"))
	case in.Event == "fail" && (x.State == "demo_requested" || x.State == "demo_purchased") && cur.Status == "pending":
		if err := finish("failed"); err != nil {
			return x, err
		}
		err = setState("failed", nil, nil, nil)
	case in.Event == "retry" && x.State == "failed" && cur.Status == "failed":
		next := "demo_requested"
		if cur.Stage == "retirement" {
			next = "demo_purchased"
		}
		if err := setState(next, nil, nil, nil); err != nil {
			return x, err
		}
		err = newAttempt(ctx, c, x.ID, cur.Stage)
	default:
		return x, conflict
	}
	if err != nil {
		return x, err
	}
	next := x.Version + 1
	c.Audit(ops.AuditEntry{Action: "offsets." + in.Event, TargetKind: "offset_record", TargetID: x.ID.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &next})
	return loadRecord(ctx, c, x.ID, false)
}

// RegisterOffsets binds offsets.*.
func RegisterOffsets(r *ops.Registry) {
	ops.Register(r, "offsets.preview", previewQuote)
	ops.Register(r, "offsets.list", listRecords)
	ops.Register(r, "offsets.simulate", simulate)
}
