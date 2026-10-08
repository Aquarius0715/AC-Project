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

	"github.com/pradita/ac-project/service/internal/modules/notify"
	"github.com/pradita/ac-project/service/internal/ops"
	"github.com/pradita/ac-project/service/internal/platform/apperr"
	"github.com/pradita/ac-project/service/internal/platform/paging"
)

// Customers resolves customers and units (Assets).
type Customers interface {
	CustomerState(ctx context.Context, c *ops.Call, customer uuid.UUID) (org uuid.UUID, active bool, found bool, err error)
	UnitState(ctx context.Context, c *ops.Call, unit uuid.UUID) (org uuid.UUID, archived bool, found bool, err error)
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
}

// Billing is the contract and invoice operation set (IR135).
type Billing struct {
	Customers    Customers
	Restrictions Restrictions
	Recipients   Recipients
	Notify       notify.Store
	Releases     Releases
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

const contractCols = `k.id, k.tenant_id, k.version, k.created_at, k.customer_id, k.customer_org_id, k.plan_type, lower(k.term), upper(k.term), k.price_minor, k.currency,
	k.restriction_eligible, k.rules_version,
	COALESCE((SELECT array_agg(cu.unit_id ORDER BY cu.unit_id) FROM billing.contract_units cu WHERE cu.contract_id = k.id AND cu.contract_version = k.version), '{}')`

func scanContract(r pgx.Row) (Contract, error) {
	var x Contract
	err := r.Scan(&x.ID, &x.TenantID, &x.Version, &x.CreatedAt, &x.CustomerID, &x.CustomerOrgID, &x.PlanType, &x.StartAt, &x.EndAt, &x.PriceMinor, &x.Currency,
		&x.RestrictionEligible, &x.RulesVersion, &x.UnitIDs)
	x.UpdatedAt = x.CreatedAt
	return x, err
}

func (m Billing) decorate(ctx context.Context, c *ops.Call, x *Contract) error {
	var err error
	x.ActiveRestrictionIDs, x.HasUnresolvedRecovery, err = m.Restrictions.ContractState(ctx, c, x.ID)
	return err
}

// clientScope restricts clients to their own customer organization.
func clientScope(c *ops.Call, col string, args *[]any) string {
	if c.Principal.Role != "client" {
		return "TRUE"
	}
	*args = append(*args, c.Principal.OrgID)
	return fmt.Sprintf("%s = $%d", col, len(*args))
}

func (m Billing) loadContract(ctx context.Context, c *ops.Call, id uuid.UUID, lock bool) (Contract, error) {
	q := "SELECT " + contractCols + " FROM billing.contracts k WHERE k.id = $1 AND k.is_current"
	if lock {
		q += " FOR UPDATE"
	}
	x, err := scanContract(c.Tx.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return x, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return x, err
	}
	return x, m.decorate(ctx, c, &x)
}

func (m Billing) listContracts(ctx context.Context, c *ops.Call, in *paging.Query) (paging.Page[Contract], error) {
	var f struct {
		CustomerID *uuid.UUID `json:"customerId,omitempty"`
		UnitID     *uuid.UUID `json:"unitId,omitempty"`
		PlanType   *string    `json:"planType,omitempty"`
	}
	if len(in.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(in.Filters)))
		dec.DisallowUnknownFields()
		if dec.Decode(&f) != nil || (f.PlanType != nil && !slices.Contains(planTypes, *f.PlanType)) {
			return paging.Page[Contract]{}, apperr.Fields(map[string]string{"filters": "error.invalid"})
		}
	}
	order, err := paging.OrderBy(in.Sort, map[string]string{"id": "k.id", "createdAt": "k.created_at", "startAt": "lower(k.term)"}, "k.id ASC")
	if err != nil {
		return paging.Page[Contract]{}, err
	}
	w, err := paging.Resolve(*in, f, c.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Contract]{}, err
	}
	var args []any
	add := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	conds := []string{"k.is_current", clientScope(c, "k.customer_org_id", &args)}
	if f.CustomerID != nil {
		conds = append(conds, "k.customer_id = "+add(*f.CustomerID))
	}
	if f.UnitID != nil {
		conds = append(conds, "EXISTS (SELECT 1 FROM billing.contract_units cu WHERE cu.contract_id = k.id AND cu.contract_version = k.version AND cu.unit_id = "+add(*f.UnitID)+")")
	}
	if f.PlanType != nil {
		conds = append(conds, "k.plan_type = "+add(*f.PlanType))
	}
	where := strings.Join(conds, " AND ")
	var total int
	if err := c.Tx.QueryRow(ctx, "SELECT count(*) FROM billing.contracts k WHERE "+where, args...).Scan(&total); err != nil {
		return paging.Page[Contract]{}, err
	}
	rows, err := c.Tx.Query(ctx, fmt.Sprintf("SELECT %s FROM billing.contracts k WHERE %s ORDER BY %s LIMIT %d OFFSET %d", contractCols, where, order, w.Limit, w.Offset), args...)
	if err != nil {
		return paging.Page[Contract]{}, err
	}
	items := []Contract{}
	for rows.Next() {
		x, err := scanContract(rows)
		if err != nil {
			rows.Close()
			return paging.Page[Contract]{}, err
		}
		items = append(items, x)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return paging.Page[Contract]{}, err
	}
	for i := range items {
		if err := m.decorate(ctx, c, &items[i]); err != nil {
			return paging.Page[Contract]{}, err
		}
	}
	return paging.Page[Contract]{Items: items, NextCursor: w.Next(total), Total: total, SnapshotVersion: w.Snapshot}, nil
}

var planTypes = []string{"rto", "general", "energy", "environment"}

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
	fe := map[string]string{}
	if in.CustomerID == uuid.Nil {
		fe["customerId"] = "error.required"
	}
	seen := map[uuid.UUID]bool{}
	if len(in.UnitIDs) < 1 || len(in.UnitIDs) > 500 {
		fe["unitIds"] = "error.count"
	}
	for _, u := range in.UnitIDs {
		if seen[u] {
			fe["unitIds"] = "error.duplicate"
		}
		seen[u] = true
	}
	if !slices.Contains(planTypes, in.PlanType) {
		fe["planType"] = "error.invalid"
	}
	if in.StartAt.IsZero() || !in.StartAt.Before(in.EndAt) {
		fe["endAt"] = "error.range"
	}
	if in.PriceMinor < 0 {
		fe["priceMinor"] = "error.range"
	}
	if in.Currency != "MYR" && in.Currency != "USD" {
		fe["currency"] = "error.invalid"
	}
	switch {
	case in.RestrictionEligible && in.PlanType != "rto":
		fe["restrictionEligible"] = "errors.rto_only"
	case in.RestrictionEligible && (in.RulesVersion == nil || utf8.RuneCountInString(strings.TrimSpace(*in.RulesVersion)) < 1 || utf8.RuneCountInString(*in.RulesVersion) > 64):
		fe["rulesVersion"] = "error.required"
	case !in.RestrictionEligible && in.RulesVersion != nil:
		fe["rulesVersion"] = "error.notAllowed"
	}
	return fe
}

func (m Billing) saveContract(ctx context.Context, c *ops.Call, in *ContractInput) (Contract, error) {
	org, active, found, err := m.Customers.CustomerState(ctx, c, in.CustomerID)
	if err != nil {
		return Contract{}, err
	}
	if !found {
		return Contract{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	if !active {
		return Contract{}, apperr.Fields(map[string]string{"customerId": "error.inactiveCustomer"})
	}
	for _, u := range in.UnitIDs {
		uorg, archived, ok, err := m.Customers.UnitState(ctx, c, u)
		if err != nil {
			return Contract{}, err
		}
		if !ok || archived || uorg != org {
			return Contract{}, apperr.Fields(map[string]string{"unitIds": "error.otherCustomerUnit"})
		}
	}
	id, version := uuid.New(), 1
	var prev *int
	if in.ID != nil {
		cur, err := m.loadContract(ctx, c, *in.ID, true)
		if err != nil {
			return cur, err
		}
		if cur.CustomerID != in.CustomerID {
			return cur, apperr.Fields(map[string]string{"customerId": "error.customerFixed"})
		}
		if cur.Version != *c.ExpectedVersion {
			return cur, apperr.E(apperr.Conflict, "error.versionConflict")
		}
		if len(cur.ActiveRestrictionIDs) > 0 || cur.HasUnresolvedRecovery {
			return cur, apperr.E(apperr.Conflict, "errors.contract_restricted")
		}
		if _, err := c.Tx.Exec(ctx, `UPDATE billing.contracts SET is_current = false WHERE id = $1 AND is_current`, cur.ID); err != nil {
			return cur, err
		}
		id, version, prev = cur.ID, cur.Version+1, c.ExpectedVersion
	}
	if _, err := c.Tx.Exec(ctx, `INSERT INTO billing.contracts (id, version, tenant_id, customer_id, customer_org_id, plan_type, term, price_minor, currency, restriction_eligible,
		rules_version, created_by, created_at) VALUES ($1, $2, current_setting('app.tenant_id')::uuid, $3, $4, $5, tstzrange($6, $7), $8, $9, $10, $11, $12, $13)`,
		id, version, in.CustomerID, org, in.PlanType, in.StartAt, in.EndAt, in.PriceMinor, in.Currency, in.RestrictionEligible, in.RulesVersion, c.Principal.MembershipID, c.Now); err != nil {
		return Contract{}, err
	}
	for _, u := range in.UnitIDs {
		if _, err := c.Tx.Exec(ctx, `INSERT INTO billing.contract_units (tenant_id, contract_id, contract_version, unit_id) VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3)`,
			id, version, u); err != nil {
			return Contract{}, err
		}
	}
	x, err := m.loadContract(ctx, c, id, false)
	if err != nil {
		return x, err
	}
	c.Emit(ops.Event{AggregateType: "contract", AggregateID: id, Type: "ContractSaved", Payload: map[string]any{"version": version}})
	c.Audit(ops.AuditEntry{Action: "contracts.save", TargetKind: "contract", TargetID: id.String(), PreviousVersion: prev, NextVersion: &x.Version})
	return x, nil
}

// ---- invoices ----

// Payment is Payment of service-contracts.ts.
type Payment struct {
	ID               uuid.UUID   `json:"id"`
	TenantID         uuid.UUID   `json:"tenantId"`
	Version          int         `json:"version"`
	CreatedAt        time.Time   `json:"createdAt"`
	UpdatedAt        time.Time   `json:"updatedAt"`
	AmountMinor      int64       `json:"amountMinor"`
	Currency         string      `json:"currency"`
	InvoiceID        uuid.UUID   `json:"invoiceId"`
	Method           *string     `json:"method"`
	Status           string      `json:"status"`
	PaymentReference *string     `json:"paymentReference"`
	ConfirmedAt      *time.Time  `json:"confirmedAt"`
	EventIDs         []uuid.UUID `json:"eventIds"`
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

const invoiceCols = `i.id, i.tenant_id, i.version, i.created_at, i.updated_at, i.amount_minor, i.currency, i.number, i.contract_id, i.contract_version, lower(i.period), upper(i.period),
	i.due_at, i.status, i.paid_at, (SELECT p.method FROM billing.payments p WHERE p.invoice_id = i.id ORDER BY p.created_at DESC LIMIT 1),
	(SELECT p.status FROM billing.payments p WHERE p.invoice_id = i.id ORDER BY p.created_at DESC LIMIT 1),
	(SELECT k.customer_org_id FROM billing.contracts k WHERE k.id = i.contract_id AND k.version = i.contract_version)`

func scanInvoice(r pgx.Row) (Invoice, error) {
	var x Invoice
	err := r.Scan(&x.ID, &x.TenantID, &x.Version, &x.CreatedAt, &x.UpdatedAt, &x.AmountMinor, &x.Currency, &x.Number, &x.ContractID, &x.ContractVersion,
		&x.Period.From, &x.Period.To, &x.DueAt, &x.Status, &x.PaidAt, &x.PaymentMethod, &x.PaymentStatus, &x.customerOrg)
	return x, err
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
	fe := map[string]string{}
	if in.AmountMinor < 1 {
		fe["amountMinor"] = "error.range"
	}
	if in.ContractID == uuid.Nil || in.ContractVersion < 1 {
		fe["contractId"] = "error.required"
	}
	if in.Period.From.IsZero() || !in.Period.From.Before(in.Period.To) {
		fe["period"] = "error.range"
	}
	if in.DueAt.IsZero() || in.DueAt.Before(in.Period.From) {
		fe["dueAt"] = "error.range"
	}
	return fe
}

func (m Billing) createInvoice(ctx context.Context, c *ops.Call, in *InvoiceInput) (Invoice, error) {
	if !in.DueAt.After(c.Now) {
		return Invoice{}, apperr.Fields(map[string]string{"dueAt": "error.past"})
	}
	k, err := m.loadContract(ctx, c, in.ContractID, true)
	if err != nil {
		return Invoice{}, err
	}
	if k.Version != in.ContractVersion {
		return Invoice{}, apperr.E(apperr.Conflict, "errors.contract_version_changed")
	}
	if in.Currency != k.Currency {
		return Invoice{}, apperr.Fields(map[string]string{"currency": "errors.contract_currency"})
	}
	if in.Period.From.Before(k.StartAt) || in.Period.To.After(k.EndAt) {
		return Invoice{}, apperr.Fields(map[string]string{"period": "errors.outside_contract"})
	}
	var seq int
	if err := c.Tx.QueryRow(ctx, `SELECT count(*) + 1 FROM billing.invoices`).Scan(&seq); err != nil {
		return Invoice{}, err
	}
	number := fmt.Sprintf("INV-%s-%04d", in.Period.From.UTC().Format("200601"), seq)
	var id uuid.UUID
	err = c.Tx.QueryRow(ctx, `INSERT INTO billing.invoices (tenant_id, number, contract_id, contract_version, customer_id, amount_minor, currency, period, due_at, status, created_at, updated_at)
		VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $5, $6, tstzrange($7, $8), $9, 'unpaid', $10, $10) RETURNING id`,
		number, k.ID, k.Version, k.CustomerID, in.AmountMinor, in.Currency, in.Period.From, in.Period.To, in.DueAt, c.Now).Scan(&id)
	if err != nil {
		if apperr.From(err).Code == apperr.Conflict {
			return Invoice{}, apperr.E(apperr.Conflict, "errors.invoice_period_exists")
		}
		return Invoice{}, err
	}
	x, err := m.loadInvoice(ctx, c, id, false)
	if err != nil {
		return x, err
	}
	c.Emit(ops.Event{AggregateType: "invoice", AggregateID: id, Type: "InvoiceCreated", Payload: map[string]any{"contractId": k.ID}})
	c.Audit(ops.AuditEntry{Action: "invoices.create", TargetKind: "invoice", TargetID: id.String(), NextVersion: &x.Version})
	return x, nil
}

func (m Billing) loadInvoice(ctx context.Context, c *ops.Call, id uuid.UUID, lock bool) (Invoice, error) {
	q := "SELECT " + invoiceCols + " FROM billing.invoices i WHERE i.id = $1"
	if lock {
		q += " FOR UPDATE"
	}
	x, err := scanInvoice(c.Tx.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && c.Principal.Role == "client" && x.customerOrg != c.Principal.OrgID) {
		return x, apperr.E(apperr.NotFound, "error.notFound")
	}
	return x, err
}

// IDInput is the {id} input.
type IDInput struct {
	ID uuid.UUID `json:"id"`
}

// Validate implements ops.Validator.
func (in *IDInput) Validate() map[string]string {
	if in.ID == uuid.Nil {
		return map[string]string{"id": "error.required"}
	}
	return nil
}

// InvoiceDetail is InvoiceDetail of service-contracts.ts.
type InvoiceDetail struct {
	Invoice
	PaymentRefs    []Payment   `json:"paymentRefs"`
	RestrictionIDs []uuid.UUID `json:"restrictionIds"`
}

func (m Billing) getInvoice(ctx context.Context, c *ops.Call, in *IDInput) (InvoiceDetail, error) {
	inv, err := m.loadInvoice(ctx, c, in.ID, false)
	x := InvoiceDetail{Invoice: inv}
	if err != nil {
		return x, err
	}
	rows, err := c.Tx.Query(ctx, `SELECT id, tenant_id, version, created_at, updated_at, amount_minor, currency, invoice_id, method, status, payment_reference, confirmed_at
		FROM billing.payments WHERE invoice_id = $1 ORDER BY created_at DESC, id`, x.ID)
	if err != nil {
		return x, err
	}
	x.PaymentRefs = []Payment{}
	for rows.Next() {
		var p Payment
		if err := rows.Scan(&p.ID, &p.TenantID, &p.Version, &p.CreatedAt, &p.UpdatedAt, &p.AmountMinor, &p.Currency, &p.InvoiceID, &p.Method, &p.Status, &p.PaymentReference, &p.ConfirmedAt); err != nil {
			rows.Close()
			return x, err
		}
		p.EventIDs = []uuid.UUID{}
		x.PaymentRefs = append(x.PaymentRefs, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return x, err
	}
	x.RestrictionIDs, err = m.Restrictions.ForInvoice(ctx, c, x.ID)
	return x, err
}

var invoiceStatuses = []string{"unpaid", "processing", "paid"}

func (m Billing) listInvoices(ctx context.Context, c *ops.Call, in *paging.Query) (paging.Page[Invoice], error) {
	var f struct {
		CustomerID  *uuid.UUID `json:"customerId,omitempty"`
		ContractID  *uuid.UUID `json:"contractId,omitempty"`
		Status      *string    `json:"status,omitempty"`
		OverdueOnly *bool      `json:"overdueOnly,omitempty"`
	}
	if len(in.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(in.Filters)))
		dec.DisallowUnknownFields()
		if dec.Decode(&f) != nil || (f.Status != nil && !slices.Contains(invoiceStatuses, *f.Status)) {
			return paging.Page[Invoice]{}, apperr.Fields(map[string]string{"filters": "error.invalid"})
		}
	}
	order, err := paging.OrderBy(in.Sort, map[string]string{"id": "i.id", "dueAt": "i.due_at", "createdAt": "i.created_at"}, "i.due_at DESC, i.id ASC")
	if err != nil {
		return paging.Page[Invoice]{}, err
	}
	w, err := paging.Resolve(*in, f, c.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Invoice]{}, err
	}
	var args []any
	add := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	conds := []string{"TRUE"}
	if c.Principal.Role == "client" {
		cust, _, err := m.Customers.CustomerOfOrg(ctx, c, c.Principal.OrgID)
		if err != nil {
			return paging.Page[Invoice]{}, err
		}
		conds = append(conds, "i.customer_id = "+add(cust))
	}
	if f.CustomerID != nil {
		conds = append(conds, "i.customer_id = "+add(*f.CustomerID))
	}
	if f.ContractID != nil {
		conds = append(conds, "i.contract_id = "+add(*f.ContractID))
	}
	if f.Status != nil {
		conds = append(conds, "i.status = "+add(*f.Status))
	}
	if f.OverdueOnly != nil && *f.OverdueOnly {
		conds = append(conds, "i.status <> 'paid' AND i.due_at < "+add(c.Now))
	}
	where := strings.Join(conds, " AND ")
	var total int
	if err := c.Tx.QueryRow(ctx, "SELECT count(*) FROM billing.invoices i WHERE "+where, args...).Scan(&total); err != nil {
		return paging.Page[Invoice]{}, err
	}
	rows, err := c.Tx.Query(ctx, fmt.Sprintf("SELECT %s FROM billing.invoices i WHERE %s ORDER BY %s LIMIT %d OFFSET %d", invoiceCols, where, order, w.Limit, w.Offset), args...)
	if err != nil {
		return paging.Page[Invoice]{}, err
	}
	defer rows.Close()
	items := []Invoice{}
	for rows.Next() {
		x, err := scanInvoice(rows)
		if err != nil {
			return paging.Page[Invoice]{}, err
		}
		items = append(items, x)
	}
	return paging.Page[Invoice]{Items: items, NextCursor: w.Next(total), Total: total, SnapshotVersion: w.Snapshot}, rows.Err()
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
	fe := map[string]string{}
	if in.InvoiceID == uuid.Nil || in.RecipientMembershipID == uuid.Nil {
		fe["invoiceId"] = "error.required"
	}
	if in.Channel != "inApp" && in.Channel != "email" && in.Channel != "whatsapp" {
		fe["channel"] = "error.invalid"
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if n := utf8.RuneCountInString(in.Reason); n < 1 || n > 1000 {
		fe["reason"] = "error.length"
	}
	return fe
}

// Receipt is InvoiceReminderReceipt.
type Receipt struct {
	InvoiceID      uuid.UUID `json:"invoiceId"`
	InvoiceVersion int       `json:"invoiceVersion"`
	NotificationID uuid.UUID `json:"notificationId"`
}

func (m Billing) remind(ctx context.Context, c *ops.Call, in *RemindInput) (Receipt, error) {
	x, err := m.loadInvoice(ctx, c, in.InvoiceID, true)
	if err != nil {
		return Receipt{}, err
	}
	if x.Version != *c.ExpectedVersion {
		return Receipt{}, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	if x.Status != "unpaid" || !c.Now.After(x.DueAt) {
		return Receipt{}, apperr.E(apperr.Conflict, "errors.not_overdue")
	}
	ok, err := m.Recipients.ActiveClientOf(ctx, c, in.RecipientMembershipID, x.customerOrg)
	if err != nil {
		return Receipt{}, err
	}
	if !ok {
		return Receipt{}, apperr.Fields(map[string]string{"recipientMembershipId": "errors.recipient_not_eligible"})
	}
	nid, err := m.Notify.Create(ctx, c, notify.New{RecipientMembershipID: in.RecipientMembershipID, Channel: in.Channel, Type: "payment_reminder", TemplateKey: "payment_reminder",
		TargetKind: "invoice", TargetID: x.ID, Params: map[string]any{"invoiceNumber": x.Number, "dueAt": x.DueAt, "amountMinor": x.AmountMinor, "currency": x.Currency}, Severity: "warning"})
	if err != nil {
		return Receipt{}, err
	}
	var v int
	if err := c.Tx.QueryRow(ctx, `UPDATE billing.invoices SET last_reminded_at = $2, version = version + 1, updated_at = $2 WHERE id = $1 RETURNING version`, x.ID, c.Now).Scan(&v); err != nil {
		return Receipt{}, err
	}
	c.Emit(ops.Event{AggregateType: "invoice", AggregateID: x.ID, Type: "InvoiceReminded", Payload: map[string]any{"notificationId": nid}})
	c.Audit(ops.AuditEntry{Action: "invoices.remind", TargetKind: "invoice", TargetID: x.ID.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &v, Reason: in.Reason})
	return Receipt{InvoiceID: x.ID, InvoiceVersion: v, NotificationID: nid}, nil
}

// Register binds the contract and invoice operations.
func Register(r *ops.Registry, m Billing) {
	ops.Register(r, "contracts.list", m.listContracts)
	ops.Register(r, "contracts.save", m.saveContract)
	ops.Register(r, "invoices.create", m.createInvoice)
	ops.Register(r, "invoices.get", m.getInvoice)
	ops.Register(r, "invoices.list", m.listInvoices)
	ops.Register(r, "invoices.remind", m.remind)
}
