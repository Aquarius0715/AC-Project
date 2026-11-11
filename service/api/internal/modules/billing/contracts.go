package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/notify"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/money"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

// Customers resolves customers and units (Assets).
type Customers interface {
	CustomerState(
		ctx context.Context,
		c *ops.Call,
		customer uuid.UUID,
	) (org uuid.UUID, active bool, found bool, err error)
	UnitState(
		ctx context.Context,
		c *ops.Call,
		unit uuid.UUID,
	) (org uuid.UUID, archived bool, found bool, err error)
	CustomerOfOrg(ctx context.Context, c *ops.Call, org uuid.UUID) (uuid.UUID, bool, error)
}

// Restrictions answers restriction facts (Restrictions).
type Restrictions interface {
	ContractState(ctx context.Context, c *ops.Call, contract uuid.UUID) ([]uuid.UUID, bool, error)
	ForInvoice(ctx context.Context, c *ops.Call, invoice uuid.UUID) ([]uuid.UUID, error)
}

// Recipients checks reminder recipients (Identity).
type Recipients interface {
	ActiveClientOf(ctx context.Context, c *ops.Call, membership, org uuid.UUID) (bool, error)
	Reachable(ctx context.Context, c *ops.Call, membership uuid.UUID, channel string) (bool, error)
}

// Billing is the contract and invoice operation set (IR135).
type Billing struct {
	Customers    Customers
	Restrictions Restrictions
	Recipients   Recipients
	Notify       notify.Store
	Releases     Releases
}

// The caller roles billing tells apart.
const (
	roleAdmin      = "admin"
	roleClient     = "client"
	roleContractor = "contractor"
)

// The kinds of billing's resources: event aggregate types, audit target kinds and notification targets.
const (
	kindContract        = "contract"
	kindInquiry         = "inquiry"
	kindInvoice         = "invoice"
	kindPayment         = "payment"
	kindPayoutStatement = "payout_statement"
)

// The API property names billing uses more than once: field-error keys, sort fields and payload keys.
const (
	fieldCreatedAt = "createdAt"
	fieldCurrency  = "currency"
	fieldDueAt     = "dueAt"
	fieldFilters   = "filters"
	fieldPeriod    = "period"
	fieldStatus    = "status"
	fieldUpdatedAt = "updatedAt"
)

// The currencies of contracts, invoices and payments; a payout statement is in MYR until a rate card says otherwise.
const (
	currencyMYR = "MYR"
	currencyUSD = "USD"
)

// The invoice statuses (billing.invoices.status).
const (
	invoiceUnpaid     = "unpaid"
	invoiceProcessing = "processing"
	invoicePaid       = "paid"
)

// SQL fragments of the reads.
const (
	sqlForUpdate = " FOR UPDATE"
	sqlTrue      = "TRUE"
)

// The text limits of the inputs, in characters.
const (
	maxReasonLength  = 1000
	maxMessageLength = 2000
	maxUnitsPerSave  = 500
	maxRulesVersion  = 64
)

// knownCurrency reports a currency billing accepts.
func knownCurrency(currency string) bool {
	return currency == currencyMYR || currency == currencyUSD
}

// decodeFilters reads the list filters strictly: an unknown filter is an error, and no filters leave dst as it is.
func decodeFilters(raw json.RawMessage, dst any) error {
	if len(raw) == 0 {
		return nil
	}

	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()

	return apperr.Wrap(dec.Decode(dst), "json.Decoder.Decode")
}

// invalidFilters is the VALIDATION error of filters that do not decode or hold an unknown value.
func invalidFilters() error {
	return apperr.Fields(map[string]string{fieldFilters: apperr.KeyInvalid})
}

// pageStmt is the SELECT of one page of a list.
func pageStmt(cols, from, where, order string, window paging.Window) string {
	return fmt.Sprintf(
		"SELECT %s FROM %s WHERE %s ORDER BY %s LIMIT %d OFFSET %d",
		cols,
		from,
		where,
		order,
		window.Limit,
		window.Offset,
	)
}

// placeholders collects query arguments and returns the placeholder of each ($1, $2 …).
type placeholders struct {
	args []any
}

func (p *placeholders) add(value any) string {
	p.args = append(p.args, value)

	return fmt.Sprintf("$%d", len(p.args))
}

// Range is Range.
type Range struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// Contract is Contract of service-contracts.ts.
type Contract struct {
	ID                    uuid.UUID   `json:"id"`
	TenantID              uuid.UUID   `json:"tenantId"`
	Version               int         `json:"version"`
	CreatedAt             time.Time   `json:"createdAt"`
	UpdatedAt             time.Time   `json:"updatedAt"`
	ActiveRestrictionIDs  []uuid.UUID `json:"activeRestrictionIds"`
	HasUnresolvedRecovery bool        `json:"hasUnresolvedRecovery"`
	CustomerID            uuid.UUID   `json:"customerId"`
	CustomerOrgID         uuid.UUID   `json:"customerOrgId"`
	UnitIDs               []uuid.UUID `json:"unitIds"`
	PlanType              string      `json:"planType"`
	StartAt               time.Time   `json:"startAt"`
	EndAt                 time.Time   `json:"endAt"`
	PriceMinor            int64       `json:"priceMinor"`
	Currency              string      `json:"currency"`
	RestrictionEligible   bool        `json:"restrictionEligible"`
	RulesVersion          *string     `json:"rulesVersion"`
}

const contractCols = `k.id, k.tenant_id, k.version, k.created_at, k.customer_id, k.customer_org_id, k.plan_type, ` +
	`lower(k.term), upper(k.term), k.price_minor, k.currency,
	k.restriction_eligible, k.rules_version,
	COALESCE((SELECT array_agg(cu.unit_id ORDER BY cu.unit_id) FROM billing.contract_units cu WHERE cu.contract_id = ` +
	`k.id AND cu.contract_version = k.version), '{}')`

func scanContract(r pgx.Row) (Contract, error) {
	var contract Contract

	err := r.Scan(
		&contract.ID,
		&contract.TenantID,
		&contract.Version,
		&contract.CreatedAt,
		&contract.CustomerID,
		&contract.CustomerOrgID,
		&contract.PlanType,
		&contract.StartAt,
		&contract.EndAt,
		&contract.PriceMinor,
		&contract.Currency,
		&contract.RestrictionEligible,
		&contract.RulesVersion,
		&contract.UnitIDs,
	)
	contract.UpdatedAt = contract.CreatedAt

	return contract, apperr.Wrap(err, "scan (scanContract)")
}

func (m Billing) decorate(ctx context.Context, c *ops.Call, contract *Contract) error {
	var err error

	contract.ActiveRestrictionIDs, contract.HasUnresolvedRecovery, err = m.Restrictions.ContractState(
		ctx,
		c,
		contract.ID,
	)

	return apperr.Wrap(err, "billing.Restrictions.ContractState")
}

// clientScope restricts clients to their own customer organization.
func clientScope(c *ops.Call, col string, args *[]any) string {
	if c.Principal.Role != roleClient {
		return sqlTrue
	}

	*args = append(*args, c.Principal.OrgID)

	return fmt.Sprintf("%s = $%d", col, len(*args))
}

func (m Billing) loadContract(
	ctx context.Context,
	call *ops.Call,
	contractID uuid.UUID,
	lock bool,
) (Contract, error) {
	stmt := "SELECT " + contractCols + " FROM billing.contracts k WHERE k.id = $1 AND k.is_current"

	if lock {
		// every version first (IR766): a write that waited for a new version saved meanwhile reads that version
		// and answers a version conflict, instead of NOT_FOUND for the row that is no longer current
		_, err := call.Tx.Exec(
			ctx,
			`SELECT 1 FROM billing.contracts WHERE id = $1 ORDER BY version FOR UPDATE`,
			contractID,
		)
		if err != nil {
			return Contract{}, apperr.Wrap(err, "select billing.contracts")
		}

		stmt += sqlForUpdate
	}

	contract, err := scanContract(call.Tx.QueryRow(ctx, stmt, contractID))
	if errors.Is(err, pgx.ErrNoRows) {
		return contract, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return contract, err
	}

	err = m.decorate(ctx, call, &contract)

	return contract, err
}

// listContracts serves contracts.list.
//
//	@Summary		contracts.list (read)
//	@ID				contracts.list
//	@Description	Authorization: client:self | admin:contract.read | admin:billing.read | admin:restriction.read
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A07, DD-A08, DD-C10, DD-A09, DD-A19 · Query: filters customerId,unitId,kind · sort \
//	@Description	id,createdAt,updatedAt,startAt (default id asc)
//	@Tags			contracts
//	@Accept			json
//	@Produce		json
//	@Param			cursor		query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit		query		integer	false	"page size 1–100, default 25"
//	@Param			sort		query		string	false	"field:direction — fields id,createdAt,updatedAt,startAt; default id asc"
//	@Param			customerId	query		string	false	"filter → customerId"
//	@Param			unitId		query		string	false	"filter → unitIds contains"
//	@Param			kind		query		string	false	"filter → planType"
//	@Success		200			{object}	ops.Envelope{data=ContractPage}
//	@Failure		401			{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403			{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404			{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409			{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422			{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429			{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503			{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504			{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/contracts [get]
func (m Billing) listContracts(
	ctx context.Context,
	call *ops.Call,
	query *paging.Query,
) (paging.Page[Contract], error) {
	var filters contractFilters

	if decodeFilters(query.Filters, &filters) != nil ||
		(filters.Kind != nil && !slices.Contains(planTypes(), *filters.Kind)) {
		return paging.Page[Contract]{}, invalidFilters()
	}

	order, err := paging.OrderBy(
		query.Sort,
		map[string]string{
			"id":           "k.id",
			fieldCreatedAt: "k.created_at",
			fieldUpdatedAt: "k.created_at",
			"startAt":      "lower(k.term)",
		},
		"k.id ASC",
	)
	if err != nil {
		return paging.Page[Contract]{}, apperr.Wrap(err, "paging.OrderBy")
	}

	window, err := paging.Resolve(*query, filters, call.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Contract]{}, apperr.Wrap(err, "paging.Resolve")
	}

	where, args := filters.conditions(call)

	var total int

	scanErr := call.Tx.QueryRow(ctx, "SELECT count(*) FROM billing.contracts k WHERE "+where, args...).
		Scan(&total)
	if scanErr != nil {
		return paging.Page[Contract]{}, apperr.Wrap(scanErr, "select billing.contracts")
	}

	items, err := m.contractPage(
		ctx,
		call,
		pageStmt(contractCols, "billing.contracts k", where, order, window),
		args,
	)
	if err != nil {
		return paging.Page[Contract]{}, err
	}

	return paging.Page[Contract]{
		Items:           items,
		NextCursor:      window.Next(total),
		Total:           total,
		SnapshotVersion: window.Snapshot,
	}, nil
}

// contractFilters are the filters of contracts.list.
type contractFilters struct {
	CustomerID *uuid.UUID `json:"customerId,omitempty"`
	UnitID     *uuid.UUID `json:"unitId,omitempty"`
	Kind       *string    `json:"kind,omitempty"` // => planType
}

// conditions is the WHERE clause of contracts.list and its arguments.
func (f contractFilters) conditions(call *ops.Call) (string, []any) {
	var params placeholders

	conds := []string{"k.is_current", clientScope(call, "k.customer_org_id", &params.args)}
	if f.CustomerID != nil {
		conds = append(conds, "k.customer_id = "+params.add(*f.CustomerID))
	}

	if f.UnitID != nil {
		conds = append(
			conds,
			"EXISTS (SELECT 1 FROM billing.contract_units cu WHERE cu.contract_id = k.id AND cu.contract_version = k.version "+
				"AND cu.unit_id = "+params.add(
				*f.UnitID,
			)+")",
		)
	}

	if f.Kind != nil {
		conds = append(conds, "k.plan_type = "+params.add(*f.Kind))
	}

	return strings.Join(conds, " AND "), params.args
}

// contractPage reads one page of contracts and decorates each with its restriction state.
func (m Billing) contractPage(
	ctx context.Context,
	call *ops.Call,
	stmt string,
	args []any,
) ([]Contract, error) {
	rows, err := call.Tx.Query(ctx, stmt, args...)
	if err != nil {
		return nil, apperr.Wrap(err, "select billing.contracts")
	}

	items := []Contract{}

	for rows.Next() {
		contract, err := scanContract(rows)
		if err != nil {
			rows.Close()

			return nil, err
		}

		items = append(items, contract)
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return nil, apperr.Wrap(rowsErr, "select billing.contracts")
	}

	for i := range items {
		err := m.decorate(ctx, call, &items[i])
		if err != nil {
			return nil, err
		}
	}

	return items, nil
}

// planTypes are the contract plans (planType).
func planTypes() []string {
	return []string{"rto", "general", "energy", "environment"}
}

// ContractInput is contracts.save input.
type ContractInput struct {
	ID                  *uuid.UUID  `json:"id,omitempty"`
	CustomerID          uuid.UUID   `json:"customerId"`
	UnitIDs             []uuid.UUID `json:"unitIds"`
	PlanType            string      `json:"planType"`
	StartAt             time.Time   `json:"startAt"`
	EndAt               time.Time   `json:"endAt"`
	PriceMinor          int64       `json:"priceMinor"`
	Currency            string      `json:"currency"`
	RestrictionEligible bool        `json:"restrictionEligible"`
	RulesVersion        *string     `json:"rulesVersion"`
}

// Validate implements ops.Validator (DD-A07 / IR135 item 1).
func (in *ContractInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.CustomerID == uuid.Nil {
		fieldErrs["customerId"] = apperr.KeyRequired
	}

	in.checkUnits(fieldErrs)

	if !slices.Contains(planTypes(), in.PlanType) {
		fieldErrs["planType"] = apperr.KeyInvalid
	}

	if in.StartAt.IsZero() || !in.StartAt.Before(in.EndAt) {
		fieldErrs["endAt"] = apperr.KeyRange
	}

	if !money.InRange(in.PriceMinor) {
		fieldErrs["priceMinor"] = apperr.KeyRange
	}

	if !knownCurrency(in.Currency) {
		fieldErrs[fieldCurrency] = apperr.KeyInvalid
	}

	in.checkRestriction(fieldErrs)

	return fieldErrs
}

// checkUnits: 1–500 units, each once (a duplicate outranks the count).
func (in *ContractInput) checkUnits(fieldErrs map[string]string) {
	seen := map[uuid.UUID]bool{}

	if len(in.UnitIDs) < 1 || len(in.UnitIDs) > maxUnitsPerSave {
		fieldErrs["unitIds"] = apperr.KeyCount
	}

	for _, unitID := range in.UnitIDs {
		if seen[unitID] {
			fieldErrs["unitIds"] = apperr.KeyDuplicate
		}

		seen[unitID] = true
	}
}

// checkRestriction: only an RTO contract is restriction-eligible, and an eligible one names its rules version.
func (in *ContractInput) checkRestriction(fieldErrs map[string]string) {
	switch {
	case in.RestrictionEligible && in.PlanType != "rto":
		fieldErrs["restrictionEligible"] = "errors.rto_only"
	case in.RestrictionEligible &&
		(in.RulesVersion == nil || utf8.RuneCountInString(strings.TrimSpace(*in.RulesVersion)) < 1 ||
			utf8.RuneCountInString(*in.RulesVersion) > maxRulesVersion):
		fieldErrs["rulesVersion"] = apperr.KeyRequired
	case !in.RestrictionEligible && in.RulesVersion != nil:
		fieldErrs["rulesVersion"] = apperr.KeyNotAllowed
	}
}

// saveContract serves contracts.save.
//
//	@Summary		contracts.save (write)
//	@ID				contracts.save
//	@Description	Authorization: admin:contract.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A07
//	@Tags			contracts
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		ContractInput	true	"input"
//	@Success		200				{object}	ops.Envelope{data=Contract}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/contracts [post]
func (m Billing) saveContract(
	ctx context.Context,
	call *ops.Call,
	input *ContractInput,
) (Contract, error) {
	org, err := m.contractOrg(ctx, call, input)
	if err != nil {
		return Contract{}, err
	}

	revision, err := m.nextRevision(ctx, call, input)
	if err != nil {
		return Contract{}, err
	}

	insertErr := insertContract(ctx, call, input, org, revision)
	if insertErr != nil {
		return Contract{}, insertErr
	}

	contract, err := m.loadContract(ctx, call, revision.id, false)
	if err != nil {
		return contract, err
	}

	call.Emit(
		ops.Event{
			AggregateType: kindContract,
			AggregateID:   revision.id,
			Type:          "ContractSaved",
			Payload:       map[string]any{"version": revision.version},
		},
	)

	changedBefore, changedAfter := ops.Changes(revision.before, contract.fields())
	call.Audit(
		ops.AuditEntry{
			Action:          "contracts.save",
			TargetKind:      kindContract,
			TargetID:        revision.id.String(),
			PreviousVersion: revision.prev,
			NextVersion:     &contract.Version,
			Before:          changedBefore,
			After:           changedAfter, Reason: "", Result: "",
		},
	)

	return contract, nil
}

// contractOrg is the customer organization of a contracts.save: the customer exists and is active, and each unit is
// one of its units and not archived.
func (m Billing) contractOrg(
	ctx context.Context,
	call *ops.Call,
	input *ContractInput,
) (uuid.UUID, error) {
	org, active, found, err := m.Customers.CustomerState(ctx, call, input.CustomerID)
	if err != nil {
		return uuid.Nil, apperr.Wrap(err, "billing.Customers.CustomerState")
	}

	if !found {
		return uuid.Nil, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if !active {
		return uuid.Nil, apperr.Fields(
			map[string]string{"customerId": apperr.KeyInactiveCustomer},
		)
	}

	for _, unitID := range input.UnitIDs {
		unitOrg, archived, unitFound, err := m.Customers.UnitState(ctx, call, unitID)
		if err != nil {
			return uuid.Nil, apperr.Wrap(err, "billing.Customers.UnitState")
		}

		if !unitFound || archived || unitOrg != org {
			return uuid.Nil, apperr.Fields(
				map[string]string{"unitIds": "error.otherCustomerUnit"},
			)
		}
	}

	return org, nil
}

// contractRevision is the contract row a save writes: a new contract, or the next version of the current one.
type contractRevision struct {
	id      uuid.UUID
	version int
	prev    *int           // the version it replaces; nil for a new contract
	before  map[string]any // the audited values of the version it replaces
}

// nextRevision is the row a contracts.save writes. An update supersedes the current version, which must keep the
// customer, be the expected version and have no active restriction or unresolved recovery.
func (m Billing) nextRevision(
	ctx context.Context,
	call *ops.Call,
	input *ContractInput,
) (contractRevision, error) {
	if input.ID == nil {
		return contractRevision{id: uuid.New(), version: 1, prev: nil, before: nil}, nil
	}

	cur, err := m.loadContract(ctx, call, *input.ID, true)
	if err != nil {
		return contractRevision{}, err
	}

	if cur.CustomerID != input.CustomerID {
		return contractRevision{}, apperr.Fields(
			map[string]string{"customerId": "error.customerFixed"},
		)
	}

	if cur.Version != *call.ExpectedVersion {
		return contractRevision{}, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	if len(cur.ActiveRestrictionIDs) > 0 || cur.HasUnresolvedRecovery {
		return contractRevision{}, apperr.E(apperr.Conflict, "errors.contract_restricted")
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE billing.contracts SET is_current = false WHERE id = $1 AND is_current`,
		cur.ID,
	)
	if execErr != nil {
		return contractRevision{}, apperr.Wrap(execErr, "update billing.contracts")
	}

	return contractRevision{
		id:      cur.ID,
		version: cur.Version + 1,
		prev:    call.ExpectedVersion,
		before:  cur.fields(),
	}, nil
}

// insertContract writes the contract version and its units.
func insertContract(
	ctx context.Context,
	call *ops.Call,
	input *ContractInput,
	org uuid.UUID,
	revision contractRevision,
) error {
	_, execErr := call.Tx.Exec(
		ctx,
		`INSERT INTO billing.contracts (id, version, tenant_id, customer_id, customer_org_id, plan_type, term, price_minor,
			currency, restriction_eligible,
		rules_version, created_by, created_at) VALUES ($1, $2, current_setting('app.tenant_id')::uuid, $3, $4, $5,
		tstzrange($6, $7), $8, $9, $10, $11, $12, $13)`,
		revision.id,
		revision.version,
		input.CustomerID,
		org,
		input.PlanType,
		input.StartAt,
		input.EndAt,
		input.PriceMinor,
		input.Currency,
		input.RestrictionEligible,
		input.RulesVersion,
		call.Principal.MembershipID,
		call.Now,
	)
	if execErr != nil {
		return apperr.Wrap(execErr, "insert billing.contracts")
	}

	for _, unitID := range input.UnitIDs {
		_, err := call.Tx.Exec(
			ctx,
			`INSERT INTO billing.contract_units (tenant_id, contract_id, contract_version, unit_id) VALUES
				(current_setting('app.tenant_id')::uuid, $1, $2, $3)`,
			revision.id,
			revision.version,
			unitID,
		)
		if err != nil {
			return apperr.Wrap(err, "insert billing.contract_units")
		}
	}

	return nil
}

// fields are the audited contract values (the before/after of contracts.save).
func (k Contract) fields() map[string]any {
	return map[string]any{
		"unitIds":             k.UnitIDs,
		"planType":            k.PlanType,
		"startAt":             k.StartAt,
		"endAt":               k.EndAt,
		"priceMinor":          k.PriceMinor,
		fieldCurrency:         k.Currency,
		"restrictionEligible": k.RestrictionEligible,
		"rulesVersion":        k.RulesVersion,
	}
}

// ---- invoices ----

// Payment is Payment of service-contracts.ts.
type Payment struct {
	ID               uuid.UUID  `json:"id"`
	TenantID         uuid.UUID  `json:"tenantId"`
	Version          int        `json:"version"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
	AmountMinor      int64      `json:"amountMinor"`
	Currency         string     `json:"currency"`
	InvoiceID        uuid.UUID  `json:"invoiceId"`
	Method           *string    `json:"method"`
	Status           string     `json:"status"`
	PaymentReference *string    `json:"paymentReference"`
	ConfirmedAt      *time.Time `json:"confirmedAt"`
	// ConfirmationReason is the reason HQ gave when confirming or recording the payment; HQ callers only.
	ConfirmationReason *string     `json:"confirmationReason"`
	EventIDs           []uuid.UUID `json:"eventIds"`
}

// Invoice is Invoice / InvoiceDetail of service-contracts.ts.
type Invoice struct {
	ID              uuid.UUID  `json:"id"`
	TenantID        uuid.UUID  `json:"tenantId"`
	Version         int        `json:"version"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
	AmountMinor     int64      `json:"amountMinor"`
	Currency        string     `json:"currency"`
	Number          string     `json:"number"`
	ContractID      uuid.UUID  `json:"contractId"`
	ContractVersion int        `json:"contractVersion"`
	Period          Range      `json:"period"`
	DueAt           time.Time  `json:"dueAt"`
	Status          string     `json:"status"`
	PaymentMethod   *string    `json:"paymentMethod"`
	PaymentStatus   *string    `json:"paymentStatus"`
	PaidAt          *time.Time `json:"paidAt"`
	customerOrg     uuid.UUID
}

const invoiceCols = `i.id, i.tenant_id, i.version, i.created_at, i.updated_at, i.amount_minor, i.currency, i.number, ` +
	`i.contract_id, i.contract_version, lower(i.period), upper(i.period),
	i.due_at, i.status, i.paid_at, (SELECT p.method FROM billing.payments p WHERE p.invoice_id = i.id ORDER BY ` +
	`p.created_at DESC LIMIT 1),
	(SELECT p.status FROM billing.payments p WHERE p.invoice_id = i.id ORDER BY p.created_at DESC LIMIT 1),
	(SELECT k.customer_org_id FROM billing.contracts k WHERE k.id = i.contract_id AND k.version = i.contract_version)`

func scanInvoice(r pgx.Row) (Invoice, error) {
	var invoice Invoice

	err := r.Scan(
		&invoice.ID,
		&invoice.TenantID,
		&invoice.Version,
		&invoice.CreatedAt,
		&invoice.UpdatedAt,
		&invoice.AmountMinor,
		&invoice.Currency,
		&invoice.Number,
		&invoice.ContractID,
		&invoice.ContractVersion,
		&invoice.Period.From,
		&invoice.Period.To,
		&invoice.DueAt,
		&invoice.Status,
		&invoice.PaidAt,
		&invoice.PaymentMethod,
		&invoice.PaymentStatus,
		&invoice.customerOrg,
	)

	return invoice, apperr.Wrap(err, "scan (scanInvoice)")
}

// InvoiceInput is invoices.create input.
type InvoiceInput struct {
	AmountMinor     int64     `json:"amountMinor"`
	Currency        string    `json:"currency"`
	ContractID      uuid.UUID `json:"contractId"`
	ContractVersion int       `json:"contractVersion"`
	Period          Range     `json:"period"`
	DueAt           time.Time `json:"dueAt"`
}

// Validate implements ops.Validator.
func (in *InvoiceInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.AmountMinor < 1 || in.AmountMinor > money.MaxMinor {
		fieldErrs["amountMinor"] = apperr.KeyRange
	}

	if in.ContractID == uuid.Nil || in.ContractVersion < 1 {
		fieldErrs["contractId"] = apperr.KeyRequired
	}

	if in.Period.From.IsZero() || !in.Period.From.Before(in.Period.To) {
		fieldErrs[fieldPeriod] = apperr.KeyRange
	}

	if in.DueAt.IsZero() || in.DueAt.Before(in.Period.From) {
		fieldErrs[fieldDueAt] = apperr.KeyRange
	}

	return fieldErrs
}

// createInvoice serves invoices.create.
//
//	@Summary		invoices.create (write)
//	@ID				invoices.create
//	@Description	Authorization: admin:billing.write
//	@Description	Validation: D01; input constraints in the corresponding DD; IR90 dueAt must be after now; no period \
//	@Description	overlapping an invoice of the contract (CONFLICT errors.invoice_period_exists; IR469)
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A08 · Input versions: contractVersion=Contract.version
//	@Tags			invoices
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		InvoiceInput	true	"input"
//	@Success		200				{object}	ops.Envelope{data=Invoice}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/invoices [post]
func (m Billing) createInvoice(
	ctx context.Context,
	call *ops.Call,
	input *InvoiceInput,
) (Invoice, error) {
	if !input.DueAt.After(call.Now) {
		return Invoice{}, apperr.Fields(map[string]string{fieldDueAt: apperr.KeyPast})
	}

	contract, err := m.loadContract(ctx, call, input.ContractID, true)
	if err != nil {
		return Invoice{}, err
	}

	fitErr := input.fits(contract)
	if fitErr != nil {
		return Invoice{}, fitErr
	}

	seq, err := nextInvoiceSeq(ctx, call)
	if err != nil {
		return Invoice{}, err
	}

	invoiceID, err := insertInvoice(
		ctx,
		call,
		input,
		contract,
		InvoiceNumber(input.Period.From, seq),
	)
	if err != nil {
		return Invoice{}, err
	}

	invoice, err := m.loadInvoice(ctx, call, invoiceID, false)
	if err != nil {
		return invoice, err
	}

	call.Emit(
		ops.Event{
			AggregateType: kindInvoice,
			AggregateID:   invoiceID,
			Type:          "InvoiceCreated",
			Payload:       map[string]any{"contractId": contract.ID},
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:          "invoices.create",
			TargetKind:      kindInvoice,
			TargetID:        invoiceID.String(),
			NextVersion:     &invoice.Version,
			PreviousVersion: nil, Reason: "", Result: "", Before: nil, After: nil,
		},
	)

	return invoice, nil
}

// insertInvoice writes the unpaid invoice; a period overlapping an invoice of the contract is CONFLICT (IR469).
func insertInvoice(
	ctx context.Context,
	call *ops.Call,
	input *InvoiceInput,
	contract Contract,
	number string,
) (uuid.UUID, error) {
	var invoiceID uuid.UUID

	err := call.Tx.QueryRow(ctx, `INSERT INTO billing.invoices (tenant_id, number, contract_id, contract_version,
		customer_id,
		amount_minor, currency, period, due_at, status, created_at, updated_at)
		VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $5, $6, tstzrange($7, $8), $9, 'unpaid', $10, $10)
		RETURNING id`,
		number, contract.ID, contract.Version, contract.CustomerID, input.AmountMinor, input.Currency, input.Period.From,
		input.Period.To, input.DueAt, call.Now).
		Scan(&invoiceID)
	if err != nil {
		if apperr.From(err).Code == apperr.Conflict {
			return invoiceID, apperr.E(apperr.Conflict, "errors.invoice_period_exists")
		}

		return invoiceID, apperr.Wrap(err, "insert billing.invoices")
	}

	return invoiceID, nil
}

// fits checks the invoice against its contract: the version it was written for, the contract's currency and a period
// within the contract term.
func (in *InvoiceInput) fits(contract Contract) error {
	if contract.Version != in.ContractVersion {
		return apperr.E(apperr.Conflict, "errors.contract_version_changed")
	}

	if in.Currency != contract.Currency {
		return apperr.Fields(map[string]string{fieldCurrency: "errors.contract_currency"})
	}

	if in.Period.From.Before(contract.StartAt) || in.Period.To.After(contract.EndAt) {
		return apperr.Fields(map[string]string{fieldPeriod: "errors.outside_contract"})
	}

	return nil
}

// nextInvoiceSeq draws the tenant's next invoice sequence number. It first serializes the concurrent creates of the
// tenant, so two invoices never draw the same number.
func nextInvoiceSeq(ctx context.Context, call *ops.Call) (int, error) {
	var seq int

	scanErr := call.Tx.QueryRow(ctx, `SELECT 1 FROM (SELECT pg_advisory_xact_lock(hashtext('billing.invoice_number:' ||
		current_setting('app.tenant_id')))) l`).
		Scan(&seq)
	if scanErr != nil {
		return seq, apperr.Wrap(scanErr, "select (createInvoice)")
	}

	scanErr = call.Tx.QueryRow(ctx, `SELECT count(*) + 1 FROM billing.invoices`).
		Scan(&seq)
	if scanErr != nil {
		return seq, apperr.Wrap(scanErr, "select billing.invoices")
	}

	return seq, nil
}

func (Billing) loadInvoice(
	ctx context.Context,
	call *ops.Call,
	invoiceID uuid.UUID,
	lock bool,
) (Invoice, error) {
	stmt := "SELECT " + invoiceCols + " FROM billing.invoices i WHERE i.id = $1"
	if lock {
		stmt += sqlForUpdate
	}

	invoice, err := scanInvoice(call.Tx.QueryRow(ctx, stmt, invoiceID))
	if errors.Is(err, pgx.ErrNoRows) ||
		(err == nil && call.Principal.Role == roleClient && invoice.customerOrg != call.Principal.OrgID) {
		return invoice, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	return invoice, err
}

// IDInput is the {id} input.
type IDInput struct {
	ID uuid.UUID `json:"id"`
}

// Validate implements ops.Validator.
func (in *IDInput) Validate() map[string]string {
	if in.ID == uuid.Nil {
		return map[string]string{"id": apperr.KeyRequired}
	}

	return nil
}

// InvoiceDetail is InvoiceDetail of service-contracts.ts.
type InvoiceDetail struct {
	Invoice

	PaymentRefs    []Payment   `json:"paymentRefs"`
	RestrictionIDs []uuid.UUID `json:"restrictionIds"`
}

// getInvoice serves invoices.get.
//
//	@Summary		invoices.get (read)
//	@ID				invoices.get
//	@Description	Authorization: client:self | admin:billing.read | admin:restriction.read
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-C11, DD-A08
//	@Tags			invoices
//	@Accept			json
//	@Produce		json
//	@Param			id	path		string	true	"input field id"
//	@Success		200	{object}	ops.Envelope{data=InvoiceDetail}
//	@Failure		401	{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403	{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404	{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409	{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422	{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429	{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503	{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504	{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/invoices/{id} [get]
func (m Billing) getInvoice(
	ctx context.Context,
	call *ops.Call,
	in *IDInput,
) (InvoiceDetail, error) {
	inv, err := m.loadInvoice(ctx, call, in.ID, false)

	invoiceDetail := InvoiceDetail{Invoice: inv, PaymentRefs: nil, RestrictionIDs: nil}
	if err != nil {
		return invoiceDetail, err
	}

	invoiceDetail.PaymentRefs, err = paymentRefs(ctx, call, invoiceDetail.ID)
	if err != nil {
		return invoiceDetail, err
	}

	invoiceDetail.RestrictionIDs, err = m.Restrictions.ForInvoice(ctx, call, invoiceDetail.ID)

	return invoiceDetail, apperr.Wrap(err, "billing.Restrictions.ForInvoice")
}

// paymentRefs are the payments of an invoice, newest first, as the caller may see them.
func paymentRefs(ctx context.Context, call *ops.Call, invoiceID uuid.UUID) ([]Payment, error) {
	rows, err := call.Tx.Query(
		ctx,
		`SELECT id, tenant_id, version, created_at, updated_at, amount_minor, currency, invoice_id, method, status,
			payment_reference, confirmed_at, confirmation_reason
		FROM billing.payments WHERE invoice_id = $1 ORDER BY created_at DESC, id`,
		invoiceID,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select billing.payments")
	}

	refs := []Payment{}

	for rows.Next() {
		var payment Payment

		err := rows.Scan(
			&payment.ID,
			&payment.TenantID,
			&payment.Version,
			&payment.CreatedAt,
			&payment.UpdatedAt,
			&payment.AmountMinor,
			&payment.Currency,
			&payment.InvoiceID,
			&payment.Method,
			&payment.Status,
			&payment.PaymentReference,
			&payment.ConfirmedAt,
			&payment.ConfirmationReason,
		)
		if err != nil {
			rows.Close()

			return nil, apperr.Wrap(err, "select billing.payments")
		}

		payment.EventIDs = []uuid.UUID{}
		refs = append(refs, forCaller(call, payment))
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return nil, apperr.Wrap(rowsErr, "select billing.payments")
	}

	return refs, nil
}

// invoiceStatuses are the statuses invoices.list may filter by.
func invoiceStatuses() []string {
	return []string{invoiceUnpaid, invoiceProcessing, invoicePaid}
}

// InvoiceNumber is INV-<YYYYMM of the period start in Kuala Lumpur>-<4-digit tenant sequence> (IR199): a period that
// starts at local midnight on the 1st belongs to that month, not to the previous UTC month.
func InvoiceNumber(periodFrom time.Time, seq int) string {
	return fmt.Sprintf("INV-%s-%04d", periodFrom.In(kualaLumpur()).Format("200601"), seq)
}

// listInvoices serves invoices.list.
//
//	@Summary		invoices.list (read)
//	@ID				invoices.list
//	@Description	Authorization: client:self | admin:billing.read | admin:restriction.read
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A08, DD-C10, DD-A09 · Query: filters \
//	@Description	contractId,status,from,to,customerId,propertyId,overdueOnly · sort id,dueAt,createdAt,updatedAt \
//	@Description	(default dueAt desc;id asc)
//	@Tags			invoices
//	@Accept			json
//	@Produce		json
//	@Param			cursor		query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit		query		integer	false	"page size 1–100, default 25"
//	@Param			sort		query		string	false	"field:direction — fields id,dueAt,createdAt,updatedAt; default dueAt desc;id asc"
//	@Param			contractId	query		string	false	"filter → contractId"
//	@Param			status		query		string	false	"filter → status"
//	@Param			from		query		string	false	"filter → [from,to) on period.from"
//	@Param			to			query		string	false	"filter → [from,to) on period.from"
//	@Param			customerId	query		string	false	"filter"
//	@Param			propertyId	query		string	false	"filter"
//	@Param			overdueOnly	query		boolean	false	"filter"
//	@Success		200			{object}	ops.Envelope{data=InvoicePage}
//	@Failure		401			{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403			{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404			{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409			{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422			{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429			{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503			{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504			{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/invoices [get]
func (m Billing) listInvoices(
	ctx context.Context,
	call *ops.Call,
	query *paging.Query,
) (paging.Page[Invoice], error) {
	var filters invoiceFilters

	filterErr := filters.decode(query.Filters)
	if filterErr != nil {
		return paging.Page[Invoice]{}, filterErr
	}

	order, err := paging.OrderBy(
		query.Sort,
		map[string]string{
			"id":           "i.id",
			fieldDueAt:     "i.due_at",
			fieldCreatedAt: "i.created_at",
			fieldUpdatedAt: "i.updated_at",
		},
		"i.due_at DESC, i.id ASC",
	)
	if err != nil {
		return paging.Page[Invoice]{}, apperr.Wrap(err, "paging.OrderBy")
	}

	window, err := paging.Resolve(*query, filters, call.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Invoice]{}, apperr.Wrap(err, "paging.Resolve")
	}

	where, args, err := m.invoiceConditions(ctx, call, filters)
	if err != nil {
		return paging.Page[Invoice]{}, err
	}

	var total int

	scanErr := call.Tx.QueryRow(ctx, "SELECT count(*) FROM billing.invoices i WHERE "+where, args...).
		Scan(&total)
	if scanErr != nil {
		return paging.Page[Invoice]{}, apperr.Wrap(scanErr, "select billing.invoices")
	}

	items, err := invoicePage(
		ctx,
		call,
		pageStmt(invoiceCols, "billing.invoices i", where, order, window),
		args,
	)
	if err != nil {
		return paging.Page[Invoice]{}, err
	}

	return paging.Page[Invoice]{
		Items:           items,
		NextCursor:      window.Next(total),
		Total:           total,
		SnapshotVersion: window.Snapshot,
	}, nil
}

// invoicePage reads one page of invoices.
func invoicePage(ctx context.Context, call *ops.Call, stmt string, args []any) ([]Invoice, error) {
	rows, err := call.Tx.Query(ctx, stmt, args...)
	if err != nil {
		return nil, apperr.Wrap(err, "select billing.invoices")
	}
	defer rows.Close()

	items := []Invoice{}

	for rows.Next() {
		invoice, err := scanInvoice(rows)
		if err != nil {
			return nil, err
		}

		items = append(items, invoice)
	}

	return items, apperr.Wrap(rows.Err(), "select billing.invoices")
}

// invoiceFilters are the filters of invoices.list.
type invoiceFilters struct {
	CustomerID  *uuid.UUID `json:"customerId,omitempty"`
	PropertyID  *uuid.UUID `json:"propertyId,omitempty"`
	ContractID  *uuid.UUID `json:"contractId,omitempty"`
	Status      *string    `json:"status,omitempty"`
	From        *time.Time `json:"from,omitempty"`
	To          *time.Time `json:"to,omitempty"`
	OverdueOnly *bool      `json:"overdueOnly,omitempty"`
}

// decode reads the filters: a known status, and from before to.
func (f *invoiceFilters) decode(raw json.RawMessage) error {
	if decodeFilters(raw, f) != nil ||
		(f.Status != nil && !slices.Contains(invoiceStatuses(), *f.Status)) {
		return invalidFilters()
	}

	if f.From != nil && f.To != nil && !f.From.Before(*f.To) {
		return apperr.Fields(
			map[string]string{"filters.to": apperr.KeyRange},
		)
	}

	return nil
}

// invoiceConditions is the WHERE clause of invoices.list and its arguments: a client sees only its customer's
// invoices.
func (m Billing) invoiceConditions(
	ctx context.Context,
	call *ops.Call,
	filters invoiceFilters,
) (string, []any, error) {
	var params placeholders

	conds := []string{sqlTrue}

	if call.Principal.Role == roleClient {
		customer, _, err := m.Customers.CustomerOfOrg(ctx, call, call.Principal.OrgID)
		if err != nil {
			return "", nil, apperr.Wrap(err, "billing.Customers.CustomerOfOrg")
		}

		conds = append(conds, "i.customer_id = "+params.add(customer))
	}

	conds = append(conds, filters.conditions(&params, call.Now)...)

	return strings.Join(conds, " AND "), params.args, nil
}

// conditions are the conditions of the filters set.
func (f *invoiceFilters) conditions(params *placeholders, now time.Time) []string {
	var conds []string

	if f.CustomerID != nil {
		conds = append(conds, "i.customer_id = "+params.add(*f.CustomerID))
	}

	if f.PropertyID != nil { // an invoice belongs to the properties of its contract version's units
		conds = append(
			conds,
			`EXISTS (SELECT 1 FROM billing.contract_units cu JOIN billing.ref_units u ON u.id = cu.unit_id
			WHERE cu.contract_id = i.contract_id AND cu.contract_version = i.contract_version AND u.property_id = `+params.add(
				*f.PropertyID,
			)+")",
		)
	}

	if f.ContractID != nil {
		conds = append(conds, "i.contract_id = "+params.add(*f.ContractID))
	}

	if f.Status != nil {
		conds = append(conds, "i.status = "+params.add(*f.Status))
	}

	if f.From != nil { // [from, to) on the period start
		conds = append(conds, "lower(i.period) >= "+params.add(*f.From))
	}

	if f.To != nil {
		conds = append(conds, "lower(i.period) < "+params.add(*f.To))
	}

	if f.OverdueOnly != nil && *f.OverdueOnly {
		conds = append(conds, "i.status <> 'paid' AND i.due_at < "+params.add(now))
	}

	return conds
}

// RemindInput is invoices.remind input.
type RemindInput struct {
	InvoiceID             uuid.UUID `json:"invoiceId"`
	RecipientMembershipID uuid.UUID `json:"recipientMembershipId"`
	Channel               string    `json:"channel"`
	Reason                string    `json:"reason"`
}

// Validate implements ops.Validator.
func (in *RemindInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.InvoiceID == uuid.Nil || in.RecipientMembershipID == uuid.Nil {
		fieldErrs["invoiceId"] = apperr.KeyRequired
	}

	if in.Channel != "inApp" && in.Channel != "email" && in.Channel != "whatsapp" {
		fieldErrs["channel"] = apperr.KeyInvalid
	}

	in.Reason = strings.TrimSpace(in.Reason)
	if n := utf8.RuneCountInString(in.Reason); n < 1 || n > maxReasonLength {
		fieldErrs["reason"] = apperr.KeyLength
	}

	return fieldErrs
}

// Receipt is InvoiceReminderReceipt.
type Receipt struct {
	InvoiceID      uuid.UUID `json:"invoiceId"`
	InvoiceVersion int       `json:"invoiceVersion"`
	NotificationID uuid.UUID `json:"notificationId"`
}

// remind serves invoices.remind.
//
//	@Summary		invoices.remind (write)
//	@ID				invoices.remind
//	@Description	Authorization: admin:billing.write
//	@Description	Validation: IR04: overdue unpaid; current recipient; expectedVersion=Invoice.version
//	@Description	Recovery: D04: same-key receipt; no duplicate notification
//	@Description	Design: DD-A08
//	@Tags			invoices
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer		true	"all: required (target invoice, read invoices.get)"
//	@Param			invoiceId			path		string		true	"input field invoiceId"
//	@Param			request				body		RemindInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=Receipt}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/invoices/{invoiceId}/remind [post]
func (m Billing) remind(ctx context.Context, call *ops.Call, input *RemindInput) (Receipt, error) {
	invoice, err := m.loadInvoice(ctx, call, input.InvoiceID, true)
	if err != nil {
		return Receipt{}, err
	}

	remindErr := m.remindable(ctx, call, input, invoice)
	if remindErr != nil {
		return Receipt{}, remindErr
	}

	nid, err := m.Notify.Create(ctx, call, notify.New{
		RecipientMembershipID: input.RecipientMembershipID,
		Channel:               input.Channel,
		Type:                  "payment_reminder",
		TemplateKey:           "payment_reminder",
		TargetKind:            kindInvoice,
		TargetID:              invoice.ID,
		Params: map[string]any{
			"invoiceNumber": invoice.Number,
			fieldDueAt:      invoice.DueAt,
			"amountMinor":   invoice.AmountMinor,
			fieldCurrency:   invoice.Currency,
		},
		Severity: "warning", SourceAlertID: nil,
	})
	if err != nil {
		return Receipt{}, apperr.Wrap(err, "notify.Store.Create")
	}

	var version int

	scanErr := call.Tx.QueryRow(ctx, `UPDATE billing.invoices SET last_reminded_at = $2, version = version + 1, updated_at
		=
		$2 WHERE id = $1 RETURNING version`, invoice.ID, call.Now).
		Scan(&version)
	if scanErr != nil {
		return Receipt{}, apperr.Wrap(scanErr, "update billing.invoices")
	}

	call.Emit(
		ops.Event{
			AggregateType: kindInvoice,
			AggregateID:   invoice.ID,
			Type:          "InvoiceReminded",
			Payload:       map[string]any{"notificationId": nid},
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:          "invoices.remind",
			TargetKind:      kindInvoice,
			TargetID:        invoice.ID.String(),
			PreviousVersion: call.ExpectedVersion,
			NextVersion:     &version,
			Reason:          input.Reason, Result: "", Before: nil, After: nil,
		},
	)

	return Receipt{InvoiceID: invoice.ID, InvoiceVersion: version, NotificationID: nid}, nil
}

// remindable checks an invoices.remind: the expected version, an unpaid invoice past its due date, and a recipient
// who is a current client of the invoice's customer.
func (m Billing) remindable(
	ctx context.Context,
	call *ops.Call,
	input *RemindInput,
	invoice Invoice,
) error {
	if invoice.Version != *call.ExpectedVersion {
		return apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	if invoice.Status != invoiceUnpaid || !call.Now.After(invoice.DueAt) {
		return apperr.E(apperr.Conflict, "errors.not_overdue")
	}

	eligible, err := m.Recipients.ActiveClientOf(
		ctx,
		call,
		input.RecipientMembershipID,
		invoice.customerOrg,
	)
	if err != nil {
		return apperr.Wrap(err, "billing.Recipients.ActiveClientOf")
	}

	if !eligible {
		return apperr.Fields(
			map[string]string{"recipientMembershipId": "errors.recipient_not_eligible"},
		)
	}

	reachable, err := m.Recipients.Reachable(ctx, call, input.RecipientMembershipID, input.Channel)
	if err != nil {
		return apperr.Wrap(err, "billing.Recipients.Reachable")
	}

	if !reachable { // IR779: a WhatsApp reminder needs the client's phone number
		return apperr.Fields(map[string]string{"channel": "errors.channel_not_allowed"})
	}

	return nil
}

// Register binds the contract and invoice operations.
func Register(reg *ops.Registry, billing Billing) {
	ops.Register(reg, "contracts.list", billing.listContracts)
	ops.Register(reg, "contracts.save", billing.saveContract)
	ops.Register(reg, "invoices.create", billing.createInvoice)
	ops.Register(reg, "invoices.get", billing.getInvoice)
	ops.Register(reg, "invoices.list", billing.listInvoices)
	ops.Register(reg, "invoices.remind", billing.remind)
}
