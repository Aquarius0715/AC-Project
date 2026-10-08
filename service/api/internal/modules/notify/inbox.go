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
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

// targetLateral resolves a notification target ({kind,id} in columns k / i) to its customer organization, display
// name, unit and property (IR142).
const targetLateral = `LATERAL (
	SELECT u.customer_org_id AS org, u.display_name AS name, u.id AS unit_id, u.property_id FROM assets.units u WHERE %[1]s = 'unit' AND u.id = %[2]s
	UNION ALL SELECT j.customer_org_id, u.display_name, u.id, u.property_id FROM maintenance.jobs j JOIN assets.units u ON u.id = j.unit_id WHERE %[1]s = 'job' AND j.id = %[2]s
	UNION ALL SELECT k.customer_org_id, i.number, NULL, NULL FROM billing.invoices i JOIN billing.contracts k ON k.id = i.contract_id AND k.version = i.contract_version
		WHERE %[1]s = 'invoice' AND i.id = %[2]s
	UNION ALL SELECT cu.organization_id, COALESCE((SELECT u.display_name FROM restrictions.restriction_units ru JOIN assets.units u ON u.id = ru.unit_id
		WHERE ru.restriction_id = r.id ORDER BY u.display_name LIMIT 1), 'Restriction'), NULL, NULL
		FROM restrictions.restrictions r JOIN assets.customers cu ON cu.id = r.customer_id WHERE %[1]s = 'restriction' AND r.id = %[2]s
	UNION ALL SELECT u.customer_org_id, d.serial, u.id, u.property_id FROM devices.devices d LEFT JOIN assets.units u ON u.id = d.unit_id WHERE %[1]s = 'device' AND d.id = %[2]s
	UNION ALL SELECT cu.organization_id, q.subject_type, NULL, NULL FROM billing.inquiries q JOIN assets.customers cu ON cu.id = q.customer_id WHERE %[1]s = 'inquiry' AND q.id = %[2]s
) t`

// adminKinds lists the target kinds an HQ membership can read and the permissions that grant each (IR142).
var adminKinds = map[string][]string{
	"unit": {"asset.read", "alert.read", "device.read", "control.execute"}, "job": {"job.read", "job.write"}, "invoice": {"billing.read", "billing.write"},
	"restriction": {"restriction.read", "restriction.write", "restriction.override"}, "device": {"device.read", "device.write"}, "inquiry": {"billing.read", "billing.write"},
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

// readableSQL is the IR58 current-scope condition on the resolved target t for the principal; kind / id are the SQL
// expressions of the target.
func readableSQL(p *ops.Principal, kind, id string, args *[]any) string {
	add := func(v any) string { *args = append(*args, v); return fmt.Sprintf("$%d", len(*args)) }
	switch p.Role {
	case "admin":
		return kind + " = ANY(" + add(kindsFor(p.Permissions)) + ")"
	case "client":
		return "t.org = " + add(p.OrgID)
	case "contractor":
		return kind + " = 'job' AND EXISTS (SELECT 1 FROM maintenance.offers o WHERE o.job_id = " + id + " AND o.contractor_org_id = " + add(p.OrgID) + ")"
	default:
		unitScope := "(t.unit_id = ANY(" + add(p.Scopes["unit"]) + ") OR t.property_id = ANY(" + add(p.Scopes["property"]) + ") OR t.org = ANY(" + add(p.Scopes["organization"]) + "))"
		return "((" + kind + " = 'job' AND EXISTS (SELECT 1 FROM maintenance.assignments a WHERE a.job_id = " + id + " AND a.technician_membership_id = " + add(p.MembershipID) + ")) OR (" +
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
	if channel == "inApp" {
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
	p := Params{TargetName: name, At: occurred, Status: typ, Reason: s.Reason, AmountMinor: s.AmountMinor, Currency: s.Currency, Method: s.Method, Message: s.Message}
	if s.At != nil {
		p.At = *s.At
	}
	if s.Status != nil {
		p.Status = *s.Status
	}
	return p
}

const inboxCols = `n.id, n.tenant_id, n.version, n.created_at, n.source_alert_id, n.type, n.recipient_membership_id, n.scope_version_at_creation, n.target->>'kind',
	n.target->>'id', n.template_key, n.params, n.channel, n.severity, n.occurred_at, n.read_at, t.name`

func scanNotification(r pgx.Row) (Notification, error) {
	var x Notification
	var kind, id, name string
	var params []byte
	err := r.Scan(&x.ID, &x.TenantID, &x.Version, &x.CreatedAt, &x.SourceAlertID, &x.Type, &x.RecipientMembershipID, &x.ScopeVersionAtCreation, &kind, &id, &x.TemplateKey,
		&params, &x.Channel, &x.Severity, &x.OccurredAt, &x.ReadAt, &name)
	x.UpdatedAt = x.CreatedAt
	if x.ReadAt != nil {
		x.UpdatedAt = *x.ReadAt
	}
	x.Target = map[string]string{"kind": kind, "id": id}
	x.Params = projectParams(params, name, x.OccurredAt, x.Type)
	x.DeliveryState = deliveryState(x.Channel)
	return x, err
}

// Inbox is the notification read / preview operation set (IR142).
type Inbox struct{}

func inboxFrom() string {
	return "notify.notifications n CROSS JOIN " + fmt.Sprintf(targetLateral, "(n.target->>'kind')", "(n.target->>'id')::uuid")
}

var (
	severities = []string{"critical", "warning", "normal"}
	types      = []string{"cleaning_due", "fault", "quality", "schedule_change", "report_return", "completion", "payment", "payment_reminder", "restriction", "inquiry", "job_update", "device_operation"}
)

func (Inbox) list(ctx context.Context, c *ops.Call, in *paging.Query) (paging.Page[Notification], error) {
	var f struct {
		Severity   *string `json:"severity,omitempty"`
		UnreadOnly *bool   `json:"unreadOnly,omitempty"`
		Type       *string `json:"type,omitempty"`
	}
	if len(in.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(in.Filters)))
		dec.DisallowUnknownFields()
		if dec.Decode(&f) != nil || (f.Severity != nil && !slices.Contains(severities, *f.Severity)) || (f.Type != nil && !slices.Contains(types, *f.Type)) {
			return paging.Page[Notification]{}, apperr.Fields(map[string]string{"filters": "error.invalid"})
		}
	}
	order, err := paging.OrderBy(in.Sort, map[string]string{"id": "n.id", "occurredAt": "n.occurred_at",
		"severity": "CASE n.severity WHEN 'critical' THEN 3 WHEN 'warning' THEN 2 ELSE 1 END"}, "n.occurred_at DESC, n.id DESC")
	if err != nil {
		return paging.Page[Notification]{}, err
	}
	w, err := paging.Resolve(*in, f, c.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Notification]{}, err
	}
	args := []any{c.Principal.MembershipID}
	add := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	conds := []string{"n.recipient_membership_id = $1", readableSQL(c.Principal, "(n.target->>'kind')", "(n.target->>'id')::uuid", &args)}
	if f.Severity != nil {
		conds = append(conds, "n.severity = "+add(*f.Severity))
	}
	if f.UnreadOnly != nil && *f.UnreadOnly {
		conds = append(conds, "n.read_at IS NULL")
	}
	if f.Type != nil {
		conds = append(conds, "n.type = "+add(*f.Type))
	}
	where := strings.Join(conds, " AND ")
	var total int
	if err := c.Tx.QueryRow(ctx, "SELECT count(*) FROM "+inboxFrom()+" WHERE "+where, args...).Scan(&total); err != nil {
		return paging.Page[Notification]{}, err
	}
	rows, err := c.Tx.Query(ctx, fmt.Sprintf("SELECT %s FROM %s WHERE %s ORDER BY %s LIMIT %d OFFSET %d", inboxCols, inboxFrom(), where, order, w.Limit, w.Offset), args...)
	if err != nil {
		return paging.Page[Notification]{}, err
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
	if err := rows.Err(); err != nil {
		return paging.Page[Notification]{}, err
	}
	return paging.Page[Notification]{Items: items, NextCursor: w.Next(total), Total: total, SnapshotVersion: w.Snapshot}, nil
}

// MarkReadInput is notifications.markRead input.
type MarkReadInput struct {
	ID uuid.UUID `json:"id"`
}

// Validate implements ops.Validator.
func (in *MarkReadInput) Validate() map[string]string {
	if in.ID == uuid.Nil {
		return map[string]string{"id": "error.required"}
	}
	return nil
}

func (Inbox) markRead(ctx context.Context, c *ops.Call, in *MarkReadInput) (Notification, error) {
	args := []any{in.ID, c.Principal.MembershipID}
	q := "SELECT " + inboxCols + " FROM " + inboxFrom() + " WHERE n.id = $1 AND n.recipient_membership_id = $2 AND " +
		readableSQL(c.Principal, "(n.target->>'kind')", "(n.target->>'id')::uuid", &args) + " FOR UPDATE OF n"
	x, err := scanNotification(c.Tx.QueryRow(ctx, q, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return x, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return x, err
	}
	if x.Version != *c.ExpectedVersion {
		return x, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	if x.ReadAt != nil { // already read: unchanged
		return x, nil
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE notify.notifications SET read_at = $2, version = version + 1 WHERE id = $1`, x.ID, c.Now); err != nil {
		return x, err
	}
	x.ReadAt, x.Version, x.UpdatedAt = &c.Now, x.Version+1, c.Now
	return x, nil
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
	kinds     = []string{"unit", "job", "invoice", "restriction", "device", "inquiry"}
	templates = []string{"alert", "quality", "schedule_change", "report_return", "completion", "payment", "payment_reminder", "restriction", "inquiry", "job_update", "device_operation"}
	channels  = []string{"inApp", "email", "whatsapp"}
	roles     = []string{"client", "contractor", "technician", "admin"}
)

func (in *TargetInput) validate(fe map[string]string) {
	if !slices.Contains(kinds, in.Target.Kind) || in.Target.ID == uuid.Nil {
		fe["target"] = "error.invalid"
	}
	if !slices.Contains(templates, in.TemplateKey) {
		fe["templateKey"] = "error.invalid"
	}
	if !slices.Contains(channels, in.Channel) {
		fe["channel"] = "error.invalid"
	}
}

type target struct {
	org  *uuid.UUID
	name string
}

// resolve loads the target and checks that the caller can read it (NOT_FOUND) and, for payment_reminder, the IR20
// preconditions (billing.write FORBIDDEN, overdue unpaid invoice CONFLICT).
func resolve(ctx context.Context, c *ops.Call, in *TargetInput) (target, error) {
	args := []any{in.Target.Kind, in.Target.ID}
	var t target
	err := c.Tx.QueryRow(ctx, "SELECT t.org, t.name FROM "+fmt.Sprintf(targetLateral, "$1::text", "$2::uuid")+" WHERE "+readableSQL(c.Principal, "$1::text", "$2::uuid", &args), args...).Scan(&t.org, &t.name)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return t, err
	}
	if in.TemplateKey == "payment_reminder" {
		if c.Principal.Role != "admin" || !c.Principal.Permissions["billing.write"] {
			return t, apperr.E(apperr.Forbidden, "error.forbidden")
		}
		if in.Target.Kind != "invoice" {
			return t, apperr.Fields(map[string]string{"target": "error.invalid"})
		}
		var ok bool
		if err := c.Tx.QueryRow(ctx, `SELECT status = 'unpaid' AND due_at < $2 FROM billing.invoices WHERE id = $1`, in.Target.ID, c.Now).Scan(&ok); err != nil {
			return t, err
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
		CASE WHEN m.role = 'client' AND u.phone IS NOT NULL THEN ARRAY['inApp','email','whatsapp'] ELSE ARRAY['inApp','email'] END
	FROM identity.memberships m JOIN identity.users u ON u.id = m.user_id
	WHERE m.valid_from <= $1 AND (m.valid_until IS NULL OR m.valid_until > $1)
	  AND ((m.role = 'client' AND m.organization_id = $2)
	    OR (NOT $5 AND m.role = 'admin' AND EXISTS (SELECT 1 FROM identity.membership_permissions mp WHERE mp.membership_id = m.id AND mp.permission = ANY($6)))
	    OR (NOT $5 AND $3 = 'job' AND m.role = 'contractor' AND m.organization_id IN (SELECT o.contractor_org_id FROM maintenance.offers o WHERE o.job_id = $4))
	    OR (NOT $5 AND $3 = 'job' AND m.role = 'technician' AND m.id IN (SELECT a.technician_membership_id FROM maintenance.assignments a WHERE a.job_id = $4 AND a.status = 'active')))`

func recipients(ctx context.Context, c *ops.Call, in *TargetInput, t target, role *string) ([]Recipient, error) {
	var org uuid.UUID
	if t.org != nil {
		org = *t.org
	}
	q := "SELECT * FROM (" + recipientsSQL + ") r(id, role, label, channels) WHERE $7 = ANY(r.channels)"
	args := []any{c.Now, org, in.Target.Kind, in.Target.ID, in.TemplateKey == "payment_reminder", adminKinds[in.Target.Kind], in.Channel}
	if role != nil {
		q += " AND r.role = $8"
		args = append(args, *role)
	}
	rows, err := c.Tx.Query(ctx, q+" ORDER BY r.role, r.label, r.id", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Recipient{}
	for rows.Next() {
		var r Recipient
		if err := rows.Scan(&r.ID, &r.Role, &r.DisplayLabel, &r.AllowedChannels); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RecipientsInput is notifications.recipients input.
type RecipientsInput struct {
	TargetInput
	Role  *string      `json:"role,omitempty"`
	Query paging.Query `json:"query"`
}

// Validate implements ops.Validator.
func (in *RecipientsInput) Validate() map[string]string {
	fe := map[string]string{}
	in.validate(fe)
	if in.Role != nil && !slices.Contains(roles, *in.Role) {
		fe["role"] = "error.invalid"
	}
	return fe
}

func (Inbox) recipients(ctx context.Context, c *ops.Call, in *RecipientsInput) (paging.Page[Recipient], error) {
	if len(in.Query.Filters) > 0 && string(in.Query.Filters) != "{}" || in.Query.Sort != nil {
		return paging.Page[Recipient]{}, apperr.Fields(map[string]string{"query": "error.invalid"})
	}
	w, err := paging.Resolve(in.Query, struct{}{}, c.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Recipient]{}, err
	}
	t, err := resolve(ctx, c, &in.TargetInput)
	if err != nil {
		return paging.Page[Recipient]{}, err
	}
	all, err := recipients(ctx, c, &in.TargetInput, t, in.Role)
	if err != nil {
		return paging.Page[Recipient]{}, err
	}
	end := min(w.Offset+w.Limit, len(all))
	page := []Recipient{}
	if w.Offset < len(all) {
		page = all[w.Offset:end]
	}
	return paging.Page[Recipient]{Items: page, NextCursor: w.Next(len(all)), Total: len(all), SnapshotVersion: w.Snapshot}, nil
}

// PreviewInput is notifications.preview input.
type PreviewInput struct {
	TargetInput
	RecipientMembershipID uuid.UUID  `json:"recipientMembershipId"`
	SourceAlertID         *uuid.UUID `json:"sourceAlertId,omitempty"`
	Message               *string    `json:"message,omitempty"`
	Reason                *string    `json:"reason,omitempty"`
}

// Validate implements ops.Validator.
func (in *PreviewInput) Validate() map[string]string {
	fe := map[string]string{}
	in.validate(fe)
	if in.RecipientMembershipID == uuid.Nil {
		fe["recipientMembershipId"] = "error.required"
	}
	for k, s := range map[string]*string{"message": in.Message, "reason": in.Reason} {
		if s != nil {
			*s = strings.TrimSpace(*s)
			if n := utf8.RuneCountInString(*s); n < 1 || n > 1000 {
				fe[k] = "error.length"
			}
		}
	}
	return fe
}

// preview renders an unsaved Notification for one eligible recipient (IR04): nothing is stored and the temporary
// ID cannot be used with markRead.
func (Inbox) preview(ctx context.Context, c *ops.Call, in *PreviewInput) (Notification, error) {
	t, err := resolve(ctx, c, &in.TargetInput)
	if err != nil {
		return Notification{}, err
	}
	all, err := recipients(ctx, c, &in.TargetInput, t, nil)
	if err != nil {
		return Notification{}, err
	}
	i := slices.IndexFunc(all, func(r Recipient) bool { return r.ID == in.RecipientMembershipID })
	if i < 0 {
		return Notification{}, apperr.Fields(map[string]string{"recipientMembershipId": "errors.recipient_ineligible"})
	}
	var scope int
	if err := c.Tx.QueryRow(ctx, `SELECT scope_version FROM identity.memberships WHERE id = $1`, in.RecipientMembershipID).Scan(&scope); err != nil {
		return Notification{}, err
	}
	typ := in.TemplateKey
	if typ == "alert" {
		typ = "fault"
	}
	severity := "normal"
	if in.TemplateKey == "payment_reminder" || in.TemplateKey == "restriction" || in.TemplateKey == "alert" {
		severity = "warning"
	}
	return Notification{ID: uuid.New(), TenantID: c.Principal.TenantID, Version: 1, CreatedAt: c.Now, UpdatedAt: c.Now, SourceAlertID: in.SourceAlertID, Type: typ,
		RecipientMembershipID: in.RecipientMembershipID, ScopeVersionAtCreation: scope, Target: map[string]string{"kind": in.Target.Kind, "id": in.Target.ID.String()},
		TemplateKey: in.TemplateKey, Params: Params{TargetName: t.name, At: c.Now, Status: typ, Reason: in.Reason, Message: in.Message}, Channel: in.Channel,
		DeliveryState: "preview", Severity: severity, OccurredAt: c.Now}, nil
}

// Register binds the notification operations.
func Register(r *ops.Registry) {
	var m Inbox
	ops.Register(r, "notifications.list", m.list)
	ops.Register(r, "notifications.markRead", m.markRead)
	ops.Register(r, "notifications.recipients", m.recipients)
	ops.Register(r, "notifications.preview", m.preview)
}
