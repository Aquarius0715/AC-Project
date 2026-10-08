package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/core/modules/maintenance"
	"github.com/pradita/ac-project/service/core/ops"
	"github.com/pradita/ac-project/service/core/platform/apperr"
	"github.com/pradita/ac-project/service/core/platform/paging"
)

// PayoutSource provides accepted jobs, rate cards and job notes (Maintenance).
type PayoutSource interface {
	AcceptedJobs(ctx context.Context, c *ops.Call, from, to time.Time) ([]maintenance.AcceptedJob, error)
	RateAt(ctx context.Context, c *ops.Call, contractor uuid.UUID, at time.Time) (map[string]int64, string, bool, error)
	AddJobNote(ctx context.Context, c *ops.Call, job uuid.UUID, message string) error
}

// Payouts is the contractor payout operation set (IR137).
type Payouts struct {
	Source PayoutSource
}

var kualaLumpur = func() *time.Location {
	l, err := time.LoadLocation("Asia/Kuala_Lumpur")
	if err != nil {
		return time.FixedZone("MYT", 8*3600)
	}
	return l
}()

// PayoutLine is PayoutLine.
type PayoutLine struct {
	ID          uuid.UUID  `json:"id"`
	JobID       uuid.UUID  `json:"jobId"`
	WorkType    string     `json:"workType"`
	AcceptedAt  *time.Time `json:"acceptedAt"`
	AmountMinor int64      `json:"amountMinor"`
	Kind        string     `json:"kind"`
	Note        *string    `json:"note"`
}

// Question is PayoutQuestion.
type Question struct {
	ID              uuid.UUID  `json:"id"`
	LineID          *uuid.UUID `json:"lineId"`
	Topic           string     `json:"topic"`
	Message         string     `json:"message"`
	State           string     `json:"state"`
	Reply           *string    `json:"reply"`
	AdjustmentMinor *int64     `json:"adjustmentMinor"`
}

// Statement is PayoutStatement.
type Statement struct {
	ID                     uuid.UUID    `json:"id"`
	TenantID               uuid.UUID    `json:"tenantId"`
	Version                int          `json:"version"`
	CreatedAt              time.Time    `json:"createdAt"`
	UpdatedAt              time.Time    `json:"updatedAt"`
	ContractorOrgID        uuid.UUID    `json:"contractorOrgId"`
	Period                 string       `json:"period"`
	Status                 string       `json:"status"`
	Currency               string       `json:"currency"`
	GrossMinor             int64        `json:"grossMinor"`
	DeductionsMinor        int64        `json:"deductionsMinor"`
	NetMinor               int64        `json:"netMinor"`
	PayDate                time.Time    `json:"payDate"`
	Lines                  []PayoutLine `json:"lines"`
	Queries                []Question   `json:"queries"`
	ApprovedByMembershipID *uuid.UUID   `json:"approvedByMembershipId"`
	PaidAt                 *time.Time   `json:"paidAt"`
}

const statementCols = `id, tenant_id, version, created_at, updated_at, contractor_org_id, period, status, currency, gross_minor, deductions_minor, net_minor, pay_date,
	approved_by_membership_id, paid_at`

func scanStatement(r pgx.Row) (Statement, error) {
	var x Statement
	err := r.Scan(&x.ID, &x.TenantID, &x.Version, &x.CreatedAt, &x.UpdatedAt, &x.ContractorOrgID, &x.Period, &x.Status, &x.Currency, &x.GrossMinor, &x.DeductionsMinor,
		&x.NetMinor, &x.PayDate, &x.ApprovedByMembershipID, &x.PaidAt)
	return x, err
}

func (m Payouts) fill(ctx context.Context, c *ops.Call, x *Statement) error {
	rows, err := c.Tx.Query(ctx, `SELECT id, job_id, work_type, accepted_at, amount_minor, kind, note FROM billing.payout_lines WHERE statement_id = $1 ORDER BY accepted_at NULLS LAST, id`, x.ID)
	if err != nil {
		return err
	}
	x.Lines = []PayoutLine{}
	for rows.Next() {
		var l PayoutLine
		if err := rows.Scan(&l.ID, &l.JobID, &l.WorkType, &l.AcceptedAt, &l.AmountMinor, &l.Kind, &l.Note); err != nil {
			rows.Close()
			return err
		}
		x.Lines = append(x.Lines, l)
	}
	rows.Close()
	rows, err = c.Tx.Query(ctx, `SELECT id, line_id, topic, message, state, resolution, adjustment_minor FROM billing.payout_queries WHERE statement_id = $1 ORDER BY created_at, id`, x.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	x.Queries = []Question{}
	for rows.Next() {
		var q Question
		if err := rows.Scan(&q.ID, &q.LineID, &q.Topic, &q.Message, &q.State, &q.Reply, &q.AdjustmentMinor); err != nil {
			return err
		}
		x.Queries = append(x.Queries, q)
	}
	return rows.Err()
}

// load reads a statement; contractors see only their own approved/paid statements.
func (m Payouts) load(ctx context.Context, c *ops.Call, id uuid.UUID, lock bool) (Statement, error) {
	q := "SELECT " + statementCols + " FROM billing.payout_statements WHERE id = $1"
	if lock {
		q += " FOR UPDATE"
	}
	x, err := scanStatement(c.Tx.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && c.Principal.Role == "contractor" && (x.ContractorOrgID != c.Principal.OrgID || x.Status == "draft")) {
		return x, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return x, err
	}
	return x, m.fill(ctx, c, &x)
}

// ---- payouts.generate ----

// GenerateInput is payouts.generate input.
type GenerateInput struct {
	Period string `json:"period"`
}

func periodBounds(p string) (time.Time, time.Time, error) {
	start, err := time.ParseInLocation("2006-01", p, kualaLumpur)
	if err != nil {
		return start, start, err
	}
	return start, start.AddDate(0, 1, 0), nil
}

// Validate implements ops.Validator.
func (in *GenerateInput) Validate() map[string]string {
	if _, _, err := periodBounds(in.Period); err != nil {
		return map[string]string{"period": "error.invalid"}
	}
	return nil
}

func (m Payouts) generate(ctx context.Context, c *ops.Call, in *GenerateInput) ([]Statement, error) {
	from, to, _ := periodBounds(in.Period)
	if c.Now.Before(to) {
		return nil, apperr.Fields(map[string]string{"period": "errors.period_not_ended"})
	}
	jobs, err := m.Source.AcceptedJobs(ctx, c, from, to)
	if err != nil {
		return nil, err
	}
	type acc struct {
		lines    []PayoutLine
		currency string
	}
	per := map[uuid.UUID]*acc{}
	var order []uuid.UUID
	get := func(org uuid.UUID) *acc {
		if per[org] == nil {
			per[org] = &acc{currency: "MYR"}
			order = append(order, org)
		}
		return per[org]
	}
	for _, j := range jobs {
		a := get(j.ContractorOrgID)
		rates, currency, found, err := m.Source.RateAt(ctx, c, j.ContractorOrgID, j.AcceptedAt)
		if err != nil {
			return nil, err
		}
		work := "repair_base"
		if j.Type == "periodic" {
			work = "periodic_inspection"
		}
		at := j.AcceptedAt
		line := PayoutLine{ID: uuid.New(), JobID: j.JobID, WorkType: work, AcceptedAt: &at, Kind: "charge"}
		if found {
			a.currency = currency
			line.AmountMinor = rates[work]
		} else {
			note := "no rate card"
			line.Note = &note
		}
		a.lines = append(a.lines, line)
		if j.Returned && found {
			a.lines = append(a.lines, PayoutLine{ID: uuid.New(), JobID: j.JobID, WorkType: "rework_deduction", AcceptedAt: &at, AmountMinor: rates["rework_deduction"], Kind: "deduction"})
		}
	}
	// adjustments of answered questions not yet applied (from earlier periods); those carried by a draft of this
	// period are released first because the draft is regenerated
	if _, err := c.Tx.Exec(ctx, `UPDATE billing.payout_queries SET applied_statement_id = NULL
		WHERE applied_statement_id IN (SELECT id FROM billing.payout_statements WHERE period = $1 AND status = 'draft')`, in.Period); err != nil {
		return nil, err
	}
	// adjustments of answered questions not yet applied (from earlier periods)
	rows, err := c.Tx.Query(ctx, `SELECT q.id, s.contractor_org_id, q.adjustment_minor, l.job_id FROM billing.payout_queries q JOIN billing.payout_statements s ON s.id = q.statement_id
		LEFT JOIN billing.payout_lines l ON l.id = q.line_id
		WHERE q.state = 'adjusted' AND q.applied_statement_id IS NULL AND s.period < $1 ORDER BY q.resolved_at, q.id`, in.Period)
	if err != nil {
		return nil, err
	}
	type adj struct {
		query, org, job uuid.UUID
		amount          int64
	}
	var adjs []adj
	for rows.Next() {
		var a adj
		var job *uuid.UUID
		if err := rows.Scan(&a.query, &a.org, &a.amount, &job); err != nil {
			rows.Close()
			return nil, err
		}
		if job != nil {
			a.job = *job
		}
		adjs = append(adjs, a)
	}
	rows.Close()
	for _, a := range adjs {
		note := "adjustment from question " + a.query.String()
		get(a.org).lines = append(get(a.org).lines, PayoutLine{ID: uuid.New(), JobID: a.job, WorkType: "repair_base", AmountMinor: a.amount, Kind: "adjustment", Note: &note})
	}
	payDate := time.Date(to.Year(), to.Month(), 15, 0, 0, 0, 0, time.UTC)
	out := []Statement{}
	for _, org := range order {
		var status string
		err := c.Tx.QueryRow(ctx, `SELECT status FROM billing.payout_statements WHERE contractor_org_id = $1 AND period = $2`, org, in.Period).Scan(&status)
		if err == nil && status != "draft" {
			continue // approved / paid statements are untouched
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		if _, err := c.Tx.Exec(ctx, `DELETE FROM billing.payout_statements WHERE contractor_org_id = $1 AND period = $2 AND status = 'draft'`, org, in.Period); err != nil {
			return nil, err
		}
		a := per[org]
		var gross, ded int64
		for _, l := range a.lines {
			switch {
			case l.Kind == "deduction":
				ded += l.AmountMinor
			case l.Kind == "adjustment" && l.AmountMinor < 0:
				ded += -l.AmountMinor
			default:
				gross += l.AmountMinor
			}
		}
		var sid uuid.UUID
		if err := c.Tx.QueryRow(ctx, `INSERT INTO billing.payout_statements (tenant_id, contractor_org_id, period, status, currency, gross_minor, deductions_minor, net_minor, pay_date, created_at, updated_at)
			VALUES (current_setting('app.tenant_id')::uuid, $1, $2, 'draft', $3, $4, $5, $6, $7, $8, $8) RETURNING id`, org, in.Period, a.currency, gross, ded, gross-ded, payDate, c.Now).Scan(&sid); err != nil {
			return nil, err
		}
		for _, l := range a.lines {
			if _, err := c.Tx.Exec(ctx, `INSERT INTO billing.payout_lines (id, statement_id, tenant_id, job_id, work_type, accepted_at, amount_minor, kind, note)
				VALUES ($1, $2, current_setting('app.tenant_id')::uuid, $3, $4, $5, $6, $7, $8)`, l.ID, sid, l.JobID, l.WorkType, l.AcceptedAt, l.AmountMinor, l.Kind, l.Note); err != nil {
				return nil, err
			}
		}
		for _, x := range adjs {
			if x.org == org {
				if _, err := c.Tx.Exec(ctx, `UPDATE billing.payout_queries SET applied_statement_id = $2 WHERE id = $1`, x.query, sid); err != nil {
					return nil, err
				}
			}
		}
		x, err := m.load(ctx, c, sid, false)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
		c.Emit(ops.Event{AggregateType: "payout_statement", AggregateID: sid, Type: "PayoutDraftGenerated", Payload: map[string]any{"period": in.Period}})
		c.Audit(ops.AuditEntry{Action: "payouts.generate", TargetKind: "payout_statement", TargetID: sid.String(), NextVersion: &x.Version})
	}
	return out, nil
}

// ---- payouts.transition ----

// TransitionInput is payouts.transition input.
type TransitionInput struct {
	StatementID uuid.UUID `json:"statementId"`
	Action      string    `json:"action"`
	Reason      *string   `json:"reason,omitempty"`
}

// Validate implements ops.Validator.
func (in *TransitionInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.StatementID == uuid.Nil {
		fe["statementId"] = "error.required"
	}
	if in.Action != "approve" && in.Action != "mark_paid" {
		fe["action"] = "error.invalid"
	}
	if in.Reason != nil {
		*in.Reason = strings.TrimSpace(*in.Reason)
		if utf8.RuneCountInString(*in.Reason) > 1000 {
			fe["reason"] = "error.length"
		}
	}
	return fe
}

func (m Payouts) transition(ctx context.Context, c *ops.Call, in *TransitionInput) (Statement, error) {
	x, err := m.load(ctx, c, in.StatementID, true)
	if err != nil {
		return x, err
	}
	if x.Version != *c.ExpectedVersion {
		return x, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	switch in.Action {
	case "approve":
		if x.Status != "draft" {
			return x, apperr.E(apperr.Conflict, "error.invalidState")
		}
		_, err = c.Tx.Exec(ctx, `UPDATE billing.payout_statements SET status = 'approved', approved_by_membership_id = $2, version = version + 1, updated_at = $3 WHERE id = $1`, x.ID, c.Principal.MembershipID, c.Now)
	default:
		today := c.Now.In(kualaLumpur).Format("2006-01-02")
		if x.Status != "approved" || today < x.PayDate.Format("2006-01-02") {
			return x, apperr.E(apperr.Conflict, "errors.not_payable_yet")
		}
		_, err = c.Tx.Exec(ctx, `UPDATE billing.payout_statements SET status = 'paid', paid_at = $2, version = version + 1, updated_at = $2 WHERE id = $1`, x.ID, c.Now)
	}
	if err != nil {
		return x, err
	}
	return m.finish(ctx, c, x.ID, "payouts.transition", reasonOf(in.Reason))
}

func reasonOf(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (m Payouts) finish(ctx context.Context, c *ops.Call, id uuid.UUID, action, reason string) (Statement, error) {
	x, err := m.load(ctx, c, id, false)
	if err != nil {
		return x, err
	}
	c.Emit(ops.Event{AggregateType: "payout_statement", AggregateID: id, Type: "PayoutStatementChanged", Payload: map[string]any{"status": x.Status}})
	c.Audit(ops.AuditEntry{Action: action, TargetKind: "payout_statement", TargetID: id.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &x.Version, Reason: reason})
	return x, nil
}

// ---- payouts.query / payouts.resolveQuery ----

// QueryInput is payouts.query input.
type QueryInput struct {
	StatementID uuid.UUID `json:"statementId"`
	LineID      uuid.UUID `json:"lineId"`
	Topic       string    `json:"topic"`
	Message     string    `json:"message"`
}

var topics = map[string]bool{"amount": true, "deduction": true, "missing_job": true, "other": true}

// Validate implements ops.Validator.
func (in *QueryInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.StatementID == uuid.Nil || in.LineID == uuid.Nil {
		fe["lineId"] = "error.required"
	}
	if !topics[in.Topic] {
		fe["topic"] = "error.invalid"
	}
	in.Message = strings.TrimSpace(in.Message)
	if n := utf8.RuneCountInString(in.Message); n < 1 || n > 2000 {
		fe["message"] = "error.length"
	}
	return fe
}

func (m Payouts) query(ctx context.Context, c *ops.Call, in *QueryInput) (Statement, error) {
	x, err := m.load(ctx, c, in.StatementID, true)
	if err != nil {
		return x, err
	}
	if x.Version != *c.ExpectedVersion {
		return x, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	if x.Status != "approved" {
		return x, apperr.E(apperr.Conflict, "error.invalidState")
	}
	var job *uuid.UUID
	for _, l := range x.Lines {
		if l.ID == in.LineID {
			j := l.JobID
			job = &j
		}
	}
	if job == nil {
		return x, apperr.E(apperr.NotFound, "error.notFound")
	}
	if _, err := c.Tx.Exec(ctx, `INSERT INTO billing.payout_queries (statement_id, line_id, tenant_id, asked_by, topic, message, state, created_at)
		VALUES ($1, $2, current_setting('app.tenant_id')::uuid, $3, $4, $5, 'open', $6)`, x.ID, in.LineID, c.Principal.MembershipID, in.Topic, in.Message, c.Now); err != nil {
		return x, err
	}
	if err := m.Source.AddJobNote(ctx, c, *job, fmt.Sprintf("Payout question (%s): %s", in.Topic, in.Message)); err != nil {
		return x, err
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE billing.payout_statements SET version = version + 1, updated_at = $2 WHERE id = $1`, x.ID, c.Now); err != nil {
		return x, err
	}
	return m.finish(ctx, c, x.ID, "payouts.query", "")
}

// ResolveInput is payouts.resolveQuery input.
type ResolveInput struct {
	StatementID     uuid.UUID `json:"statementId"`
	QueryID         uuid.UUID `json:"queryId"`
	Reply           string    `json:"reply"`
	AdjustmentMinor *int64    `json:"adjustmentMinor,omitempty"`
}

// Validate implements ops.Validator.
func (in *ResolveInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.StatementID == uuid.Nil || in.QueryID == uuid.Nil {
		fe["queryId"] = "error.required"
	}
	in.Reply = strings.TrimSpace(in.Reply)
	if n := utf8.RuneCountInString(in.Reply); n < 1 || n > 2000 {
		fe["reply"] = "error.length"
	}
	if in.AdjustmentMinor != nil && *in.AdjustmentMinor == 0 {
		fe["adjustmentMinor"] = "error.nonZero"
	}
	return fe
}

func (m Payouts) resolve(ctx context.Context, c *ops.Call, in *ResolveInput) (Statement, error) {
	x, err := m.load(ctx, c, in.StatementID, true)
	if err != nil {
		return x, err
	}
	if x.Version != *c.ExpectedVersion {
		return x, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	var state string
	var line *uuid.UUID
	err = c.Tx.QueryRow(ctx, `SELECT state, line_id FROM billing.payout_queries WHERE id = $1 AND statement_id = $2 FOR UPDATE`, in.QueryID, x.ID).Scan(&state, &line)
	if errors.Is(err, pgx.ErrNoRows) {
		return x, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return x, err
	}
	if state != "open" {
		return x, apperr.E(apperr.Conflict, "error.invalidState")
	}
	next := "answered"
	if in.AdjustmentMinor != nil {
		next = "adjusted"
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE billing.payout_queries SET state = $2, resolution = $3, adjustment_minor = $4, resolved_by = $5, resolved_at = $6 WHERE id = $1`,
		in.QueryID, next, in.Reply, in.AdjustmentMinor, c.Principal.MembershipID, c.Now); err != nil {
		return x, err
	}
	for _, l := range x.Lines {
		if line != nil && l.ID == *line {
			if err := m.Source.AddJobNote(ctx, c, l.JobID, "Payout reply: "+in.Reply); err != nil {
				return x, err
			}
		}
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE billing.payout_statements SET version = version + 1, updated_at = $2 WHERE id = $1`, x.ID, c.Now); err != nil {
		return x, err
	}
	return m.finish(ctx, c, x.ID, "payouts.resolveQuery", in.Reply)
}

// ---- reads ----

// PayoutIDInput is the {id} input.
type PayoutIDInput struct {
	ID uuid.UUID `json:"id"`
}

// Validate implements ops.Validator.
func (in *PayoutIDInput) Validate() map[string]string {
	if in.ID == uuid.Nil {
		return map[string]string{"id": "error.required"}
	}
	return nil
}

func (m Payouts) get(ctx context.Context, c *ops.Call, in *PayoutIDInput) (Statement, error) {
	return m.load(ctx, c, in.ID, false)
}

func (m Payouts) list(ctx context.Context, c *ops.Call, in *paging.Query) (paging.Page[Statement], error) {
	var f struct {
		Period          *string    `json:"period,omitempty"`
		ContractorOrgID *uuid.UUID `json:"contractorOrgId,omitempty"`
		Status          *string    `json:"status,omitempty"`
	}
	if len(in.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(in.Filters)))
		dec.DisallowUnknownFields()
		if dec.Decode(&f) != nil || (f.Status != nil && *f.Status != "draft" && *f.Status != "approved" && *f.Status != "paid") {
			return paging.Page[Statement]{}, apperr.Fields(map[string]string{"filters": "error.invalid"})
		}
	}
	w, err := paging.Resolve(*in, f, c.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Statement]{}, err
	}
	var args []any
	add := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	conds := []string{"TRUE"}
	if c.Principal.Role == "contractor" {
		conds = append(conds, "contractor_org_id = "+add(c.Principal.OrgID)+" AND status <> 'draft'")
	}
	if f.Period != nil {
		conds = append(conds, "period = "+add(*f.Period))
	}
	if f.ContractorOrgID != nil {
		conds = append(conds, "contractor_org_id = "+add(*f.ContractorOrgID))
	}
	if f.Status != nil {
		conds = append(conds, "status = "+add(*f.Status))
	}
	where := strings.Join(conds, " AND ")
	var total int
	if err := c.Tx.QueryRow(ctx, "SELECT count(*) FROM billing.payout_statements WHERE "+where, args...).Scan(&total); err != nil {
		return paging.Page[Statement]{}, err
	}
	rows, err := c.Tx.Query(ctx, fmt.Sprintf("SELECT %s FROM billing.payout_statements WHERE %s ORDER BY period DESC, contractor_org_id, id LIMIT %d OFFSET %d", statementCols, where, w.Limit, w.Offset), args...)
	if err != nil {
		return paging.Page[Statement]{}, err
	}
	items := []Statement{}
	for rows.Next() {
		x, err := scanStatement(rows)
		if err != nil {
			rows.Close()
			return paging.Page[Statement]{}, err
		}
		items = append(items, x)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return paging.Page[Statement]{}, err
	}
	for i := range items {
		if err := m.fill(ctx, c, &items[i]); err != nil {
			return paging.Page[Statement]{}, err
		}
	}
	return paging.Page[Statement]{Items: items, NextCursor: w.Next(total), Total: total, SnapshotVersion: w.Snapshot}, nil
}

// RegisterPayouts binds the payout operations.
func RegisterPayouts(r *ops.Registry, m Payouts) {
	ops.Register(r, "payouts.generate", m.generate)
	ops.Register(r, "payouts.transition", m.transition)
	ops.Register(r, "payouts.query", m.query)
	ops.Register(r, "payouts.resolveQuery", m.resolve)
	ops.Register(r, "payouts.get", m.get)
	ops.Register(r, "payouts.list", m.list)
}
