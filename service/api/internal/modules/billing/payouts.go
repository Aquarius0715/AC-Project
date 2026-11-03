package billing

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/maintenance"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

// PayoutSource provides accepted jobs, rate cards and job notes (Maintenance).
type PayoutSource interface {
	AcceptedJobs(
		ctx context.Context,
		c *ops.Call,
		from, to time.Time,
	) ([]maintenance.AcceptedJob, error)
	RateAt(
		ctx context.Context,
		c *ops.Call,
		contractor uuid.UUID,
		at time.Time,
	) (map[string]int64, string, bool, error)
	AddJobNote(ctx context.Context, c *ops.Call, job uuid.UUID, message string) error
}

// Payouts is the contractor payout operation set (IR137).
type Payouts struct {
	Source PayoutSource
}

// ---- monthly drafts (scheduler job) ----

// endedPeriod is the month (YYYY-MM) that ended last before now in Kuala Lumpur.
func endedPeriod(now time.Time) string {
	zone := kualaLumpur()
	local := now.In(zone)

	return time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, zone).
		AddDate(0, -1, 0).
		Format("2006-01")
}

// GenerateMonthly is the billing scheduler job of BR-A23 / BR-P10 (IR341). On the 1st of each month (Kuala Lumpur)
// it generates the drafts of the month that ended, exactly as payouts.generate does, once per month
// (billing.payout_runs). A month that ended while the scheduler was down is generated at its first tick. HQ can
// still regenerate the drafts; approved and paid statements are never touched.
func (m Payouts) GenerateMonthly(ctx context.Context, call *ops.Call) (int, error) {
	period := endedPeriod(call.Now)

	tag, err := call.Tx.Exec(
		ctx,
		`INSERT INTO billing.payout_runs (tenant_id, period, ran_at) VALUES (current_setting('app.tenant_id')::uuid, $1, $2)
			ON CONFLICT DO NOTHING`,
		period,
		call.Now,
	)
	if err != nil || tag.RowsAffected() == 0 { // generated already
		return 0, apperr.Wrap(err, "insert billing.payout_runs")
	}

	drafts, err := m.generate(ctx, call, &GenerateInput{Period: period})
	if err != nil {
		return 0, err
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE billing.payout_runs SET statements = $2 WHERE tenant_id = current_setting('app.tenant_id')::uuid AND period =
			$1`,
		period,
		len(drafts),
	)
	if execErr != nil {
		return 0, apperr.Wrap(execErr, "update billing.payout_runs")
	}

	return 1, nil
}

// mytOffset is the UTC offset of Malaysia Time, in seconds.
const mytOffset = 8 * 60 * 60

// kualaLumpur is the time zone of periods, pay dates and invoice numbers; a fixed MYT zone when the zone database is
// missing.
func kualaLumpur() *time.Location {
	zone, err := time.LoadLocation("Asia/Kuala_Lumpur")
	if err != nil {
		return time.FixedZone("MYT", mytOffset)
	}

	return zone
}

// The payout statement statuses, the transitions between them and the kinds of statement lines.
const (
	statementDraft    = "draft"
	statementApproved = "approved"
	statementPaid     = "paid"

	actionApprove  = "approve"
	actionMarkPaid = "mark_paid"

	lineCharge     = "charge"
	lineDeduction  = "deduction"
	lineAdjustment = "adjustment"
)

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

const statementCols = `id, tenant_id, version, created_at, updated_at, contractor_org_id, period, status, currency, ` +
	`gross_minor, deductions_minor, net_minor, pay_date,
	approved_by_membership_id, paid_at`

func scanStatement(r pgx.Row) (Statement, error) {
	var statement Statement

	err := r.Scan(
		&statement.ID,
		&statement.TenantID,
		&statement.Version,
		&statement.CreatedAt,
		&statement.UpdatedAt,
		&statement.ContractorOrgID,
		&statement.Period,
		&statement.Status,
		&statement.Currency,
		&statement.GrossMinor,
		&statement.DeductionsMinor,
		&statement.NetMinor,
		&statement.PayDate,
		&statement.ApprovedByMembershipID,
		&statement.PaidAt,
	)

	return statement, apperr.Wrap(err, "scan (scanStatement)")
}

func (m Payouts) fill(ctx context.Context, call *ops.Call, statement *Statement) error {
	lines, err := payoutLines(ctx, call, statement.ID)
	if err != nil {
		return err
	}

	statement.Lines = lines

	statement.Queries, err = questions(ctx, call, statement.ID)

	return err
}

// payoutLines are the lines of a statement in acceptance order, lines without an acceptance last.
func payoutLines(ctx context.Context, call *ops.Call, statementID uuid.UUID) ([]PayoutLine, error) {
	rows, err := call.Tx.Query(
		ctx,
		`SELECT id, job_id, work_type, accepted_at, amount_minor, kind, note FROM billing.payout_lines WHERE statement_id = $1
			ORDER BY accepted_at NULLS LAST, id`,
		statementID,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select billing.payout_lines")
	}

	lines := []PayoutLine{}

	for rows.Next() {
		var payoutLine PayoutLine

		err := rows.Scan(
			&payoutLine.ID,
			&payoutLine.JobID,
			&payoutLine.WorkType,
			&payoutLine.AcceptedAt,
			&payoutLine.AmountMinor,
			&payoutLine.Kind,
			&payoutLine.Note,
		)
		if err != nil {
			rows.Close()

			return nil, apperr.Wrap(err, "select billing.payout_lines")
		}

		lines = append(lines, payoutLine)
	}

	rows.Close()

	return lines, nil
}

// questions are the contractor's questions about a statement, oldest first.
func questions(ctx context.Context, call *ops.Call, statementID uuid.UUID) ([]Question, error) {
	rows, err := call.Tx.Query(
		ctx,
		`SELECT id, line_id, topic, message, state, resolution, adjustment_minor FROM billing.payout_queries WHERE
			statement_id = $1 ORDER BY created_at, id`,
		statementID,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select billing.payout_queries")
	}
	defer rows.Close()

	out := []Question{}

	for rows.Next() {
		var question Question

		err := rows.Scan(
			&question.ID,
			&question.LineID,
			&question.Topic,
			&question.Message,
			&question.State,
			&question.Reply,
			&question.AdjustmentMinor,
		)
		if err != nil {
			return nil, apperr.Wrap(err, "select billing.payout_queries")
		}

		out = append(out, question)
	}

	return out, apperr.Wrap(rows.Err(), "select billing.payout_queries")
}

// load reads a statement; contractors see only their own approved/paid statements.
func (m Payouts) load(
	ctx context.Context,
	call *ops.Call,
	statementID uuid.UUID,
	lock bool,
) (Statement, error) {
	stmt := "SELECT " + statementCols + " FROM billing.payout_statements WHERE id = $1"
	if lock {
		stmt += sqlForUpdate
	}

	statement, err := scanStatement(call.Tx.QueryRow(ctx, stmt, statementID))
	if errors.Is(err, pgx.ErrNoRows) ||
		(err == nil && call.Principal.Role == roleContractor &&
			(statement.ContractorOrgID != call.Principal.OrgID || statement.Status == statementDraft)) {
		return statement, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return statement, err
	}

	return statement, m.fill(ctx, call, &statement)
}

// ---- payouts.generate ----

// GenerateInput is payouts.generate input.
type GenerateInput struct {
	Period string `json:"period"`
}

func periodBounds(p string) (time.Time, time.Time, error) {
	start, err := time.ParseInLocation("2006-01", p, kualaLumpur())
	if err != nil {
		return start, start, apperr.Wrap(err, "time.ParseInLocation")
	}

	return start, start.AddDate(0, 1, 0), nil
}

// Validate implements ops.Validator.
func (in *GenerateInput) Validate() map[string]string {
	_, _, err := periodBounds(in.Period)
	if err != nil {
		return map[string]string{fieldPeriod: apperr.KeyInvalid}
	}

	return nil
}

// generate serves payouts.generate.
//
//	@Summary		payouts.generate (write)
//	@ID				payouts.generate
//	@Description	Authorization: admin:billing.payment
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR111 period YYYY-MM already ended; approved or paid statements are untouched
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A23
//	@Tags			payouts
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		GenerateInput	true	"input"
//	@Success		200				{object}	ops.Envelope{data=[]Statement}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/payouts/generate [post]
func (m Payouts) generate(
	ctx context.Context,
	call *ops.Call,
	input *GenerateInput,
) ([]Statement, error) {
	from, until, _ := periodBounds(input.Period)
	if call.Now.Before(until) {
		return nil, apperr.Fields(map[string]string{fieldPeriod: "errors.period_not_ended"})
	}

	jobs, err := m.Source.AcceptedJobs(ctx, call, from, until)
	if err != nil {
		return nil, apperr.Wrap(err, "billing.PayoutSource.AcceptedJobs")
	}

	drafts := contractorDrafts{per: map[uuid.UUID]*contractorDraft{}, order: nil}

	for _, acceptedJob := range jobs {
		jobErr := m.addJob(ctx, call, &drafts, acceptedJob)
		if jobErr != nil {
			return nil, jobErr
		}
	}

	adjs, err := pendingAdjustments(ctx, call, input.Period)
	if err != nil {
		return nil, err
	}

	for _, adjustment := range adjs {
		drafts.addAdjustment(adjustment)
	}

	run := draftRun{
		period:  input.Period,
		payDate: time.Date(until.Year(), until.Month(), 15, 0, 0, 0, 0, time.UTC),
		adjs:    adjs,
	}

	return m.writeDrafts(ctx, call, run, &drafts)
}

// writeDrafts writes the draft of each contractor in the order the drafts were opened.
func (m Payouts) writeDrafts(
	ctx context.Context,
	call *ops.Call,
	run draftRun,
	drafts *contractorDrafts,
) ([]Statement, error) {
	out := []Statement{}

	for _, org := range drafts.order {
		replaceable, err := draftReplaceable(ctx, call, run.period, org)
		if err != nil {
			return nil, err
		}

		if !replaceable {
			continue // approved / paid statements are untouched
		}

		statement, err := m.writeDraft(ctx, call, run, org, drafts.per[org])
		if err != nil {
			return nil, err
		}

		out = append(out, statement)
	}

	return out, nil
}

// contractorDraft collects the lines of one contractor's statement and its currency.
type contractorDraft struct {
	lines    []PayoutLine
	currency string
}

// totals are the gross and the deductions of the draft: deduction lines and negative adjustments deduct, the other
// lines are gross.
func (d *contractorDraft) totals() (int64, int64) {
	var gross, ded int64

	for _, payoutLine := range d.lines {
		switch {
		case payoutLine.Kind == lineDeduction:
			ded += payoutLine.AmountMinor
		case payoutLine.Kind == lineAdjustment && payoutLine.AmountMinor < 0:
			ded += -payoutLine.AmountMinor
		default:
			gross += payoutLine.AmountMinor
		}
	}

	return gross, ded
}

// contractorDrafts are the drafts of a generation by contractor organization, in the order they were opened.
type contractorDrafts struct {
	per   map[uuid.UUID]*contractorDraft
	order []uuid.UUID
}

// get returns the contractor's draft, opening one in MYR the first time.
func (d *contractorDrafts) get(org uuid.UUID) *contractorDraft {
	if d.per[org] == nil {
		d.per[org] = &contractorDraft{currency: currencyMYR, lines: nil}
		d.order = append(d.order, org)
	}

	return d.per[org]
}

// addAdjustment adds the adjustment line of an answered question to its contractor's draft.
func (d *contractorDrafts) addAdjustment(adjustment queryAdjustment) {
	note := "adjustment from question " + adjustment.query.String()
	d.get(adjustment.org).lines = append(
		d.get(adjustment.org).lines,
		PayoutLine{
			ID:          uuid.New(),
			JobID:       adjustment.job,
			WorkType:    "repair_base",
			AmountMinor: adjustment.amount,
			Kind:        lineAdjustment,
			Note:        &note, AcceptedAt: nil,
		},
	)
}

// addJob adds an accepted job to its contractor's draft: its charge at the rate card of its acceptance, and the
// rework deduction of a returned job. A job without a rate card is charged nothing, with a note.
func (m Payouts) addJob(
	ctx context.Context,
	call *ops.Call,
	drafts *contractorDrafts,
	acceptedJob maintenance.AcceptedJob,
) error {
	draft := drafts.get(acceptedJob.ContractorOrgID)

	rates, currency, found, err := m.Source.RateAt(
		ctx,
		call,
		acceptedJob.ContractorOrgID,
		acceptedJob.AcceptedAt,
	)
	if err != nil {
		return apperr.Wrap(err, "billing.PayoutSource.RateAt")
	}

	work := "repair_base"
	if acceptedJob.Type == "periodic" {
		work = "periodic_inspection"
	}

	when := acceptedJob.AcceptedAt
	line := PayoutLine{
		ID:         uuid.New(),
		JobID:      acceptedJob.JobID,
		WorkType:   work,
		AcceptedAt: &when,
		Kind:       lineCharge, AmountMinor: 0, Note: nil,
	}

	if found {
		draft.currency = currency
		line.AmountMinor = rates[work]
	} else {
		note := "no rate card"
		line.Note = &note
	}

	draft.lines = append(draft.lines, line)
	if acceptedJob.Returned && found {
		draft.lines = append(
			draft.lines,
			PayoutLine{
				ID:          uuid.New(),
				JobID:       acceptedJob.JobID,
				WorkType:    "rework_deduction",
				AcceptedAt:  &when,
				AmountMinor: rates["rework_deduction"],
				Kind:        lineDeduction, Note: nil,
			},
		)
	}

	return nil
}

// queryAdjustment is the adjustment of an answered payout question, applied on a later statement.
type queryAdjustment struct {
	query, org, job uuid.UUID
	amount          int64
}

// pendingAdjustments are the adjustments of answered questions not yet applied (from earlier periods). Those carried
// by a draft of this period are released first, because the draft is regenerated.
func pendingAdjustments(
	ctx context.Context,
	call *ops.Call,
	period string,
) ([]queryAdjustment, error) {
	_, execErr := call.Tx.Exec(ctx, `UPDATE billing.payout_queries SET applied_statement_id = NULL
		WHERE applied_statement_id IN (SELECT id FROM billing.payout_statements WHERE period = $1 AND status =
		'draft')`, period)
	if execErr != nil {
		return nil, apperr.Wrap(execErr, "update billing.payout_queries")
	}

	rows, err := call.Tx.Query(
		ctx,
		`SELECT q.id, s.contractor_org_id, q.adjustment_minor, l.job_id FROM billing.payout_queries q JOIN
			billing.payout_statements s ON s.id = q.statement_id
		LEFT JOIN billing.payout_lines l ON l.id = q.line_id
		WHERE q.state = 'adjusted' AND q.applied_statement_id IS NULL AND s.period < $1 ORDER BY q.resolved_at, q.id`,
		period,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select billing.payout_queries")
	}

	var adjs []queryAdjustment

	for rows.Next() {
		var (
			adjustment queryAdjustment
			job        *uuid.UUID
		)

		err := rows.Scan(&adjustment.query, &adjustment.org, &adjustment.amount, &job)
		if err != nil {
			rows.Close()

			return nil, apperr.Wrap(err, "select billing.payout_queries")
		}

		if job != nil {
			adjustment.job = *job
		}

		adjs = append(adjs, adjustment)
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return nil, apperr.Wrap(rowsErr, "select billing.payout_queries")
	}

	return adjs, nil
}

// draftRun is one generation: the period, the pay date of its statements and the adjustments it applies.
type draftRun struct {
	period  string
	payDate time.Time
	adjs    []queryAdjustment
}

// draftReplaceable reports whether the contractor's statement of the period may be generated: there is none yet, or
// it is a draft.
func draftReplaceable(
	ctx context.Context,
	call *ops.Call,
	period string,
	org uuid.UUID,
) (bool, error) {
	var status string

	err := call.Tx.QueryRow(ctx, `SELECT status FROM billing.payout_statements WHERE contractor_org_id = $1 AND period =
		$2`, org, period).
		Scan(&status)
	if err == nil && status != statementDraft {
		return false, nil
	}

	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, apperr.Wrap(err, "select billing.payout_statements")
	}

	return true, nil
}

// writeDraft replaces the contractor's draft statement of the period with the collected lines and applies the
// contractor's adjustments to it.
func (m Payouts) writeDraft(
	ctx context.Context,
	call *ops.Call,
	run draftRun,
	org uuid.UUID,
	draft *contractorDraft,
) (Statement, error) {
	_, execErr := call.Tx.Exec(
		ctx,
		`DELETE FROM billing.payout_statements WHERE contractor_org_id = $1 AND period = $2 AND status = 'draft'`,
		org,
		run.period,
	)
	if execErr != nil {
		return Statement{}, apperr.Wrap(execErr, "delete billing.payout_statements")
	}

	sid, err := insertDraft(ctx, call, run, org, draft)
	if err != nil {
		return Statement{}, err
	}

	for _, adjustment := range run.adjs {
		if adjustment.org == org {
			_, err := call.Tx.Exec(
				ctx,
				`UPDATE billing.payout_queries SET applied_statement_id = $2 WHERE id = $1`,
				adjustment.query,
				sid,
			)
			if err != nil {
				return Statement{}, apperr.Wrap(err, "update billing.payout_queries")
			}
		}
	}

	statement, err := m.load(ctx, call, sid, false)
	if err != nil {
		return Statement{}, err
	}

	recordDraft(call, sid, run.period, &statement.Version)

	return statement, nil
}

// insertDraft writes the draft statement and its lines.
func insertDraft(
	ctx context.Context,
	call *ops.Call,
	run draftRun,
	org uuid.UUID,
	draft *contractorDraft,
) (uuid.UUID, error) {
	gross, ded := draft.totals()

	var sid uuid.UUID

	scanErr := call.Tx.QueryRow(ctx, `INSERT INTO billing.payout_statements (tenant_id, contractor_org_id, period, status,
		currency, gross_minor, deductions_minor, net_minor, pay_date, created_at, updated_at)
		VALUES (current_setting('app.tenant_id')::uuid, $1, $2, 'draft', $3, $4, $5, $6, $7, $8, $8) RETURNING
		id`, org, run.period, draft.currency, gross, ded, gross-ded, run.payDate, call.Now).
		Scan(&sid)
	if scanErr != nil {
		return sid, apperr.Wrap(scanErr, "insert billing.payout_statements")
	}

	for _, payoutLine := range draft.lines {
		_, err := call.Tx.Exec(
			ctx,
			`INSERT INTO billing.payout_lines (id, statement_id, tenant_id, job_id, work_type, accepted_at, amount_minor, kind,
				note)
			VALUES ($1, $2, current_setting('app.tenant_id')::uuid, $3, $4, $5, $6, $7, $8)`,
			payoutLine.ID,
			sid,
			payoutLine.JobID,
			payoutLine.WorkType,
			payoutLine.AcceptedAt,
			payoutLine.AmountMinor,
			payoutLine.Kind,
			payoutLine.Note,
		)
		if err != nil {
			return sid, apperr.Wrap(err, "insert billing.payout_lines")
		}
	}

	return sid, nil
}

// recordDraft emits PayoutDraftGenerated and audits payouts.generate for one draft.
func recordDraft(call *ops.Call, sid uuid.UUID, period string, version *int) {
	call.Emit(
		ops.Event{
			AggregateType: kindPayoutStatement,
			AggregateID:   sid,
			Type:          "PayoutDraftGenerated",
			Payload:       map[string]any{fieldPeriod: period},
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:          "payouts.generate",
			TargetKind:      kindPayoutStatement,
			TargetID:        sid.String(),
			NextVersion:     version,
			PreviousVersion: nil,
			Reason:          "",
			Result:          "",
			Before:          nil,
			After:           nil,
		},
	)
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
	fieldErrs := map[string]string{}
	if in.StatementID == uuid.Nil {
		fieldErrs["statementId"] = apperr.KeyRequired
	}

	if in.Action != actionApprove && in.Action != actionMarkPaid {
		fieldErrs["action"] = apperr.KeyInvalid
	}

	if in.Reason != nil {
		*in.Reason = strings.TrimSpace(*in.Reason)
		if utf8.RuneCountInString(*in.Reason) > maxReasonLength {
			fieldErrs["reason"] = apperr.KeyLength
		}
	}

	return fieldErrs
}

// transition serves payouts.transition.
//
//	@Summary		payouts.transition (write)
//	@ID				payouts.transition
//	@Description	Authorization: admin:billing.payment
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR111 draft→approved; approved→paid only on or after payDate
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A23 · This route sets action=approve
//	@Tags			payouts
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer			true	"all: required (target payout_statement, read payouts.get)"
//	@Param			statementId			path		string			true	"input field statementId"
//	@Param			request				body		TransitionInput	true	"input; the path parameters and the fixed field come from the route"
//	@Success		200					{object}	ops.Envelope{data=Statement}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/payouts/{statementId}/approve [post]
func (m Payouts) transition(
	ctx context.Context,
	call *ops.Call,
	input *TransitionInput,
) (Statement, error) {
	statement, err := m.load(ctx, call, input.StatementID, true)
	if err != nil {
		return statement, err
	}

	if statement.Version != *call.ExpectedVersion {
		return statement, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	switch input.Action {
	case actionApprove:
		if statement.Status != statementDraft {
			return statement, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
		}

		_, err = call.Tx.Exec(
			ctx,
			`UPDATE billing.payout_statements SET status = 'approved', approved_by_membership_id = $2, version = version + 1,
				updated_at = $3 WHERE id = $1`,
			statement.ID,
			call.Principal.MembershipID,
			call.Now,
		)
	default:
		today := call.Now.In(kualaLumpur()).Format("2006-01-02")
		if statement.Status != statementApproved || today < statement.PayDate.Format("2006-01-02") {
			return statement, apperr.E(apperr.Conflict, "errors.not_payable_yet")
		}

		_, err = call.Tx.Exec(
			ctx,
			`UPDATE billing.payout_statements SET status = 'paid', paid_at = $2, version = version + 1, updated_at = $2 WHERE id
				= $1`,
			statement.ID,
			call.Now,
		)
	}

	if err != nil {
		return statement, apperr.Wrap(err, "update billing.payout_statements")
	}

	return m.finish(ctx, call, statement.ID, "payouts.transition", reasonOf(input.Reason))
}

func reasonOf(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}

func (m Payouts) finish(
	ctx context.Context,
	call *ops.Call,
	statementID uuid.UUID,
	action, reason string,
) (Statement, error) {
	statement, err := m.load(ctx, call, statementID, false)
	if err != nil {
		return statement, err
	}

	call.Emit(
		ops.Event{
			AggregateType: kindPayoutStatement,
			AggregateID:   statementID,
			Type:          "PayoutStatementChanged",
			Payload:       map[string]any{fieldStatus: statement.Status},
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:          action,
			TargetKind:      kindPayoutStatement,
			TargetID:        statementID.String(),
			PreviousVersion: call.ExpectedVersion,
			NextVersion:     &statement.Version,
			Reason:          reason, Result: "", Before: nil, After: nil,
		},
	)

	return statement, nil
}

// ---- payouts.query / payouts.resolveQuery ----

// QueryInput is payouts.query input.
type QueryInput struct {
	StatementID uuid.UUID `json:"statementId"`
	LineID      uuid.UUID `json:"lineId"`
	Topic       string    `json:"topic"`
	Message     string    `json:"message"`
}

// questionTopics are the topics a payout question may have.
func questionTopics() map[string]bool {
	return map[string]bool{"amount": true, "deduction": true, "missing_job": true, "other": true}
}

// Validate implements ops.Validator.
func (in *QueryInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.StatementID == uuid.Nil || in.LineID == uuid.Nil {
		fieldErrs["lineId"] = apperr.KeyRequired
	}

	if !questionTopics()[in.Topic] {
		fieldErrs["topic"] = apperr.KeyInvalid
	}

	in.Message = strings.TrimSpace(in.Message)
	if n := utf8.RuneCountInString(in.Message); n < 1 || n > maxMessageLength {
		fieldErrs["message"] = apperr.KeyLength
	}

	return fieldErrs
}

// query serves payouts.query.
//
//	@Summary		payouts.query (write)
//	@ID				payouts.query
//	@Description	Authorization: contractor:own-company:approved
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR111 message 1–2000; adds a job history note
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-P10
//	@Tags			payouts
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer		true	"all: required (target payout_statement, read payouts.get)"
//	@Param			statementId			path		string		true	"input field statementId"
//	@Param			request				body		QueryInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=Statement}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/payouts/{statementId}/queries [post]
func (m Payouts) query(ctx context.Context, call *ops.Call, input *QueryInput) (Statement, error) {
	statement, err := m.load(ctx, call, input.StatementID, true)
	if err != nil {
		return statement, err
	}

	if statement.Version != *call.ExpectedVersion {
		return statement, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	if statement.Status != statementApproved {
		return statement, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	}

	job := lineJob(statement, input.LineID)
	if job == nil {
		return statement, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`INSERT INTO billing.payout_queries (statement_id, line_id, tenant_id, asked_by, topic, message, state, created_at)
		VALUES ($1, $2, current_setting('app.tenant_id')::uuid, $3, $4, $5, 'open', $6)`,
		statement.ID,
		input.LineID,
		call.Principal.MembershipID,
		input.Topic,
		input.Message,
		call.Now,
	)
	if execErr != nil {
		return statement, apperr.Wrap(execErr, "insert billing.payout_queries")
	}

	addJobNoteErr := m.Source.AddJobNote(
		ctx,
		call,
		*job,
		fmt.Sprintf("Payout question (%s): %s", input.Topic, input.Message),
	)
	if addJobNoteErr != nil {
		return statement, apperr.Wrap(addJobNoteErr, "billing.PayoutSource.AddJobNote")
	}

	_, execErr = call.Tx.Exec(
		ctx,
		`UPDATE billing.payout_statements SET version = version + 1, updated_at = $2 WHERE id = $1`,
		statement.ID,
		call.Now,
	)
	if execErr != nil {
		return statement, apperr.Wrap(execErr, "update billing.payout_statements")
	}

	return m.finish(ctx, call, statement.ID, "payouts.query", "")
}

// lineJob is the job of the statement line (the last line with that ID); nil when the statement has no such line.
func lineJob(statement Statement, lineID uuid.UUID) *uuid.UUID {
	var job *uuid.UUID

	for _, l := range statement.Lines {
		if l.ID == lineID {
			j := l.JobID
			job = &j
		}
	}

	return job
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
	fieldErrs := map[string]string{}
	if in.StatementID == uuid.Nil || in.QueryID == uuid.Nil {
		fieldErrs["queryId"] = apperr.KeyRequired
	}

	in.Reply = strings.TrimSpace(in.Reply)
	if n := utf8.RuneCountInString(in.Reply); n < 1 || n > maxMessageLength {
		fieldErrs["reply"] = apperr.KeyLength
	}

	if in.AdjustmentMinor != nil && *in.AdjustmentMinor == 0 {
		fieldErrs["adjustmentMinor"] = "error.nonZero"
	}

	return fieldErrs
}

// resolve serves payouts.resolveQuery.
//
//	@Summary		payouts.resolveQuery (write)
//	@ID				payouts.resolveQuery
//	@Description	Authorization: admin:billing.payment
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR111 reply 1–2000; an adjustment goes on the next statement
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A23
//	@Tags			payouts
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer			true	"all: required (target payout_statement, read payouts.get)"
//	@Param			statementId			path		string			true	"input field statementId"
//	@Param			queryId				path		string			true	"input field queryId"
//	@Param			request				body		ResolveInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=Statement}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/payouts/{statementId}/queries/{queryId}/resolve [post]
func (m Payouts) resolve(
	ctx context.Context,
	call *ops.Call,
	input *ResolveInput,
) (Statement, error) {
	statement, err := m.load(ctx, call, input.StatementID, true)
	if err != nil {
		return statement, err
	}

	if statement.Version != *call.ExpectedVersion {
		return statement, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	line, err := openQuestionLine(ctx, call, input.QueryID, statement.ID)
	if err != nil {
		return statement, err
	}

	next := "answered"
	if input.AdjustmentMinor != nil {
		next = "adjusted"
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE billing.payout_queries SET state = $2, resolution = $3, adjustment_minor = $4, resolved_by = $5, resolved_at =
			$6 WHERE id = $1`,
		input.QueryID,
		next,
		input.Reply,
		input.AdjustmentMinor,
		call.Principal.MembershipID,
		call.Now,
	)
	if execErr != nil {
		return statement, apperr.Wrap(execErr, "update billing.payout_queries")
	}

	noteErr := m.noteReply(ctx, call, statement, line, input.Reply)
	if noteErr != nil {
		return statement, noteErr
	}

	_, execErr = call.Tx.Exec(
		ctx,
		`UPDATE billing.payout_statements SET version = version + 1, updated_at = $2 WHERE id = $1`,
		statement.ID,
		call.Now,
	)
	if execErr != nil {
		return statement, apperr.Wrap(execErr, "update billing.payout_statements")
	}

	return m.finish(ctx, call, statement.ID, "payouts.resolveQuery", input.Reply)
}

// noteReply adds the reply to the history of the job of the line the question is about (none: no note).
func (m Payouts) noteReply(
	ctx context.Context,
	call *ops.Call,
	statement Statement,
	line *uuid.UUID,
	reply string,
) error {
	for _, l := range statement.Lines {
		if line != nil && l.ID == *line {
			err := m.Source.AddJobNote(ctx, call, l.JobID, "Payout reply: "+reply)
			if err != nil {
				return apperr.Wrap(err, "billing.PayoutSource.AddJobNote")
			}
		}
	}

	return nil
}

// openQuestionLine locks an open question of the statement and returns the line it is about (nil: none).
func openQuestionLine(
	ctx context.Context,
	call *ops.Call,
	queryID, statementID uuid.UUID,
) (*uuid.UUID, error) {
	var (
		state string
		line  *uuid.UUID
	)

	err := call.Tx.QueryRow(ctx, `SELECT state, line_id FROM billing.payout_queries WHERE id = $1 AND statement_id = $2 FOR
		UPDATE`, queryID, statementID).
		Scan(&state, &line)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return nil, apperr.Wrap(err, "select billing.payout_queries")
	}

	if state != "open" {
		return nil, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	}

	return line, nil
}

// ---- reads ----

// PayoutIDInput is the {id} input.
type PayoutIDInput struct {
	ID uuid.UUID `json:"id"`
}

// Validate implements ops.Validator.
func (in *PayoutIDInput) Validate() map[string]string {
	if in.ID == uuid.Nil {
		return map[string]string{"id": apperr.KeyRequired}
	}

	return nil
}

// get serves payouts.get.
//
//	@Summary		payouts.get (read)
//	@ID				payouts.get
//	@Description	Authorization: admin:billing.read | contractor:own-company:approved-or-paid
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A23, DD-P10
//	@Tags			payouts
//	@Accept			json
//	@Produce		json
//	@Param			id	path		string	true	"input field id"
//	@Success		200	{object}	ops.Envelope{data=Statement}
//	@Failure		401	{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403	{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404	{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409	{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422	{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429	{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503	{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504	{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/payouts/{id} [get]
func (m Payouts) get(ctx context.Context, c *ops.Call, in *PayoutIDInput) (Statement, error) {
	return m.load(ctx, c, in.ID, false)
}

// list serves payouts.list.
//
//	@Summary		payouts.list (read)
//	@ID				payouts.list
//	@Description	Authorization: admin:billing.read | contractor:own-company:approved-or-paid
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A23, DD-P10 · Query: filters contractorOrgId,period,status · sort \
//	@Description	id,createdAt,status,period (default period desc;contractorOrgId asc;id asc)
//	@Tags			payouts
//	@Accept			json
//	@Produce		json
//	@Param			cursor			query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit			query		integer	false	"page size 1–100, default 25"
//	@Param			sort			query		string	false	"field:direction — fields id,createdAt,status,period; default period desc;contractorOrgId asc;id asc"
//	@Param			contractorOrgId	query		string	false	"filter → contractorOrgId"
//	@Param			period			query		string	false	"filter → period (YYYY-MM)"
//	@Param			status			query		string	false	"filter → status"
//	@Success		200				{object}	ops.Envelope{data=StatementPage}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/payouts [get]
func (m Payouts) list(
	ctx context.Context,
	call *ops.Call,
	query *paging.Query,
) (paging.Page[Statement], error) {
	var filters statementFilters

	if decodeFilters(query.Filters, &filters) != nil || !filters.knownStatus() {
		return paging.Page[Statement]{}, invalidFilters()
	}

	order, err := paging.OrderBy(
		query.Sort,
		map[string]string{
			"id":           "id",
			fieldCreatedAt: "created_at",
			fieldStatus:    "status",
			fieldPeriod:    "period",
		},
		"period DESC, contractor_org_id, id",
	)
	if err != nil {
		return paging.Page[Statement]{}, apperr.Wrap(err, "paging.OrderBy")
	}

	window, err := paging.Resolve(*query, filters, call.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Statement]{}, apperr.Wrap(err, "paging.Resolve")
	}

	where, args := filters.conditions(call)

	var total int

	scanErr := call.Tx.QueryRow(ctx, "SELECT count(*) FROM billing.payout_statements WHERE "+where, args...).
		Scan(&total)
	if scanErr != nil {
		return paging.Page[Statement]{}, apperr.Wrap(scanErr, "select billing.payout_statements")
	}

	items, err := m.statementPage(
		ctx,
		call,
		pageStmt(statementCols, "billing.payout_statements", where, order, window),
		args,
	)
	if err != nil {
		return paging.Page[Statement]{}, err
	}

	return paging.Page[Statement]{
		Items:           items,
		NextCursor:      window.Next(total),
		Total:           total,
		SnapshotVersion: window.Snapshot,
	}, nil
}

// statementFilters are the filters of payouts.list.
type statementFilters struct {
	Period          *string    `json:"period,omitempty"`
	ContractorOrgID *uuid.UUID `json:"contractorOrgId,omitempty"`
	Status          *string    `json:"status,omitempty"`
}

// knownStatus reports no status filter or a statement status.
func (f statementFilters) knownStatus() bool {
	return f.Status == nil || *f.Status == statementDraft || *f.Status == statementApproved ||
		*f.Status == statementPaid
}

// conditions is the WHERE clause of payouts.list and its arguments: a contractor sees its own statements, drafts
// excluded.
func (f statementFilters) conditions(call *ops.Call) (string, []any) {
	var params placeholders

	conds := []string{sqlTrue}
	if call.Principal.Role == roleContractor {
		conds = append(
			conds,
			"contractor_org_id = "+params.add(call.Principal.OrgID)+" AND status <> 'draft'",
		)
	}

	if f.Period != nil {
		conds = append(conds, "period = "+params.add(*f.Period))
	}

	if f.ContractorOrgID != nil {
		conds = append(conds, "contractor_org_id = "+params.add(*f.ContractorOrgID))
	}

	if f.Status != nil {
		conds = append(conds, "status = "+params.add(*f.Status))
	}

	return strings.Join(conds, " AND "), params.args
}

// statementPage reads one page of statements with their lines and questions.
func (m Payouts) statementPage(
	ctx context.Context,
	call *ops.Call,
	stmt string,
	args []any,
) ([]Statement, error) {
	rows, err := call.Tx.Query(ctx, stmt, args...)
	if err != nil {
		return nil, apperr.Wrap(err, "select billing.payout_statements")
	}

	items := []Statement{}

	for rows.Next() {
		statement, err := scanStatement(rows)
		if err != nil {
			rows.Close()

			return nil, err
		}

		items = append(items, statement)
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return nil, apperr.Wrap(rowsErr, "select billing.payout_statements")
	}

	for i := range items {
		err := m.fill(ctx, call, &items[i])
		if err != nil {
			return nil, err
		}
	}

	return items, nil
}

// RegisterPayouts binds the payout operations.
func RegisterPayouts(reg *ops.Registry, payouts Payouts) {
	ops.Register(reg, "payouts.generate", payouts.generate)
	ops.Register(reg, "payouts.transition", payouts.transition)
	ops.Register(reg, "payouts.query", payouts.query)
	ops.Register(reg, "payouts.resolveQuery", payouts.resolve)
	ops.Register(reg, "payouts.get", payouts.get)
	ops.Register(reg, "payouts.list", payouts.list)
}
