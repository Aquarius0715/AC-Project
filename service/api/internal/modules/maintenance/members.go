package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
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

// Workforce is members.eligible / members.capacity / members.setUnavailability (IR132 items 3–5).
type Workforce struct {
	Delivery Delivery
}

// technicians lists the active technician memberships the caller may plan (HQ: all, internal only when internalOnly; contractor: own).
func (m Workforce) technicians(ctx context.Context, c *ops.Call, internalOnly bool) ([]identity.Member, error) {
	q := `SELECT m.id, m.tenant_id, m.version, m.created_at, m.updated_at, m.user_id, m.organization_id, m.role, m.employment, m.scope_version, m.valid_from,
		m.valid_until, m.client_role FROM maintenance.ref_memberships m WHERE m.role = 'technician' AND m.valid_from <= $1 AND (m.valid_until IS NULL OR m.valid_until > $1)`
	args := []any{c.Now}
	switch {
	case c.Principal.Role == "contractor":
		q += " AND m.organization_id = $2"
		args = append(args, c.Principal.OrgID)
	case internalOnly:
		q += " AND m.employment = 'internal'"
	}
	rows, err := c.Tx.Query(ctx, q+" ORDER BY m.id", args...)
	if err != nil {
		return nil, err
	}
	var out []identity.Member
	for rows.Next() {
		var x identity.Member
		if err := rows.Scan(&x.ID, &x.TenantID, &x.Version, &x.CreatedAt, &x.UpdatedAt, &x.UserID, &x.OrganizationID, &x.Role, &x.Employment, &x.ScopeVersion,
			&x.ValidFrom, &x.ValidUntil, &x.ClientRole); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, x)
	}
	rows.Close()
	return out, rows.Err()
}

// techFilter is the members.eligible / members.capacity filter set of the query catalog: organizationId,
// qualification (a grant of that code valid now, SR13) and activeOnly (candidates are always memberships valid now, so
// it narrows nothing further).
type techFilter struct {
	OrganizationID *uuid.UUID `json:"organizationId,omitempty"`
	Qualification  *string    `json:"qualification,omitempty"`
	ActiveOnly     *bool      `json:"activeOnly,omitempty"`
}

func decodeTechFilter(q paging.Query) (techFilter, error) {
	var f techFilter
	if len(q.Filters) == 0 {
		return f, nil
	}
	dec := json.NewDecoder(strings.NewReader(string(q.Filters)))
	dec.DisallowUnknownFields()
	if dec.Decode(&f) != nil || (f.Qualification != nil && (!certCodes[*f.Qualification] || *f.Qualification == "other")) {
		return f, apperr.Fields(map[string]string{"filters": "error.invalid"})
	}
	return f, nil
}

// keep returns the technicians that match the filter; qualifications are loaded from identity only when needed.
func (f techFilter) keep(ctx context.Context, c *ops.Call, techs []identity.Member) ([]identity.Member, error) {
	if f.Qualification != nil {
		if err := identity.LoadMembers(ctx, c, techs); err != nil {
			return nil, err
		}
	}
	out := []identity.Member{}
	for _, t := range techs {
		if f.OrganizationID != nil && t.OrganizationID != *f.OrganizationID {
			continue
		}
		if f.Qualification != nil && !holds(t, *f.Qualification, c.Now) {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

// holds reports whether the member has a grant of code valid at now: [validFrom, validUntil) and not revoked.
func holds(m identity.Member, code string, now time.Time) bool {
	for _, g := range m.Qualifications {
		if g.Code == code && !g.ValidFrom.After(now) && now.Before(g.ValidUntil) && (g.RevokedAt == nil || now.Before(*g.RevokedAt)) {
			return true
		}
	}
	return false
}

func pageOf[T any](all []T, q paging.Query, hashOf any, scopeVersion int) (paging.Page[T], error) {
	w, err := paging.Resolve(q, hashOf, scopeVersion, 1)
	if err != nil {
		return paging.Page[T]{}, err
	}
	items := []T{}
	for i := w.Offset; i < len(all) && i < w.Offset+w.Limit; i++ {
		items = append(items, all[i])
	}
	return paging.Page[T]{Items: items, NextCursor: w.Next(len(all)), Total: len(all), SnapshotVersion: w.Snapshot}, nil
}

func klDates(from, to time.Time) (string, string) {
	return from.In(kualaLumpur).Format("2006-01-02"), to.Add(-time.Nanosecond).In(kualaLumpur).Format("2006-01-02")
}

// unavailable reports whether a membership (or its whole organization) is unavailable on any date in [d1, d2].
func unavailable(ctx context.Context, c *ops.Call, membership, org uuid.UUID, d1, d2 string) (string, bool, error) {
	var typ string
	err := c.Tx.QueryRow(ctx, `SELECT type FROM maintenance.unavailability WHERE (membership_id = $1 OR (membership_id IS NULL AND organization_id = $2))
		AND days && daterange($3::date, $4::date, '[]') ORDER BY created_at LIMIT 1`, membership, org, d1, d2).Scan(&typ)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	return typ, err == nil, err
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
	fe := map[string]string{}
	if in.JobID == uuid.Nil {
		fe["jobId"] = "error.required"
	}
	if !in.StartAt.Before(in.EndAt) {
		fe["endAt"] = "error.range"
	}
	return fe
}

func (m Workforce) eligible(ctx context.Context, c *ops.Call, in *EligibleInput) (paging.Page[identity.Member], error) {
	var unit uuid.UUID
	var contractor *uuid.UUID
	err := c.Tx.QueryRow(ctx, `SELECT unit_id, contractor_org_id FROM maintenance.jobs WHERE id = $1`, in.JobID).Scan(&unit, &contractor)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && c.Principal.Role == "contractor" && (contractor == nil || *contractor != c.Principal.OrgID)) {
		return paging.Page[identity.Member]{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return paging.Page[identity.Member]{}, err
	}
	f, err := decodeTechFilter(in.Query)
	if err != nil {
		return paging.Page[identity.Member]{}, err
	}
	techs, err := m.technicians(ctx, c, true)
	if err == nil {
		techs, err = f.keep(ctx, c, techs)
	}
	if err != nil {
		return paging.Page[identity.Member]{}, err
	}
	slot := Slot{in.StartAt, in.EndAt}
	d1, d2 := klDates(in.StartAt, in.EndAt)
	out := []identity.Member{}
	for _, t := range techs {
		isContractor := c.Principal.Role == "contractor"
		if m.Delivery.checkTechnician(ctx, c, unit, t.ID, isContractor, nil) != nil || m.Delivery.checkQualified(ctx, c, unit, t.ID, slot) != nil {
			continue
		}
		if _, off, err := unavailable(ctx, c, t.ID, t.OrganizationID, d1, d2); err != nil {
			return paging.Page[identity.Member]{}, err
		} else if off {
			continue
		}
		var busy bool
		if err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM maintenance.assignments WHERE technician_membership_id = $1 AND status = 'active' AND job_id <> $2
			AND scheduled && tstzrange($3, $4))`, t.ID, in.JobID, in.StartAt, in.EndAt).Scan(&busy); err != nil {
			return paging.Page[identity.Member]{}, err
		}
		if !busy {
			out = append(out, t)
		}
	}
	if err := identity.LoadMembers(ctx, c, out); err != nil {
		return paging.Page[identity.Member]{}, err
	}
	if err := paging.SortSlice(out, in.Query.Sort, map[string]func(a, b identity.Member) int{
		"id":        func(a, b identity.Member) int { return strings.Compare(a.ID.String(), b.ID.String()) },
		"createdAt": func(a, b identity.Member) int { return a.CreatedAt.Compare(b.CreatedAt) },
		"updatedAt": func(a, b identity.Member) int { return a.UpdatedAt.Compare(b.UpdatedAt) },
	}); err != nil {
		return paging.Page[identity.Member]{}, err
	}
	return pageOf(out, in.Query, in, c.Principal.ScopeVersion)
}

// ---- members.capacity ----

// CapacityInput is members.capacity input.
type CapacityInput struct {
	Date  string       `json:"date"`
	Query paging.Query `json:"query"`
}

// Validate implements ops.Validator.
func (in *CapacityInput) Validate() map[string]string {
	if _, err := time.ParseInLocation("2006-01-02", in.Date, kualaLumpur); err != nil {
		return map[string]string{"date": "error.invalid"}
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

func (m Workforce) capacity(ctx context.Context, c *ops.Call, in *CapacityInput) (paging.Page[Capacity], error) {
	day, _ := time.ParseInLocation("2006-01-02", in.Date, kualaLumpur)
	work := Slot{day.Add(9 * time.Hour).UTC(), day.Add(17 * time.Hour).UTC()}
	weekend := day.Weekday() == time.Saturday || day.Weekday() == time.Sunday
	f, err := decodeTechFilter(in.Query)
	if err != nil {
		return paging.Page[Capacity]{}, err
	}
	techs, err := m.technicians(ctx, c, false)
	if err == nil {
		techs, err = f.keep(ctx, c, techs)
	}
	if err != nil {
		return paging.Page[Capacity]{}, err
	}
	out := []Capacity{}
	for _, t := range techs {
		cp := Capacity{MembershipID: t.ID, Date: in.Date, AvailableSlots: []Slot{}, AssignedSlots: []Slot{}}
		typ, off, err := unavailable(ctx, c, t.ID, t.OrganizationID, in.Date, in.Date)
		if err != nil {
			return paging.Page[Capacity]{}, err
		}
		if off {
			cp.Unavailability = &typ
		}
		avail := 0
		if !weekend && !off {
			cp.AvailableSlots = []Slot{work}
			avail = int(work.EndAt.Sub(work.StartAt) / time.Minute)
		}
		cp.AvailableMinutes = &avail
		rows, err := c.Tx.Query(ctx, `SELECT lower(scheduled), upper(scheduled) FROM maintenance.assignments WHERE technician_membership_id = $1 AND status = 'active'
			AND scheduled && tstzrange($2, $3) ORDER BY lower(scheduled)`, t.ID, work.StartAt, work.EndAt)
		if err != nil {
			return paging.Page[Capacity]{}, err
		}
		for rows.Next() {
			var s Slot
			if err := rows.Scan(&s.StartAt, &s.EndAt); err != nil {
				rows.Close()
				return paging.Page[Capacity]{}, err
			}
			if s.StartAt.Before(work.StartAt) {
				s.StartAt = work.StartAt
			}
			if s.EndAt.After(work.EndAt) {
				s.EndAt = work.EndAt
			}
			cp.AssignedSlots = append(cp.AssignedSlots, s)
		}
		rows.Close()
		if len(cp.AvailableSlots) > 0 {
			cp.AssignedMinutes = unionMinutes(cp.AssignedSlots)
		}
		if avail > 0 {
			u := math.Round(float64(cp.AssignedMinutes)*1000/float64(avail)) / 10
			cp.Utilization = &u
		}
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MembershipID.String() < out[j].MembershipID.String() })
	if err := paging.SortSlice(out, in.Query.Sort, map[string]func(a, b Capacity) int{
		"id": func(a, b Capacity) int { return strings.Compare(a.MembershipID.String(), b.MembershipID.String()) },
	}); err != nil {
		return paging.Page[Capacity]{}, err
	}
	return pageOf(out, in.Query, in.Date, c.Principal.ScopeVersion)
}

// unionMinutes returns the minutes covered by the union of the slots.
func unionMinutes(slots []Slot) int {
	s := append([]Slot(nil), slots...)
	sort.Slice(s, func(i, j int) bool { return s[i].StartAt.Before(s[j].StartAt) })
	var total time.Duration
	var cur *Slot
	for i := range s {
		switch {
		case cur == nil:
			c := s[i]
			cur = &c
		case !s[i].StartAt.After(cur.EndAt):
			if s[i].EndAt.After(cur.EndAt) {
				cur.EndAt = s[i].EndAt
			}
		default:
			total += cur.EndAt.Sub(cur.StartAt)
			c := s[i]
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

var unavailabilityTypes = map[string]bool{"annual_leave": true, "training": true, "public_holiday": true, "sick": true, "other": true}

// Validate implements ops.Validator.
func (in *UnavailabilityInput) Validate() map[string]string {
	fe := map[string]string{}
	f, err1 := time.Parse("2006-01-02", in.From)
	t, err2 := time.Parse("2006-01-02", in.To)
	if err1 != nil || err2 != nil || t.Before(f) || t.Sub(f) > 30*24*time.Hour {
		fe["to"] = "error.range"
	}
	if !unavailabilityTypes[in.Type] {
		fe["type"] = "error.invalid"
	}
	if in.Note != nil {
		*in.Note = strings.TrimSpace(*in.Note)
		if utf8.RuneCountInString(*in.Note) > 500 {
			fe["note"] = "error.length"
		}
		if *in.Note == "" {
			in.Note = nil
		}
	}
	return fe
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

func (m Workforce) setUnavailability(ctx context.Context, c *ops.Call, in *UnavailabilityInput) (Unavailability, error) {
	org := c.Principal.OrgID
	if in.MembershipID != nil {
		t, found, err := m.Delivery.Directory.Technician(ctx, c, *in.MembershipID)
		if err != nil {
			return Unavailability{}, err
		}
		if !found || (c.Principal.Role == "contractor" && t.OrgID != c.Principal.OrgID) {
			return Unavailability{}, apperr.E(apperr.NotFound, "error.notFound")
		}
		org = t.OrgID
	}
	f, _ := time.ParseInLocation("2006-01-02", in.From, kualaLumpur)
	t, _ := time.ParseInLocation("2006-01-02", in.To, kualaLumpur)
	q := `SELECT a.id FROM maintenance.assignments a JOIN maintenance.ref_memberships mb ON mb.id = a.technician_membership_id
		WHERE a.status = 'active' AND a.scheduled && tstzrange($1, $2) AND `
	args := []any{f.UTC(), t.AddDate(0, 0, 1).UTC()}
	if in.MembershipID != nil {
		q += "a.technician_membership_id = $3"
		args = append(args, *in.MembershipID)
	} else {
		q += "mb.organization_id = $3"
		args = append(args, org)
	}
	rows, err := c.Tx.Query(ctx, q+" ORDER BY a.id", args...)
	if err != nil {
		return Unavailability{}, err
	}
	conflicts := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return Unavailability{}, err
		}
		conflicts = append(conflicts, id)
	}
	rows.Close()
	u := Unavailability{TenantID: c.Principal.TenantID, Version: 1, CreatedAt: c.Now, UpdatedAt: c.Now, OrganizationID: org, MembershipID: in.MembershipID,
		From: in.From, To: in.To, Type: in.Type, Note: in.Note, ConflictingAssignmentIDs: conflicts}
	if err := c.Tx.QueryRow(ctx, `INSERT INTO maintenance.unavailability (tenant_id, organization_id, membership_id, days, type, note, conflicting_assignment_ids, created_at)
		VALUES (current_setting('app.tenant_id')::uuid, $1, $2, daterange($3::date, $4::date, '[]'), $5, $6, $7, $8) RETURNING id`,
		org, in.MembershipID, in.From, in.To, in.Type, in.Note, conflicts, c.Now).Scan(&u.ID); err != nil {
		return u, err
	}
	c.Emit(ops.Event{AggregateType: "unavailability", AggregateID: u.ID, Type: "UnavailabilitySet", Payload: map[string]any{"organizationId": org}})
	c.Audit(ops.AuditEntry{Action: "members.setUnavailability", TargetKind: "unavailability", TargetID: u.ID.String(), NextVersion: &u.Version})
	return u, nil
}

// RegisterWorkforce binds the IR132 maintenance-side member operations.
func RegisterWorkforce(r *ops.Registry, m Workforce) {
	ops.Register(r, "members.eligible", m.eligible)
	ops.Register(r, "members.capacity", m.capacity)
	ops.Register(r, "members.setUnavailability", m.setUnavailability)
}
