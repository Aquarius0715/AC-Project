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

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

// OrgNames resolves organization names (Identity).
type OrgNames interface {
	OrgState(
		ctx context.Context,
		c *ops.Call,
		org uuid.UUID,
	) (kind, status string, found bool, err error)
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

func pct(n, d int) *float64 {
	if d == 0 {
		return nil
	}

	v := math.Round(float64(n)*1000/float64(d)) / 10

	return &v
}

// kpis computes ContractorKpis for the 90 days ending now (IR131 item 6).
func (m Partners) kpis(ctx context.Context, call *ops.Call, org uuid.UUID) (KPIs, error) {
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

			if metric.firstReview == "accept" {
				firstOK++
			}
		}

		if metric.status == "completed" {
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
		v := math.Round(float64(stars)*10/float64(kpIs.RatingCount)) / 10
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
		q += " FOR UPDATE"
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
	var filters struct {
		Status *string `json:"status,omitempty"`
		Search *string `json:"search,omitempty"` // name, registration number or a service area contains
	}

	if len(query.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(query.Filters)))
		dec.DisallowUnknownFields()

		if dec.Decode(&filters) != nil ||
			(filters.Status != nil && *filters.Status != "active" && *filters.Status != "suspended") {
			return paging.Page[Profile]{}, apperr.Fields(
				map[string]string{"filters": apperr.KeyInvalid},
			)
		}
	}

	order, err := paging.OrderBy(
		query.Sort,
		map[string]string{"id": "p.id", "name": "p.name", "updatedAt": "p.updated_at"},
		"p.name ASC, p.id ASC",
	)
	if err != nil {
		return paging.Page[Profile]{}, apperr.Wrap(err, "paging.OrderBy")
	}

	window, err := paging.Resolve(*query, filters, call.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Profile]{}, apperr.Wrap(err, "paging.Resolve")
	}

	args := []any{call.Now}
	conds := []string{"TRUE"}

	if filters.Status != nil {
		args = append(args, *filters.Status)
		conds = append(conds, fmt.Sprintf("p.status = $%d", len(args)))
	}

	if filters.Search != nil && strings.TrimSpace(*filters.Search) != "" {
		args = append(args, paging.Contains(*filters.Search))
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

	where := strings.Join(conds, " AND ")

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
		fieldErrs["organizationId"] = apperr.KeyRequired
	}

	trimmedReason(&in.RegistrationNo, 64, "registrationNo", fieldErrs)

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
	kind, _, found, err := m.Orgs.OrgState(ctx, call, input.OrganizationID)
	if err != nil {
		return Profile{}, apperr.Wrap(err, "maintenance.OrgNames.OrgState")
	}

	if !found {
		return Profile{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if kind != "contractor" {
		return Profile{}, apperr.Fields(map[string]string{"organizationId": "error.notContractor"})
	}

	name, err := m.Orgs.OrgName(ctx, call, input.OrganizationID)
	if err != nil {
		return Profile{}, apperr.Wrap(err, "maintenance.OrgNames.OrgName")
	}

	delegationEnd := func(from time.Time) time.Time {
		if input.InsuranceValidUntil != nil && input.InsuranceValidUntil.After(from) {
			return *input.InsuranceValidUntil
		}

		return from.Add(365 * 24 * time.Hour)
	}

	var profileID uuid.UUID

	if input.ID == nil {
		var exists bool

		err := call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM maintenance.contractors WHERE organization_id =
			$1)`, input.OrganizationID).
			Scan(&exists)
		if err != nil {
			return Profile{}, apperr.Wrap(err, "select maintenance.contractors")
		}

		if exists {
			return Profile{}, apperr.E(apperr.Conflict, "errors.profile_exists")
		}

		err = call.Tx.QueryRow(ctx, `INSERT INTO maintenance.contractors (tenant_id, organization_id, name, status,
			registration_no, service_areas, contact_email,
			insurance_valid_until, delegation, created_at, updated_at) VALUES (current_setting('app.tenant_id')::uuid, $1, $2,
			'active', $3, $4, $5, $6, tstzrange($7, $8), $7, $7)
			RETURNING
			id`, input.OrganizationID, name, input.RegistrationNo, input.ServiceAreas, input.ContactEmail,
			input.InsuranceValidUntil, call.Now,
			delegationEnd(call.Now)).
			Scan(&profileID)
		if err != nil {
			return Profile{}, apperr.Wrap(err, "insert maintenance.contractors")
		}
	} else {
		profile, err := m.loadProfile(ctx, call, "p.id = $2", []any{*input.ID}, true)
		if err != nil {
			return profile, err
		}

		if profile.OrganizationID != input.OrganizationID {
			return profile, apperr.Fields(
				map[string]string{"organizationId": "error.organizationFixed"},
			)
		}

		if profile.Version != *call.ExpectedVersion {
			return profile, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
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
			delegationEnd(profile.CreatedAt),
			call.Now,
		)
		if execErr != nil {
			return profile, apperr.Wrap(execErr, "update maintenance.contractors")
		}

		profileID = profile.ID
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
			map[string]any{"organizationId": input.OrganizationID},
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

	if in.Status != "active" && in.Status != "suspended" {
		fieldErrs["status"] = apperr.KeyInvalid
	}

	trimmedReason(&in.Reason, 1000, "reason", fieldErrs)

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
	if input.Status == "suspended" {
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
			map[string]any{"status": input.Status},
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

var workTypes = []string{"periodic_inspection", "repair_base", "emergency", "rework_deduction"}

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
		fieldErrs["effectiveFrom"] = apperr.KeyRequired
	}

	if in.Currency != "MYR" && in.Currency != "USD" {
		fieldErrs["currency"] = apperr.KeyInvalid
	}

	if len(in.Lines) < 1 || len(in.Lines) > 4 {
		fieldErrs["lines"] = apperr.KeyCount
	}

	seen := map[string]bool{}

	for i := range in.Lines {
		rateLine := &in.Lines[i]
		if !slices.Contains(workTypes, rateLine.WorkType) || seen[rateLine.WorkType] ||
			rateLine.AmountMinor < 0 {
			fieldErrs["lines"] = apperr.KeyInvalid
		}

		seen[rateLine.WorkType] = true
		if rateLine.Note != nil {
			*rateLine.Note = strings.TrimSpace(*rateLine.Note)
			if utf8.RuneCountInString(*rateLine.Note) > 200 {
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
func (m Partners) saveRateCard(
	ctx context.Context,
	call *ops.Call,
	input *RateCardInput,
) (RateCard, error) {
	if !input.EffectiveFrom.After(call.Now) {
		return RateCard{}, apperr.Fields(map[string]string{"effectiveFrom": apperr.KeyPast})
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

	lines, _ := json.Marshal(input.Lines)

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
func (m Partners) listRateCards(
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
				map[string]string{"filters": apperr.KeyInvalid},
			)
		}
	}

	if call.Principal.Role == "contractor" {
		filters.ContractorOrgID = &call.Principal.OrgID
	}

	order, err := paging.OrderBy(
		query.Sort,
		map[string]string{"id": "id", "createdAt": "created_at", "effectiveFrom": "effective_from"},
		"effective_from DESC, id",
	)
	if err != nil {
		return paging.Page[RateCard]{}, apperr.Wrap(err, "paging.OrderBy")
	}

	window, err := paging.Resolve(*query, filters, call.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[RateCard]{}, apperr.Wrap(err, "paging.Resolve")
	}

	where, args := "TRUE", []any{}
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

var planTypes = []string{"rto", "general", "energy", "environment"}

// DefaultSLA is the target used when no SlaTargets row applies (IR131 item 4).
var DefaultSLA = SlaTargets{
	ResponseHours:          4,
	ArrivalInWindowPercent: 90,
	FirstTimeFixPercent:    85,
	ID:                     uuid.UUID{},
	TenantID:               uuid.UUID{},
	Version:                0,
	CreatedAt:              time.Time{},
	UpdatedAt:              time.Time{},
	PlanType:               "",
	EffectiveFrom:          time.Time{},
}

// SlaTargets is SlaTargets of service-contracts.ts.
type SlaTargets struct {
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
	if !slices.Contains(planTypes, in.PlanType) {
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
		fieldErrs["effectiveFrom"] = apperr.KeyRequired
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
func (m Partners) saveTargets(
	ctx context.Context,
	call *ops.Call,
	input *TargetsInput,
) (SlaTargets, error) {
	if input.EffectiveFrom.Before(call.Now) {
		return SlaTargets{}, apperr.Fields(map[string]string{"effectiveFrom": apperr.KeyPast})
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
	best := DefaultSLA

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
		if state != "default" {
			e := slaTargets.EffectiveFrom
			targetView.EffectiveFrom = &e
		}

		return targetView
	}

	for _, plan := range planTypes {
		cur := targetFor(all, plan, now)
		if cur.EffectiveFrom.IsZero() {
			cur.PlanType = plan
			out = append(out, view(cur, "default"))
		} else {
			out = append(out, view(cur, "in_effect"))
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
		out = append(out, view(t, "scheduled"))
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
		v := math.Round(float64(t.stars)*10/float64(t.rated)) / 10
		metrics.AverageRating = &v
	}

	return metrics
}

func durationText(d time.Duration) string {
	h, mnt := int(d.Hours()), int(d.Minutes())%60
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
	extra, args := "TRUE", []any{input.Period.From, input.Period.To}
	if input.ContractorOrgID != nil {
		extra, args = "j.contractor_org_id = $3", append(args, *input.ContractorOrgID)
	}

	jobs, err := jobMetrics(ctx, call, extra, args)
	if err != nil {
		return Scorecard{}, err
	}

	var targets []SlaTargets

	rows, err := call.Tx.Query(
		ctx,
		`SELECT plan_type, response_hours, arrival_in_window_percent::float8, first_time_fix_percent::float8, effective_from,
			version FROM maintenance.sla_targets`,
	)
	if err != nil {
		return Scorecard{}, apperr.Wrap(err, "select maintenance.sla_targets")
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

			return Scorecard{}, apperr.Wrap(err, "select maintenance.sla_targets")
		}

		targets = append(targets, slaTargets)
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return Scorecard{}, apperr.Wrap(rowsErr, "select maintenance.sla_targets")
	}

	orgs := []uuid.UUID{}
	for _, j := range jobs {
		orgs = append(orgs, j.customerOrg)
	}

	profiles, err := m.Customers.CustomerProfiles(ctx, call, orgs)
	if err != nil {
		return Scorecard{}, apperr.Wrap(err, "maintenance.CustomerProfiles.CustomerProfiles")
	}

	out := Scorecard{
		Period:          input.Period,
		ContractorOrgID: input.ContractorOrgID,
		Customers:       []CustomerMetrics{},
		Breaches:        []Breach{},
		Targets: targetViews(
			targets,
			call.Now,
		),
		Totals: Metrics{
			ResponseWithinTarget: nil,
			ArrivalInWindow:      nil,
			FirstTimeFix:         nil,
			AverageRating:        nil,
			RatingCount:          0,
			OpenOverdue:          0,
		},
	}
	total := &tally{
		respN:   0,
		respOK:  0,
		arrN:    0,
		arrOK:   0,
		ftfN:    0,
		ftfOK:   0,
		stars:   0,
		rated:   0,
		overdue: 0,
		jobs:    0,
	}
	per := map[uuid.UUID]*tally{}
	planOf := map[uuid.UUID]string{}

	var order []uuid.UUID

	for _, metric := range jobs {
		prof := profiles[metric.customerOrg]

		cust, _ := uuid.Parse(prof[0])
		if per[cust] == nil {
			per[cust] = &tally{
				respN:   0,
				respOK:  0,
				arrN:    0,
				arrOK:   0,
				ftfN:    0,
				ftfOK:   0,
				stars:   0,
				rated:   0,
				overdue: 0,
				jobs:    0,
			}
			planOf[cust] = prof[1]
			order = append(order, cust)
		}

		tgt := targetFor(targets, prof[1], metric.createdAt)
		limit := time.Duration(tgt.ResponseHours) * time.Hour
		limitMin := int(limit.Minutes())

		for _, t := range []*tally{total, per[cust]} {
			t.jobs++
		}

		breach := func(b Breach) {
			b.JobID, b.CustomerID, b.at = metric.id, cust, metric.createdAt
			out.Breaches = append(out.Breaches, b)
		}

		switch {
		case metric.planned:
			// a plan's visit is generated ahead of its date: the response target applies to requests (IR451)
		case metric.responseAt != nil:
			onTime := metric.responseAt.Sub(metric.createdAt) <= limit

			for _, t := range []*tally{total, per[cust]} {
				t.respN++
				if onTime {
					t.respOK++
				}
			}

			if !onTime {
				took := int(metric.responseAt.Sub(metric.createdAt).Minutes())
				breach(
					Breach{
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
					},
				)
			}
		case metric.cancelledAt != nil && metric.cancelledAt.Sub(metric.createdAt) <= limit:
			// cancelled before its response was due: neither within the target nor a miss (IR291)
		case call.Now.Sub(metric.createdAt) > limit:
			for _, t := range []*tally{total, per[cust]} {
				t.respN++
			}

			breach(
				Breach{
					Kind:         "response",
					Detail:       "no response within " + durationText(limit),
					LimitMinutes: &limitMin,
					JobID:        uuid.UUID{},
					CustomerID:   uuid.UUID{},
					TookMinutes:  nil,
					at:           time.Time{},
				},
			)
		}

		if metric.arrivedAt != nil {
			for _, t := range []*tally{total, per[cust]} {
				t.arrN++
				if metric.inWindow {
					t.arrOK++
				}
			}

			if !metric.inWindow {
				breach(
					Breach{
						Kind:         "arrival",
						Detail:       "arrival outside the scheduled window",
						JobID:        uuid.UUID{},
						CustomerID:   uuid.UUID{},
						TookMinutes:  nil,
						LimitMinutes: nil,
						at:           time.Time{},
					},
				)
			}
		}

		if metric.status == "completed" {
			fixedFirstTime := !metric.everReturned && !metric.followUpWithin30

			for _, t := range []*tally{total, per[cust]} {
				t.ftfN++
				if fixedFirstTime {
					t.ftfOK++
				}
			}

			if !fixedFirstTime {
				breach(
					Breach{
						Kind:         "first_time_fix",
						Detail:       "not fixed the first time",
						JobID:        uuid.UUID{},
						CustomerID:   uuid.UUID{},
						TookMinutes:  nil,
						LimitMinutes: nil,
						at:           time.Time{},
					},
				)
			}
		}

		if metric.stars > 0 {
			for _, t := range []*tally{total, per[cust]} {
				t.rated++
				t.stars += metric.stars
			}
		}

		if metric.status != "completed" && metric.status != "cancelled" &&
			call.Now.After(metric.dueAt) {
			for _, t := range []*tally{total, per[cust]} {
				t.overdue++
			}

			breach(
				Breach{
					Kind:         "overdue",
					Detail:       "open past its due time",
					JobID:        uuid.UUID{},
					CustomerID:   uuid.UUID{},
					TookMinutes:  nil,
					LimitMinutes: nil,
					at:           time.Time{},
				},
			)
		}
	}

	out.Totals = total.metrics()

	for _, cust := range order {
		counts := per[cust]
		metrics := counts.metrics()
		out.Customers = append(
			out.Customers,
			CustomerMetrics{
				Metrics:    metrics,
				CustomerID: cust,
				PlanType:   planOf[cust],
				JobCount:   counts.jobs,
				Status:     customerStatus(metrics, targetFor(targets, planOf[cust], call.Now)),
			},
		)
	}

	sort.SliceStable(
		out.Breaches,
		func(i, k int) bool { return out.Breaches[i].at.After(out.Breaches[k].at) },
	)

	if len(out.Breaches) > 50 {
		out.Breaches = out.Breaches[:50]
	}

	return out, nil
}

// customerStatus is a customer's SLA standing (DD-A22): every response is due within the plan's hours, so the response
// share is held to 100 %, arrival in window and first-time fix to the plan's targets. A share at or above its target
// is on track; up to 10 points below is at risk, more than 10 below — or any open overdue job — is breached. A share
// without jobs to measure says nothing.
func customerStatus(metrics Metrics, tgt SlaTargets) string {
	status := "on_track"
	check := func(v *float64, target float64) {
		if v == nil || *v >= target {
			return
		}

		if target-*v > 10 {
			status = "breached"
		} else if status != "breached" {
			status = "at_risk"
		}
	}
	check(metrics.ResponseWithinTarget, 100)
	check(metrics.ArrivalInWindow, tgt.ArrivalInWindowPercent)
	check(metrics.FirstTimeFix, tgt.FirstTimeFixPercent)

	if metrics.OpenOverdue > 0 {
		status = "breached"
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
