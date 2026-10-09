package maintenance

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// listItem is one jobs.list row of any projection with its public sort keys (IR23).
type listItem struct {
	rank int        // status rank of the public status
	sev  int        // severity rank; -1 = null (undisclosed)
	due  *time.Time // nil = null
	id   string
	item any
}

var rankOf = func() map[string]int {
	m := map[string]int{}
	for i, s := range jobStatuses {
		m[s] = i
	}
	return m
}()

// sortItems applies the IR23/IR34 order: desc reverses the rank only, null last both ways, ties id asc.
func sortItems(items []listItem, field string, desc bool) {
	slices.SortStableFunc(items, func(a, b listItem) int {
		var d int
		switch field {
		case "status":
			d = a.rank - b.rank
		case "severity":
			switch {
			case a.sev < 0 && b.sev < 0:
			case a.sev < 0:
				return 1
			case b.sev < 0:
				return -1
			default:
				d = a.sev - b.sev
			}
		case "dueAt":
			switch {
			case a.due == nil && b.due == nil:
			case a.due == nil:
				return 1
			case b.due == nil:
				return -1
			default:
				d = a.due.Compare(*b.due)
			}
		case "id":
			d = strings.Compare(a.id, b.id)
		}
		if desc {
			d = -d
		}
		if d == 0 && field != "id" {
			d = strings.Compare(a.id, b.id)
		}
		return d
	})
}

// OfferSummary is JobOfferSummary of service-contracts.ts.
type OfferSummary struct {
	Projection             string    `json:"projection"`
	Status                 string    `json:"status"`
	Severity               *string   `json:"severity"`
	JobID                  uuid.UUID `json:"jobId"`
	JobVersion             int       `json:"jobVersion"`
	OfferID                uuid.UUID `json:"offerId"`
	Type                   string    `json:"type"`
	SiteAddress            *string   `json:"siteAddress"`
	RequiredQualifications []string  `json:"requiredQualifications"`
	RequestedSlot          Slot      `json:"requestedSlot"`
	DueAt                  time.Time `json:"dueAt"`
	OfferExpiresAt         time.Time `json:"offerExpiresAt"`
	TermsVersion           string    `json:"termsVersion"`
	Origin                 string    `json:"origin"`
	VisitSlot              Slot      `json:"visitSlot"`
	OfferedAt              time.Time `json:"offeredAt"`        // when HQ sent the offer (IR226)
	AccessValidFrom        time.Time `json:"accessValidFrom"`  // the delegation (access) period if accepted (IR226)
	AccessValidUntil       time.Time `json:"accessValidUntil"` //
	PartnerSlotProposal    any       `json:"partnerSlotProposal"`
}

// PartnerSlotProposal is the company's latest time-change proposal of an offer (IR113), withdrawn ones excluded.
type PartnerSlotProposal struct {
	ID                     uuid.UUID `json:"id"`
	OfferID                uuid.UUID `json:"offerId"`
	Slot                   Slot      `json:"slot"`
	TechnicianMembershipID uuid.UUID `json:"technicianMembershipId"`
	Reason                 string    `json:"reason"`
	SentAt                 time.Time `json:"sentAt"`
	Status                 string    `json:"status"`
}

// History is JobHistorySnapshot of service-contracts.ts.
type History struct {
	Projection            string         `json:"projection"`
	AsOf                  time.Time      `json:"asOf"`
	Severity              *string        `json:"severity"`
	DueAt                 *time.Time     `json:"dueAt"`
	JobID                 uuid.UUID      `json:"jobId"`
	Type                  string         `json:"type"`
	Status                string         `json:"status"`
	ContractorOrgID       *uuid.UUID     `json:"contractorOrgId"`
	CompletedAt           *time.Time     `json:"completedAt"`
	OwnDecisionEvents     []Event        `json:"ownDecisionEvents"`
	RedactedReportSummary map[string]any `json:"redactedReportSummary"`
}

// SiteAddresses resolves IR25 site addresses (Assets).
type SiteAddresses interface {
	SiteAddress(ctx context.Context, c *ops.Call, unit uuid.UUID) (*string, []string, error)
}

// offers returns the contractor's JobOfferSummary rows valid now (IR124 item 2); job limits the result to one job.
func (m Jobs) offers(ctx context.Context, c *ops.Call, job *uuid.UUID) ([]OfferSummary, error) {
	q := `SELECT j.id, j.version, j.type, j.unit_id, lower(j.requested_slot), upper(j.requested_slot), j.due_at, j.origin,
		o.id, o.decision, o.offer_expires_at, o.terms_version, lower(o.visit_slot), upper(o.visit_slot), o.offered_at, o.access_valid_from, o.access_valid_until
		FROM maintenance.offers o JOIN maintenance.jobs j ON j.id = o.job_id
		WHERE o.contractor_org_id = $1 AND ((o.decision IS NULL AND $2 < o.offer_expires_at AND j.status = 'offered') OR (o.decision = 'accept' AND $2 < o.access_valid_from))`
	args := []any{c.Principal.OrgID, c.Now}
	if job != nil {
		q += " AND j.id = $3"
		args = append(args, *job)
	}
	rows, err := c.Tx.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	var out []OfferSummary
	var units []uuid.UUID
	for rows.Next() {
		var o OfferSummary
		var unit uuid.UUID
		var decision *string
		if err := rows.Scan(&o.JobID, &o.JobVersion, &o.Type, &unit, &o.RequestedSlot.StartAt, &o.RequestedSlot.EndAt, &o.DueAt, &o.Origin,
			&o.OfferID, &decision, &o.OfferExpiresAt, &o.TermsVersion, &o.VisitSlot.StartAt, &o.VisitSlot.EndAt, &o.OfferedAt, &o.AccessValidFrom, &o.AccessValidUntil); err != nil {
			rows.Close()
			return nil, err
		}
		o.Projection, o.Status = "offer", "offered"
		if decision != nil {
			o.Status = "accepted"
		}
		out, units = append(out, o), append(units, unit)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		addr, scope, err := m.Sites.SiteAddress(ctx, c, units[i])
		if err != nil {
			return nil, err
		}
		out[i].SiteAddress, out[i].RequiredQualifications = addr, requiredCodes(scope)
		// the offer's latest time-change proposal (it was never filled before IR226)
		var pp PartnerSlotProposal
		err = c.Tx.QueryRow(ctx, `SELECT id, offer_id, lower(slot), upper(slot), technician_membership_id, reason, sent_at, status FROM maintenance.partner_slot_proposals
			WHERE offer_id = $1 AND status <> 'withdrawn' ORDER BY sent_at DESC, id LIMIT 1`, out[i].OfferID).Scan(&pp.ID, &pp.OfferID, &pp.Slot.StartAt, &pp.Slot.EndAt,
			&pp.TechnicianMembershipID, &pp.Reason, &pp.SentAt, &pp.Status)
		switch {
		case err == nil:
			out[i].PartnerSlotProposal = pp
		case !errors.Is(err, pgx.ErrNoRows):
			return nil, err
		}
	}
	return out, nil
}

func requiredCodes(scope []string) []string {
	codes := []string{}
	for _, s := range scope {
		if q, ok := QualificationRequirements[s]; ok && !slices.Contains(codes, q) {
			codes = append(codes, q)
		}
	}
	return codes
}

// histories returns the caller's frozen snapshots (contractor organization or technician membership).
func (m Jobs) histories(ctx context.Context, c *ops.Call, job *uuid.UUID) ([]History, error) {
	kind, owner := "organization", c.Principal.OrgID
	if c.Principal.Role == "technician" {
		kind, owner = "membership", c.Principal.MembershipID
	}
	q := `SELECT job_id, as_of, type, status, contractor_org_id, completed_at, has_report, acceptance, decision_event_ids
		FROM maintenance.job_history_snapshots WHERE owner_kind = $1 AND owner_id = $2`
	args := []any{kind, owner}
	if job != nil {
		q += " AND job_id = $3"
		args = append(args, *job)
	}
	rows, err := c.Tx.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	var out []History
	var evIDs [][]uuid.UUID
	for rows.Next() {
		var h History
		var has bool
		var acc string
		var ids []uuid.UUID
		if err := rows.Scan(&h.JobID, &h.AsOf, &h.Type, &h.Status, &h.ContractorOrgID, &h.CompletedAt, &has, &acc, &ids); err != nil {
			rows.Close()
			return nil, err
		}
		h.Projection = "history"
		h.RedactedReportSummary = map[string]any{"hasReport": has, "acceptance": acc}
		out, evIDs = append(out, h), append(evIDs, ids)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		out[i].OwnDecisionEvents = []Event{}
		if len(evIDs[i]) == 0 {
			continue
		}
		evs, err := m.eventRows(ctx, c, "e.id = ANY($1)", []any{evIDs[i]}, "e.occurred_at ASC, e.id ASC", 1000, 0)
		if err != nil {
			return nil, err
		}
		for _, e := range evs {
			e.Note, e.ReportRef = nil, nil
			out[i].OwnDecisionEvents = append(out[i].OwnDecisionEvents, e)
		}
	}
	return out, nil
}

// private reports whether a filter that offer/history projections never match is set (IR23).
func (f *listFilters) private() bool {
	return f.Severity != nil || f.UnitID != nil || f.UnitIDs != nil || f.MembershipID != nil || f.CustomerID != nil || f.PropertyID != nil || f.ProposalPending != nil
}

func (f *listFilters) statusOK(s string) bool {
	return (f.Status == nil || *f.Status == s) && (f.Statuses == nil || slices.Contains(*f.Statuses, s))
}

func inPeriod(f *listFilters, t *time.Time) bool {
	if f.From == nil && f.To == nil {
		return true
	}
	if t == nil {
		return false
	}
	return (f.From == nil || !t.Before(*f.From)) && (f.To == nil || t.Before(*f.To))
}

// extraProjections returns the offer and history rows of contractors and technicians after the IR23 filters.
func (m Jobs) extraProjections(ctx context.Context, c *ops.Call, f *listFilters) ([]listItem, error) {
	if f.private() {
		return nil, nil
	}
	var out []listItem
	if c.Principal.Role == "contractor" {
		offers, err := m.offers(ctx, c, nil)
		if err != nil {
			return nil, err
		}
		for _, o := range offers {
			start := o.RequestedSlot.StartAt
			if !f.statusOK(o.Status) || !inPeriod(f, &start) || (f.Origin != nil && *f.Origin != o.Origin) ||
				(f.OrganizationID != nil && *f.OrganizationID != c.Principal.OrgID) || (f.OverdueOnly != nil && *f.OverdueOnly && !c.Now.After(o.DueAt)) {
				continue
			}
			due := o.DueAt
			out = append(out, listItem{rank: rankOf[o.Status], sev: -1, due: &due, id: o.JobID.String(), item: o})
		}
	}
	hs, err := m.histories(ctx, c, nil)
	if err != nil {
		return nil, err
	}
	for _, h := range hs {
		if !f.statusOK(h.Status) || !inPeriod(f, h.CompletedAt) || f.Origin != nil || (f.OverdueOnly != nil && *f.OverdueOnly) ||
			(f.OrganizationID != nil && (h.ContractorOrgID == nil || *h.ContractorOrgID != *f.OrganizationID)) {
			continue
		}
		out = append(out, listItem{rank: rankOf[h.Status], sev: -1, id: h.JobID.String(), item: h})
	}
	return out, nil
}

// projection resolves jobs.get for contractors and technicians: detail, then offer, then history (IR124 item 5).
func (m Jobs) projection(ctx context.Context, c *ops.Call, id uuid.UUID) (any, string, error) {
	j, err := m.detail(ctx, c, id)
	if err == nil {
		return j, "detail", nil
	}
	if apperr.From(err).Code != apperr.NotFound {
		return nil, "", err
	}
	if c.Principal.Role == "contractor" {
		offers, err := m.offers(ctx, c, &id)
		if err != nil {
			return nil, "", err
		}
		if len(offers) > 0 {
			return offers[0], "offer", nil
		}
	}
	hs, err := m.histories(ctx, c, &id)
	if err != nil {
		return nil, "", err
	}
	if len(hs) > 0 {
		return hs[0], "history", nil
	}
	return nil, "", apperr.E(apperr.NotFound, "error.notFound")
}

// ---- worker: freeze histories (IR124 item 1) ----

// FreezeEnded stores a JobHistorySnapshot for every contractor access window and technician viewing window that has
// ended by now and has none yet. It runs in the worker tick (one transaction per tenant).
func FreezeEnded(ctx context.Context, tx pgx.Tx, now time.Time) (int, error) {
	n := 0
	// contractors: accepted Offers whose access window ended
	tag, err := tx.Exec(ctx, `INSERT INTO maintenance.job_history_snapshots (tenant_id, job_id, owner_kind, owner_id, as_of, type, status, contractor_org_id,
			completed_at, has_report, acceptance, decision_event_ids)
		SELECT j.tenant_id, j.id, 'organization', o.contractor_org_id, o.access_valid_until, j.type, j.status, j.contractor_org_id, j.completed_at,
			EXISTS (SELECT 1 FROM maintenance.work_reports r WHERE r.job_id = j.id AND r.state <> 'draft'),
			CASE WHEN EXISTS (SELECT 1 FROM maintenance.work_reports r WHERE r.job_id = j.id AND r.state = 'accepted') THEN 'accepted' ELSE 'not_accepted' END,
			COALESCE((SELECT array_agg(e.id ORDER BY e.occurred_at, e.id) FROM maintenance.job_events e
				JOIN maintenance.ref_memberships mb ON mb.user_id = e.actor_user_id AND mb.organization_id = o.contractor_org_id
				WHERE e.job_id = j.id AND e.action IN ('offer.accepted','offer.declined')), '{}')
		FROM maintenance.offers o JOIN maintenance.jobs j ON j.id = o.job_id
		WHERE o.decision = 'accept' AND o.access_valid_until <= $1
		ON CONFLICT (job_id, owner_kind, owner_id) DO NOTHING`, now)
	if err != nil {
		return 0, err
	}
	n += int(tag.RowsAffected())
	t, err := freezeTechnicians(ctx, tx, now, nil)
	return n + t, err
}

// freezeTechnicians stores the technicians' snapshots whose viewing window has ended: the active Assignment's
// scheduled end passed, it was revoked after the work window started, or the job completed and released it (IR234);
// job narrows it to one job.
func freezeTechnicians(ctx context.Context, tx pgx.Tx, now time.Time, job *uuid.UUID) (int, error) {
	q := `INSERT INTO maintenance.job_history_snapshots (tenant_id, job_id, owner_kind, owner_id, owner_user_id, as_of, type, status,
			contractor_org_id, completed_at, has_report, acceptance)
		SELECT DISTINCT ON (a.job_id, a.technician_membership_id) j.tenant_id, j.id, 'membership', a.technician_membership_id, mb.user_id,
			CASE WHEN a.status IN ('revoked', 'completed') THEN a.updated_at ELSE upper(a.scheduled) END, j.type, j.status, j.contractor_org_id, j.completed_at,
			EXISTS (SELECT 1 FROM maintenance.work_reports r WHERE r.job_id = j.id AND r.state <> 'draft'),
			CASE WHEN EXISTS (SELECT 1 FROM maintenance.work_reports r WHERE r.job_id = j.id AND r.state = 'accepted') THEN 'accepted' ELSE 'not_accepted' END
		FROM maintenance.assignments a JOIN maintenance.jobs j ON j.id = a.job_id JOIN maintenance.ref_memberships mb ON mb.id = a.technician_membership_id
		WHERE ((a.status = 'active' AND upper(a.scheduled) <= $1) OR (a.status = 'revoked' AND lower(a.scheduled) <= a.updated_at AND a.updated_at <= $1)
			OR (a.status = 'completed' AND a.updated_at <= $1))`
	args := []any{now}
	if job != nil {
		q += " AND a.job_id = $2"
		args = append(args, *job)
	}
	tag, err := tx.Exec(ctx, q+`
		ORDER BY a.job_id, a.technician_membership_id, a.updated_at DESC
		ON CONFLICT (job_id, owner_kind, owner_id) DO NOTHING`, args...)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// ReleaseOnCompletion applies DEC-73 / IR234 when a job completes: its active Assignment becomes completed (reason
// job_completed), so it no longer books the technician (overlap constraint, members.eligible, members.capacity) and
// no longer grants unit access (change capture publishes active=false); the technician's viewing window ends with it
// and the history snapshot is stored at once, so the finished job stays in the technician's list (IR124).
func ReleaseOnCompletion(ctx context.Context, tx pgx.Tx, job uuid.UUID, now time.Time) error {
	if _, err := tx.Exec(ctx, `UPDATE maintenance.assignments SET status = 'completed', reason = 'job_completed', version = version + 1, updated_at = $2
		WHERE job_id = $1 AND status = 'active'`, job, now); err != nil {
		return err
	}
	_, err := freezeTechnicians(ctx, tx, now, &job)
	return err
}

// ExpireOffers applies IR48: undecided Offers whose offerExpiresAt has passed return their job from offered to
// requested (contractorOrgId cleared, version + 1, one offer_expired event by the system actor). Offer.decision stays null.
func ExpireOffers(ctx context.Context, tx pgx.Tx, now time.Time, actor uuid.UUID) (int, error) {
	rows, err := tx.Query(ctx, `UPDATE maintenance.jobs j SET status = 'requested', contractor_org_id = NULL, version = j.version + 1, updated_at = platform.app_now()
		FROM maintenance.offers o WHERE o.job_id = j.id AND o.decision IS NULL AND o.expired_at IS NULL AND o.offer_expires_at <= $1 AND j.status = 'offered'
		RETURNING j.id, j.tenant_id`, now)
	if err != nil {
		return 0, err
	}
	type hit struct{ job, tenant uuid.UUID }
	var hits []hit
	for rows.Next() {
		var h hit
		if err := rows.Scan(&h.job, &h.tenant); err != nil {
			rows.Close()
			return 0, err
		}
		hits = append(hits, h)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `UPDATE maintenance.offers SET expired_at = $1 WHERE decision IS NULL AND expired_at IS NULL AND offer_expires_at <= $1`, now); err != nil {
		return 0, err
	}
	for _, h := range hits {
		if _, err := tx.Exec(ctx, `INSERT INTO maintenance.job_events (id, tenant_id, job_id, actor_user_id, action, occurred_at) VALUES ($1,$2,$3,$4,'offer_expired',$5)`,
			uuid.Must(uuid.NewV7()), h.tenant, h.job, actor, now); err != nil {
			return 0, err
		}
	}
	return len(hits), nil
}
