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

// jobStatusRank is the position of a job status in the status order, 0 for a status that is not one.
func jobStatusRank(status string) int { return max(slices.Index(jobStatuses(), status), 0) }

// sortItems applies the IR23/IR34 order: desc reverses the rank only, null last both ways, ties id asc.
func sortItems(items []listItem, field string, desc bool) {
	slices.SortStableFunc(items, func(left, right listItem) int {
		var diff int

		switch field {
		case fieldStatus:
			diff = left.rank - right.rank
		case "severity":
			switch {
			case left.sev < 0 && right.sev < 0:
			case left.sev < 0:
				return 1
			case right.sev < 0:
				return -1
			default:
				diff = left.sev - right.sev
			}
		case fieldDueAt:
			switch {
			case left.due == nil && right.due == nil:
			case left.due == nil:
				return 1
			case right.due == nil:
				return -1
			default:
				diff = left.due.Compare(*right.due)
			}
		case "id":
			diff = strings.Compare(left.id, right.id)
		}

		if desc {
			diff = -diff
		}

		if diff == 0 && field != "id" {
			diff = strings.Compare(left.id, right.id)
		}

		return diff
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
	CandidateSlots         []Slot    `json:"candidateSlots"`   // the times the contractor may pick from (IR636)
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
func (m Jobs) offers(ctx context.Context, call *ops.Call, job *uuid.UUID) ([]OfferSummary, error) {
	stmt := `SELECT j.id, j.version, j.type, j.unit_id, lower(j.requested_slot), upper(j.requested_slot), j.due_at,
		j.origin,
		o.id, o.decision, o.offer_expires_at, o.terms_version, o.candidate_slots, o.offered_at,
		o.access_valid_from, o.access_valid_until
		FROM maintenance.offers o JOIN maintenance.jobs j ON j.id = o.job_id
		WHERE o.contractor_org_id = $1 AND ((o.decision IS NULL AND $2 < o.offer_expires_at AND j.status = 'offered') OR
		(o.decision = 'accept' AND $2 < o.access_valid_from))`
	args := []any{call.Principal.OrgID, call.Now}

	if job != nil {
		stmt += " AND j.id = $3"

		args = append(args, *job)
	}

	rows, err := call.Tx.Query(ctx, stmt, args...)
	if err != nil {
		return nil, apperr.Wrap(err, "query (offers)")
	}

	var (
		out   []OfferSummary
		units []uuid.UUID
	)

	for rows.Next() {
		var (
			offerSummary OfferSummary
			unit         uuid.UUID
			decision     *string
		)

		err := rows.Scan(
			&offerSummary.JobID,
			&offerSummary.JobVersion,
			&offerSummary.Type,
			&unit,
			&offerSummary.RequestedSlot.StartAt,
			&offerSummary.RequestedSlot.EndAt,
			&offerSummary.DueAt,
			&offerSummary.Origin,
			&offerSummary.OfferID,
			&decision,
			&offerSummary.OfferExpiresAt,
			&offerSummary.TermsVersion,
			&offerSummary.CandidateSlots,
			&offerSummary.OfferedAt,
			&offerSummary.AccessValidFrom,
			&offerSummary.AccessValidUntil,
		)
		if err != nil {
			rows.Close()

			return nil, apperr.Wrap(err, "query (offers)")
		}

		offerSummary.Projection, offerSummary.Status = projectionOffer, statusOffered
		if decision != nil {
			offerSummary.Status = statusAccepted
		}

		out, units = append(out, offerSummary), append(units, unit)
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return nil, apperr.Wrap(rowsErr, "query (offers)")
	}

	for idx := range out {
		addr, scope, err := m.Sites.SiteAddress(ctx, call, units[idx])
		if err != nil {
			return nil, apperr.Wrap(err, "maintenance.SiteAddresses.SiteAddress")
		}

		out[idx].SiteAddress, out[idx].RequiredQualifications = addr, requiredCodes(scope)
		// the offer's latest time-change proposal (it was never filled before IR226)
		var partnerSlotProposal PartnerSlotProposal

		err = call.Tx.QueryRow(ctx, `SELECT id, offer_id, lower(slot), upper(slot), technician_membership_id, reason, sent_at,
			status FROM maintenance.partner_slot_proposals
			WHERE offer_id = $1 AND status <> 'withdrawn' ORDER BY sent_at DESC, id LIMIT 1`, out[idx].OfferID).
			Scan(&partnerSlotProposal.ID, &partnerSlotProposal.OfferID, &partnerSlotProposal.Slot.StartAt,
				&partnerSlotProposal.Slot.EndAt,
				&partnerSlotProposal.TechnicianMembershipID, &partnerSlotProposal.Reason, &partnerSlotProposal.SentAt,
				&partnerSlotProposal.Status)
		switch {
		case err == nil:
			out[idx].PartnerSlotProposal = partnerSlotProposal
		case !errors.Is(err, pgx.ErrNoRows):
			return nil, apperr.Wrap(err, "select maintenance.partner_slot_proposals")
		}
	}

	return out, nil
}

func requiredCodes(scope []string) []string {
	codes := []string{}
	for _, s := range scope {
		if q, ok := qualificationFor(s); ok && !slices.Contains(codes, q) {
			codes = append(codes, q)
		}
	}

	return codes
}

// histories returns the caller's frozen snapshots (contractor organization or technician membership).
func (m Jobs) histories(ctx context.Context, call *ops.Call, job *uuid.UUID) ([]History, error) {
	kind, owner := "organization", call.Principal.OrgID
	if call.Principal.Role == roleTechnician {
		kind, owner = "membership", call.Principal.MembershipID
	}

	stmt := `SELECT job_id, as_of, type, status, contractor_org_id, completed_at, has_report, acceptance,
		decision_event_ids
		FROM maintenance.job_history_snapshots WHERE owner_kind = $1 AND owner_id = $2`
	args := []any{kind, owner}

	if job != nil {
		stmt += " AND job_id = $3"

		args = append(args, *job)
	}

	rows, err := call.Tx.Query(ctx, stmt, args...)
	if err != nil {
		return nil, apperr.Wrap(err, "query (histories)")
	}

	var (
		out   []History
		evIDs [][]uuid.UUID
	)

	for rows.Next() {
		var (
			history History
			has     bool
			acc     string
			ids     []uuid.UUID
		)

		err := rows.Scan(
			&history.JobID,
			&history.AsOf,
			&history.Type,
			&history.Status,
			&history.ContractorOrgID,
			&history.CompletedAt,
			&has,
			&acc,
			&ids,
		)
		if err != nil {
			rows.Close()

			return nil, apperr.Wrap(err, "query (histories)")
		}

		history.Projection = projectionHistory
		history.RedactedReportSummary = map[string]any{"hasReport": has, "acceptance": acc}
		out, evIDs = append(out, history), append(evIDs, ids)
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return nil, apperr.Wrap(rowsErr, "query (histories)")
	}
	// the decision events of every snapshot in one query (IR506), then each snapshot's own in time order
	var all []uuid.UUID
	for _, ids := range evIDs {
		all = append(all, ids...)
	}

	byID := map[uuid.UUID]Event{}

	if len(all) > 0 {
		evs, err := m.eventRows(
			ctx,
			call,
			"e.id = ANY($1)",
			[]any{all},
			"e.occurred_at ASC, e.id ASC",
			len(all),
			0,
		)
		if err != nil {
			return nil, err
		}

		for _, e := range evs {
			e.Note, e.ReportRef = nil, nil
			byID[e.ID] = e
		}
	}

	for i := range out {
		out[i].OwnDecisionEvents = []Event{}
		for _, id := range evIDs[i] { // stored in time order (FreezeEnded)
			if e, ok := byID[id]; ok {
				out[i].OwnDecisionEvents = append(out[i].OwnDecisionEvents, e)
			}
		}
	}

	return out, nil
}

// private reports whether a filter that offer/history projections never match is set (IR23).
func (f *listFilters) private() bool {
	return f.Severity != nil || f.UnitID != nil || f.UnitIDs != nil || f.MembershipID != nil ||
		f.CustomerID != nil ||
		f.PropertyID != nil ||
		f.ProposalPending != nil
}

func (f *listFilters) statusOK(s string) bool {
	return (f.Status == nil || *f.Status == s) &&
		(f.Statuses == nil || slices.Contains(*f.Statuses, s))
}

func inPeriod(filters *listFilters, when *time.Time) bool {
	if filters.From == nil && filters.To == nil {
		return true
	}

	if when == nil {
		return false
	}

	return (filters.From == nil || !when.Before(*filters.From)) &&
		(filters.To == nil || when.Before(*filters.To))
}

// periodTime is the time a history counts at in a period (IR29, IR614): its frozen completion, or, without one
// (cancelled, or ended unfinished), when its access ended — the snapshot's as-of — so recent ends fall in a recent
// period.
func periodTime(h History) *time.Time {
	if h.CompletedAt != nil {
		return h.CompletedAt
	}

	return &h.AsOf
}

// extraProjections returns the offer and history rows of contractors and technicians after the IR23 filters.
func (m Jobs) extraProjections(
	ctx context.Context,
	call *ops.Call,
	filters *listFilters,
) ([]listItem, error) {
	if filters.private() {
		return nil, nil
	}

	var out []listItem

	if call.Principal.Role == roleContractor {
		offers, err := m.offers(ctx, call, nil)
		if err != nil {
			return nil, err
		}

		for _, offerSummary := range offers {
			start := offerSummary.RequestedSlot.StartAt
			if !filters.statusOK(offerSummary.Status) || !inPeriod(filters, &start) ||
				(filters.Origin != nil && *filters.Origin != offerSummary.Origin) ||
				(filters.Type != nil && *filters.Type != offerSummary.Type) ||
				(filters.OrganizationID != nil && *filters.OrganizationID != call.Principal.OrgID) ||
				(filters.OverdueOnly != nil && *filters.OverdueOnly && !call.Now.After(offerSummary.DueAt)) {
				continue
			}

			due := offerSummary.DueAt
			out = append(
				out,
				listItem{
					rank: jobStatusRank(offerSummary.Status),
					sev:  -1,
					due:  &due,
					id:   offerSummary.JobID.String(),
					item: offerSummary,
				},
			)
		}
	}

	hs, err := m.histories(ctx, call, nil)
	if err != nil {
		return nil, err
	}

	for _, history := range hs {
		if !filters.statusOK(history.Status) || !inPeriod(filters, periodTime(history)) ||
			filters.Origin != nil ||
			(filters.Type != nil && *filters.Type != history.Type) ||
			(filters.OverdueOnly != nil && *filters.OverdueOnly) ||
			(filters.OrganizationID != nil &&
				(history.ContractorOrgID == nil || *history.ContractorOrgID != *filters.OrganizationID)) {
			continue
		}

		out = append(
			out,
			listItem{
				rank: jobStatusRank(history.Status),
				sev:  -1,
				id:   history.JobID.String(),
				item: history,
				due:  nil,
			},
		)
	}

	return out, nil
}

// projection resolves jobs.get for contractors and technicians: detail, then offer, then history (IR124 item 5).
func (m Jobs) projection(
	ctx context.Context,
	call *ops.Call,
	jobID uuid.UUID,
) (any, string, error) {
	j, err := m.detail(ctx, call, jobID)
	if err == nil {
		return j, projectionDetail, nil
	}

	if apperr.From(err).Code != apperr.NotFound {
		return nil, "", err
	}

	if call.Principal.Role == roleContractor {
		offers, err := m.offers(ctx, call, &jobID)
		if err != nil {
			return nil, "", err
		}

		if len(offers) > 0 {
			return offers[0], projectionOffer, nil
		}
	}

	histories, err := m.histories(ctx, call, &jobID)
	if err != nil {
		return nil, "", err
	}

	if len(histories) > 0 {
		return histories[0], projectionHistory, nil
	}

	return nil, "", apperr.E(apperr.NotFound, apperr.KeyNotFound)
}

// ---- worker: freeze histories (IR124 item 1) ----

// FreezeEnded stores a JobHistorySnapshot for every contractor access window and technician viewing window that has
// ended by now and has none yet. It runs in the worker tick (one transaction per tenant).
func FreezeEnded(ctx context.Context, txn pgx.Tx, now time.Time) (int, error) {
	count := 0
	// contractors: accepted Offers whose access window ended
	tag, err := txn.Exec(
		ctx,
		`INSERT INTO maintenance.job_history_snapshots (tenant_id, job_id, owner_kind, owner_id, as_of, type, status,
			contractor_org_id,
			completed_at, has_report, acceptance, decision_event_ids)
		SELECT j.tenant_id, j.id, 'organization', o.contractor_org_id, o.access_valid_until, j.type, j.status,
		j.contractor_org_id, j.completed_at,
			EXISTS (SELECT 1 FROM maintenance.work_reports r WHERE r.job_id = j.id AND r.state <> 'draft'),
			CASE WHEN EXISTS (SELECT 1 FROM maintenance.work_reports r WHERE r.job_id = j.id AND r.state = 'accepted') THEN
			'accepted' ELSE 'not_accepted' END,
			COALESCE((SELECT array_agg(e.id ORDER BY e.occurred_at, e.id) FROM maintenance.job_events e
				JOIN maintenance.ref_memberships mb ON mb.user_id = e.actor_user_id AND mb.organization_id = o.contractor_org_id
				WHERE e.job_id = j.id AND e.action IN ('offer.accepted','offer.declined')), '{}')
		FROM maintenance.offers o JOIN maintenance.jobs j ON j.id = o.job_id
		WHERE o.decision = 'accept' AND o.access_valid_until <= $1
		ON CONFLICT (job_id, owner_kind, owner_id) DO NOTHING`,
		now,
	)
	if err != nil {
		return 0, apperr.Wrap(err, "insert maintenance.job_history_snapshots")
	}

	count += int(tag.RowsAffected())
	t, err := freezeTechnicians(ctx, txn, now, nil)

	return count + t, err
}

// freezeTechnicians stores the technicians' snapshots whose viewing window has ended: the active Assignment's
// scheduled end passed, it was revoked after the work window started, or the job completed and released it (IR234);
// job narrows it to one job.
func freezeTechnicians(
	ctx context.Context,
	txn pgx.Tx,
	now time.Time,
	job *uuid.UUID,
) (int, error) {
	stmt := `INSERT INTO maintenance.job_history_snapshots (tenant_id, job_id, owner_kind, owner_id, owner_user_id, as_of,
		type, status,
			contractor_org_id, completed_at, has_report, acceptance)
		SELECT DISTINCT ON (a.job_id, a.technician_membership_id) j.tenant_id, j.id, 'membership', a.technician_membership_id,
		mb.user_id,
			CASE WHEN a.status IN ('revoked', 'completed') THEN a.updated_at ELSE upper(a.scheduled) END, j.type, j.status,
			j.contractor_org_id, j.completed_at,
			EXISTS (SELECT 1 FROM maintenance.work_reports r WHERE r.job_id = j.id AND r.state <> 'draft'),
			CASE WHEN EXISTS (SELECT 1 FROM maintenance.work_reports r WHERE r.job_id = j.id AND r.state = 'accepted') THEN
			'accepted' ELSE 'not_accepted' END
		FROM maintenance.assignments a JOIN maintenance.jobs j ON j.id = a.job_id JOIN maintenance.ref_memberships mb ON mb.id
		= a.technician_membership_id
		WHERE ((a.status = 'active' AND upper(a.scheduled) <= $1) OR (a.status = 'revoked' AND lower(a.scheduled) <=
		a.updated_at AND a.updated_at <= $1)
			OR (a.status = 'completed' AND a.updated_at <= $1))
			-- not while the same technician still holds an open window on the job (an extension or a new assignment, IR459)
			AND NOT EXISTS (SELECT 1 FROM maintenance.assignments b WHERE b.job_id = a.job_id AND b.technician_membership_id =
			a.technician_membership_id
				AND b.status = 'active' AND upper(b.scheduled) > $1)`
	args := []any{now}

	if job != nil {
		stmt += " AND a.job_id = $2"

		args = append(args, *job)
	}

	tag, err := txn.Exec(ctx, stmt+`
		ORDER BY a.job_id, a.technician_membership_id, CASE WHEN a.status IN ('revoked', 'completed') THEN a.updated_at ELSE
		upper(a.scheduled) END DESC, a.updated_at DESC
		ON CONFLICT (job_id, owner_kind, owner_id) DO NOTHING`, args...)
	if err != nil {
		return 0, apperr.Wrap(err, "exec (freezeTechnicians)")
	}

	return int(tag.RowsAffected()), nil
}

// ReleaseOnCompletion applies DEC-73 / IR234 when a job completes: its active Assignment becomes completed (reason
// job_completed), so it no longer books the technician (overlap constraint, members.eligible, members.capacity) and
// no longer grants unit access (change capture publishes active=false); the technician's viewing window ends with it
// and the history snapshot is stored at once, so the finished job stays in the technician's list (IR124).
func ReleaseOnCompletion(ctx context.Context, txn pgx.Tx, job uuid.UUID, now time.Time) error {
	_, execErr := txn.Exec(
		ctx,
		`UPDATE maintenance.assignments SET status = 'completed', reason = 'job_completed', version = version + 1, updated_at
			= $2
		WHERE job_id = $1 AND status = 'active'`,
		job,
		now,
	)
	if execErr != nil {
		return apperr.Wrap(execErr, "update maintenance.assignments")
	}

	_, err := freezeTechnicians(ctx, txn, now, &job)

	return err
}

// ExpireOffers applies IR48: undecided Offers whose offerExpiresAt has passed return their job from offered to
// requested (contractorOrgId cleared, version + 1, one offer_expired event by the system actor). Offer.decision stays
// null.
func ExpireOffers(ctx context.Context, txn pgx.Tx, now time.Time, actor uuid.UUID) (int, error) {
	rows, err := txn.Query(
		ctx,
		`UPDATE maintenance.jobs j SET status = 'requested', contractor_org_id = NULL, version = j.version + 1, updated_at =
			platform.app_now()
		FROM maintenance.offers o WHERE o.job_id = j.id AND o.decision IS NULL AND o.expired_at IS NULL AND o.offer_expires_at
		<= $1 AND j.status = 'offered'
		RETURNING j.id, j.tenant_id`,
		now,
	)
	if err != nil {
		return 0, apperr.Wrap(err, "update maintenance.jobs")
	}

	type hit struct{ job, tenant uuid.UUID }

	var hits []hit

	for rows.Next() {
		var expired hit

		err := rows.Scan(&expired.job, &expired.tenant)
		if err != nil {
			rows.Close()

			return 0, apperr.Wrap(err, "update maintenance.jobs")
		}

		hits = append(hits, expired)
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return 0, apperr.Wrap(rowsErr, "update maintenance.jobs")
	}
	// a contractor's time change still waiting for HQ ends with its expired offer (IR444); one sent to the client stays,
	// as the client's acceptance offers the job to the contractor again (IR128)
	_, execErr := txn.Exec(
		ctx,
		`UPDATE maintenance.partner_slot_proposals SET status = 'withdrawn' WHERE status = 'pending' AND offer_id IN
		(SELECT id FROM maintenance.offers WHERE decision IS NULL AND expired_at IS NULL AND offer_expires_at <= $1)`,
		now,
	)
	if execErr != nil {
		return 0, apperr.Wrap(execErr, "update maintenance.partner_slot_proposals")
	}

	_, execErr = txn.Exec(
		ctx,
		`UPDATE maintenance.offers SET expired_at = $1 WHERE decision IS NULL AND expired_at IS NULL AND offer_expires_at <=
			$1`,
		now,
	)
	if execErr != nil {
		return 0, apperr.Wrap(execErr, "update maintenance.offers")
	}

	for _, expired := range hits {
		_, err := txn.Exec(
			ctx,
			`INSERT INTO maintenance.job_events (id, tenant_id, job_id, actor_user_id, action, occurred_at) VALUES ($1,$2,$3,$4,
				'offer_expired',$5)`,
			uuid.Must(uuid.NewV7()),
			expired.tenant,
			expired.job,
			actor,
			now,
		)
		if err != nil {
			return 0, apperr.Wrap(err, "insert maintenance.job_events")
		}
	}

	return len(hits), nil
}
