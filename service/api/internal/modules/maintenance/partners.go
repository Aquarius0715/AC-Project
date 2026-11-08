package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/mail"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/identity"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/money"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

// OrgNames resolves organization names (Identity).
type OrgNames interface {
	OrgState(ctx context.Context, c *ops.Call, org uuid.UUID) (identity.OrgStateResult, error)
	OrgName(ctx context.Context, c *ops.Call, org uuid.UUID) (string, error)
}

// CustomerProfiles resolves the customer and service profile of customer organizations (Assets).
type CustomerProfiles interface {
	CustomerProfiles(
		ctx context.Context,
		c *ops.Call,
		orgs []uuid.UUID,
	) (map[uuid.UUID][2]string, error) // org → {customerId, serviceProfile}
}

// Partners is the contractor register, rate card and SLA operation set (IR131).
type Partners struct {
	Orgs      OrgNames
	Customers CustomerProfiles
}

// Range is Range of service-contracts.ts.
type Range struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// KPIs is ContractorKpis.
type KPIs struct {
	Period            Range    `json:"period"`
	OfferAcceptance   *float64 `json:"offerAcceptance"`
	ArrivalInWindow   *float64 `json:"arrivalInWindow"`
	FirstTimeAccepted *float64 `json:"firstTimeAccepted"`
	AverageRating     *float64 `json:"averageRating"`
	RatingCount       int      `json:"ratingCount"`
	ReworkRate        *float64 `json:"reworkRate"`
}

// Profile is ContractorProfile.
type Profile struct {
	ID                  uuid.UUID  `json:"id"`
	TenantID            uuid.UUID  `json:"tenantId"`
	Version             int        `json:"version"`
	CreatedAt           time.Time  `json:"createdAt"`
	UpdatedAt           time.Time  `json:"updatedAt"`
	OrganizationID      uuid.UUID  `json:"organizationId"`
	Name                string     `json:"name"`
	Status              string     `json:"status"`
	RegistrationNo      string     `json:"registrationNo"`
	ServiceAreas        []string   `json:"serviceAreas"`
	ContactEmail        string     `json:"contactEmail"`
	InsuranceValidUntil *time.Time `json:"insuranceValidUntil"`
	Delegation          Range      `json:"delegation"`
	RateCardID          *uuid.UUID `json:"rateCardId"`
	SuspendedReason     *string    `json:"suspendedReason"`
	KPIs                KPIs       `json:"kpis"`
}

const profileCols = `p.id, p.tenant_id, p.version, p.created_at, p.updated_at, p.organization_id, p.name, p.status, ` +
	`p.registration_no, p.service_areas, p.contact_email::text,
	p.insurance_valid_until, lower(p.delegation), upper(p.delegation), p.suspended_reason,
	(SELECT r.id FROM maintenance.rate_cards r WHERE r.contractor_org_id = p.organization_id AND r.effective_from <= $1 ` +
	`ORDER BY r.effective_from DESC LIMIT 1)`

func scanProfile(r pgx.Row) (Profile, error) {
	var profile Profile

	err := r.Scan(
		&profile.ID,
		&profile.TenantID,
		&profile.Version,
		&profile.CreatedAt,
		&profile.UpdatedAt,
		&profile.OrganizationID,
		&profile.Name,
		&profile.Status,
		&profile.RegistrationNo,
		&profile.ServiceAreas,
		&profile.ContactEmail,
		&profile.InsuranceValidUntil,
		&profile.Delegation.From,
		&profile.Delegation.To,
		&profile.SuspendedReason,
		&profile.RateCardID,
	)

	return profile, apperr.Wrap(err, "scan (scanProfile)")
}

// The precision of the shares and ratings answered: a share in tenths of a percent, a rating to a tenth.
const (
	tenthsOfPercent = 1000
	tenths          = 10
)

// pct is n of d as a percentage to a tenth, nil when d is 0.
func pct(n, d int) *float64 {
	if d == 0 {
		return nil
	}

	v := math.Round(float64(n)*tenthsOfPercent/float64(d)) / tenths

	return &v
}

// averageRating is the average of the stars over the ratings, to a tenth.
func averageRating(stars, ratings int) float64 {
	return math.Round(float64(stars)*tenths/float64(ratings)) / tenths
}

// kpis computes ContractorKpis for the 90 days ending now (IR131 item 6).
func (Partners) kpis(ctx context.Context, call *ops.Call, org uuid.UUID) (KPIs, error) {
	kpIs := KPIs{
		Period:            Range{call.Now.Add(-90 * 24 * time.Hour), call.Now},
		OfferAcceptance:   nil,
		ArrivalInWindow:   nil,
		FirstTimeAccepted: nil,
		AverageRating:     nil,
		RatingCount:       0,
		ReworkRate:        nil,
	}

	var acc, dec int
	// offers closed by HQ's cancellation or the client's reschedule were no decision of the contractor (IR452)
	err := call.Tx.QueryRow(ctx, `SELECT count(*) FILTER (WHERE decision = 'accept'), count(*) FILTER (WHERE decision IS
		NOT
		NULL OR expired_at IS NOT NULL)
		FROM maintenance.offers WHERE contractor_org_id = $1 AND offered_at >= $2 AND offered_at < $3
		AND NOT COALESCE(decision = 'decline' AND decline_reason IN ('job_cancelled','rescheduled'), false)`,
		org, kpIs.Period.From, kpIs.Period.To).
		Scan(&acc, &dec)
	if err != nil {
		return kpIs, apperr.Wrap(err, "select maintenance.offers")
	}

	kpIs.OfferAcceptance = pct(acc, dec)

	metrics, err := jobMetrics(
		ctx,
		call,
		"j.contractor_org_id = $3",
		[]any{kpIs.Period.From, kpIs.Period.To, org},
	)
	if err != nil {
		return kpIs, err
	}

	var arrived, inWin, firstSub, firstOK, completed, rework, stars int

	for _, metric := range metrics {
		if metric.arrivedAt != nil {
			arrived++

			if metric.inWindow {
				inWin++
			}
		}

		if metric.firstReview != "" {
			firstSub++

			if metric.firstReview == decisionAccept {
				firstOK++
			}
		}

		if metric.status == statusCompleted {
			completed++

			if metric.reworkFollowUp {
				rework++
			}
		}

		if metric.stars > 0 {
			kpIs.RatingCount++
			stars += metric.stars
		}
	}

	kpIs.ArrivalInWindow, kpIs.FirstTimeAccepted, kpIs.ReworkRate = pct(
		inWin,
		arrived,
	), pct(
		firstOK,
		firstSub,
	), pct(
		rework,
		completed,
	)
	if kpIs.RatingCount > 0 {
		v := averageRating(stars, kpIs.RatingCount)
		kpIs.AverageRating = &v
	}

	return kpIs, nil
}

type jobMetric struct {
	id, customerOrg, unit uuid.UUID
	createdAt, dueAt      time.Time
	status                string
	responseAt            *time.Time
	arrivedAt             *time.Time
	inWindow              bool
	firstReview           string // accept | return | "" (none)
	everReturned          bool
	followUpWithin30      bool
	reworkFollowUp        bool
	stars                 int
	cancelledAt           *time.Time // the job.cancelled event
	planned               bool       // generated from a maintenance plan: no response target (IR451)
}

// jobMetrics loads the per-job facts for SLA / KPI metrics of jobs created in [$1, $2) matching extra.
func jobMetrics(ctx context.Context, c *ops.Call, extra string, args []any) ([]jobMetric, error) {
	rows, err := c.Tx.Query(
		ctx,
		`SELECT j.id, j.customer_org_id, j.unit_id, j.created_at, j.due_at, j.status,
		COALESCE((SELECT min(o.decided_at) FROM maintenance.offers o WHERE o.job_id = j.id AND o.decision = 'accept'),
			(SELECT min(a.created_at) FROM maintenance.assignments a WHERE a.job_id = j.id)),
		(j.time_on_site->>'arrivedAt')::timestamptz,
		COALESCE((j.time_on_site->>'arrivedAt')::timestamptz >= lower(j.scheduled_slot) AND
		(j.time_on_site->>'arrivedAt')::timestamptz < upper(j.scheduled_slot), false),
		COALESCE((SELECT rr.decision FROM maintenance.report_reviews rr JOIN maintenance.work_reports w ON w.id = rr.report_id
		AND w.version = rr.report_version
			WHERE w.job_id = j.id ORDER BY rr.occurred_at, rr.report_version LIMIT 1), ''),
		EXISTS (SELECT 1 FROM maintenance.report_reviews rr JOIN maintenance.work_reports w ON w.id = rr.report_id AND
		w.version = rr.report_version
			WHERE w.job_id = j.id AND rr.decision = 'return'),
		EXISTS (SELECT 1 FROM maintenance.jobs f WHERE f.unit_id = j.unit_id AND f.id <> j.id AND j.completed_at IS NOT NULL
		AND f.origin <> 'periodic_plan'
			AND f.created_at > j.completed_at AND f.created_at <= j.completed_at + interval '30 days'),
			-- a plan's next visit is no follow-up (IR450)
		EXISTS (SELECT 1 FROM maintenance.jobs f WHERE f.follow_up_of_job_id = j.id AND f.follow_up_class = 'rework'),
		COALESCE((j.rating->>'stars')::int, 0),
		(SELECT min(e.occurred_at) FROM maintenance.job_events e WHERE e.job_id = j.id AND e.action = 'job.cancelled'),
		j.origin = 'periodic_plan'
		FROM maintenance.jobs j WHERE j.created_at >= $1 AND j.created_at < $2 AND `+extra+` ORDER BY j.created_at, j.id`,
		args...)
	if err != nil {
		return nil, apperr.Wrap(err, "select maintenance.offers")
	}
	defer rows.Close()

	var out []jobMetric

	for rows.Next() {
		var metric jobMetric

		err := rows.Scan(
			&metric.id,
			&metric.customerOrg,
			&metric.unit,
			&metric.createdAt,
			&metric.dueAt,
			&metric.status,
			&metric.responseAt,
			&metric.arrivedAt,
			&metric.inWindow,
			&metric.firstReview,
			&metric.everReturned,
			&metric.followUpWithin30,
			&metric.reworkFollowUp,
			&metric.stars,
			&metric.cancelledAt,
			&metric.planned,
		)
		if err != nil {
			return nil, apperr.Wrap(err, "select maintenance.offers")
		}

		out = append(out, metric)
	}

	return out, apperr.Wrap(rows.Err(), "select maintenance.offers")
}

// ---- contractors.list / save / setOfferStatus ----

func (m Partners) loadProfile(
	ctx context.Context,
	call *ops.Call,
	where string,
	args []any,
	lock bool,
) (Profile, error) {
	q := "SELECT " + profileCols + " FROM maintenance.contractors p WHERE " + where
	if lock {
		q += forUpdate
	}

	profile, err := scanProfile(call.Tx.QueryRow(ctx, q, append([]any{call.Now}, args...)...))
	if errors.Is(err, pgx.ErrNoRows) {
		return profile, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return profile, err
	}

	profile.KPIs, err = m.kpis(ctx, call, profile.OrganizationID)

	return profile, err
}

// contractorFilter is the filters of contractors.list.
type contractorFilter struct {
	Status *string `json:"status,omitempty"`
	Search *string `json:"search,omitempty"` // name, registration number or a service area contains
}

// contractorFilterOf decodes and checks the filters of contractors.list.
func contractorFilterOf(raw json.RawMessage) (contractorFilter, error) {
	var filter contractorFilter

	if len(raw) == 0 {
		return filter, nil
	}

	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()

	if dec.Decode(&filter) != nil ||
		(filter.Status != nil && *filter.Status != contractorActive && *filter.Status != contractorSuspended) {
		return contractorFilter{}, apperr.Fields(
			map[string]string{fieldFilters: apperr.KeyInvalid},
		)
	}

	return filter, nil
}

// where is the filter's WHERE clause, with its arguments.
func (filter contractorFilter) where(call *ops.Call) (string, []any) {
	args := []any{call.Now}
	conds := []string{sqlTrue}

	if filter.Status != nil {
		args = append(args, *filter.Status)
		conds = append(conds, fmt.Sprintf("p.status = $%d", len(args)))
	}

	if filter.Search != nil && strings.TrimSpace(*filter.Search) != "" {
		args = append(args, paging.Contains(*filter.Search))
		searchArg := len(args)
		conds = append(
			conds,
			fmt.Sprintf(
				"(p.name ILIKE $%d OR p.registration_no ILIKE $%d OR EXISTS (SELECT 1 FROM unnest(p.service_areas) a WHERE a "+
					"ILIKE $%d))",
				searchArg,
				searchArg,
				searchArg,
			),
		)
	}

	return strings.Join(conds, " AND "), args
}

// list serves contractors.list.
//
//	@Summary		contractors.list (read)
//	@ID				contractors.list
//	@Description	Authorization: admin:job.read
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A21 · Query: filters status,search · sort id,name,updatedAt (default name asc;id asc)
//	@Tags			contractors
//	@Accept			json
//	@Produce		json
//	@Param			cursor	query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit	query		integer	false	"page size 1–100, default 25"
//	@Param			sort	query		string	false	"field:direction — fields id,name,updatedAt; default name asc;id asc"
//	@Param			status	query		string	false	"filter → status"
//	@Param			search	query		string	false	"filter → name, registrationNo or service area contains"
//	@Success		200		{object}	ops.Envelope{data=ProfilePage}
//	@Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504		{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/contractors [get]
func (m Partners) list(
	ctx context.Context,
	call *ops.Call,
	query *paging.Query,
) (paging.Page[Profile], error) {
	filters, err := contractorFilterOf(query.Filters)
	if err != nil {
		return paging.Page[Profile]{}, err
	}

	order, err := paging.OrderBy(
		query.Sort,
		map[string]string{"id": "p.id", fieldName: "p.name", fieldUpdatedAt: "p.updated_at"},
		"p.name ASC, p.id ASC",
	)
	if err != nil {
		return paging.Page[Profile]{}, apperr.Wrap(err, "paging.OrderBy")
	}

	window, err := paging.Resolve(*query, filters, call.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Profile]{}, apperr.Wrap(err, "paging.Resolve")
	}

	where, args := filters.where(call)

	var total int

	scanErr := call.Tx.QueryRow(ctx, "SELECT count(*) FROM maintenance.contractors p WHERE "+where+" AND $1::timestamptz "+
		"IS NOT NULL", args...).
		Scan(&total)
	if scanErr != nil {
		return paging.Page[Profile]{}, apperr.Wrap(scanErr, "select maintenance.contractors")
	}

	rows, err := call.Tx.Query(
		ctx,
		fmt.Sprintf(
			"SELECT %s FROM maintenance.contractors p WHERE %s ORDER BY %s LIMIT %d OFFSET %d",
			profileCols,
			where,
			order,
			window.Limit,
			window.Offset,
		),
		args...)
	if err != nil {
		return paging.Page[Profile]{}, apperr.Wrap(err, "select maintenance.contractors")
	}

	items := []Profile{}

	for rows.Next() {
		profile, err := scanProfile(rows)
		if err != nil {
			rows.Close()

			return paging.Page[Profile]{}, err
		}

		items = append(items, profile)
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return paging.Page[Profile]{}, apperr.Wrap(rowsErr, "select maintenance.contractors")
	}

	for i := range items {
		items[i].KPIs, err = m.kpis(ctx, call, items[i].OrganizationID)
		if err != nil {
			return paging.Page[Profile]{}, err
		}
	}

	return paging.Page[Profile]{
		Items:           items,
		NextCursor:      window.Next(total),
		Total:           total,
		SnapshotVersion: window.Snapshot,
	}, nil
}

// ProfileInput is contractors.save input.
type ProfileInput struct {
	ID                  *uuid.UUID `json:"id,omitempty"`
	OrganizationID      uuid.UUID  `json:"organizationId"`
	RegistrationNo      string     `json:"registrationNo"`
	ServiceAreas        []string   `json:"serviceAreas"`
	ContactEmail        string     `json:"contactEmail"`
	InsuranceValidUntil *time.Time `json:"insuranceValidUntil"`
}

// Validate implements ops.Validator (IR131 item 1).
func (in *ProfileInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.OrganizationID == uuid.Nil {
		fieldErrs[fieldOrganizationID] = apperr.KeyRequired
	}

	trimmedReason(&in.RegistrationNo, maxCodeLength, "registrationNo", fieldErrs)

	seen := map[string]bool{}

	if len(in.ServiceAreas) < 1 || len(in.ServiceAreas) > 20 {
		fieldErrs["serviceAreas"] = apperr.KeyCount
	}

	for idx := range in.ServiceAreas {
		in.ServiceAreas[idx] = strings.TrimSpace(in.ServiceAreas[idx])

		n := utf8.RuneCountInString(in.ServiceAreas[idx])
		if n < 1 || n > 80 || seen[strings.ToLower(in.ServiceAreas[idx])] {
			fieldErrs["serviceAreas"] = apperr.KeyInvalid
		}

		seen[strings.ToLower(in.ServiceAreas[idx])] = true
	}

	in.ContactEmail = strings.TrimSpace(in.ContactEmail)

	a, err := mail.ParseAddress(
		in.ContactEmail,
	)
	if err != nil || a.Address != in.ContactEmail ||
		len(in.ContactEmail) > 254 {
		fieldErrs["contactEmail"] = apperr.KeyInvalid
	}

	return fieldErrs
}

// save serves contractors.save.
//
//	@Summary		contractors.save (write)
//	@ID				contractors.save
//	@Description	Authorization: admin:job.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR111 organization kind=contractor; service areas at least one
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A21
//	@Tags			contractors
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		ProfileInput	true	"input"
//	@Success		200				{object}	ops.Envelope{data=Profile}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/contractors [post]
func (m Partners) save(ctx context.Context, call *ops.Call, input *ProfileInput) (Profile, error) {
	name, err := m.contractorName(ctx, call, input.OrganizationID)
	if err != nil {
		return Profile{}, err
	}

	var profileID uuid.UUID

	if input.ID == nil {
		profileID, err = addProfile(ctx, call, input, name)
	} else {
		profileID, err = m.updateProfile(ctx, call, input, name)
	}

	if err != nil {
		return Profile{}, err
	}

	saved, err := m.loadProfile(ctx, call, "p.id = $2", []any{profileID}, false)
	if err != nil {
		return saved, err
	}

	call.Emit(
		outboxEvent(
			"contractor",
			profileID,
			"ContractorSaved",
			map[string]any{fieldOrganizationID: input.OrganizationID},
		),
	)
	call.Audit(
		auditEntry(
			"contractors.save",
			"contractor",
			profileID,
			call.ExpectedVersion,
			&saved.Version,
			"",
		),
	)

	return saved, nil
}

// contractorName is the name of the contractor company a profile is saved for; another kind of organization is refused.
func (m Partners) contractorName(
	ctx context.Context,
	call *ops.Call,
	org uuid.UUID,
) (string, error) {
	state, err := m.Orgs.OrgState(ctx, call, org)
	if err != nil {
		return "", apperr.Wrap(err, "maintenance.OrgNames.OrgState")
	}

	if !state.Found {
		return "", apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if state.Kind != orgKindContractor {
		return "", apperr.Fields(
			map[string]string{fieldOrganizationID: "error.notContractor"},
		)
	}

	name, err := m.Orgs.OrgName(ctx, call, org)

	return name, apperr.Wrap(err, "maintenance.OrgNames.OrgName")
}

// delegationEnd is when a contractor's delegation starting at from ends: with its insurance, or a year on without one.
func delegationEnd(insuranceValidUntil *time.Time, from time.Time) time.Time {
	if insuranceValidUntil != nil && insuranceValidUntil.After(from) {
		return *insuranceValidUntil
	}

	return from.Add(365 * 24 * time.Hour)
}

// addProfile creates the company's contractor profile, active from now; a company has one.
func addProfile(
	ctx context.Context,
	call *ops.Call,
	input *ProfileInput,
	name string,
) (uuid.UUID, error) {
	var exists bool

	err := call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM maintenance.contractors WHERE organization_id =
		$1)`, input.OrganizationID).
		Scan(&exists)
	if err != nil {
		return uuid.Nil, apperr.Wrap(err, "select maintenance.contractors")
	}

	if exists {
		return uuid.Nil, apperr.E(apperr.Conflict, "errors.profile_exists")
	}

	var profileID uuid.UUID

	err = call.Tx.QueryRow(ctx, `INSERT INTO maintenance.contractors (tenant_id, organization_id, name, status,
		registration_no, service_areas, contact_email,
		insurance_valid_until, delegation, created_at, updated_at) VALUES (current_setting('app.tenant_id')::uuid, $1, $2,
		'active', $3, $4, $5, $6, tstzrange($7, $8), $7, $7)
		RETURNING
		id`, input.OrganizationID, name, input.RegistrationNo, input.ServiceAreas, input.ContactEmail,
		input.InsuranceValidUntil, call.Now,
		delegationEnd(input.InsuranceValidUntil, call.Now)).
		Scan(&profileID)

	return profileID, apperr.Wrap(err, "insert maintenance.contractors")
}

// updateProfile saves the company's profile at the version shown; its organization stays as it is.
func (m Partners) updateProfile(
	ctx context.Context,
	call *ops.Call,
	input *ProfileInput,
	name string,
) (uuid.UUID, error) {
	profile, err := m.loadProfile(ctx, call, "p.id = $2", []any{*input.ID}, true)
	if err != nil {
		return uuid.Nil, err
	}

	if profile.OrganizationID != input.OrganizationID {
		return uuid.Nil, apperr.Fields(
			map[string]string{fieldOrganizationID: "error.organizationFixed"},
		)
	}

	if profile.Version != *call.ExpectedVersion {
		return uuid.Nil, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE maintenance.contractors SET name = $2, registration_no = $3, service_areas = $4, contact_email = $5,
			insurance_valid_until = $6,
		delegation = tstzrange(lower(delegation), $7), version = version + 1, updated_at = $8 WHERE id = $1`,
		profile.ID,
		name,
		input.RegistrationNo,
		input.ServiceAreas,
		input.ContactEmail,
		input.InsuranceValidUntil,
		delegationEnd(input.InsuranceValidUntil, profile.CreatedAt),
		call.Now,
	)
	if execErr != nil {
		return uuid.Nil, apperr.Wrap(execErr, "update maintenance.contractors")
	}

	return profile.ID, nil
}

// The offer statuses of a contractor profile: an active contractor is offered jobs, a suspended one is not.
const (
	contractorActive    = "active"
	contractorSuspended = "suspended"
)

// OfferStatusInput is contractors.setOfferStatus input.
type OfferStatusInput struct {
	ContractorOrgID uuid.UUID `json:"contractorOrgId"`
	Status          string    `json:"status"`
	Reason          string    `json:"reason"`
}

// Validate implements ops.Validator.
func (in *OfferStatusInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.ContractorOrgID == uuid.Nil {
		fieldErrs["contractorOrgId"] = apperr.KeyRequired
	}

	if in.Status != contractorActive && in.Status != contractorSuspended {
		fieldErrs[fieldStatus] = apperr.KeyInvalid
	}

	trimmedReason(&in.Reason, maxReasonLength, fieldReason, fieldErrs)

	return fieldErrs
}

// setOfferStatus serves contractors.setOfferStatus.
//
//	@Summary		contractors.setOfferStatus (write)
//	@ID				contractors.setOfferStatus
//	@Description	Authorization: admin:job.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR111 reason 1–1000; suspension blocks new jobs.offer only
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A21
//	@Tags			contractors
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string				true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer				true	"all: required (target contractor, read contractors.list)"
//	@Param			contractorOrgId		path		string				true	"input field contractorOrgId"
//	@Param			request				body		OfferStatusInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=Profile}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/contractors/{contractorOrgId}/offer-status [put]
func (m Partners) setOfferStatus(
	ctx context.Context,
	call *ops.Call,
	input *OfferStatusInput,
) (Profile, error) {
	profile, err := m.loadProfile(
		ctx,
		call,
		"p.organization_id = $2",
		[]any{input.ContractorOrgID},
		true,
	)
	if err != nil {
		return profile, err
	}

	if profile.Version != *call.ExpectedVersion {
		return profile, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	if profile.Status == input.Status {
		return profile, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	}

	var reason *string
	if input.Status == contractorSuspended {
		reason = &input.Reason
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE maintenance.contractors SET status = $2, suspended_reason = $3, version = version + 1, updated_at = $4 WHERE
			id = $1`,
		profile.ID,
		input.Status,
		reason,
		call.Now,
	)
	if execErr != nil {
		return profile, apperr.Wrap(execErr, "update maintenance.contractors")
	}

	profile, err = m.loadProfile(ctx, call, "p.id = $2", []any{profile.ID}, false)
	if err != nil {
		return profile, err
	}

	call.Emit(
		outboxEvent(
			"contractor",
			profile.ID,
			"ContractorOfferStatusChanged",
			map[string]any{fieldStatus: input.Status},
		),
	)
	call.Audit(
		auditEntry(
			"contractors.setOfferStatus",
			"contractor",
			profile.ID,
			call.ExpectedVersion,
			&profile.Version,
			input.Reason,
		),
	)

	return profile, nil
}

// ---- rate cards ----

// RateLine is one RateCard line.
type RateLine struct {
	WorkType    string  `json:"workType"`
	AmountMinor int     `json:"amountMinor"`
	Note        *string `json:"note"`
}

// RateCard is RateCard of service-contracts.ts.
type RateCard struct {
	ID              uuid.UUID  `json:"id"`
	TenantID        uuid.UUID  `json:"tenantId"`
	Version         int        `json:"version"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
	ContractorOrgID uuid.UUID  `json:"contractorOrgId"`
	EffectiveFrom   time.Time  `json:"effectiveFrom"`
	Currency        string     `json:"currency"`
	Lines           []RateLine `json:"lines"`
}

// workTypes are the kinds of work a rate card prices.
func workTypes() []string {
	return []string{"periodic_inspection", "repair_base", "emergency", "rework_deduction"}
}

// RateCardInput is rateCards.save input.
type RateCardInput struct {
	ContractorOrgID uuid.UUID  `json:"contractorOrgId"`
	EffectiveFrom   time.Time  `json:"effectiveFrom"`
	Currency        string     `json:"currency"`
	Lines           []RateLine `json:"lines"`
}

// Validate implements ops.Validator (IR131 item 3).
func (in *RateCardInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.ContractorOrgID == uuid.Nil {
		fieldErrs["contractorOrgId"] = apperr.KeyRequired
	}

	if in.EffectiveFrom.IsZero() {
		fieldErrs[fieldEffectiveFrom] = apperr.KeyRequired
	}

	if in.Currency != currencyMYR && in.Currency != currencyUSD {
		fieldErrs["currency"] = apperr.KeyInvalid
	}

	if len(in.Lines) < 1 || len(in.Lines) > 4 {
		fieldErrs["lines"] = apperr.KeyCount
	}

	seen := map[string]bool{}

	for i := range in.Lines {
		rateLine := &in.Lines[i]
		if !slices.Contains(workTypes(), rateLine.WorkType) || seen[rateLine.WorkType] ||
			!money.InRange(int64(rateLine.AmountMinor)) {
			fieldErrs["lines"] = apperr.KeyInvalid
		}

		seen[rateLine.WorkType] = true
		if rateLine.Note != nil {
			*rateLine.Note = strings.TrimSpace(*rateLine.Note)
			if utf8.RuneCountInString(*rateLine.Note) > maxRateNoteLength {
				fieldErrs["lines"] = apperr.KeyLength
			}

			if *rateLine.Note == "" {
				rateLine.Note = nil
			}
		}
	}

	return fieldErrs
}

func scanRateCard(row pgx.Row) (RateCard, error) {
	var (
		rateCard RateCard
		lines    []byte
	)

	err := row.Scan(
		&rateCard.ID,
		&rateCard.TenantID,
		&rateCard.Version,
		&rateCard.CreatedAt,
		&rateCard.ContractorOrgID,
		&rateCard.EffectiveFrom,
		&rateCard.Currency,
		&lines,
	)

	rateCard.UpdatedAt = rateCard.CreatedAt
	if err == nil {
		err = json.Unmarshal(lines, &rateCard.Lines)
	}

	return rateCard, apperr.Wrap(err, "json.Unmarshal")
}

const rateCols = `id, tenant_id, version, created_at, contractor_org_id, effective_from, currency, lines`

// saveRateCard serves rateCards.save.
//
//	@Summary		rateCards.save (write)
//	@ID				rateCards.save
//	@Description	Authorization: admin:job.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR111 effectiveFrom in the future; creates a new version
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A21
//	@Tags			rateCards
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		RateCardInput	true	"input"
//	@Success		200				{object}	ops.Envelope{data=RateCard}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/rate-cards [post]
func (Partners) saveRateCard(
	ctx context.Context,
	call *ops.Call,
	input *RateCardInput,
) (RateCard, error) {
	if !input.EffectiveFrom.After(call.Now) {
		return RateCard{}, apperr.Fields(map[string]string{fieldEffectiveFrom: apperr.KeyPast})
	}

	var exists bool

	err := call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM maintenance.contractors WHERE organization_id =
		$1)`, input.ContractorOrgID).
		Scan(&exists)
	if err != nil {
		return RateCard{}, apperr.Wrap(err, "select maintenance.contractors")
	}

	if !exists {
		return RateCard{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	lines, marshalErr := json.Marshal(input.Lines)
	if marshalErr != nil {
		return RateCard{}, apperr.Wrap(marshalErr, "json.Marshal")
	}

	rateCard, err := scanRateCard(
		call.Tx.QueryRow(
			ctx,
			`INSERT INTO maintenance.rate_cards (tenant_id, contractor_org_id, effective_from, currency, lines, version,
				created_at)
		VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4,
			COALESCE((SELECT max(version) FROM maintenance.rate_cards WHERE contractor_org_id = $1), 0) + 1, $5)
		RETURNING `+rateCols,
			input.ContractorOrgID,
			input.EffectiveFrom,
			input.Currency,
			lines,
			call.Now,
		),
	)
	if err != nil {
		return rateCard, err
	}

	call.Emit(
		outboxEvent(
			"rate_card",
			rateCard.ID,
			"RateCardSaved",
			map[string]any{"contractorOrgId": input.ContractorOrgID},
		),
	)
	call.Audit(auditEntry("rateCards.save", "rate_card", rateCard.ID, nil, &rateCard.Version, ""))

	return rateCard, nil
}

// listRateCards serves rateCards.list.
//
//	@Summary		rateCards.list (read)
//	@ID				rateCards.list
//	@Description	Authorization: admin:job.read | admin:billing.read | contractor:own-company
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A21, DD-A23, DD-P10 · Query: filters contractorOrgId · sort id,createdAt,effectiveFrom \
//	@Description	(default effectiveFrom desc;id asc)
//	@Tags			rateCards
//	@Accept			json
//	@Produce		json
//	@Param			cursor			query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit			query		integer	false	"page size 1–100, default 25"
//	@Param			sort			query		string	false	"field:direction — fields id,createdAt,effectiveFrom; default effectiveFrom desc;id asc"
//	@Param			contractorOrgId	query		string	false	"filter → contractorOrgId"
//	@Success		200				{object}	ops.Envelope{data=RateCardPage}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/rate-cards [get]
func (Partners) listRateCards(
	ctx context.Context,
	call *ops.Call,
	query *paging.Query,
) (paging.Page[RateCard], error) {
	var filters struct {
		ContractorOrgID *uuid.UUID `json:"contractorOrgId,omitempty"`
	}

	if len(query.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(query.Filters)))
		dec.DisallowUnknownFields()

		if dec.Decode(&filters) != nil {
			return paging.Page[RateCard]{}, apperr.Fields(
				map[string]string{fieldFilters: apperr.KeyInvalid},
			)
		}
	}

	if call.Principal.Role == roleContractor {
		filters.ContractorOrgID = &call.Principal.OrgID
	}

	order, err := paging.OrderBy(
		query.Sort,
		map[string]string{
			"id":               "id",
			fieldCreatedAt:     "created_at",
			fieldEffectiveFrom: "effective_from",
		},
		"effective_from DESC, id",
	)
	if err != nil {
		return paging.Page[RateCard]{}, apperr.Wrap(err, "paging.OrderBy")
	}

	window, err := paging.Resolve(*query, filters, call.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[RateCard]{}, apperr.Wrap(err, "paging.Resolve")
	}

	where, args := sqlTrue, []any{}
	if filters.ContractorOrgID != nil {
		where, args = "contractor_org_id = $1", append(args, *filters.ContractorOrgID)
	}

	var total int

	scanErr := call.Tx.QueryRow(ctx, "SELECT count(*) FROM maintenance.rate_cards WHERE "+where, args...).
		Scan(&total)
	if scanErr != nil {
		return paging.Page[RateCard]{}, apperr.Wrap(scanErr, "select maintenance.rate_cards")
	}

	rows, err := call.Tx.Query(
		ctx,
		fmt.Sprintf(
			"SELECT %s FROM maintenance.rate_cards WHERE %s ORDER BY %s LIMIT %d OFFSET %d",
			rateCols,
			where,
			order,
			window.Limit,
			window.Offset,
		),
		args...)
	if err != nil {
		return paging.Page[RateCard]{}, apperr.Wrap(err, "select maintenance.rate_cards")
	}
	defer rows.Close()

	items := []RateCard{}

	for rows.Next() {
		x, err := scanRateCard(rows)
		if err != nil {
			return paging.Page[RateCard]{}, err
		}

		items = append(items, x)
	}

	return paging.Page[RateCard]{
		Items:           items,
		NextCursor:      window.Next(total),
		Total:           total,
		SnapshotVersion: window.Snapshot,
	}, apperr.Wrap(rows.Err(), "select maintenance.rate_cards")
}

// ---- SLA ----

// The service profiles the SLA targets are set for (SlaTargets.planType).
const (
	planRTO         = "rto"
	planGeneral     = "general"
	planEnergy      = "energy"
	planEnvironment = "environment"
)

// planTypes lists the service profiles in the order the target views show them.
func planTypes() []string { return []string{planRTO, planGeneral, planEnergy, planEnvironment} }

// The default targets of IR131 item 4.
const (
	defaultResponseHours          = 4
	defaultArrivalInWindowPercent = 90
	defaultFirstTimeFixPercent    = 85
)

// defaultSLA is the target used when no SlaTargets row applies (IR131 item 4).
func defaultSLA() SlaTargets {
	return SlaTargets{
		ResponseHours:          defaultResponseHours,
		ArrivalInWindowPercent: defaultArrivalInWindowPercent,
		FirstTimeFixPercent:    defaultFirstTimeFixPercent,
		ID:                     uuid.UUID{},
		TenantID:               uuid.UUID{},
		Version:                0,
		CreatedAt:              time.Time{},
		UpdatedAt:              time.Time{},
		PlanType:               "",
		EffectiveFrom:          time.Time{},
	}
}

// SlaTargets is SlaTargets of service-contracts.ts.
type SlaTargets struct { //nolint:revive // the name service-contracts.ts and the API description use
	ID                     uuid.UUID `json:"id"`
	TenantID               uuid.UUID `json:"tenantId"`
	Version                int       `json:"version"`
	CreatedAt              time.Time `json:"createdAt"`
	UpdatedAt              time.Time `json:"updatedAt"`
	PlanType               string    `json:"planType"`
	ResponseHours          int       `json:"responseHours"`
	ArrivalInWindowPercent float64   `json:"arrivalInWindowPercent"`
	FirstTimeFixPercent    float64   `json:"firstTimeFixPercent"`
	EffectiveFrom          time.Time `json:"effectiveFrom"`
}

// TargetsInput is sla.saveTargets input.
type TargetsInput struct {
	PlanType               string    `json:"planType"`
	ResponseHours          int       `json:"responseHours"`
	ArrivalInWindowPercent float64   `json:"arrivalInWindowPercent"`
	FirstTimeFixPercent    float64   `json:"firstTimeFixPercent"`
	EffectiveFrom          time.Time `json:"effectiveFrom"`
}

// Validate implements ops.Validator (IR111).
func (in *TargetsInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if !slices.Contains(planTypes(), in.PlanType) {
		fieldErrs["planType"] = apperr.KeyInvalid
	}

	if in.ResponseHours < 1 || in.ResponseHours > 168 {
		fieldErrs["responseHours"] = apperr.KeyRange
	}

	if in.ArrivalInWindowPercent < 0 || in.ArrivalInWindowPercent > 100 {
		fieldErrs["arrivalInWindowPercent"] = apperr.KeyRange
	}

	if in.FirstTimeFixPercent < 0 || in.FirstTimeFixPercent > 100 {
		fieldErrs["firstTimeFixPercent"] = apperr.KeyRange
	}

	if in.EffectiveFrom.IsZero() {
		fieldErrs[fieldEffectiveFrom] = apperr.KeyRequired
	}

	return fieldErrs
}

// saveTargets serves sla.saveTargets.
//
//	@Summary		sla.saveTargets (write)
//	@ID				sla.saveTargets
//	@Description	Authorization: admin:job.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR111 percentages 0–100; responseHours 1–168; applies to jobs created after effectiveFrom
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A22
//	@Tags			sla
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			planType		path		string			true	"input field planType"
//	@Param			request			body		TargetsInput	true	"input; the path parameters come from the route"
//	@Success		200				{object}	ops.Envelope{data=SlaTargets}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/sla/targets/{planType} [put]
func (Partners) saveTargets(
	ctx context.Context,
	call *ops.Call,
	input *TargetsInput,
) (SlaTargets, error) {
	if input.EffectiveFrom.Before(call.Now) {
		return SlaTargets{}, apperr.Fields(map[string]string{fieldEffectiveFrom: apperr.KeyPast})
	}

	var slaTargets SlaTargets

	err := call.Tx.QueryRow(ctx, `INSERT INTO maintenance.sla_targets (tenant_id, plan_type, response_hours,
		arrival_in_window_percent, first_time_fix_percent, effective_from, version)
		VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $5, COALESCE((SELECT max(version) FROM
		maintenance.sla_targets WHERE plan_type = $1), 0) + 1)
		RETURNING id, tenant_id, version, plan_type, response_hours, arrival_in_window_percent::float8,
		first_time_fix_percent::float8, effective_from`,
		input.PlanType, input.ResponseHours, input.ArrivalInWindowPercent, input.FirstTimeFixPercent, input.EffectiveFrom).
		Scan(&slaTargets.ID, &slaTargets.TenantID, &slaTargets.Version, &slaTargets.PlanType, &slaTargets.ResponseHours,
			&slaTargets.ArrivalInWindowPercent, &slaTargets.FirstTimeFixPercent,
			&slaTargets.EffectiveFrom)
	if err != nil {
		if apperr.From(err).Code == apperr.Conflict {
			return slaTargets, apperr.E(apperr.Conflict, "errors.targets_exist")
		}

		return slaTargets, apperr.Wrap(err, "insert maintenance.sla_targets")
	}

	slaTargets.CreatedAt, slaTargets.UpdatedAt = call.Now, call.Now
	call.Emit(
		outboxEvent(
			"sla_targets",
			slaTargets.ID,
			"SlaTargetsSaved",
			map[string]any{"planType": slaTargets.PlanType},
		),
	)
	call.Audit(
		auditEntry("sla.saveTargets", "sla_targets", slaTargets.ID, nil, &slaTargets.Version, ""),
	)

	return slaTargets, nil
}

// targetFor returns the SLA target for a plan type at a job's creation (IR131 item 4).
func targetFor(all []SlaTargets, plan string, at time.Time) SlaTargets {
	best := defaultSLA()

	var bestAt *time.Time
	for _, t := range all {
		if t.PlanType == plan && !t.EffectiveFrom.After(at) &&
			(bestAt == nil || t.EffectiveFrom.After(*bestAt)) {
			e := t.EffectiveFrom
			best, bestAt = t, &e
		}
	}

	return best
}

// The states of a target view (SlaTargetView.state, IR236): a saved row in effect, the IR131 default in effect, or a
// saved row that starts later.
const (
	targetsInEffect  = "in_effect"
	targetsDefault   = "default"
	targetsScheduled = "scheduled"
)

// targetViews lists, per plan type, the targets in effect at now (a saved row, or the default with no version) and then
// the saved rows that start later, earliest first (IR236).
func targetViews(all []SlaTargets, now time.Time) []TargetView {
	out := []TargetView{}
	view := func(slaTargets SlaTargets, state string) TargetView {
		targetView := TargetView{
			PlanType:               slaTargets.PlanType,
			ResponseHours:          slaTargets.ResponseHours,
			ArrivalInWindowPercent: slaTargets.ArrivalInWindowPercent,
			FirstTimeFixPercent:    slaTargets.FirstTimeFixPercent,
			Version:                slaTargets.Version,
			State:                  state, EffectiveFrom: nil,
		}
		if state != targetsDefault {
			e := slaTargets.EffectiveFrom
			targetView.EffectiveFrom = &e
		}

		return targetView
	}

	for _, plan := range planTypes() {
		cur := targetFor(all, plan, now)
		if cur.EffectiveFrom.IsZero() {
			cur.PlanType = plan
			out = append(out, view(cur, targetsDefault))
		} else {
			out = append(out, view(cur, targetsInEffect))
		}
	}

	later := []SlaTargets{}

	for _, t := range all {
		if t.EffectiveFrom.After(now) {
			later = append(later, t)
		}
	}

	sort.SliceStable(
		later,
		func(i, k int) bool { return later[i].EffectiveFrom.Before(later[k].EffectiveFrom) },
	)

	for _, t := range later {
		out = append(out, view(t, targetsScheduled))
	}

	return out
}

// Metrics is SlaMetrics.
type Metrics struct {
	ResponseWithinTarget *float64 `json:"responseWithinTarget"`
	ArrivalInWindow      *float64 `json:"arrivalInWindow"`
	FirstTimeFix         *float64 `json:"firstTimeFix"`
	AverageRating        *float64 `json:"averageRating"`
	RatingCount          int      `json:"ratingCount"`
	OpenOverdue          int      `json:"openOverdue"`
}

// CustomerMetrics is one scorecard customer row; PlanType is the customer's service profile, whose targets decide the
// status (IR236).
type CustomerMetrics struct {
	Metrics

	CustomerID uuid.UUID `json:"customerId"`
	PlanType   string    `json:"planType"`
	JobCount   int       `json:"jobCount"`
	Status     string    `json:"status"`
}

// TargetView is SlaTargetView (IR236): a plan type's targets in effect now — a saved row or the IR131 default — or a
// saved row that starts later.
type TargetView struct {
	PlanType               string     `json:"planType"`
	ResponseHours          int        `json:"responseHours"`
	ArrivalInWindowPercent float64    `json:"arrivalInWindowPercent"`
	FirstTimeFixPercent    float64    `json:"firstTimeFixPercent"`
	Version                int        `json:"version"`
	EffectiveFrom          *time.Time `json:"effectiveFrom"`
	State                  string     `json:"state"` // in_effect | default | scheduled
}

// Breach is one scorecard breach. Detail is English for logs; screens phrase the kind with the response minutes in
// the user's language (IR291).
type Breach struct {
	JobID       uuid.UUID `json:"jobId"`
	CustomerID  uuid.UUID `json:"customerId"`
	Kind        string    `json:"kind"`
	Detail      string    `json:"detail"`
	TookMinutes *int      `json:"tookMinutes"` // response: request → response; null while unanswered and for other kinds
	// response: the target in effect when the job was created; null for other kinds
	LimitMinutes *int `json:"limitMinutes"`
	at           time.Time
}

// Scorecard is SlaScorecard.
type Scorecard struct {
	Period          Range             `json:"period"`
	ContractorOrgID *uuid.UUID        `json:"contractorOrgId"`
	Totals          Metrics           `json:"totals"`
	Customers       []CustomerMetrics `json:"customers"`
	Breaches        []Breach          `json:"breaches"`
	Targets         []TargetView      `json:"targets"`
}

// ScorecardInput is sla.scorecard input.
type ScorecardInput struct {
	Period          Range      `json:"period"`
	ContractorOrgID *uuid.UUID `json:"contractorOrgId,omitempty"`
}

// Validate implements ops.Validator.
func (in *ScorecardInput) Validate() map[string]string {
	if in.Period.From.IsZero() || in.Period.To.IsZero() || !in.Period.From.Before(in.Period.To) ||
		in.Period.To.Sub(in.Period.From) > 366*24*time.Hour {
		return map[string]string{"period": apperr.KeyRange}
	}

	return nil
}

type tally struct {
	respN, respOK, arrN, arrOK, ftfN, ftfOK, stars, rated, overdue, jobs int
}

func (t *tally) metrics() Metrics {
	metrics := Metrics{
		ResponseWithinTarget: pct(t.respOK, t.respN),
		ArrivalInWindow:      pct(t.arrOK, t.arrN),
		FirstTimeFix:         pct(t.ftfOK, t.ftfN),
		RatingCount:          t.rated,
		OpenOverdue:          t.overdue, AverageRating: nil,
	}
	if t.rated > 0 {
		v := averageRating(t.stars, t.rated)
		metrics.AverageRating = &v
	}

	return metrics
}

// minutesPerHour splits a duration's minutes from its hours.
const minutesPerHour = 60

func durationText(d time.Duration) string {
	h, mnt := int(d.Hours()), int(d.Minutes())%minutesPerHour
	if mnt == 0 {
		return fmt.Sprintf("%d h", h)
	}

	return fmt.Sprintf("%d h %d min", h, mnt)
}

// scorecard serves sla.scorecard.
//
//	@Summary		sla.scorecard (read)
//	@ID				sla.scorecard
//	@Description	Authorization: admin:job.read
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A22
//	@Tags			sla
//	@Accept			json
//	@Produce		json
//	@Param			period.from		query		string	false	"input field period.from"
//	@Param			period.to		query		string	false	"input field period.to"
//	@Param			contractorOrgId	query		string	false	"input field contractorOrgId"
//	@Success		200				{object}	ops.Envelope{data=Scorecard}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/sla/scorecard [get]
func (m Partners) scorecard(
	ctx context.Context,
	call *ops.Call,
	input *ScorecardInput,
) (Scorecard, error) {
	extra, args := sqlTrue, []any{input.Period.From, input.Period.To}
	if input.ContractorOrgID != nil {
		extra, args = "j.contractor_org_id = $3", append(args, *input.ContractorOrgID)
	}

	jobs, err := jobMetrics(ctx, call, extra, args)
	if err != nil {
		return Scorecard{}, err
	}

	targets, err := slaTargets(ctx, call)
	if err != nil {
		return Scorecard{}, err
	}

	orgs := []uuid.UUID{}
	for _, j := range jobs {
		orgs = append(orgs, j.customerOrg)
	}

	profiles, err := m.Customers.CustomerProfiles(ctx, call, orgs)
	if err != nil {
		return Scorecard{}, apperr.Wrap(err, "maintenance.CustomerProfiles.CustomerProfiles")
	}

	tallied := newScoring(call.Now, targets)
	for _, metric := range jobs {
		tallied.add(metric, profiles[metric.customerOrg])
	}

	return tallied.scorecard(input), nil
}

// slaTargets are every version of every plan's SLA targets.
func slaTargets(ctx context.Context, call *ops.Call) ([]SlaTargets, error) {
	var targets []SlaTargets

	rows, err := call.Tx.Query(
		ctx,
		`SELECT plan_type, response_hours, arrival_in_window_percent::float8, first_time_fix_percent::float8, effective_from,
			version FROM maintenance.sla_targets`,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select maintenance.sla_targets")
	}

	for rows.Next() {
		var slaTargets SlaTargets

		err := rows.Scan(
			&slaTargets.PlanType,
			&slaTargets.ResponseHours,
			&slaTargets.ArrivalInWindowPercent,
			&slaTargets.FirstTimeFixPercent,
			&slaTargets.EffectiveFrom,
			&slaTargets.Version,
		)
		if err != nil {
			rows.Close()

			return nil, apperr.Wrap(err, "select maintenance.sla_targets")
		}

		targets = append(targets, slaTargets)
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return nil, apperr.Wrap(rowsErr, "select maintenance.sla_targets")
	}

	return targets, nil
}

// scoring tallies the SLA measures of a scorecard's jobs, in total and per customer in the order the customers first
// appear, and lists the breaches.
type scoring struct {
	now      time.Time
	targets  []SlaTargets
	total    *tally
	per      map[uuid.UUID]*tally
	planOf   map[uuid.UUID]string
	order    []uuid.UUID
	breaches []Breach
}

// newScoring starts the tallies of a scorecard at now against the plans' targets.
func newScoring(now time.Time, targets []SlaTargets) *scoring {
	return &scoring{
		now:      now,
		targets:  targets,
		total:    new(tally),
		per:      map[uuid.UUID]*tally{},
		planOf:   map[uuid.UUID]string{},
		order:    nil,
		breaches: []Breach{},
	}
}

// add counts one job for its customer and in total against the target in effect when the job was created, and lists
// its breaches: of the response, the arrival, the first-time fix and the due time, in that order.
func (sc *scoring) add(metric jobMetric, profile [2]string) {
	cust, _ := uuid.Parse(profile[0])
	if sc.per[cust] == nil {
		sc.per[cust] = new(tally)
		sc.planOf[cust] = profile[1]
		sc.order = append(sc.order, cust)
	}

	tallies := []*tally{sc.total, sc.per[cust]}
	for _, t := range tallies {
		t.jobs++
	}

	target := targetFor(sc.targets, profile[1], metric.createdAt)
	responseBreach := sc.response(metric, target, tallies)
	arrivalBreach := arrival(metric, tallies)
	fixBreach := firstTimeFix(metric, tallies)
	rating(metric, tallies)
	overdueBreach := sc.overdue(metric, tallies)

	for _, breach := range []*Breach{responseBreach, arrivalBreach, fixBreach, overdueBreach} {
		if breach != nil {
			breach.JobID, breach.CustomerID, breach.at = metric.id, cust, metric.createdAt
			sc.breaches = append(sc.breaches, *breach)
		}
	}
}

// response counts the job's response against the plan's response hours. A plan's visit is generated ahead of its date,
// so the response target applies to requests only (IR451); a job cancelled before its response was due is neither
// within the target nor a miss (IR291).
func (sc *scoring) response(metric jobMetric, target SlaTargets, tallies []*tally) *Breach {
	limit := time.Duration(target.ResponseHours) * time.Hour
	limitMin := int(limit.Minutes())

	switch {
	case metric.planned:
		return nil
	case metric.responseAt != nil:
		onTime := metric.responseAt.Sub(metric.createdAt) <= limit

		for _, t := range tallies {
			t.respN++
			if onTime {
				t.respOK++
			}
		}

		if onTime {
			return nil
		}

		took := int(metric.responseAt.Sub(metric.createdAt).Minutes())

		return &Breach{
			Kind: "response",
			Detail: "response " + durationText(
				metric.responseAt.Sub(metric.createdAt),
			) + " vs " + durationText(
				limit,
			),
			TookMinutes:  &took,
			LimitMinutes: &limitMin,
			JobID:        uuid.UUID{},
			CustomerID:   uuid.UUID{},
			at:           time.Time{},
		}
	case metric.cancelledAt != nil && metric.cancelledAt.Sub(metric.createdAt) <= limit:
		return nil
	case sc.now.Sub(metric.createdAt) > limit:
		for _, t := range tallies {
			t.respN++
		}

		return &Breach{
			Kind:         "response",
			Detail:       "no response within " + durationText(limit),
			LimitMinutes: &limitMin,
			JobID:        uuid.UUID{},
			CustomerID:   uuid.UUID{},
			TookMinutes:  nil,
			at:           time.Time{},
		}
	}

	return nil
}

// arrival counts whether the technician arrived within the scheduled window.
func arrival(metric jobMetric, tallies []*tally) *Breach {
	if metric.arrivedAt == nil {
		return nil
	}

	for _, t := range tallies {
		t.arrN++
		if metric.inWindow {
			t.arrOK++
		}
	}

	if metric.inWindow {
		return nil
	}

	return &Breach{
		Kind:         "arrival",
		Detail:       "arrival outside the scheduled window",
		JobID:        uuid.UUID{},
		CustomerID:   uuid.UUID{},
		TookMinutes:  nil,
		LimitMinutes: nil,
		at:           time.Time{},
	}
}

// firstTimeFix counts whether a completed job was fixed the first time: its report never sent back, and no follow-up
// within 30 days.
func firstTimeFix(metric jobMetric, tallies []*tally) *Breach {
	if metric.status != statusCompleted {
		return nil
	}

	fixedFirstTime := !metric.everReturned && !metric.followUpWithin30

	for _, t := range tallies {
		t.ftfN++
		if fixedFirstTime {
			t.ftfOK++
		}
	}

	if fixedFirstTime {
		return nil
	}

	return &Breach{
		Kind:         "first_time_fix",
		Detail:       "not fixed the first time",
		JobID:        uuid.UUID{},
		CustomerID:   uuid.UUID{},
		TookMinutes:  nil,
		LimitMinutes: nil,
		at:           time.Time{},
	}
}

// rating counts the customer's stars for the job, when it was rated.
func rating(metric jobMetric, tallies []*tally) {
	if metric.stars <= 0 {
		return
	}

	for _, t := range tallies {
		t.rated++
		t.stars += metric.stars
	}
}

// overdue counts a job still open past its due time.
func (sc *scoring) overdue(metric jobMetric, tallies []*tally) *Breach {
	if metric.status == statusCompleted || metric.status == statusCancelled ||
		!sc.now.After(metric.dueAt) {
		return nil
	}

	for _, t := range tallies {
		t.overdue++
	}

	return &Breach{
		Kind:         "overdue",
		Detail:       "open past its due time",
		JobID:        uuid.UUID{},
		CustomerID:   uuid.UUID{},
		TookMinutes:  nil,
		LimitMinutes: nil,
		at:           time.Time{},
	}
}

// scorecard is the tallies as the scorecard answers them: the totals, each customer's measures and standing against
// its plan's targets now (DD-A22), and the newest breaches first, at most maxBreaches.
func (sc *scoring) scorecard(input *ScorecardInput) Scorecard {
	out := Scorecard{
		Period:          input.Period,
		ContractorOrgID: input.ContractorOrgID,
		Customers:       []CustomerMetrics{},
		Breaches:        sc.breaches,
		Targets:         targetViews(sc.targets, sc.now),
		Totals:          sc.total.metrics(),
	}

	for _, cust := range sc.order {
		counts := sc.per[cust]
		metrics := counts.metrics()
		out.Customers = append(
			out.Customers,
			CustomerMetrics{
				Metrics:    metrics,
				CustomerID: cust,
				PlanType:   sc.planOf[cust],
				JobCount:   counts.jobs,
				Status:     customerStatus(metrics, targetFor(sc.targets, sc.planOf[cust], sc.now)),
			},
		)
	}

	sort.SliceStable(
		out.Breaches,
		func(i, k int) bool { return out.Breaches[i].at.After(out.Breaches[k].at) },
	)

	if len(out.Breaches) > maxBreaches {
		out.Breaches = out.Breaches[:maxBreaches]
	}

	return out
}

// maxBreaches is how many SLA breaches a scorecard lists, the newest first.
const maxBreaches = 50

// The response share every plan holds a customer to (100 %), and how far below its target a share is still at risk
// rather than breached (10 points).
const (
	fullShare  = 100
	riskMargin = 10
)

// The SLA standings of a customer (CustomerMetrics.status, DD-A22).
const (
	slaOnTrack  = "on_track"
	slaAtRisk   = "at_risk"
	slaBreached = "breached"
)

// customerStatus is a customer's SLA standing (DD-A22): every response is due within the plan's hours, so the response
// share is held to 100 %, arrival in window and first-time fix to the plan's targets. A share at or above its target
// is on track; up to 10 points below is at risk, more than 10 below — or any open overdue job — is breached. A share
// without jobs to measure says nothing.
func customerStatus(metrics Metrics, tgt SlaTargets) string {
	status := slaOnTrack
	check := func(v *float64, target float64) {
		if v == nil || *v >= target {
			return
		}

		if target-*v > riskMargin {
			status = slaBreached
		} else if status != slaBreached {
			status = slaAtRisk
		}
	}
	check(metrics.ResponseWithinTarget, fullShare)
	check(metrics.ArrivalInWindow, tgt.ArrivalInWindowPercent)
	check(metrics.FirstTimeFix, tgt.FirstTimeFixPercent)

	if metrics.OpenOverdue > 0 {
		status = slaBreached
	}

	return status
}

// RegisterPartners binds the IR131 operations.
func RegisterPartners(reg *ops.Registry, partners Partners) {
	ops.Register(reg, "contractors.list", partners.list)
	ops.Register(reg, "contractors.save", partners.save)
	ops.Register(reg, "contractors.setOfferStatus", partners.setOfferStatus)
	ops.Register(reg, "rateCards.save", partners.saveRateCard)
	ops.Register(reg, "rateCards.list", partners.listRateCards)
	ops.Register(reg, "sla.saveTargets", partners.saveTargets)
	ops.Register(reg, "sla.scorecard", partners.scorecard)
}
