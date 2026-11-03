package notify

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

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/authz"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

// targetLateral resolves a notification target ({kind,id} in columns k / i) to its customer organization, display
// name, unit and property (IR142), from notify's reference copies of the owners' rows (IR188).
const targetLateral = `LATERAL (
	SELECT u.customer_org_id AS org, u.display_name AS name, u.id AS unit_id, u.property_id FROM notify.ref_units u ` +
	`WHERE %[1]s = 'unit' AND u.id = %[2]s
	UNION ALL SELECT j.customer_org_id, u.display_name, u.id, u.property_id FROM notify.ref_jobs j JOIN ` +
	`notify.ref_units u ON u.id = j.unit_id WHERE %[1]s = 'job' AND j.id = %[2]s
	UNION ALL SELECT cu.organization_id, i.number, NULL, NULL FROM notify.ref_invoices i JOIN notify.ref_customers cu ` +
	`ON cu.id = i.customer_id
		WHERE %[1]s = 'invoice' AND i.id = %[2]s
	UNION ALL SELECT cu.organization_id, COALESCE((SELECT u.display_name FROM notify.ref_restriction_units ru JOIN ` +
	`notify.ref_units u ON u.id = ru.unit_id
		WHERE ru.restriction_id = r.id ORDER BY u.display_name LIMIT 1), 'Restriction'), NULL, NULL
		FROM notify.ref_restrictions r JOIN notify.ref_customers cu ON cu.id = r.customer_id WHERE %[1]s = 'restriction' ` +
	`AND r.id = %[2]s
	UNION ALL SELECT u.customer_org_id, d.serial, u.id, u.property_id FROM notify.ref_devices d LEFT JOIN ` +
	`notify.ref_units u ON u.id = d.unit_id WHERE %[1]s = 'device' AND d.id = %[2]s
	UNION ALL SELECT cu.organization_id, q.subject_type, NULL, NULL FROM notify.ref_inquiries q JOIN ` +
	`notify.ref_customers cu ON cu.id = q.customer_id WHERE %[1]s = 'inquiry' AND q.id = %[2]s
	UNION ALL SELECT c.organization_id, COALESCE(us.display_name || ' · ', '') || c.name, NULL, NULL FROM ` +
	`notify.ref_certificates c
		LEFT JOIN identity.memberships m ON m.id = c.membership_id LEFT JOIN identity.users us ON us.id = m.user_id WHERE ` +
	`%[1]s = 'certificate' AND c.id = %[2]s
) t`

// adminKinds lists the target kinds an HQ membership can read and the permissions that grant each (IR142).
var adminKinds = map[string][]string{
	targetUnit: {
		"asset.read",
		"alert.read",
		"device.read",
		"control.execute",
	},
	targetJob:     {"job.read", "job.write"},
	targetInvoice: {"billing.read", "billing.write"},
	targetRestriction: {
		"restriction.read",
		"restriction.write",
		"restriction.override",
	},
	"device":      {"device.read", "device.write"},
	targetInquiry: {"billing.read", "billing.write"},
}

func kindsFor(perms map[string]bool) []string {
	out := []string{}

	for k, ps := range adminKinds {
		for _, p := range ps {
			if perms[p] {
				out = append(out, k)

				break
			}
		}
	}

	return out
}

// readableSQL is the IR58 current-scope condition on the resolved target t for the principal at now; kind / id are the
// SQL expressions of the target. A technician reads a job while the assignment lets them open it (IR49: active and its
// scheduled window not over, as jobs.get; IR253) and afterwards while the job's history snapshot is theirs, as jobs.get
// then shows it (IR124): the notice of a cancellation or a reassignment stays readable (IR601). A revoked or ended
// assignment without a snapshot hides the job's notifications. A contractor reads the jobs offered to its organization
// and the certificates of its technicians (IR336).
func readableSQL(principal *ops.Principal, now time.Time, kind, id string, args *[]any) string {
	add := func(v any) string {
		*args = append(*args, v)

		return fmt.Sprintf("$%d", len(*args))
	}

	switch principal.Role {
	case authz.RoleAdmin:
		return kind + " = ANY(" + add(kindsFor(principal.Permissions)) + ")"
	case authz.RoleClient:
		return "t.org = " + add(principal.OrgID)
	case authz.RoleContractor:
		org := add(principal.OrgID)

		return "((" + kind + " = 'job' AND EXISTS (SELECT 1 FROM notify.ref_offers o WHERE o.job_id = " + id +
			" AND o.contractor_org_id = " + org + ")) OR (" +
			kind + " = 'certificate' AND t.org = " + org + "))"
	default:
		unitScope := "(t.unit_id = ANY(" + add(
			principal.Scopes[targetUnit],
		) + ") OR t.property_id = ANY(" + add(
			principal.Scopes["property"],
		) + ") OR t.org = ANY(" + add(
			principal.Scopes["organization"],
		) + "))"
		me := add(principal.MembershipID)

		return "((" + kind + " = 'job' AND (EXISTS (SELECT 1 FROM notify.ref_assignments a WHERE a.job_id = " + id +
			" AND a.technician_membership_id = " + me +
			" AND a.status = 'active' AND (a.scheduled IS NULL OR " + add(
			now,
		) + " < upper(a.scheduled)))" +
			" OR EXISTS (SELECT 1 FROM notify.ref_job_histories h WHERE h.job_id = " + id + " AND h.owner_kind = 'membership' " +
			"AND h.owner_id = " + me + "))) OR (" +
			kind + " IN ('unit','device') AND " + unitScope + "))"
	}
}

// Notification is Notification of service-contracts.ts.
type Notification struct {
	ID                     uuid.UUID         `json:"id"`
	TenantID               uuid.UUID         `json:"tenantId"`
	Version                int               `json:"version"`
	CreatedAt              time.Time         `json:"createdAt"`
	UpdatedAt              time.Time         `json:"updatedAt"`
	SourceAlertID          *uuid.UUID        `json:"sourceAlertId"`
	Type                   string            `json:"type"`
	RecipientMembershipID  uuid.UUID         `json:"recipientMembershipId"`
	ScopeVersionAtCreation int               `json:"scopeVersionAtCreation"`
	Target                 map[string]string `json:"target"`
	TemplateKey            string            `json:"templateKey"`
	Params                 Params            `json:"params"`
	Channel                string            `json:"channel"`
	DeliveryState          string            `json:"deliveryState"`
	Severity               string            `json:"severity"`
	OccurredAt             time.Time         `json:"occurredAt"`
	ReadAt                 *time.Time        `json:"readAt"`
	// the target's page opens for the reader, so the notice may link it (IR602)
	TargetOpen bool `json:"targetOpen"`
}

// Params is Notification.params: the stored params projected onto the canonical shape (IR142).
type Params struct {
	TargetName  string    `json:"targetName"`
	At          time.Time `json:"at"`
	Status      string    `json:"status"`
	Reason      *string   `json:"reason"`
	AmountMinor *int64    `json:"amountMinor"`
	Currency    *string   `json:"currency"`
	Method      *string   `json:"method"`
	Message     *string   `json:"message"`
}

func deliveryState(channel string) string {
	if channel == channelInApp {
		return "simulated"
	}

	return "preview"
}

func projectParams(raw []byte, name string, occurred time.Time, typ string) Params {
	var s struct {
		At          *time.Time `json:"at"`
		Status      *string    `json:"status"`
		Reason      *string    `json:"reason"`
		AmountMinor *int64     `json:"amountMinor"`
		Currency    *string    `json:"currency"`
		Method      *string    `json:"method"`
		Message     *string    `json:"message"`
	}

	_ = json.Unmarshal(raw, &s)

	params := Params{
		TargetName:  name,
		At:          occurred,
		Status:      typ,
		Reason:      s.Reason,
		AmountMinor: s.AmountMinor,
		Currency:    s.Currency,
		Method:      s.Method,
		Message:     s.Message,
	}
	if s.At != nil {
		params.At = *s.At
	}

	if s.Status != nil {
		params.Status = *s.Status
	}

	return params
}

const inboxCols = `n.id, n.tenant_id, n.version, n.created_at, n.source_alert_id, n.type, n.recipient_membership_id, ` +
	`n.scope_version_at_creation, n.target->>'kind',
	n.target->>'id', n.template_key, n.params, n.channel, n.severity, n.occurred_at, n.read_at, t.name`

func scanNotification(row pgx.Row) (Notification, error) {
	var (
		notification   Notification
		kind, id, name string
		params         []byte
	)

	err := row.Scan(
		&notification.ID,
		&notification.TenantID,
		&notification.Version,
		&notification.CreatedAt,
		&notification.SourceAlertID,
		&notification.Type,
		&notification.RecipientMembershipID,
		&notification.ScopeVersionAtCreation,
		&kind,
		&id,
		&notification.TemplateKey,
		&params,
		&notification.Channel,
		&notification.Severity,
		&notification.OccurredAt,
		&notification.ReadAt,
		&name,
	)

	notification.UpdatedAt = notification.CreatedAt
	if notification.ReadAt != nil {
		notification.UpdatedAt = *notification.ReadAt
	}

	notification.Target = map[string]string{"kind": kind, "id": id}
	notification.Params = projectParams(params, name, notification.OccurredAt, notification.Type)
	notification.DeliveryState = deliveryState(notification.Channel)
	notification.TargetOpen = true

	return notification, apperr.Wrap(err, "scan (scanNotification)")
}

// QueryJobsOpen asks maintenance which jobs open for the caller — jobs.get answers them, live or as the caller's
// history — so a contractor's or a technician's notice links its job only while the page opens: an offer withdrawn
// before the company accepted it leaves a notice without a link (IR602). Maintenance serves it under this name;
// notify cannot import maintenance.
const QueryJobsOpen = "maintenance.jobsOpen"

// JobsOpenInput is the input of QueryJobsOpen: the jobs of one page of notices.
type JobsOpenInput struct {
	JobIDs []uuid.UUID `json:"jobIds"`
}

// openTargets clears TargetOpen on the job notices of a contractor or a technician whose job page does not open for
// them (IR602); every other target opens wherever its notice is readable (IR58).
func openTargets(ctx context.Context, call *ops.Call, items []Notification) error {
	if call.Principal.Role != authz.RoleContractor && call.Principal.Role != authz.RoleTechnician {
		return nil
	}

	ids := []uuid.UUID{}

	for _, x := range items {
		id, err := uuid.Parse(
			x.Target["id"],
		)
		if err == nil && x.Target["kind"] == targetJob &&
			!slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}

	if len(ids) == 0 {
		return nil
	}

	open, err := ops.Query[[]uuid.UUID](ctx, call, QueryJobsOpen, JobsOpenInput{JobIDs: ids})
	if err != nil {
		return err
	}

	for i, x := range items {
		id, err := uuid.Parse(x.Target["id"])
		if err == nil && x.Target["kind"] == targetJob {
			items[i].TargetOpen = slices.Contains(open, id)
		}
	}

	return nil
}

// Inbox is the notification read / preview operation set (IR142).
type Inbox struct{}

func inboxFrom() string {
	return "notify.notifications n CROSS JOIN " + fmt.Sprintf(
		targetLateral,
		"(n.target->>'kind')",
		"(n.target->>'id')::uuid",
	)
}

var (
	severities = []string{"critical", "warning", severityNormal}
	types      = []string{
		"cleaning_due",
		"fault",
		noticeQuality,
		noticeScheduleChange,
		noticeReportReturn,
		noticeCompletion,
		noticePayment,
		noticePaymentReminder,
		targetRestriction,
		targetInquiry,
		noticeJobUpdate,
		noticeDeviceOperation,
		"certificate_expiry",
		"warranty_expiring",
	}
)

// list serves notifications.list.
//
//	@Summary		notifications.list (read)
//	@ID				notifications.list
//	@Description	Authorization: authenticated:recipient-only; current-target-scope
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot; IR58 out-of-scope \
//	@Description	targets excluded from items/total; IR95 business event notification table
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-C08, DDC-07 · Query: filters unreadOnly,severity,type,from,to · sort \
//	@Description	id,createdAt,updatedAt,severity,occurredAt (default occurredAt desc;id desc)
//	@Tags			notifications
//	@Accept			json
//	@Produce		json
//	@Param			cursor		query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit		query		integer	false	"page size 1–100, default 25"
//	@Param			sort		query		string	false	"field:direction — fields id,createdAt,updatedAt,severity,occurredAt; default occurredAt desc;id desc"
//	@Param			unreadOnly	query		boolean	false	"filter → true => readAt=null"
//	@Param			severity	query		string	false	"filter → severity"
//	@Param			type		query		string	false	"filter → type"
//	@Param			from		query		string	false	"filter → [from,to) on occurredAt"
//	@Param			to			query		string	false	"filter → [from,to) on occurredAt"
//	@Success		200			{object}	ops.Envelope{data=NotificationPage}
//	@Failure		401			{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403			{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404			{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409			{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422			{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429			{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503			{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504			{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/notifications [get]
func (Inbox) list(
	ctx context.Context,
	call *ops.Call,
	query *paging.Query,
) (paging.Page[Notification], error) {
	var f struct {
		Severity   *string    `json:"severity,omitempty"`
		UnreadOnly *bool      `json:"unreadOnly,omitempty"`
		Type       *string    `json:"type,omitempty"`
		From       *time.Time `json:"from,omitempty"` // [from, to) on occurredAt
		To         *time.Time `json:"to,omitempty"`
	}

	if len(query.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(query.Filters)))
		dec.DisallowUnknownFields()

		if dec.Decode(&f) != nil ||
			(f.Severity != nil && !slices.Contains(severities, *f.Severity)) ||
			(f.Type != nil && !slices.Contains(types, *f.Type)) {
			return paging.Page[Notification]{}, apperr.Fields(
				map[string]string{"filters": apperr.KeyInvalid},
			)
		}

		if f.From != nil && f.To != nil && !f.From.Before(*f.To) {
			return paging.Page[Notification]{}, apperr.Fields(
				map[string]string{"filters.to": apperr.KeyRange},
			)
		}
	}

	order, err := paging.OrderBy(query.Sort, map[string]string{
		"id":         "n.id",
		"occurredAt": "n.occurred_at",
		"createdAt":  "n.created_at",
		"updatedAt":  "COALESCE(n.read_at, n.created_at)", // reading is the only update
		"severity":   "CASE n.severity WHEN 'critical' THEN 3 WHEN 'warning' THEN 2 ELSE 1 END",
	}, "n.occurred_at DESC, n.id DESC")
	if err != nil {
		return paging.Page[Notification]{}, apperr.Wrap(err, "paging.OrderBy")
	}

	window, err := paging.Resolve(*query, f, call.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Notification]{}, apperr.Wrap(err, "paging.Resolve")
	}

	args := []any{call.Principal.MembershipID}
	add := func(v any) string {
		args = append(args, v)

		return fmt.Sprintf("$%d", len(args))
	}

	conds := []string{
		"n.recipient_membership_id = $1",
		readableSQL(
			call.Principal,
			call.Now,
			"(n.target->>'kind')",
			"(n.target->>'id')::uuid",
			&args,
		),
	}
	if f.Severity != nil {
		conds = append(conds, "n.severity = "+add(*f.Severity))
	}

	if f.UnreadOnly != nil && *f.UnreadOnly {
		conds = append(conds, "n.read_at IS NULL")
	}

	if f.Type != nil {
		conds = append(conds, "n.type = "+add(*f.Type))
	}

	if f.From != nil {
		conds = append(conds, "n.occurred_at >= "+add(*f.From))
	}

	if f.To != nil {
		conds = append(conds, "n.occurred_at < "+add(*f.To))
	}

	where := strings.Join(conds, " AND ")

	var total int

	scanErr := call.Tx.QueryRow(ctx, "SELECT count(*) FROM "+inboxFrom()+" WHERE "+where, args...).
		Scan(&total)
	if scanErr != nil {
		return paging.Page[Notification]{}, apperr.Wrap(scanErr, "select count")
	}

	rows, err := call.Tx.Query(
		ctx,
		fmt.Sprintf(
			"SELECT %s FROM %s WHERE %s ORDER BY %s LIMIT %d OFFSET %d",
			inboxCols,
			inboxFrom(),
			where,
			order,
			window.Limit,
			window.Offset,
		),
		args...)
	if err != nil {
		return paging.Page[Notification]{}, apperr.Wrap(err, "select (list)")
	}
	defer rows.Close()

	items := []Notification{}

	for rows.Next() {
		x, err := scanNotification(rows)
		if err != nil {
			return paging.Page[Notification]{}, err
		}

		items = append(items, x)
	}

	rowsErr := rows.Err()
	if rowsErr != nil {
		return paging.Page[Notification]{}, apperr.Wrap(rowsErr, "select (list)")
	}

	openTargetsErr := openTargets(ctx, call, items)
	if openTargetsErr != nil {
		return paging.Page[Notification]{}, openTargetsErr
	}

	return paging.Page[Notification]{
		Items:           items,
		NextCursor:      window.Next(total),
		Total:           total,
		SnapshotVersion: window.Snapshot,
	}, nil
}

// MarkReadInput is notifications.markRead input.
type MarkReadInput struct {
	ID uuid.UUID `json:"id"`
}

// Validate implements ops.Validator.
func (in *MarkReadInput) Validate() map[string]string {
	if in.ID == uuid.Nil {
		return map[string]string{"id": apperr.KeyRequired}
	}

	return nil
}

// markRead serves notifications.markRead.
//
//	@Summary		notifications.markRead (write)
//	@ID				notifications.markRead
//	@Description	Authorization: authenticated:recipient-only
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-C08
//	@Tags			notifications
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer			true	"all: required (target notifications, read notifications.list)"
//	@Param			id					path		string			true	"input field id"
//	@Param			request				body		MarkReadInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=Notification}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/notifications/{id}/mark-read [post]
func (Inbox) markRead(
	ctx context.Context,
	call *ops.Call,
	in *MarkReadInput,
) (Notification, error) {
	args := []any{in.ID, call.Principal.MembershipID}
	q := "SELECT " + inboxCols + " FROM " + inboxFrom() + " WHERE n.id = $1 AND n.recipient_membership_id = $2 AND " +
		readableSQL(
			call.Principal,
			call.Now,
			"(n.target->>'kind')",
			"(n.target->>'id')::uuid",
			&args,
		) + " FOR UPDATE OF n"

	notification, err := scanNotification(call.Tx.QueryRow(ctx, q, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return notification, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return notification, err
	}

	if notification.Version != *call.ExpectedVersion {
		return notification, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	one := []Notification{notification}

	openTargetsErr := openTargets(ctx, call, one)
	if openTargetsErr != nil {
		return notification, openTargetsErr
	}

	notification = one[0]
	if notification.ReadAt != nil { // already read: unchanged
		return notification, nil
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE notify.notifications SET read_at = $2, version = version + 1 WHERE id = $1`,
		notification.ID,
		call.Now,
	)
	if execErr != nil {
		return notification, apperr.Wrap(execErr, "update notify.notifications")
	}

	notification.ReadAt, notification.Version, notification.UpdatedAt = &call.Now, notification.Version+1, call.Now

	return notification, nil
}

// MarkAllReadInput is notifications.markAllRead input: the listed unread notifications (IR379).
type MarkAllReadInput struct {
	IDs []uuid.UUID `json:"ids"`
}

// Validate implements ops.Validator: 1–100 distinct IDs, the size of one inbox page.
func (in *MarkAllReadInput) Validate() map[string]string {
	if len(in.IDs) == 0 {
		return map[string]string{keyIDs: apperr.KeyRequired}
	}

	if len(in.IDs) > 100 {
		return map[string]string{keyIDs: apperr.KeyCount}
	}

	seen := map[uuid.UUID]bool{}
	for _, id := range in.IDs {
		if id == uuid.Nil || seen[id] {
			return map[string]string{keyIDs: apperr.KeyInvalid}
		}

		seen[id] = true
	}

	return nil
}

// Marked is NotificationsMarked: the IDs marked read now and the others, in input order.
type Marked struct {
	MarkedIDs  []uuid.UUID `json:"markedIds"`
	SkippedIDs []uuid.UUID `json:"skippedIds"`
}

// markAllRead marks the listed notifications read in one write, so "Mark all as read" spends one write of the
// membership's limit (IR375) instead of one per notification (IR379). Only the caller's unread notifications with a
// readable target (IR58) change; any other ID (already read, another member's, out of scope, unknown) is skipped
// alike, so the answer never tells which of them exist. Marking read only moves forward, so no version is expected.
//
//	@Summary		notifications.markAllRead (write)
//	@ID				notifications.markAllRead
//	@Description	Authorization: authenticated:recipient-only
//	@Description	Validation: D01; input constraints in the corresponding DD; IR379 1–100 distinct IDs; the caller's \
//	@Description	unread notifications with a readable target are marked read, every other ID is skipped (never told \
//	@Description	apart); one write for the whole list
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DDC-07, DD-C08
//	@Tags			notifications
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string				true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		MarkAllReadInput	true	"input"
//	@Success		200				{object}	ops.Envelope{data=Marked}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/notifications/mark-all-read [post]
func (Inbox) markAllRead(
	ctx context.Context,
	call *ops.Call,
	in *MarkAllReadInput,
) (Marked, error) {
	args := []any{in.IDs, call.Principal.MembershipID, call.Now}
	q := "UPDATE notify.notifications x SET read_at = $3, version = x.version + 1 FROM (SELECT n.id FROM " + inboxFrom() +
		" WHERE n.id = ANY($1) AND n.recipient_membership_id = $2 AND n.read_at IS NULL AND " +
		readableSQL(
			call.Principal,
			call.Now,
			"(n.target->>'kind')",
			"(n.target->>'id')::uuid",
			&args,
		) + ") r WHERE x.id = r.id RETURNING x.id"

	rows, err := call.Tx.Query(ctx, q, args...)
	if err != nil {
		return Marked{}, apperr.Wrap(err, "update notify.notifications")
	}

	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return Marked{}, apperr.Wrap(err, "update notify.notifications")
	}

	out := Marked{MarkedIDs: []uuid.UUID{}, SkippedIDs: []uuid.UUID{}}

	for _, id := range in.IDs {
		if slices.Contains(ids, id) {
			out.MarkedIDs = append(out.MarkedIDs, id)
		} else {
			out.SkippedIDs = append(out.SkippedIDs, id)
		}
	}

	return out, nil
}

// TargetInput is the shared {target, templateKey, channel} input of preview / recipients.
type TargetInput struct {
	Target struct {
		Kind string    `json:"kind"`
		ID   uuid.UUID `json:"id"`
	} `json:"target"`
	TemplateKey string `json:"templateKey"`
	Channel     string `json:"channel"`
}

var (
	kinds = []string{
		targetUnit,
		targetJob,
		targetInvoice,
		targetRestriction,
		"device",
		targetInquiry,
	}
	templates = []string{
		noticeAlert,
		noticeQuality,
		noticeScheduleChange,
		noticeReportReturn,
		noticeCompletion,
		noticePayment,
		noticePaymentReminder,
		targetRestriction,
		targetInquiry,
		noticeJobUpdate,
		noticeDeviceOperation,
	}
	channels = []string{channelInApp, "email", "whatsapp"}
	roles    = []string{
		authz.RoleClient,
		authz.RoleContractor,
		authz.RoleTechnician,
		authz.RoleAdmin,
	}
)

func (in *TargetInput) validate(fieldErrs map[string]string) {
	if !slices.Contains(kinds, in.Target.Kind) || in.Target.ID == uuid.Nil {
		fieldErrs["target"] = apperr.KeyInvalid
	}

	if !slices.Contains(templates, in.TemplateKey) {
		fieldErrs["templateKey"] = apperr.KeyInvalid
	}

	if !slices.Contains(channels, in.Channel) {
		fieldErrs["channel"] = apperr.KeyInvalid
	}
}

type target struct {
	org  *uuid.UUID
	name string
}

// resolve loads the target and checks that the caller can read it (NOT_FOUND) and, for payment_reminder, the IR20
// preconditions (billing.write FORBIDDEN, overdue unpaid invoice CONFLICT).
func resolve(ctx context.Context, c *ops.Call, input *TargetInput) (target, error) {
	args := []any{input.Target.Kind, input.Target.ID}

	var t target

	err := c.Tx.QueryRow(ctx, "SELECT t.org, t.name FROM "+fmt.Sprintf(targetLateral, "$1::text", "$2::uuid")+" WHERE "+
		readableSQL(c.Principal, c.Now, "$1::text", "$2::uuid", &args), args...).
		Scan(&t.org, &t.name)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return t, apperr.Wrap(err, "select t.org")
	}

	if input.TemplateKey == noticePaymentReminder {
		if c.Principal.Role != authz.RoleAdmin || !c.Principal.Permissions["billing.write"] {
			return t, apperr.E(apperr.Forbidden, apperr.KeyForbidden)
		}

		if input.Target.Kind != targetInvoice {
			return t, apperr.Fields(map[string]string{"target": apperr.KeyInvalid})
		}

		var ok bool

		err := c.Tx.QueryRow(ctx, `SELECT status = 'unpaid' AND due_at < $2 FROM notify.ref_invoices WHERE id =
			$1`, input.Target.ID, c.Now).
			Scan(&ok)
		if err != nil {
			return t, apperr.Wrap(err, "select notify.ref_invoices")
		}

		if !ok {
			return t, apperr.E(apperr.Conflict, "errors.reminder_not_due")
		}
	}

	return t, nil
}

// Recipient is NotificationRecipient.
type Recipient struct {
	ID              uuid.UUID `json:"id"`
	Role            string    `json:"role"`
	DisplayLabel    string    `json:"displayLabel"`
	AllowedChannels []string  `json:"allowedChannels"`
}

// recipientsSQL selects active memberships that can read the target (IR142): clients of the target's customer
// organization; HQ members with a permission for the target kind; for jobs, contractor members of offered
// organizations and assigned technicians. payment_reminder is limited to the customer's clients (IR04).
const recipientsSQL = `SELECT m.id, m.role, u.display_name,
		CASE WHEN m.role = 'client' AND u.phone IS NOT NULL THEN ARRAY['inApp','email','whatsapp'] ELSE ARRAY['inApp','email']
		END
	FROM identity.memberships m JOIN identity.users u ON u.id = m.user_id
	WHERE m.valid_from <= $1 AND (m.valid_until IS NULL OR m.valid_until > $1)
	  AND ((m.role = 'client' AND m.organization_id = $2)
	    OR (NOT $5 AND m.role = 'admin' AND EXISTS (SELECT 1 FROM identity.membership_permissions mp WHERE mp.membership_id
	    = m.id AND mp.permission = ANY($6)))
	    OR (NOT $5 AND $3 = 'job' AND m.role = 'contractor' AND m.organization_id IN (SELECT o.contractor_org_id FROM
	    notify.ref_offers o WHERE o.job_id = $4))
	    OR (NOT $5 AND $3 = 'job' AND m.role = 'technician' AND m.id IN (SELECT a.technician_membership_id FROM
	    notify.ref_assignments a WHERE a.job_id = $4 AND a.status = 'active')))`

// adminPerms are the permissions that make an HQ member a recipient: one that reads the target kind (IR142); for an
// alert about a unit only alert.read, as for the recipients an alert policy may notify (IR120 item 3, IR345).
func adminPerms(in *TargetInput) []string {
	if in.TemplateKey == noticeAlert && in.Target.Kind == targetUnit {
		return []string{"alert.read"}
	}

	return adminKinds[in.Target.Kind]
}

func recipients(
	ctx context.Context,
	call *ops.Call,
	input *TargetInput,
	t target,
	role *string,
) ([]Recipient, error) {
	var org uuid.UUID
	if t.org != nil {
		org = *t.org
	}

	q := "SELECT * FROM (" + recipientsSQL + ") r(id, role, label, channels) WHERE $7 = ANY(r.channels)"
	args := []any{
		call.Now,
		org,
		input.Target.Kind,
		input.Target.ID,
		input.TemplateKey == noticePaymentReminder,
		adminPerms(input),
		input.Channel,
	}

	if role != nil {
		q += " AND r.role = $8"

		args = append(args, *role)
	}

	rows, err := call.Tx.Query(ctx, q+" ORDER BY r.role, r.label, r.id", args...)
	if err != nil {
		return nil, apperr.Wrap(err, "query (recipients)")
	}
	defer rows.Close()

	out := []Recipient{}

	for rows.Next() {
		var recipient Recipient

		err := rows.Scan(
			&recipient.ID,
			&recipient.Role,
			&recipient.DisplayLabel,
			&recipient.AllowedChannels,
		)
		if err != nil {
			return nil, apperr.Wrap(err, "query (recipients)")
		}

		out = append(out, recipient)
	}

	return out, apperr.Wrap(rows.Err(), "query (recipients)")
}

// RecipientsInput is notifications.recipients input.
type RecipientsInput struct {
	TargetInput

	Role  *string      `json:"role,omitempty"`
	Query paging.Query `json:"query"`
}

// Validate implements ops.Validator.
func (in *RecipientsInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	in.validate(fieldErrs)

	if in.Role != nil && !slices.Contains(roles, *in.Role) {
		fieldErrs["role"] = apperr.KeyInvalid
	}

	return fieldErrs
}

// recipients serves notifications.recipients.
//
//	@Summary		notifications.recipients (read)
//	@ID				notifications.recipients
//	@Description	Authorization: authenticated:current-target-party; D08 recipient projection; \
//	@Description	payment_reminder:admin:billing.write:overdue-unpaid-only (IR20)
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A05, DD-A08, DD-C11, DD-P07, DD-A05, DD-A12 · Query: filters none · sort none (default \
//	@Description	role asc;displayLabel asc;id asc)
//	@Tags			notifications
//	@Accept			json
//	@Produce		json
//	@Param			target.kind	query		string	false	"input field target.kind"
//	@Param			target.id	query		string	false	"input field target.id"
//	@Param			templateKey	query		string	false	"input field templateKey"
//	@Param			channel		query		string	false	"input field channel"
//	@Param			role		query		string	false	"input field role"
//	@Param			cursor		query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit		query		integer	false	"page size 1–100, default 25"
//	@Param			sort		query		string	false	"field:direction — fields none; default role asc;displayLabel asc;id asc"
//	@Success		200			{object}	ops.Envelope{data=RecipientPage}
//	@Failure		401			{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403			{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404			{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409			{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422			{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429			{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503			{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504			{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/notifications/recipients [get]
func (Inbox) recipients(
	ctx context.Context,
	call *ops.Call,
	input *RecipientsInput,
) (paging.Page[Recipient], error) {
	if len(input.Query.Filters) > 0 && string(input.Query.Filters) != "{}" ||
		input.Query.Sort != nil {
		return paging.Page[Recipient]{}, apperr.Fields(
			map[string]string{"query": apperr.KeyInvalid},
		)
	}

	window, err := paging.Resolve(input.Query, struct{}{}, call.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Recipient]{}, apperr.Wrap(err, "paging.Resolve")
	}

	t, err := resolve(ctx, call, &input.TargetInput)
	if err != nil {
		return paging.Page[Recipient]{}, err
	}

	all, err := recipients(ctx, call, &input.TargetInput, t, input.Role)
	if err != nil {
		return paging.Page[Recipient]{}, err
	}

	end := min(window.Offset+window.Limit, len(all))

	page := []Recipient{}
	if window.Offset < len(all) {
		page = all[window.Offset:end]
	}

	return paging.Page[Recipient]{
		Items:           page,
		NextCursor:      window.Next(len(all)),
		Total:           len(all),
		SnapshotVersion: window.Snapshot,
	}, nil
}

// PreviewInput is notifications.preview input.
type PreviewInput struct {
	TargetInput

	RecipientMembershipID uuid.UUID  `json:"recipientMembershipId"`
	SourceAlertID         *uuid.UUID `json:"sourceAlertId,omitempty"`
	Message               *string    `json:"message,omitempty"`
	Reason                *string    `json:"reason,omitempty"`
	// Severity is an alert's severity (the policy's or rule's, IR348); other templates keep their own
	Severity *string `json:"severity,omitempty"`
}

// Validate implements ops.Validator.
func (in *PreviewInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	in.validate(fieldErrs)

	if in.RecipientMembershipID == uuid.Nil {
		fieldErrs["recipientMembershipId"] = apperr.KeyRequired
	}

	if in.Severity != nil {
		switch {
		case in.TemplateKey != noticeAlert:
			fieldErrs["severity"] = apperr.KeyNotAllowed
		case !slices.Contains(severities, *in.Severity):
			fieldErrs["severity"] = apperr.KeyInvalid
		}
	}
	// the message carries a job note (1–2000, DD-P07) or a reminder text; the reason stays 1–1000 (IR228)
	for k, f := range map[string]struct {
		s   *string
		max int
	}{"message": {in.Message, 2000}, "reason": {in.Reason, 1000}} {
		if f.s != nil {
			*f.s = strings.TrimSpace(*f.s)
			if n := utf8.RuneCountInString(*f.s); n < 1 || n > f.max {
				fieldErrs[k] = apperr.KeyLength
			}
		}
	}

	return fieldErrs
}

// preview renders an unsaved Notification for one eligible recipient (IR04): nothing is stored and the temporary
// ID cannot be used with markRead.
//
//	@Summary		notifications.preview (read)
//	@ID				notifications.preview
//	@Description	Authorization: authenticated:recipient-or-current-target-party; D08 billing/internal projection; \
//	@Description	payment_reminder:admin:billing.write:overdue-unpaid-only (IR20)
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A05, DD-A08, DD-C11, DD-P07
//	@Tags			notifications
//	@Accept			json
//	@Produce		json
//	@Param			request	body		PreviewInput	true	"input"
//	@Success		200		{object}	ops.Envelope{data=Notification}
//	@Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504		{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/notifications/preview [post]
func (Inbox) preview(ctx context.Context, c *ops.Call, input *PreviewInput) (Notification, error) {
	t, err := resolve(ctx, c, &input.TargetInput)
	if err != nil {
		return Notification{}, err
	}

	all, err := recipients(ctx, c, &input.TargetInput, t, nil)
	if err != nil {
		return Notification{}, err
	}

	i := slices.IndexFunc(
		all,
		func(r Recipient) bool { return r.ID == input.RecipientMembershipID },
	)
	if i < 0 {
		return Notification{}, apperr.Fields(
			map[string]string{"recipientMembershipId": "errors.recipient_ineligible"},
		)
	}

	var scope int

	scanErr := c.Tx.QueryRow(ctx, `SELECT scope_version FROM identity.memberships WHERE id =
		$1`, input.RecipientMembershipID).
		Scan(&scope)
	if scanErr != nil {
		return Notification{}, apperr.Wrap(scanErr, "select identity.memberships")
	}

	typ, status := input.TemplateKey, input.TemplateKey
	if typ == noticeAlert { // as the notifier sends it: a fault of the policy's severity about an open alert (IR104, IR348)
		typ, status = "fault", "open"
	}

	severity := severityNormal
	if input.TemplateKey == noticePaymentReminder || input.TemplateKey == targetRestriction ||
		input.TemplateKey == noticeAlert {
		severity = "warning"
	}

	if input.Severity != nil {
		severity = *input.Severity
	}

	return Notification{
		ID:                     uuid.New(),
		TenantID:               c.Principal.TenantID,
		Version:                1,
		CreatedAt:              c.Now,
		UpdatedAt:              c.Now,
		SourceAlertID:          input.SourceAlertID,
		Type:                   typ,
		RecipientMembershipID:  input.RecipientMembershipID,
		ScopeVersionAtCreation: scope,
		Target: map[string]string{
			"kind": input.Target.Kind,
			"id":   input.Target.ID.String(),
		},
		TemplateKey: input.TemplateKey,
		Params: Params{
			TargetName: t.name,
			At:         c.Now,
			Status:     status,
			Reason:     input.Reason,
			Message:    input.Message, AmountMinor: nil, Currency: nil, Method: nil,
		},
		Channel:       input.Channel,
		DeliveryState: "preview",
		Severity:      severity,
		OccurredAt:    c.Now,
		TargetOpen:    true, ReadAt: nil,
	}, nil
}

// Register binds the notification operations.
func Register(reg *ops.Registry) {
	var m Inbox
	ops.Register(reg, "notifications.list", m.list)
	ops.Register(reg, "notifications.markRead", m.markRead)
	ops.Register(reg, "notifications.markAllRead", m.markAllRead)
	ops.Register(reg, "notifications.recipients", m.recipients)
	ops.Register(reg, "notifications.preview", m.preview)
}
