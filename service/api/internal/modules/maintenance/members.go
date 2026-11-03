package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/identity"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

// Workforce is members.eligible / members.capacity / members.setUnavailability (IR132 items 3–5) and the unavailable
// days' list and clear (IR612).
type Workforce struct {
	Delivery Delivery
}

// technicians lists the active technician memberships the caller may plan (HQ: all, internal only when internalOnly;
// contractor: own).
func (m Workforce) technicians(
	ctx context.Context,
	call *ops.Call,
	internalOnly bool,
) ([]identity.Member, error) {
	stmt := `SELECT m.id, m.tenant_id, m.version, m.created_at, m.updated_at, m.user_id, m.organization_id, m.role,
		m.employment, m.scope_version, m.valid_from,
		m.valid_until, m.client_role FROM maintenance.ref_memberships m WHERE m.role = 'technician' AND m.valid_from <= $1 AND
		(m.valid_until IS NULL OR m.valid_until > $1)`

	args := []any{call.Now}
	switch {
	case call.Principal.Role == "contractor":
		stmt += " AND m.organization_id = $2"

		args = append(args, call.Principal.OrgID)
	case internalOnly:
		stmt += " AND m.employment = 'internal'"
	}

	rows, err := call.Tx.Query(ctx, stmt+" ORDER BY m.id", args...)
	if err != nil {
		return nil, apperr.Wrap(err, "query (technicians)")
	}

	var out []identity.Member

	for rows.Next() {
		var member identity.Member

		err := rows.Scan(
			&member.ID,
			&member.TenantID,
			&member.Version,
			&member.CreatedAt,
			&member.UpdatedAt,
			&member.UserID,
			&member.OrganizationID,
			&member.Role,
			&member.Employment,
			&member.ScopeVersion,
			&member.ValidFrom,
			&member.ValidUntil,
			&member.ClientRole,
		)
		if err != nil {
			rows.Close()

			return nil, apperr.Wrap(err, "query (technicians)")
		}

		out = append(out, member)
	}

	rows.Close()

	return out, apperr.Wrap(rows.Err(), "query (technicians)")
}

// techFilter is the members.eligible / members.capacity filter set of the query catalog: organizationId,
// qualification (a grant of that code valid now, SR13) and activeOnly (candidates are always memberships valid now, so
// it narrows nothing further).
type techFilter struct {
	OrganizationID *uuid.UUID `json:"organizationId,omitempty"`
	Qualification  *string    `json:"qualification,omitempty"`
	ActiveOnly     *bool      `json:"activeOnly,omitempty"`
}

func decodeTechFilter(query paging.Query) (techFilter, error) {
	var filter techFilter
	if len(query.Filters) == 0 {
		return filter, nil
	}

	dec := json.NewDecoder(strings.NewReader(string(query.Filters)))
	dec.DisallowUnknownFields()

	if dec.Decode(&filter) != nil ||
		(filter.Qualification != nil && (!certCodes[*filter.Qualification] || *filter.Qualification == "other")) {
		return filter, apperr.Fields(map[string]string{"filters": apperr.KeyInvalid})
	}

	return filter, nil
}

// keep returns the technicians that match the filter; qualifications are loaded from identity only when needed.
func (f techFilter) keep(
	ctx context.Context,
	call *ops.Call,
	techs []identity.Member,
) ([]identity.Member, error) {
	if f.Qualification != nil {
		err := identity.LoadMembers(ctx, call, techs)
		if err != nil {
			return nil, apperr.Wrap(err, "identity.LoadMembers")
		}
	}

	out := []identity.Member{}

	for _, member := range techs {
		if f.OrganizationID != nil && member.OrganizationID != *f.OrganizationID {
			continue
		}

		if f.Qualification != nil && !holds(member, *f.Qualification, call.Now) {
			continue
		}

		out = append(out, member)
	}

	return out, nil
}

// holds reports whether the member has a grant of code valid at now: [validFrom, validUntil) and not revoked.
func holds(m identity.Member, code string, now time.Time) bool {
	for _, g := range m.Qualifications {
		if g.Code == code && !g.ValidFrom.After(now) && now.Before(g.ValidUntil) &&
			(g.RevokedAt == nil || now.Before(*g.RevokedAt)) {
			return true
		}
	}

	return false
}

func pageOf[T any](all []T, q paging.Query, hashOf any, scopeVersion int) (paging.Page[T], error) {
	window, err := paging.Resolve(q, hashOf, scopeVersion, 1)
	if err != nil {
		return paging.Page[T]{}, apperr.Wrap(err, "paging.Resolve")
	}

	items := []T{}
	for i := window.Offset; i < len(all) && i < window.Offset+window.Limit; i++ {
		items = append(items, all[i])
	}

	return paging.Page[T]{
		Items:           items,
		NextCursor:      window.Next(len(all)),
		Total:           len(all),
		SnapshotVersion: window.Snapshot,
	}, nil
}

func klDates(from, to time.Time) (string, string) {
	return from.In(kualaLumpur).
			Format("2006-01-02"),
		to.Add(-time.Nanosecond).
			In(kualaLumpur).
			Format("2006-01-02")
}

// unavailable reports whether a membership (or its whole organization) is unavailable on any date in [d1, d2].
func unavailable(
	ctx context.Context,
	call *ops.Call,
	membership, org uuid.UUID,
	firstDay, lastDay string,
) (string, bool, error) {
	var typ string

	err := call.Tx.QueryRow(ctx, `SELECT type FROM maintenance.unavailability WHERE (membership_id = $1 OR (membership_id
		IS
		NULL AND organization_id = $2))
		AND days && daterange($3::date, $4::date, '[]') ORDER BY created_at LIMIT 1`, membership, org, firstDay, lastDay).
		Scan(&typ)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}

	return typ, err == nil, apperr.Wrap(err, "select maintenance.unavailability")
}

// ---- members.eligible ----

// EligibleInput is members.eligible input.
type EligibleInput struct {
	JobID   uuid.UUID    `json:"jobId"`
	StartAt time.Time    `json:"startAt"`
	EndAt   time.Time    `json:"endAt"`
	Query   paging.Query `json:"query"`
}

// Validate implements ops.Validator.
func (in *EligibleInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.JobID == uuid.Nil {
		fieldErrs["jobId"] = apperr.KeyRequired
	}

	if !in.StartAt.Before(in.EndAt) {
		fieldErrs["endAt"] = apperr.KeyRange
	}

	return fieldErrs
}

// eligible serves members.eligible.
//
//	@Summary		members.eligible (read)
//	@ID				members.eligible
//	@Description	Authorization: admin:identity.read | admin:job.read | contractor:partner.assign:own-company
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot; IR94 \
//	@Description	role=technician only; scope includes Job.unitId
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A06, DD-P03 · Query: filters organizationId,qualification,activeOnly · sort \
//	@Description	id,createdAt,updatedAt (default id asc)
//	@Tags			members
//	@Accept			json
//	@Produce		json
//	@Param			jobId			query		string	false	"input field jobId"
//	@Param			startAt			query		string	false	"input field startAt"
//	@Param			endAt			query		string	false	"input field endAt"
//	@Param			cursor			query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit			query		integer	false	"page size 1–100, default 25"
//	@Param			sort			query		string	false	"field:direction — fields id,createdAt,updatedAt; default id asc"
//	@Param			organizationId	query		string	false	"filter → Membership.organizationId"
//	@Param			qualification	query		string	false	"filter → matching valid Membership.qualifications[].code (SR13)"
//	@Param			activeOnly		query		boolean	false	"filter → true => validFrom<=now<validUntil (null end infinite)"
//	@Success		200				{object}	ops.Envelope{data=MemberPage}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/members/eligible [get]
func (m Workforce) eligible(
	ctx context.Context,
	call *ops.Call,
	input *EligibleInput,
) (paging.Page[identity.Member], error) {
	var (
		unit       uuid.UUID
		contractor *uuid.UUID
	)

	err := call.Tx.QueryRow(ctx, `SELECT unit_id, contractor_org_id FROM maintenance.jobs WHERE id = $1`, input.JobID).
		Scan(&unit, &contractor)
	if errors.Is(err, pgx.ErrNoRows) ||
		(err == nil && call.Principal.Role == "contractor" && (contractor == nil || *contractor != call.Principal.OrgID)) {
		return paging.Page[identity.Member]{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return paging.Page[identity.Member]{}, apperr.Wrap(err, "select maintenance.jobs")
	}

	filter, err := decodeTechFilter(input.Query)
	if err != nil {
		return paging.Page[identity.Member]{}, err
	}

	techs, err := m.technicians(ctx, call, true)
	if err == nil {
		techs, err = filter.keep(ctx, call, techs)
	}

	if err != nil {
		return paging.Page[identity.Member]{}, err
	}

	slot := Slot{input.StartAt, input.EndAt}
	firstDay, lastDay := klDates(input.StartAt, input.EndAt)
	out := []identity.Member{}

	for _, member := range techs {
		isContractor := call.Principal.Role == "contractor"
		if m.Delivery.checkTechnician(ctx, call, unit, member.ID, isContractor, nil) != nil ||
			m.Delivery.checkQualified(ctx, call, unit, member.ID, slot) != nil {
			continue
		}

		_, off, err := unavailable(ctx, call, member.ID, member.OrganizationID, firstDay, lastDay)
		if err != nil {
			return paging.Page[identity.Member]{}, err
		} else if off {
			continue
		}

		var busy bool

		err = call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM maintenance.assignments WHERE technician_membership_id = $1
			AND
			status = 'active' AND job_id <> $2
			AND scheduled && tstzrange($3, $4))`, member.ID, input.JobID, input.StartAt, input.EndAt).
			Scan(&busy)
		if err != nil {
			return paging.Page[identity.Member]{}, apperr.Wrap(
				err,
				"select maintenance.assignments",
			)
		}

		if !busy {
			out = append(out, member)
		}
	}

	loadMembersErr := identity.LoadMembers(ctx, call, out)
	if loadMembersErr != nil {
		return paging.Page[identity.Member]{}, apperr.Wrap(loadMembersErr, "identity.LoadMembers")
	}

	sortSliceErr := paging.SortSlice(
		out,
		input.Query.Sort,
		map[string]func(a, b identity.Member) int{
			"id":        func(a, b identity.Member) int { return strings.Compare(a.ID.String(), b.ID.String()) },
			"createdAt": func(a, b identity.Member) int { return a.CreatedAt.Compare(b.CreatedAt) },
			"updatedAt": func(a, b identity.Member) int { return a.UpdatedAt.Compare(b.UpdatedAt) },
		},
	)
	if sortSliceErr != nil {
		return paging.Page[identity.Member]{}, apperr.Wrap(sortSliceErr, "paging.SortSlice")
	}

	return pageOf(out, input.Query, input, call.Principal.ScopeVersion)
}

// ---- members.capacity ----

// CapacityInput is members.capacity input.
type CapacityInput struct {
	Date  string       `json:"date"`
	Query paging.Query `json:"query"`
}

// Validate implements ops.Validator.
func (in *CapacityInput) Validate() map[string]string {
	_, err := time.ParseInLocation("2006-01-02", in.Date, kualaLumpur)
	if err != nil {
		return map[string]string{"date": apperr.KeyInvalid}
	}

	return nil
}

// Capacity is Capacity of service-contracts.ts.
type Capacity struct {
	MembershipID     uuid.UUID `json:"membershipId"`
	Date             string    `json:"date"`
	AvailableSlots   []Slot    `json:"availableSlots"`
	AssignedSlots    []Slot    `json:"assignedSlots"`
	AvailableMinutes *int      `json:"availableMinutes"`
	AssignedMinutes  int       `json:"assignedMinutes"`
	Utilization      *float64  `json:"utilization"`
	Unavailability   *string   `json:"unavailability"`
}

// capacity serves members.capacity.
//
//	@Summary		members.capacity (read)
//	@ID				members.capacity
//	@Description	Authorization: admin:identity.read | admin:job.read | contractor:partner.assign:own-company
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot; IR94 \
//	@Description	role=technician only
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-P06, DD-P01, DD-P02, DD-P03 · Query: filters organizationId,qualification,activeOnly · \
//	@Description	sort id (default id asc)
//	@Tags			members
//	@Accept			json
//	@Produce		json
//	@Param			date			query		string	false	"input field date"
//	@Param			cursor			query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit			query		integer	false	"page size 1–100, default 25"
//	@Param			sort			query		string	false	"field:direction — fields id; default id asc"
//	@Param			organizationId	query		string	false	"filter → Membership.organizationId"
//	@Param			qualification	query		string	false	"filter → matching valid Membership.qualifications[].code (SR13)"
//	@Param			activeOnly		query		boolean	false	"filter → true => validFrom<=now<validUntil (null end infinite)"
//	@Success		200				{object}	ops.Envelope{data=CapacityPage}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/members/capacity [get]
func (m Workforce) capacity(
	ctx context.Context,
	call *ops.Call,
	input *CapacityInput,
) (paging.Page[Capacity], error) {
	day, _ := time.ParseInLocation("2006-01-02", input.Date, kualaLumpur)
	work := Slot{day.Add(9 * time.Hour).UTC(), day.Add(17 * time.Hour).UTC()}
	weekend := day.Weekday() == time.Saturday || day.Weekday() == time.Sunday

	filter, err := decodeTechFilter(input.Query)
	if err != nil {
		return paging.Page[Capacity]{}, err
	}

	techs, err := m.technicians(ctx, call, false)
	if err == nil {
		techs, err = filter.keep(ctx, call, techs)
	}

	if err != nil {
		return paging.Page[Capacity]{}, err
	}

	out := []Capacity{}

	for _, member := range techs {
		capacity := Capacity{
			MembershipID:     member.ID,
			Date:             input.Date,
			AvailableSlots:   []Slot{},
			AssignedSlots:    []Slot{},
			AvailableMinutes: nil,
			AssignedMinutes:  0,
			Utilization:      nil,
			Unavailability:   nil,
		}

		typ, off, err := unavailable(
			ctx,
			call,
			member.ID,
			member.OrganizationID,
			input.Date,
			input.Date,
		)
		if err != nil {
			return paging.Page[Capacity]{}, err
		}

		if off {
			capacity.Unavailability = &typ
		}

		avail := 0

		if !weekend && !off {
			capacity.AvailableSlots = []Slot{work}
			avail = int(work.EndAt.Sub(work.StartAt) / time.Minute)
		}

		capacity.AvailableMinutes = &avail

		rows, err := call.Tx.Query(
			ctx,
			`SELECT lower(scheduled), upper(scheduled) FROM maintenance.assignments WHERE technician_membership_id = $1 AND
				status = 'active'
			AND scheduled && tstzrange($2, $3) ORDER BY lower(scheduled)`,
			member.ID,
			work.StartAt,
			work.EndAt,
		)
		if err != nil {
			return paging.Page[Capacity]{}, apperr.Wrap(err, "select maintenance.assignments")
		}

		for rows.Next() {
			var slot Slot

			err := rows.Scan(&slot.StartAt, &slot.EndAt)
			if err != nil {
				rows.Close()

				return paging.Page[Capacity]{}, apperr.Wrap(err, "select maintenance.assignments")
			}

			if slot.StartAt.Before(work.StartAt) {
				slot.StartAt = work.StartAt
			}

			if slot.EndAt.After(work.EndAt) {
				slot.EndAt = work.EndAt
			}

			capacity.AssignedSlots = append(capacity.AssignedSlots, slot)
		}

		rows.Close()

		rowsErr := rows.Err()
		if rowsErr != nil {
			return paging.Page[Capacity]{}, apperr.Wrap(rowsErr, "select maintenance.assignments")
		}

		if len(capacity.AvailableSlots) > 0 {
			capacity.AssignedMinutes = unionMinutes(capacity.AssignedSlots)
		}

		if avail > 0 {
			u := math.Round(float64(capacity.AssignedMinutes)*1000/float64(avail)) / 10
			capacity.Utilization = &u
		}

		out = append(out, capacity)
	}

	sort.Slice(
		out,
		func(i, j int) bool { return out[i].MembershipID.String() < out[j].MembershipID.String() },
	)

	sortSliceErr := paging.SortSlice(out, input.Query.Sort, map[string]func(a, b Capacity) int{
		"id": func(a, b Capacity) int { return strings.Compare(a.MembershipID.String(), b.MembershipID.String()) },
	})
	if sortSliceErr != nil {
		return paging.Page[Capacity]{}, apperr.Wrap(sortSliceErr, "paging.SortSlice")
	}

	return pageOf(out, input.Query, input.Date, call.Principal.ScopeVersion)
}

// unionMinutes returns the minutes covered by the union of the slots.
func unionMinutes(slots []Slot) int {
	sorted := append([]Slot(nil), slots...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].StartAt.Before(sorted[j].StartAt) })

	var (
		total time.Duration
		cur   *Slot
	)

	for idx := range sorted {
		switch {
		case cur == nil:
			c := sorted[idx]
			cur = &c
		case !sorted[idx].StartAt.After(cur.EndAt):
			if sorted[idx].EndAt.After(cur.EndAt) {
				cur.EndAt = sorted[idx].EndAt
			}
		default:
			total += cur.EndAt.Sub(cur.StartAt)
			c := sorted[idx]
			cur = &c
		}
	}

	if cur != nil {
		total += cur.EndAt.Sub(cur.StartAt)
	}

	return int(total / time.Minute)
}

// ---- members.setUnavailability ----

// UnavailabilityInput is members.setUnavailability input.
type UnavailabilityInput struct {
	MembershipID *uuid.UUID `json:"membershipId"`
	From         string     `json:"from"`
	To           string     `json:"to"`
	Type         string     `json:"type"`
	Note         *string    `json:"note,omitempty"`
}

var unavailabilityTypes = map[string]bool{
	"annual_leave":   true,
	"training":       true,
	"public_holiday": true,
	"sick":           true,
	"other":          true,
}

// Validate implements ops.Validator.
func (in *UnavailabilityInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	f, err1 := time.Parse("2006-01-02", in.From)

	t, err2 := time.Parse("2006-01-02", in.To)
	if err1 != nil || err2 != nil || t.Before(f) || t.Sub(f) > 30*24*time.Hour {
		fieldErrs["to"] = apperr.KeyRange
	}

	if !unavailabilityTypes[in.Type] {
		fieldErrs["type"] = apperr.KeyInvalid
	}

	if in.Note != nil {
		*in.Note = strings.TrimSpace(*in.Note)
		if utf8.RuneCountInString(*in.Note) > 500 {
			fieldErrs["note"] = apperr.KeyLength
		}

		if *in.Note == "" {
			in.Note = nil
		}
	}

	return fieldErrs
}

// Unavailability is Unavailability of service-contracts.ts.
type Unavailability struct {
	ID                       uuid.UUID   `json:"id"`
	TenantID                 uuid.UUID   `json:"tenantId"`
	Version                  int         `json:"version"`
	CreatedAt                time.Time   `json:"createdAt"`
	UpdatedAt                time.Time   `json:"updatedAt"`
	OrganizationID           uuid.UUID   `json:"organizationId"`
	MembershipID             *uuid.UUID  `json:"membershipId"`
	From                     string      `json:"from"`
	To                       string      `json:"to"`
	Type                     string      `json:"type"`
	Note                     *string     `json:"note"`
	ConflictingAssignmentIDs []uuid.UUID `json:"conflictingAssignmentIds"`
}

// setUnavailability serves members.setUnavailability.
//
//	@Summary		members.setUnavailability (write)
//	@ID				members.setUnavailability
//	@Description	Authorization: contractor:partner.assign:own-company | admin:job.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR111 from<=to, at most 31 days; conflicting assignments are reported, not changed
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-P06
//	@Tags			members
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string				true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		UnavailabilityInput	true	"input"
//	@Success		200				{object}	ops.Envelope{data=Unavailability}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/members/unavailability [post]
func (m Workforce) setUnavailability(
	ctx context.Context,
	call *ops.Call,
	input *UnavailabilityInput,
) (Unavailability, error) {
	org := call.Principal.OrgID
	if input.MembershipID != nil {
		technician, found, err := m.Delivery.Directory.Technician(ctx, call, *input.MembershipID)
		if err != nil {
			return Unavailability{}, apperr.Wrap(err, "maintenance.Directory.Technician")
		}

		if !found ||
			(call.Principal.Role == "contractor" && technician.OrgID != call.Principal.OrgID) {
			return Unavailability{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
		}

		org = technician.OrgID
	}

	f, _ := time.ParseInLocation("2006-01-02", input.From, kualaLumpur)
	t, _ := time.ParseInLocation("2006-01-02", input.To, kualaLumpur)
	stmt := `SELECT a.id FROM maintenance.assignments a JOIN maintenance.ref_memberships mb ON mb.id =
		a.technician_membership_id
		WHERE a.status = 'active' AND a.scheduled && tstzrange($1, $2) AND `
	args := []any{f.UTC(), t.AddDate(0, 0, 1).UTC()}

	if input.MembershipID != nil {
		stmt += "a.technician_membership_id = $3"

		args = append(args, *input.MembershipID)
	} else {
		stmt += "mb.organization_id = $3"

		args = append(args, org)
	}

	rows, err := call.Tx.Query(ctx, stmt+" ORDER BY a.id", args...)
	if err != nil {
		return Unavailability{}, apperr.Wrap(err, "query (setUnavailability)")
	}

	conflicts := []uuid.UUID{}

	for rows.Next() {
		var assignmentID uuid.UUID

		err := rows.Scan(&assignmentID)
		if err != nil {
			rows.Close()

			return Unavailability{}, apperr.Wrap(err, "query (setUnavailability)")
		}

		conflicts = append(conflicts, assignmentID)
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return Unavailability{}, apperr.Wrap(rowsErr, "query (setUnavailability)")
	}

	unavailability := Unavailability{
		TenantID:                 call.Principal.TenantID,
		Version:                  1,
		CreatedAt:                call.Now,
		UpdatedAt:                call.Now,
		OrganizationID:           org,
		MembershipID:             input.MembershipID,
		From:                     input.From,
		To:                       input.To,
		Type:                     input.Type,
		Note:                     input.Note,
		ConflictingAssignmentIDs: conflicts, ID: uuid.UUID{},
	}

	scanErr := call.Tx.QueryRow(ctx, `INSERT INTO maintenance.unavailability (tenant_id, organization_id, membership_id,
		days,
		type, note, conflicting_assignment_ids, created_at, updated_at)
		VALUES (current_setting('app.tenant_id')::uuid, $1, $2, daterange($3::date, $4::date, '[]'), $5, $6, $7, $8, $8)
		RETURNING id`,
		org, input.MembershipID, input.From, input.To, input.Type, input.Note, conflicts, call.Now).
		Scan(&unavailability.ID)
	if scanErr != nil {
		return unavailability, apperr.Wrap(scanErr, "insert maintenance.unavailability")
	}

	call.Emit(
		outboxEvent(
			"unavailability",
			unavailability.ID,
			"UnavailabilitySet",
			map[string]any{"organizationId": org},
		),
	)
	call.Audit(
		auditEntry(
			"members.setUnavailability",
			"unavailability",
			unavailability.ID,
			nil,
			&unavailability.Version,
			"",
		),
	)

	return unavailability, nil
}

// unavailabilityCols are the columns scanUnavailability reads: the days as the first and the last day.
const unavailabilityCols = `id, tenant_id, version, created_at, updated_at, organization_id, membership_id, ` +
	`lower(days), upper(days) - 1, type, note, conflicting_assignment_ids`

func scanUnavailability(row pgx.Row) (Unavailability, error) {
	var (
		unavailability Unavailability
		first, last    time.Time
	)

	err := row.Scan(
		&unavailability.ID,
		&unavailability.TenantID,
		&unavailability.Version,
		&unavailability.CreatedAt,
		&unavailability.UpdatedAt,
		&unavailability.OrganizationID,
		&unavailability.MembershipID,
		&first,
		&last,
		&unavailability.Type,
		&unavailability.Note,
		&unavailability.ConflictingAssignmentIDs,
	)
	unavailability.From, unavailability.To = first.Format("2006-01-02"), last.Format("2006-01-02")

	return unavailability, apperr.Wrap(err, "scan (scanUnavailability)")
}

// ---- members.unavailability ----

// unavailabilityList answers the unavailable days that are not over yet: the rows whose last Kuala Lumpur day is today
// or later, a contractor's own company only (IR612).
//
//	@Summary		members.unavailability (read)
//	@ID				members.unavailability
//	@Description	Authorization: admin:identity.read | admin:job.read | contractor:partner.assign:own-company
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot; IR612 rows whose \
//	@Description	last Kuala Lumpur day is today or later; contractor sees own-company rows only
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-P06 · Query: filters organizationId,membershipId · sort id,from (default from asc;id asc)
//	@Tags			members
//	@Accept			json
//	@Produce		json
//	@Param			cursor			query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit			query		integer	false	"page size 1–100, default 25"
//	@Param			sort			query		string	false	"field:direction — fields id,from; default from asc;id asc"
//	@Param			organizationId	query		string	false	"filter → organizationId"
//	@Param			membershipId	query		string	false	"filter → membershipId, with the rows of that technician's whole company (membershipId null)"
//	@Success		200				{object}	ops.Envelope{data=UnavailabilityPage}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/members/unavailability [get]
func (m Workforce) unavailabilityList(
	ctx context.Context,
	call *ops.Call,
	query *paging.Query,
) (paging.Page[Unavailability], error) {
	var filters struct {
		OrganizationID *uuid.UUID `json:"organizationId,omitempty"`
		MembershipID   *uuid.UUID `json:"membershipId,omitempty"` // with the rows of the technician's whole company
	}

	if len(query.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(query.Filters)))
		dec.DisallowUnknownFields()

		if dec.Decode(&filters) != nil {
			return paging.Page[Unavailability]{}, apperr.Fields(
				map[string]string{"filters": apperr.KeyInvalid},
			)
		}
	}

	args := []any{call.Now.In(kualaLumpur).Format("2006-01-02")}
	add := func(v any) string {
		args = append(args, v)

		return "$" + strconv.Itoa(len(args))
	}

	conds := []string{"upper(days) > $1::date"}
	if call.Principal.Role == "contractor" {
		conds = append(conds, "organization_id = "+add(call.Principal.OrgID))
	}

	if filters.OrganizationID != nil {
		conds = append(conds, "organization_id = "+add(*filters.OrganizationID))
	}

	if filters.MembershipID != nil {
		p := add(*filters.MembershipID)
		conds = append(
			conds,
			"(membership_id = "+p+" OR (membership_id IS NULL AND organization_id IN (SELECT organization_id FROM "+
				"maintenance.ref_memberships WHERE id = "+p+")))",
		)
	}

	rows, err := call.Tx.Query(
		ctx,
		"SELECT "+unavailabilityCols+" FROM maintenance.unavailability WHERE "+strings.Join(
			conds,
			" AND ",
		)+" ORDER BY lower(days), id",
		args...)
	if err != nil {
		return paging.Page[Unavailability]{}, apperr.Wrap(err, "select maintenance.unavailability")
	}

	all := []Unavailability{}

	for rows.Next() {
		unavailability, err := scanUnavailability(rows)
		if err != nil {
			rows.Close()

			return paging.Page[Unavailability]{}, err
		}

		all = append(all, unavailability)
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return paging.Page[Unavailability]{}, apperr.Wrap(
			rowsErr,
			"select maintenance.unavailability",
		)
	}

	sortSliceErr := paging.SortSlice(all, query.Sort, map[string]func(a, b Unavailability) int{
		"id":   func(a, b Unavailability) int { return strings.Compare(a.ID.String(), b.ID.String()) },
		"from": func(a, b Unavailability) int { return strings.Compare(a.From, b.From) },
	})
	if sortSliceErr != nil {
		return paging.Page[Unavailability]{}, apperr.Wrap(sortSliceErr, "paging.SortSlice")
	}

	return pageOf(all, *query, filters, call.Principal.ScopeVersion)
}

// ---- members.clearUnavailability ----

// ClearUnavailabilityInput is members.clearUnavailability input.
type ClearUnavailabilityInput struct {
	ID   uuid.UUID `json:"id"`
	From string    `json:"from"`
}

// Validate implements ops.Validator.
func (in *ClearUnavailabilityInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.ID == uuid.Nil {
		fieldErrs["id"] = apperr.KeyRequired
	}

	_, err := time.Parse("2006-01-02", in.From)
	if err != nil {
		fieldErrs["from"] = apperr.KeyInvalid
	}

	return fieldErrs
}

// UnavailabilityCleared is UnavailabilityCleared of service-contracts.ts: the row's days that remain, or null when the
// row is gone.
type UnavailabilityCleared struct {
	ID        uuid.UUID       `json:"id"`
	Remaining *Unavailability `json:"remaining"`
}

// clearUnavailability clears a row's days from `from` (a Kuala Lumpur day, today or later) to its last day (IR612):
// from on or before its first day removes the row; a later day keeps the earlier days, so days already passed stay
// recorded.
//
//	@Summary		members.clearUnavailability (write)
//	@ID				members.clearUnavailability
//	@Description	Authorization: contractor:partner.assign:own-company | admin:job.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR612 from is a Kuala Lumpur day from today to the row's last day; from on or before its first day \
//	@Description	removes the row, a later day keeps the earlier days (new last day = from - 1); days before today \
//	@Description	stay recorded
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-P06
//	@Tags			members
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string						true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer						true	"all: required (target unavailability, read members.unavailability)"
//	@Param			id					path		string						true	"input field id"
//	@Param			request				body		ClearUnavailabilityInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=UnavailabilityCleared}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/members/unavailability/{id}/clear [post]
func (m Workforce) clearUnavailability(
	ctx context.Context,
	call *ops.Call,
	input *ClearUnavailabilityInput,
) (UnavailabilityCleared, error) {
	unavailability, err := scanUnavailability(
		call.Tx.QueryRow(
			ctx,
			`SELECT `+unavailabilityCols+` FROM maintenance.unavailability WHERE id = $1 FOR UPDATE`,
			input.ID,
		),
	)
	if errors.Is(err, pgx.ErrNoRows) ||
		(err == nil && call.Principal.Role == "contractor" && unavailability.OrganizationID != call.Principal.OrgID) {
		return UnavailabilityCleared{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return UnavailabilityCleared{}, err
	}

	if unavailability.Version != *call.ExpectedVersion {
		return UnavailabilityCleared{}, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	if input.From < call.Now.In(kualaLumpur).Format("2006-01-02") ||
		input.From > unavailability.To {
		return UnavailabilityCleared{}, apperr.Fields(map[string]string{"from": apperr.KeyRange})
	}

	payload := map[string]any{"organizationId": unavailability.OrganizationID, "from": input.From}
	if input.From <= unavailability.From {
		_, err := call.Tx.Exec(
			ctx,
			`DELETE FROM maintenance.unavailability WHERE id = $1`,
			unavailability.ID,
		)
		if err != nil {
			return UnavailabilityCleared{}, apperr.Wrap(err, "delete maintenance.unavailability")
		}

		call.Emit(
			outboxEvent("unavailability", unavailability.ID, "UnavailabilityCleared", payload),
		)
		call.Audit(
			auditEntry(
				"members.clearUnavailability",
				"unavailability",
				unavailability.ID,
				call.ExpectedVersion,
				nil,
				"",
			),
		)

		return UnavailabilityCleared{ID: unavailability.ID, Remaining: nil}, nil
	}

	from, _ := time.Parse("2006-01-02", input.From)

	unavailability.To, unavailability.Version, unavailability.UpdatedAt = from.AddDate(0, 0, -1).
		Format("2006-01-02"),
		unavailability.Version+1, call.Now

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE maintenance.unavailability SET days = daterange(lower(days), $2::date, '[)'), version = $3, updated_at = $4
			WHERE id = $1`,
		unavailability.ID,
		input.From,
		unavailability.Version,
		call.Now,
	)
	if execErr != nil {
		return UnavailabilityCleared{}, apperr.Wrap(execErr, "update maintenance.unavailability")
	}

	call.Emit(outboxEvent("unavailability", unavailability.ID, "UnavailabilityCleared", payload))
	call.Audit(
		auditEntry(
			"members.clearUnavailability",
			"unavailability",
			unavailability.ID,
			call.ExpectedVersion,
			&unavailability.Version,
			"",
		),
	)

	return UnavailabilityCleared{ID: unavailability.ID, Remaining: &unavailability}, nil
}

// RegisterWorkforce binds the IR132 maintenance-side member operations and the unavailable days' list and clear
// (IR612).
func RegisterWorkforce(r *ops.Registry, m Workforce) {
	ops.Register(r, "members.eligible", m.eligible)
	ops.Register(r, "members.capacity", m.capacity)
	ops.Register(r, "members.setUnavailability", m.setUnavailability)
	ops.Register(r, "members.unavailability", m.unavailabilityList)
	ops.Register(r, "members.clearUnavailability", m.clearUnavailability)
}
