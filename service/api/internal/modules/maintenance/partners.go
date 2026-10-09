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
	OrgState(ctx context.Context, c *ops.Call, org uuid.UUID) (kind, status string, found bool, err error)
	OrgName(ctx context.Context, c *ops.Call, org uuid.UUID) (string, error)
}

// CustomerProfiles resolves the customer and service profile of customer organizations (Assets).
type CustomerProfiles interface {
	CustomerProfiles(ctx context.Context, c *ops.Call, orgs []uuid.UUID) (map[uuid.UUID][2]string, error) // org → {customerId, serviceProfile}
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

const profileCols = `p.id, p.tenant_id, p.version, p.created_at, p.updated_at, p.organization_id, p.name, p.status, p.registration_no, p.service_areas, p.contact_email::text,
	p.insurance_valid_until, lower(p.delegation), upper(p.delegation), p.suspended_reason,
	(SELECT r.id FROM maintenance.rate_cards r WHERE r.contractor_org_id = p.organization_id AND r.effective_from <= $1 ORDER BY r.effective_from DESC LIMIT 1)`

func scanProfile(r pgx.Row) (Profile, error) {
	var p Profile
	err := r.Scan(&p.ID, &p.TenantID, &p.Version, &p.CreatedAt, &p.UpdatedAt, &p.OrganizationID, &p.Name, &p.Status, &p.RegistrationNo, &p.ServiceAreas, &p.ContactEmail,
		&p.InsuranceValidUntil, &p.Delegation.From, &p.Delegation.To, &p.SuspendedReason, &p.RateCardID)
	return p, err
}

func pct(n, d int) *float64 {
	if d == 0 {
		return nil
	}
	v := math.Round(float64(n)*1000/float64(d)) / 10
	return &v
}

// kpis computes ContractorKpis for the 90 days ending now (IR131 item 6).
func (m Partners) kpis(ctx context.Context, c *ops.Call, org uuid.UUID) (KPIs, error) {
	k := KPIs{Period: Range{c.Now.Add(-90 * 24 * time.Hour), c.Now}}
	var acc, dec int
	if err := c.Tx.QueryRow(ctx, `SELECT count(*) FILTER (WHERE decision = 'accept'), count(*) FILTER (WHERE decision IS NOT NULL OR expired_at IS NOT NULL)
		FROM maintenance.offers WHERE contractor_org_id = $1 AND offered_at >= $2 AND offered_at < $3`, org, k.Period.From, k.Period.To).Scan(&acc, &dec); err != nil {
		return k, err
	}
	k.OfferAcceptance = pct(acc, dec)
	jm, err := jobMetrics(ctx, c, "j.contractor_org_id = $3", []any{k.Period.From, k.Period.To, org})
	if err != nil {
		return k, err
	}
	var arrived, inWin, firstSub, firstOK, completed, rework, stars int
	for _, j := range jm {
		if j.arrivedAt != nil {
			arrived++
			if j.inWindow {
				inWin++
			}
		}
		if j.firstReview != "" {
			firstSub++
			if j.firstReview == "accept" {
				firstOK++
			}
		}
		if j.status == "completed" {
			completed++
			if j.reworkFollowUp {
				rework++
			}
		}
		if j.stars > 0 {
			k.RatingCount++
			stars += j.stars
		}
	}
	k.ArrivalInWindow, k.FirstTimeAccepted, k.ReworkRate = pct(inWin, arrived), pct(firstOK, firstSub), pct(rework, completed)
	if k.RatingCount > 0 {
		v := math.Round(float64(stars)*10/float64(k.RatingCount)) / 10
		k.AverageRating = &v
	}
	return k, nil
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
}

// jobMetrics loads the per-job facts for SLA / KPI metrics of jobs created in [$1, $2) matching extra.
func jobMetrics(ctx context.Context, c *ops.Call, extra string, args []any) ([]jobMetric, error) {
	rows, err := c.Tx.Query(ctx, `SELECT j.id, j.customer_org_id, j.unit_id, j.created_at, j.due_at, j.status,
		COALESCE((SELECT min(o.decided_at) FROM maintenance.offers o WHERE o.job_id = j.id AND o.decision = 'accept'),
			(SELECT min(a.created_at) FROM maintenance.assignments a WHERE a.job_id = j.id)),
		(j.time_on_site->>'arrivedAt')::timestamptz,
		COALESCE((j.time_on_site->>'arrivedAt')::timestamptz >= lower(j.scheduled_slot) AND (j.time_on_site->>'arrivedAt')::timestamptz < upper(j.scheduled_slot), false),
		COALESCE((SELECT rr.decision FROM maintenance.report_reviews rr JOIN maintenance.work_reports w ON w.id = rr.report_id AND w.version = rr.report_version
			WHERE w.job_id = j.id ORDER BY rr.occurred_at, rr.report_version LIMIT 1), ''),
		EXISTS (SELECT 1 FROM maintenance.report_reviews rr JOIN maintenance.work_reports w ON w.id = rr.report_id AND w.version = rr.report_version
			WHERE w.job_id = j.id AND rr.decision = 'return'),
		EXISTS (SELECT 1 FROM maintenance.jobs f WHERE f.unit_id = j.unit_id AND f.id <> j.id AND j.completed_at IS NOT NULL
			AND f.created_at > j.completed_at AND f.created_at <= j.completed_at + interval '30 days'),
		EXISTS (SELECT 1 FROM maintenance.jobs f WHERE f.follow_up_of_job_id = j.id AND f.follow_up_class = 'rework'),
		COALESCE((j.rating->>'stars')::int, 0)
		FROM maintenance.jobs j WHERE j.created_at >= $1 AND j.created_at < $2 AND `+extra+` ORDER BY j.created_at, j.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []jobMetric
	for rows.Next() {
		var j jobMetric
		if err := rows.Scan(&j.id, &j.customerOrg, &j.unit, &j.createdAt, &j.dueAt, &j.status, &j.responseAt, &j.arrivedAt, &j.inWindow, &j.firstReview,
			&j.everReturned, &j.followUpWithin30, &j.reworkFollowUp, &j.stars); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// ---- contractors.list / save / setOfferStatus ----

func (m Partners) loadProfile(ctx context.Context, c *ops.Call, where string, args []any, lock bool) (Profile, error) {
	q := "SELECT " + profileCols + " FROM maintenance.contractors p WHERE " + where
	if lock {
		q += " FOR UPDATE"
	}
	p, err := scanProfile(c.Tx.QueryRow(ctx, q, append([]any{c.Now}, args...)...))
	if errors.Is(err, pgx.ErrNoRows) {
		return p, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return p, err
	}
	p.KPIs, err = m.kpis(ctx, c, p.OrganizationID)
	return p, err
}

// @Summary		contractors.list (read)
// @ID				contractors.list
// @Description	Authorization: admin:job.read
// @Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
// @Description	Recovery: D04: retry only UNAVAILABLE, at most twice
// @Description	Design: DD-A21 · Query: filters status,search · sort id,name,updatedAt (default name asc;id asc)
// @Tags			contractors
// @Accept			json
// @Produce		json
// @Param			cursor	query		string	false	"page cursor: nextCursor of the previous page (D12)"
// @Param			limit	query		integer	false	"page size 1–100, default 25"
// @Param			sort	query		string	false	"field:direction — fields id,name,updatedAt; default name asc;id asc"
// @Param			status	query		string	false	"filter → status"
// @Param			search	query		string	false	"filter → name, registrationNo or service area contains"
// @Success		200		{object}	ops.Envelope{data=ProfilePage}
// @Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504		{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/contractors [get]
func (m Partners) list(ctx context.Context, c *ops.Call, in *paging.Query) (paging.Page[Profile], error) {
	var f struct {
		Status *string `json:"status,omitempty"`
		Search *string `json:"search,omitempty"` // name, registration number or a service area contains
	}
	if len(in.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(in.Filters)))
		dec.DisallowUnknownFields()
		if dec.Decode(&f) != nil || (f.Status != nil && *f.Status != "active" && *f.Status != "suspended") {
			return paging.Page[Profile]{}, apperr.Fields(map[string]string{"filters": "error.invalid"})
		}
	}
	order, err := paging.OrderBy(in.Sort, map[string]string{"id": "p.id", "name": "p.name", "updatedAt": "p.updated_at"}, "p.name ASC, p.id ASC")
	if err != nil {
		return paging.Page[Profile]{}, err
	}
	w, err := paging.Resolve(*in, f, c.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Profile]{}, err
	}
	args := []any{c.Now}
	conds := []string{"TRUE"}
	if f.Status != nil {
		args = append(args, *f.Status)
		conds = append(conds, fmt.Sprintf("p.status = $%d", len(args)))
	}
	if f.Search != nil && strings.TrimSpace(*f.Search) != "" {
		args = append(args, "%"+strings.TrimSpace(*f.Search)+"%")
		n := len(args)
		conds = append(conds, fmt.Sprintf("(p.name ILIKE $%d OR p.registration_no ILIKE $%d OR EXISTS (SELECT 1 FROM unnest(p.service_areas) a WHERE a ILIKE $%d))", n, n, n))
	}
	where := strings.Join(conds, " AND ")
	var total int
	if err := c.Tx.QueryRow(ctx, "SELECT count(*) FROM maintenance.contractors p WHERE "+where+" AND $1::timestamptz IS NOT NULL", args...).Scan(&total); err != nil {
		return paging.Page[Profile]{}, err
	}
	rows, err := c.Tx.Query(ctx, fmt.Sprintf("SELECT %s FROM maintenance.contractors p WHERE %s ORDER BY %s LIMIT %d OFFSET %d", profileCols, where, order, w.Limit, w.Offset), args...)
	if err != nil {
		return paging.Page[Profile]{}, err
	}
	items := []Profile{}
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			rows.Close()
			return paging.Page[Profile]{}, err
		}
		items = append(items, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return paging.Page[Profile]{}, err
	}
	for i := range items {
		if items[i].KPIs, err = m.kpis(ctx, c, items[i].OrganizationID); err != nil {
			return paging.Page[Profile]{}, err
		}
	}
	return paging.Page[Profile]{Items: items, NextCursor: w.Next(total), Total: total, SnapshotVersion: w.Snapshot}, nil
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
	fe := map[string]string{}
	if in.OrganizationID == uuid.Nil {
		fe["organizationId"] = "error.required"
	}
	trimmedReason(&in.RegistrationNo, 64, "registrationNo", fe)
	seen := map[string]bool{}
	if len(in.ServiceAreas) < 1 || len(in.ServiceAreas) > 20 {
		fe["serviceAreas"] = "error.count"
	}
	for i := range in.ServiceAreas {
		in.ServiceAreas[i] = strings.TrimSpace(in.ServiceAreas[i])
		n := utf8.RuneCountInString(in.ServiceAreas[i])
		if n < 1 || n > 80 || seen[strings.ToLower(in.ServiceAreas[i])] {
			fe["serviceAreas"] = "error.invalid"
		}
		seen[strings.ToLower(in.ServiceAreas[i])] = true
	}
	in.ContactEmail = strings.TrimSpace(in.ContactEmail)
	if a, err := mail.ParseAddress(in.ContactEmail); err != nil || a.Address != in.ContactEmail || len(in.ContactEmail) > 254 {
		fe["contactEmail"] = "error.invalid"
	}
	return fe
}

// @Summary		contractors.save (write)
// @ID				contractors.save
// @Description	Authorization: admin:job.write
// @Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; IR111 organization kind=contractor; service areas at least one
// @Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
// @Description	Design: DD-A21
// @Tags			contractors
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key	header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Param			request			body		ProfileInput	true	"input"
// @Success		200				{object}	ops.Envelope{data=Profile}
// @Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504				{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/contractors [post]
func (m Partners) save(ctx context.Context, c *ops.Call, in *ProfileInput) (Profile, error) {
	kind, _, found, err := m.Orgs.OrgState(ctx, c, in.OrganizationID)
	if err != nil {
		return Profile{}, err
	}
	if !found {
		return Profile{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	if kind != "contractor" {
		return Profile{}, apperr.Fields(map[string]string{"organizationId": "error.notContractor"})
	}
	name, err := m.Orgs.OrgName(ctx, c, in.OrganizationID)
	if err != nil {
		return Profile{}, err
	}
	delegationEnd := func(from time.Time) time.Time {
		if in.InsuranceValidUntil != nil && in.InsuranceValidUntil.After(from) {
			return *in.InsuranceValidUntil
		}
		return from.Add(365 * 24 * time.Hour)
	}
	var id uuid.UUID
	if in.ID == nil {
		var exists bool
		if err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM maintenance.contractors WHERE organization_id = $1)`, in.OrganizationID).Scan(&exists); err != nil {
			return Profile{}, err
		}
		if exists {
			return Profile{}, apperr.E(apperr.Conflict, "errors.profile_exists")
		}
		if err := c.Tx.QueryRow(ctx, `INSERT INTO maintenance.contractors (tenant_id, organization_id, name, status, registration_no, service_areas, contact_email,
			insurance_valid_until, delegation, created_at, updated_at) VALUES (current_setting('app.tenant_id')::uuid, $1, $2, 'active', $3, $4, $5, $6, tstzrange($7, $8), $7, $7)
			RETURNING id`, in.OrganizationID, name, in.RegistrationNo, in.ServiceAreas, in.ContactEmail, in.InsuranceValidUntil, c.Now, delegationEnd(c.Now)).Scan(&id); err != nil {
			return Profile{}, err
		}
	} else {
		p, err := m.loadProfile(ctx, c, "p.id = $2", []any{*in.ID}, true)
		if err != nil {
			return p, err
		}
		if p.OrganizationID != in.OrganizationID {
			return p, apperr.Fields(map[string]string{"organizationId": "error.organizationFixed"})
		}
		if p.Version != *c.ExpectedVersion {
			return p, apperr.E(apperr.Conflict, "error.versionConflict")
		}
		if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.contractors SET name = $2, registration_no = $3, service_areas = $4, contact_email = $5, insurance_valid_until = $6,
			delegation = tstzrange(lower(delegation), $7), version = version + 1, updated_at = $8 WHERE id = $1`, p.ID, name, in.RegistrationNo, in.ServiceAreas,
			in.ContactEmail, in.InsuranceValidUntil, delegationEnd(p.CreatedAt), c.Now); err != nil {
			return p, err
		}
		id = p.ID
	}
	p, err := m.loadProfile(ctx, c, "p.id = $2", []any{id}, false)
	if err != nil {
		return p, err
	}
	c.Emit(ops.Event{AggregateType: "contractor", AggregateID: id, Type: "ContractorSaved", Payload: map[string]any{"organizationId": in.OrganizationID}})
	c.Audit(ops.AuditEntry{Action: "contractors.save", TargetKind: "contractor", TargetID: id.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &p.Version})
	return p, nil
}

// OfferStatusInput is contractors.setOfferStatus input.
type OfferStatusInput struct {
	ContractorOrgID uuid.UUID `json:"contractorOrgId"`
	Status          string    `json:"status"`
	Reason          string    `json:"reason"`
}

// Validate implements ops.Validator.
func (in *OfferStatusInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.ContractorOrgID == uuid.Nil {
		fe["contractorOrgId"] = "error.required"
	}
	if in.Status != "active" && in.Status != "suspended" {
		fe["status"] = "error.invalid"
	}
	trimmedReason(&in.Reason, 1000, "reason", fe)
	return fe
}

// @Summary		contractors.setOfferStatus (write)
// @ID				contractors.setOfferStatus
// @Description	Authorization: admin:job.write
// @Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; IR111 reason 1–1000; suspension blocks new jobs.offer only
// @Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
// @Description	Design: DD-A21
// @Tags			contractors
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key		header		string				true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Param			X-Expected-Version	header		integer				true	"all: required (target contractor, read contractors.list)"
// @Param			contractorOrgId		path		string				true	"input field contractorOrgId"
// @Param			request				body		OfferStatusInput	true	"input; the path parameters come from the route"
// @Success		200					{object}	ops.Envelope{data=Profile}
// @Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504					{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/contractors/{contractorOrgId}/offer-status [put]
func (m Partners) setOfferStatus(ctx context.Context, c *ops.Call, in *OfferStatusInput) (Profile, error) {
	p, err := m.loadProfile(ctx, c, "p.organization_id = $2", []any{in.ContractorOrgID}, true)
	if err != nil {
		return p, err
	}
	if p.Version != *c.ExpectedVersion {
		return p, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	if p.Status == in.Status {
		return p, apperr.E(apperr.Conflict, "error.invalidState")
	}
	var reason *string
	if in.Status == "suspended" {
		reason = &in.Reason
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.contractors SET status = $2, suspended_reason = $3, version = version + 1, updated_at = $4 WHERE id = $1`,
		p.ID, in.Status, reason, c.Now); err != nil {
		return p, err
	}
	p, err = m.loadProfile(ctx, c, "p.id = $2", []any{p.ID}, false)
	if err != nil {
		return p, err
	}
	c.Emit(ops.Event{AggregateType: "contractor", AggregateID: p.ID, Type: "ContractorOfferStatusChanged", Payload: map[string]any{"status": in.Status}})
	c.Audit(ops.AuditEntry{Action: "contractors.setOfferStatus", TargetKind: "contractor", TargetID: p.ID.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &p.Version, Reason: in.Reason})
	return p, nil
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
	fe := map[string]string{}
	if in.ContractorOrgID == uuid.Nil {
		fe["contractorOrgId"] = "error.required"
	}
	if in.EffectiveFrom.IsZero() {
		fe["effectiveFrom"] = "error.required"
	}
	if in.Currency != "MYR" && in.Currency != "USD" {
		fe["currency"] = "error.invalid"
	}
	if len(in.Lines) < 1 || len(in.Lines) > 4 {
		fe["lines"] = "error.count"
	}
	seen := map[string]bool{}
	for i := range in.Lines {
		l := &in.Lines[i]
		if !slices.Contains(workTypes, l.WorkType) || seen[l.WorkType] || l.AmountMinor < 0 {
			fe["lines"] = "error.invalid"
		}
		seen[l.WorkType] = true
		if l.Note != nil {
			*l.Note = strings.TrimSpace(*l.Note)
			if utf8.RuneCountInString(*l.Note) > 200 {
				fe["lines"] = "error.length"
			}
			if *l.Note == "" {
				l.Note = nil
			}
		}
	}
	return fe
}

func scanRateCard(r pgx.Row) (RateCard, error) {
	var x RateCard
	var lines []byte
	err := r.Scan(&x.ID, &x.TenantID, &x.Version, &x.CreatedAt, &x.ContractorOrgID, &x.EffectiveFrom, &x.Currency, &lines)
	x.UpdatedAt = x.CreatedAt
	if err == nil {
		err = json.Unmarshal(lines, &x.Lines)
	}
	return x, err
}

const rateCols = `id, tenant_id, version, created_at, contractor_org_id, effective_from, currency, lines`

// @Summary		rateCards.save (write)
// @ID				rateCards.save
// @Description	Authorization: admin:job.write
// @Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; IR111 effectiveFrom in the future; creates a new version
// @Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
// @Description	Design: DD-A21
// @Tags			rateCards
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key	header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Param			request			body		RateCardInput	true	"input"
// @Success		200				{object}	ops.Envelope{data=RateCard}
// @Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504				{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/rate-cards [post]
func (m Partners) saveRateCard(ctx context.Context, c *ops.Call, in *RateCardInput) (RateCard, error) {
	if !in.EffectiveFrom.After(c.Now) {
		return RateCard{}, apperr.Fields(map[string]string{"effectiveFrom": "error.past"})
	}
	var exists bool
	if err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM maintenance.contractors WHERE organization_id = $1)`, in.ContractorOrgID).Scan(&exists); err != nil {
		return RateCard{}, err
	}
	if !exists {
		return RateCard{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	lines, _ := json.Marshal(in.Lines)
	x, err := scanRateCard(c.Tx.QueryRow(ctx, `INSERT INTO maintenance.rate_cards (tenant_id, contractor_org_id, effective_from, currency, lines, version, created_at)
		VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4,
			COALESCE((SELECT max(version) FROM maintenance.rate_cards WHERE contractor_org_id = $1), 0) + 1, $5)
		RETURNING `+rateCols, in.ContractorOrgID, in.EffectiveFrom, in.Currency, lines, c.Now))
	if err != nil {
		return x, err
	}
	c.Emit(ops.Event{AggregateType: "rate_card", AggregateID: x.ID, Type: "RateCardSaved", Payload: map[string]any{"contractorOrgId": in.ContractorOrgID}})
	c.Audit(ops.AuditEntry{Action: "rateCards.save", TargetKind: "rate_card", TargetID: x.ID.String(), NextVersion: &x.Version})
	return x, nil
}

// @Summary		rateCards.list (read)
// @ID				rateCards.list
// @Description	Authorization: admin:job.read | admin:billing.read | contractor:own-company
// @Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
// @Description	Recovery: D04: retry only UNAVAILABLE, at most twice
// @Description	Design: DD-A21, DD-A23, DD-P10 · Query: filters contractorOrgId · sort id,createdAt,effectiveFrom (default effectiveFrom desc;id asc)
// @Tags			rateCards
// @Accept			json
// @Produce		json
// @Param			cursor			query		string	false	"page cursor: nextCursor of the previous page (D12)"
// @Param			limit			query		integer	false	"page size 1–100, default 25"
// @Param			sort			query		string	false	"field:direction — fields id,createdAt,effectiveFrom; default effectiveFrom desc;id asc"
// @Param			contractorOrgId	query		string	false	"filter → contractorOrgId"
// @Success		200				{object}	ops.Envelope{data=RateCardPage}
// @Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504				{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/rate-cards [get]
func (m Partners) listRateCards(ctx context.Context, c *ops.Call, in *paging.Query) (paging.Page[RateCard], error) {
	var f struct {
		ContractorOrgID *uuid.UUID `json:"contractorOrgId,omitempty"`
	}
	if len(in.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(in.Filters)))
		dec.DisallowUnknownFields()
		if dec.Decode(&f) != nil {
			return paging.Page[RateCard]{}, apperr.Fields(map[string]string{"filters": "error.invalid"})
		}
	}
	if c.Principal.Role == "contractor" {
		f.ContractorOrgID = &c.Principal.OrgID
	}
	order, err := paging.OrderBy(in.Sort, map[string]string{"id": "id", "createdAt": "created_at", "effectiveFrom": "effective_from"}, "effective_from DESC, id")
	if err != nil {
		return paging.Page[RateCard]{}, err
	}
	w, err := paging.Resolve(*in, f, c.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[RateCard]{}, err
	}
	where, args := "TRUE", []any{}
	if f.ContractorOrgID != nil {
		where, args = "contractor_org_id = $1", append(args, *f.ContractorOrgID)
	}
	var total int
	if err := c.Tx.QueryRow(ctx, "SELECT count(*) FROM maintenance.rate_cards WHERE "+where, args...).Scan(&total); err != nil {
		return paging.Page[RateCard]{}, err
	}
	rows, err := c.Tx.Query(ctx, fmt.Sprintf("SELECT %s FROM maintenance.rate_cards WHERE %s ORDER BY %s LIMIT %d OFFSET %d", rateCols, where, order, w.Limit, w.Offset), args...)
	if err != nil {
		return paging.Page[RateCard]{}, err
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
	return paging.Page[RateCard]{Items: items, NextCursor: w.Next(total), Total: total, SnapshotVersion: w.Snapshot}, rows.Err()
}

// ---- SLA ----

var planTypes = []string{"rto", "general", "energy", "environment"}

// DefaultSLA is the target used when no SlaTargets row applies (IR131 item 4).
var DefaultSLA = SlaTargets{ResponseHours: 4, ArrivalInWindowPercent: 90, FirstTimeFixPercent: 85}

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
	fe := map[string]string{}
	if !slices.Contains(planTypes, in.PlanType) {
		fe["planType"] = "error.invalid"
	}
	if in.ResponseHours < 1 || in.ResponseHours > 168 {
		fe["responseHours"] = "error.range"
	}
	if in.ArrivalInWindowPercent < 0 || in.ArrivalInWindowPercent > 100 {
		fe["arrivalInWindowPercent"] = "error.range"
	}
	if in.FirstTimeFixPercent < 0 || in.FirstTimeFixPercent > 100 {
		fe["firstTimeFixPercent"] = "error.range"
	}
	if in.EffectiveFrom.IsZero() {
		fe["effectiveFrom"] = "error.required"
	}
	return fe
}

// @Summary		sla.saveTargets (write)
// @ID				sla.saveTargets
// @Description	Authorization: admin:job.write
// @Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; IR111 percentages 0–100; responseHours 1–168; applies to jobs created after effectiveFrom
// @Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
// @Description	Design: DD-A22
// @Tags			sla
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key	header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Param			planType		path		string			true	"input field planType"
// @Param			request			body		TargetsInput	true	"input; the path parameters come from the route"
// @Success		200				{object}	ops.Envelope{data=SlaTargets}
// @Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504				{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/sla/targets/{planType} [put]
func (m Partners) saveTargets(ctx context.Context, c *ops.Call, in *TargetsInput) (SlaTargets, error) {
	if in.EffectiveFrom.Before(c.Now) {
		return SlaTargets{}, apperr.Fields(map[string]string{"effectiveFrom": "error.past"})
	}
	var t SlaTargets
	err := c.Tx.QueryRow(ctx, `INSERT INTO maintenance.sla_targets (tenant_id, plan_type, response_hours, arrival_in_window_percent, first_time_fix_percent, effective_from, version)
		VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $5, COALESCE((SELECT max(version) FROM maintenance.sla_targets WHERE plan_type = $1), 0) + 1)
		RETURNING id, tenant_id, version, plan_type, response_hours, arrival_in_window_percent::float8, first_time_fix_percent::float8, effective_from`,
		in.PlanType, in.ResponseHours, in.ArrivalInWindowPercent, in.FirstTimeFixPercent, in.EffectiveFrom).
		Scan(&t.ID, &t.TenantID, &t.Version, &t.PlanType, &t.ResponseHours, &t.ArrivalInWindowPercent, &t.FirstTimeFixPercent, &t.EffectiveFrom)
	if err != nil {
		if apperr.From(err).Code == apperr.Conflict {
			return t, apperr.E(apperr.Conflict, "errors.targets_exist")
		}
		return t, err
	}
	t.CreatedAt, t.UpdatedAt = c.Now, c.Now
	c.Emit(ops.Event{AggregateType: "sla_targets", AggregateID: t.ID, Type: "SlaTargetsSaved", Payload: map[string]any{"planType": t.PlanType}})
	c.Audit(ops.AuditEntry{Action: "sla.saveTargets", TargetKind: "sla_targets", TargetID: t.ID.String(), NextVersion: &t.Version})
	return t, nil
}

// targetFor returns the SLA target for a plan type at a job's creation (IR131 item 4).
func targetFor(all []SlaTargets, plan string, at time.Time) SlaTargets {
	best := DefaultSLA
	var bestAt *time.Time
	for _, t := range all {
		if t.PlanType == plan && !t.EffectiveFrom.After(at) && (bestAt == nil || t.EffectiveFrom.After(*bestAt)) {
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
	view := func(t SlaTargets, state string) TargetView {
		v := TargetView{PlanType: t.PlanType, ResponseHours: t.ResponseHours, ArrivalInWindowPercent: t.ArrivalInWindowPercent, FirstTimeFixPercent: t.FirstTimeFixPercent, Version: t.Version, State: state}
		if state != "default" {
			e := t.EffectiveFrom
			v.EffectiveFrom = &e
		}
		return v
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
	sort.SliceStable(later, func(i, k int) bool { return later[i].EffectiveFrom.Before(later[k].EffectiveFrom) })
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

// Breach is one scorecard breach.
type Breach struct {
	JobID      uuid.UUID `json:"jobId"`
	CustomerID uuid.UUID `json:"customerId"`
	Kind       string    `json:"kind"`
	Detail     string    `json:"detail"`
	at         time.Time
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
	if in.Period.From.IsZero() || in.Period.To.IsZero() || !in.Period.From.Before(in.Period.To) || in.Period.To.Sub(in.Period.From) > 366*24*time.Hour {
		return map[string]string{"period": "error.range"}
	}
	return nil
}

type tally struct {
	respN, respOK, arrN, arrOK, ftfN, ftfOK, stars, rated, overdue, jobs int
	missResp, missArr, missFtf                                           float64
}

func (t *tally) metrics() Metrics {
	m := Metrics{ResponseWithinTarget: pct(t.respOK, t.respN), ArrivalInWindow: pct(t.arrOK, t.arrN), FirstTimeFix: pct(t.ftfOK, t.ftfN), RatingCount: t.rated, OpenOverdue: t.overdue}
	if t.rated > 0 {
		v := math.Round(float64(t.stars)*10/float64(t.rated)) / 10
		m.AverageRating = &v
	}
	return m
}

func durationText(d time.Duration) string {
	h, mnt := int(d.Hours()), int(d.Minutes())%60
	if mnt == 0 {
		return fmt.Sprintf("%d h", h)
	}
	return fmt.Sprintf("%d h %d min", h, mnt)
}

// @Summary		sla.scorecard (read)
// @ID				sla.scorecard
// @Description	Authorization: admin:job.read
// @Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
// @Description	Recovery: D04: retry only UNAVAILABLE, at most twice
// @Description	Design: DD-A22
// @Tags			sla
// @Accept			json
// @Produce		json
// @Param			period.from		query		string	false	"input field period.from"
// @Param			period.to		query		string	false	"input field period.to"
// @Param			contractorOrgId	query		string	false	"input field contractorOrgId"
// @Success		200				{object}	ops.Envelope{data=Scorecard}
// @Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504				{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/sla/scorecard [get]
func (m Partners) scorecard(ctx context.Context, c *ops.Call, in *ScorecardInput) (Scorecard, error) {
	extra, args := "TRUE", []any{in.Period.From, in.Period.To}
	if in.ContractorOrgID != nil {
		extra, args = "j.contractor_org_id = $3", append(args, *in.ContractorOrgID)
	}
	jobs, err := jobMetrics(ctx, c, extra, args)
	if err != nil {
		return Scorecard{}, err
	}
	var targets []SlaTargets
	rows, err := c.Tx.Query(ctx, `SELECT plan_type, response_hours, arrival_in_window_percent::float8, first_time_fix_percent::float8, effective_from, version FROM maintenance.sla_targets`)
	if err != nil {
		return Scorecard{}, err
	}
	for rows.Next() {
		var t SlaTargets
		if err := rows.Scan(&t.PlanType, &t.ResponseHours, &t.ArrivalInWindowPercent, &t.FirstTimeFixPercent, &t.EffectiveFrom, &t.Version); err != nil {
			rows.Close()
			return Scorecard{}, err
		}
		targets = append(targets, t)
	}
	rows.Close()
	orgs := []uuid.UUID{}
	for _, j := range jobs {
		orgs = append(orgs, j.customerOrg)
	}
	profiles, err := m.Customers.CustomerProfiles(ctx, c, orgs)
	if err != nil {
		return Scorecard{}, err
	}
	out := Scorecard{Period: in.Period, ContractorOrgID: in.ContractorOrgID, Customers: []CustomerMetrics{}, Breaches: []Breach{}, Targets: targetViews(targets, c.Now)}
	total := &tally{}
	per := map[uuid.UUID]*tally{}
	planOf := map[uuid.UUID]string{}
	var order []uuid.UUID
	for _, j := range jobs {
		prof := profiles[j.customerOrg]
		cust, _ := uuid.Parse(prof[0])
		if per[cust] == nil {
			per[cust] = &tally{}
			planOf[cust] = prof[1]
			order = append(order, cust)
		}
		tgt := targetFor(targets, prof[1], j.createdAt)
		limit := time.Duration(tgt.ResponseHours) * time.Hour
		for _, t := range []*tally{total, per[cust]} {
			t.jobs++
		}
		breach := func(kind, detail string) {
			out.Breaches = append(out.Breaches, Breach{JobID: j.id, CustomerID: cust, Kind: kind, Detail: detail, at: j.createdAt})
		}
		switch {
		case j.responseAt != nil:
			ok := j.responseAt.Sub(j.createdAt) <= limit
			for _, t := range []*tally{total, per[cust]} {
				t.respN++
				if ok {
					t.respOK++
				}
			}
			if !ok {
				breach("response", "response "+durationText(j.responseAt.Sub(j.createdAt))+" vs "+durationText(limit))
			}
		case c.Now.Sub(j.createdAt) > limit:
			for _, t := range []*tally{total, per[cust]} {
				t.respN++
			}
			breach("response", "no response within "+durationText(limit))
		}
		if j.arrivedAt != nil {
			for _, t := range []*tally{total, per[cust]} {
				t.arrN++
				if j.inWindow {
					t.arrOK++
				}
			}
			if !j.inWindow {
				breach("arrival", "arrival outside the scheduled window")
			}
		}
		if j.status == "completed" {
			ok := !j.everReturned && !j.followUpWithin30
			for _, t := range []*tally{total, per[cust]} {
				t.ftfN++
				if ok {
					t.ftfOK++
				}
			}
			if !ok {
				breach("first_time_fix", "not fixed the first time")
			}
		}
		if j.stars > 0 {
			for _, t := range []*tally{total, per[cust]} {
				t.rated++
				t.stars += j.stars
			}
		}
		if j.status != "completed" && j.status != "cancelled" && c.Now.After(j.dueAt) {
			for _, t := range []*tally{total, per[cust]} {
				t.overdue++
			}
			breach("overdue", "open past its due time")
		}
	}
	out.Totals = total.metrics()
	for _, cust := range order {
		t := per[cust]
		mm := t.metrics()
		tgt := targetFor(targets, planOf[cust], c.Now)
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
		check(mm.ResponseWithinTarget, 100)
		check(mm.ArrivalInWindow, tgt.ArrivalInWindowPercent)
		check(mm.FirstTimeFix, tgt.FirstTimeFixPercent)
		if mm.OpenOverdue > 0 {
			status = "breached"
		}
		out.Customers = append(out.Customers, CustomerMetrics{Metrics: mm, CustomerID: cust, PlanType: planOf[cust], JobCount: t.jobs, Status: status})
	}
	sort.SliceStable(out.Breaches, func(i, k int) bool { return out.Breaches[i].at.After(out.Breaches[k].at) })
	if len(out.Breaches) > 50 {
		out.Breaches = out.Breaches[:50]
	}
	return out, nil
}

// RegisterPartners binds the IR131 operations.
func RegisterPartners(r *ops.Registry, m Partners) {
	ops.Register(r, "contractors.list", m.list)
	ops.Register(r, "contractors.save", m.save)
	ops.Register(r, "contractors.setOfferStatus", m.setOfferStatus)
	ops.Register(r, "rateCards.save", m.saveRateCard)
	ops.Register(r, "rateCards.list", m.listRateCards)
	ops.Register(r, "sla.saveTargets", m.saveTargets)
	ops.Register(r, "sla.scorecard", m.scorecard)
}
