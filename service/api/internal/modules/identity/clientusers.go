package identity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
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

// ClientUser is ClientUser of service-contracts.ts.
type ClientUser struct {
	ID                    uuid.UUID  `json:"id"`
	TenantID              uuid.UUID  `json:"tenantId"`
	Version               int        `json:"version"`
	CreatedAt             time.Time  `json:"createdAt"`
	UpdatedAt             time.Time  `json:"updatedAt"`
	CustomerID            uuid.UUID  `json:"customerId"`
	MembershipID          *uuid.UUID `json:"membershipId"`
	Email                 string     `json:"email"`
	DisplayName           *string    `json:"displayName"`
	ClientRole            string     `json:"clientRole"`
	Status                string     `json:"status"`
	LastSignInAt          *time.Time `json:"lastSignInAt"`
	AllowedChannels       []string   `json:"allowedChannels"`
	InvitedAt             time.Time  `json:"invitedAt"`
	InvitedByMembershipID uuid.UUID  `json:"invitedByMembershipId"`
	customerOrg           uuid.UUID
}

const clientUserCols = `x.id, x.tenant_id, x.version, x.created_at, x.updated_at, x.customer_id, x.membership_id, x.email::text, x.display_name, x.client_role, x.status,
	(SELECT u.last_sign_in_at FROM identity.memberships m JOIN identity.users u ON u.id = m.user_id WHERE m.id = x.membership_id), x.allowed_channels, x.invited_at,
	x.invited_by_membership_id, cu.organization_id`

const clientUserFrom = `identity.client_users x JOIN notify.ref_customers cu ON cu.id = x.customer_id`

func scanClientUser(r pgx.Row) (ClientUser, error) {
	var x ClientUser
	err := r.Scan(&x.ID, &x.TenantID, &x.Version, &x.CreatedAt, &x.UpdatedAt, &x.CustomerID, &x.MembershipID, &x.Email, &x.DisplayName, &x.ClientRole, &x.Status,
		&x.LastSignInAt, &x.AllowedChannels, &x.InvitedAt, &x.InvitedByMembershipID, &x.customerOrg)
	return x, err
}

// ownerOnly enforces the client:self-customer:owner alternative: clients must be owners (FORBIDDEN).
func ownerOnly(c *ops.Call) error {
	if c.Principal.Role == "client" && c.Principal.ClientRole != "owner" {
		return apperr.E(apperr.Forbidden, "error.forbidden")
	}
	return nil
}

// @Summary		clientUsers.list (read)
// @ID				clientUsers.list
// @Description	Authorization: client:self-customer:owner | admin:asset.read
// @Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
// @Description	Recovery: D04: retry only UNAVAILABLE, at most twice
// @Description	Design: DD-A17, DD-C19 · Query: filters customerId,status,clientRole,search · sort id,name,email,invitedAt,createdAt,updatedAt (default name asc;id asc)
// @Tags			clientUsers
// @Accept			json
// @Produce		json
// @Param			cursor		query		string	false	"page cursor: nextCursor of the previous page (D12)"
// @Param			limit		query		integer	false	"page size 1–100, default 25"
// @Param			sort		query		string	false	"field:direction — fields id,name,email,invitedAt,createdAt,updatedAt; default name asc;id asc"
// @Param			customerId	query		string	false	"filter → customerId"
// @Param			status		query		string	false	"filter → status"
// @Param			clientRole	query		string	false	"filter → clientRole"
// @Param			search		query		string	false	"filter → email or displayName contains (case-insensitive)"
// @Success		200			{object}	ops.Envelope{data=ClientUserPage}
// @Failure		401			{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403			{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404			{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409			{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422			{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429			{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503			{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504			{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/client-users [get]
func clientUsersList(ctx context.Context, c *ops.Call, in *paging.Query) (paging.Page[ClientUser], error) {
	if err := ownerOnly(c); err != nil {
		return paging.Page[ClientUser]{}, err
	}
	var f struct {
		CustomerID *uuid.UUID `json:"customerId,omitempty"`
		Status     *string    `json:"status,omitempty"`
		ClientRole *string    `json:"clientRole,omitempty"`
		Search     *string    `json:"search,omitempty"` // email or display name contains, case-insensitive
	}
	if len(in.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(in.Filters)))
		dec.DisallowUnknownFields()
		if dec.Decode(&f) != nil || (f.Status != nil && !slices.Contains([]string{"invited", "active", "disabled"}, *f.Status)) ||
			(f.ClientRole != nil && *f.ClientRole != "owner" && *f.ClientRole != "member") {
			return paging.Page[ClientUser]{}, apperr.Fields(map[string]string{"filters": "error.invalid"})
		}
	}
	order, err := paging.OrderBy(in.Sort, map[string]string{"id": "x.id", "name": "lower(COALESCE(x.display_name, x.email::text))", "email": "x.email", "invitedAt": "x.invited_at", "createdAt": "x.created_at", "updatedAt": "x.updated_at"}, "lower(COALESCE(x.display_name, x.email::text)) ASC, x.id ASC")
	if err != nil {
		return paging.Page[ClientUser]{}, err
	}
	w, err := paging.Resolve(*in, f, c.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[ClientUser]{}, err
	}
	var args []any
	add := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	conds := []string{"TRUE"}
	if c.Principal.Role == "client" {
		conds = append(conds, "cu.organization_id = "+add(c.Principal.OrgID))
	}
	if f.CustomerID != nil {
		conds = append(conds, "x.customer_id = "+add(*f.CustomerID))
	}
	if f.Status != nil {
		conds = append(conds, "x.status = "+add(*f.Status))
	}
	if f.ClientRole != nil {
		conds = append(conds, "x.client_role = "+add(*f.ClientRole))
	}
	if f.Search != nil && strings.TrimSpace(*f.Search) != "" {
		p := add("%" + strings.TrimSpace(*f.Search) + "%")
		conds = append(conds, "(x.email ILIKE "+p+" OR x.display_name ILIKE "+p+")")
	}
	where := strings.Join(conds, " AND ")
	var total int
	if err := c.Tx.QueryRow(ctx, "SELECT count(*) FROM "+clientUserFrom+" WHERE "+where, args...).Scan(&total); err != nil {
		return paging.Page[ClientUser]{}, err
	}
	rows, err := c.Tx.Query(ctx, fmt.Sprintf("SELECT %s FROM %s WHERE %s ORDER BY %s LIMIT %d OFFSET %d", clientUserCols, clientUserFrom, where, order, w.Limit, w.Offset), args...)
	if err != nil {
		return paging.Page[ClientUser]{}, err
	}
	defer rows.Close()
	items := []ClientUser{}
	for rows.Next() {
		x, err := scanClientUser(rows)
		if err != nil {
			return paging.Page[ClientUser]{}, err
		}
		items = append(items, x)
	}
	if err := rows.Err(); err != nil {
		return paging.Page[ClientUser]{}, err
	}
	return paging.Page[ClientUser]{Items: items, NextCursor: w.Next(total), Total: total, SnapshotVersion: w.Snapshot}, nil
}

func loadClientUser(ctx context.Context, c *ops.Call, id uuid.UUID, lock bool) (ClientUser, error) {
	q := "SELECT " + clientUserCols + " FROM " + clientUserFrom + " WHERE x.id = $1"
	if lock {
		q += " FOR UPDATE OF x"
	}
	x, err := scanClientUser(c.Tx.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && c.Principal.Role == "client" && x.customerOrg != c.Principal.OrgID) {
		return ClientUser{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	return x, err
}

// ClientUserInput is clientUsers.save input.
type ClientUserInput struct {
	ID         *uuid.UUID `json:"id,omitempty"`
	CustomerID uuid.UUID  `json:"customerId"`
	Email      string     `json:"email"`
	ClientRole string     `json:"clientRole"`
	Status     *string    `json:"status,omitempty"`
	Reason     *string    `json:"reason,omitempty"`
}

// Validate implements ops.Validator.
func (in *ClientUserInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.CustomerID == uuid.Nil {
		fe["customerId"] = "error.required"
	}
	in.Email = strings.TrimSpace(in.Email)
	if a, err := mail.ParseAddress(in.Email); err != nil || a.Address != in.Email || len(in.Email) > 254 {
		fe["email"] = "error.invalid"
	}
	if in.ClientRole != "owner" && in.ClientRole != "member" {
		fe["clientRole"] = "error.invalid"
	}
	if in.Status != nil && *in.Status != "active" && *in.Status != "disabled" {
		fe["status"] = "error.invalid"
	}
	if in.Reason != nil {
		*in.Reason = strings.TrimSpace(*in.Reason)
		if n := utf8.RuneCountInString(*in.Reason); n < 1 || n > 1000 {
			fe["reason"] = "error.length"
		}
	}
	return fe
}

// otherActiveOwner reports whether the customer keeps another active owner besides the given client user.
func otherActiveOwner(ctx context.Context, c *ops.Call, customer, except uuid.UUID) (bool, error) {
	var ok bool
	err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.client_users WHERE customer_id = $1 AND id <> $2 AND client_role = 'owner' AND status = 'active')`, customer, except).Scan(&ok)
	return ok, err
}

// @Summary		clientUsers.save (write)
// @ID				clientUsers.save
// @Description	Authorization: client:self-customer:owner:invite-member-only | admin:asset.write
// @Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; IR111 email unique per customer; last active owner cannot be demoted or disabled (CONFLICT)
// @Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
// @Description	Design: DD-A17, DD-C19
// @Tags			clientUsers
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key	header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Param			request			body		ClientUserInput	true	"input"
// @Success		200				{object}	ops.Envelope{data=ClientUser}
// @Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504				{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/client-users [post]
func clientUsersSave(ctx context.Context, c *ops.Call, in *ClientUserInput) (ClientUser, error) {
	if err := ownerOnly(c); err != nil {
		return ClientUser{}, err
	}
	var org uuid.UUID
	err := c.Tx.QueryRow(ctx, `SELECT organization_id FROM notify.ref_customers WHERE id = $1`, in.CustomerID).Scan(&org)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && c.Principal.Role == "client" && org != c.Principal.OrgID) {
		return ClientUser{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return ClientUser{}, err
	}
	var dup bool
	var self uuid.UUID
	if in.ID != nil {
		self = *in.ID
	}
	if err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.client_users WHERE customer_id = $1 AND email = $2::citext AND id <> $3)`, in.CustomerID, in.Email, self).Scan(&dup); err != nil {
		return ClientUser{}, err
	}
	if dup {
		return ClientUser{}, apperr.Fields(map[string]string{"email": "errors.email_taken"})
	}
	if in.ID == nil { // invite
		if c.Principal.Role == "client" && (in.ClientRole != "member" || in.Status != nil || in.Reason != nil) { // invite-member-only
			return ClientUser{}, apperr.E(apperr.Forbidden, "error.forbidden")
		}
		if in.Status != nil {
			return ClientUser{}, apperr.Fields(map[string]string{"status": "error.notAllowed"})
		}
		id := uuid.New()
		if _, err := c.Tx.Exec(ctx, `INSERT INTO identity.client_users (id, tenant_id, customer_id, email, client_role, status, invited_at, invited_by_membership_id, last_invite_sent_at, created_at, updated_at)
			VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, 'invited', $5, $6, $5, $5, $5)`, id, in.CustomerID, in.Email, in.ClientRole, c.Now, c.Principal.MembershipID); err != nil {
			return ClientUser{}, err
		}
		v := 1
		c.Audit(ops.AuditEntry{Action: "clientUsers.save", TargetKind: "client_user", TargetID: id.String(), NextVersion: &v, Reason: "invite"})
		return loadClientUser(ctx, c, id, false)
	}
	if c.Principal.Role == "client" {
		return ClientUser{}, apperr.E(apperr.Forbidden, "error.forbidden")
	}
	x, err := loadClientUser(ctx, c, *in.ID, true)
	if err != nil {
		return x, err
	}
	if x.Version != *c.ExpectedVersion {
		return x, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	if x.CustomerID != in.CustomerID {
		return x, apperr.Fields(map[string]string{"customerId": "error.immutable"})
	}
	if in.Reason == nil {
		return x, apperr.Fields(map[string]string{"reason": "error.required"})
	}
	status := x.Status
	if in.Status != nil {
		status = *in.Status
	}
	if status == "active" && x.Status != "active" && x.MembershipID == nil { // an invite becomes active only on first sign-in
		return x, apperr.E(apperr.Conflict, "errors.invite_pending")
	}
	if x.ClientRole == "owner" && x.Status == "active" && (in.ClientRole != "owner" || status != "active") {
		ok, err := otherActiveOwner(ctx, c, x.CustomerID, x.ID)
		if err != nil {
			return x, err
		}
		if !ok {
			return x, apperr.E(apperr.Conflict, "errors.last_owner")
		}
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE identity.client_users SET email = $2, client_role = $3, status = $4, version = version + 1, updated_at = $5 WHERE id = $1`,
		x.ID, in.Email, in.ClientRole, status, c.Now); err != nil {
		return x, err
	}
	if x.MembershipID != nil { // keep the membership in step: role and access window
		if _, err := c.Tx.Exec(ctx, `UPDATE identity.memberships SET client_role = $2, valid_until = CASE WHEN $3 = 'disabled' THEN $4 ELSE NULL END,
			scope_version = scope_version + 1, version = version + 1, updated_at = $4 WHERE id = $1`, *x.MembershipID, in.ClientRole, status, c.Now); err != nil {
			return x, err
		}
	}
	next := x.Version + 1
	c.Audit(ops.AuditEntry{Action: "clientUsers.save", TargetKind: "client_user", TargetID: x.ID.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &next, Reason: *in.Reason})
	return loadClientUser(ctx, c, x.ID, false)
}

// RemoveInput is clientUsers.remove input.
type RemoveInput struct {
	ID     uuid.UUID `json:"id"`
	Reason string    `json:"reason"`
}

// Validate implements ops.Validator.
func (in *RemoveInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.ID == uuid.Nil {
		fe["id"] = "error.required"
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if n := utf8.RuneCountInString(in.Reason); n < 1 || n > 1000 {
		fe["reason"] = "error.length"
	}
	return fe
}

// Deleted is DeletedResource.
type Deleted struct {
	ID      uuid.UUID `json:"id"`
	Deleted bool      `json:"deleted"`
}

// @Summary		clientUsers.remove (write)
// @ID				clientUsers.remove
// @Description	Authorization: admin:asset.write
// @Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; IR111 reason 1–1000; last active owner cannot be removed (CONFLICT)
// @Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
// @Description	Design: DD-A17
// @Tags			clientUsers
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key		header		string	true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Param			X-Expected-Version	header		integer	true	"all: required (target client_user, read clientUsers.list)"
// @Param			id					path		string	true	"input field id"
// @Param			reason				query		string	false	"input field reason"
// @Success		200					{object}	ops.Envelope{data=Deleted}
// @Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504					{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/client-users/{id} [delete]
func clientUsersRemove(ctx context.Context, c *ops.Call, in *RemoveInput) (Deleted, error) {
	x, err := loadClientUser(ctx, c, in.ID, true)
	if err != nil {
		return Deleted{}, err
	}
	if x.Version != *c.ExpectedVersion {
		return Deleted{}, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	if x.ClientRole == "owner" && x.Status == "active" {
		ok, err := otherActiveOwner(ctx, c, x.CustomerID, x.ID)
		if err != nil {
			return Deleted{}, err
		}
		if !ok {
			return Deleted{}, apperr.E(apperr.Conflict, "errors.last_owner")
		}
	}
	if _, err := c.Tx.Exec(ctx, `DELETE FROM identity.client_users WHERE id = $1`, x.ID); err != nil {
		return Deleted{}, err
	}
	if x.MembershipID != nil {
		if _, err := c.Tx.Exec(ctx, `UPDATE identity.memberships SET valid_until = $2, scope_version = scope_version + 1, version = version + 1, updated_at = $2
			WHERE id = $1 AND (valid_until IS NULL OR valid_until > $2)`, *x.MembershipID, c.Now); err != nil {
			return Deleted{}, err
		}
	}
	c.Audit(ops.AuditEntry{Action: "clientUsers.remove", TargetKind: "client_user", TargetID: x.ID.String(), PreviousVersion: c.ExpectedVersion, Reason: in.Reason})
	return Deleted{ID: x.ID, Deleted: true}, nil
}

// ResendInput is clientUsers.resendInvite input.
type ResendInput struct {
	ID uuid.UUID `json:"id"`
}

// Validate implements ops.Validator.
func (in *ResendInput) Validate() map[string]string {
	if in.ID == uuid.Nil {
		return map[string]string{"id": "error.required"}
	}
	return nil
}

// InvitePreview is the NotificationPreview returned by resendInvite (IR111: preview only, nothing saved or sent).
type InvitePreview struct {
	ID                     uuid.UUID         `json:"id"`
	TenantID               uuid.UUID         `json:"tenantId"`
	Version                int               `json:"version"`
	CreatedAt              time.Time         `json:"createdAt"`
	UpdatedAt              time.Time         `json:"updatedAt"`
	SourceAlertID          *uuid.UUID        `json:"sourceAlertId"`
	Type                   string            `json:"type"`
	RecipientMembershipID  *uuid.UUID        `json:"recipientMembershipId"`
	ScopeVersionAtCreation int               `json:"scopeVersionAtCreation"`
	Target                 map[string]string `json:"target"`
	TemplateKey            string            `json:"templateKey"`
	Params                 map[string]any    `json:"params"`
	Channel                string            `json:"channel"`
	DeliveryState          string            `json:"deliveryState"`
	Severity               string            `json:"severity"`
	OccurredAt             time.Time         `json:"occurredAt"`
	ReadAt                 *time.Time        `json:"readAt"`
}

// @Summary		clientUsers.resendInvite (read)
// @ID				clientUsers.resendInvite
// @Description	Authorization: client:self-customer:owner | admin:asset.write
// @Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
// @Description	Recovery: D04: retry only UNAVAILABLE, at most twice
// @Description	Design: DD-A17, DD-C19
// @Tags			clientUsers
// @Accept			json
// @Produce		json
// @Param			id		path		string		true	"input field id"
// @Param			request	body		ResendInput	true	"input; the path parameters come from the route"
// @Success		200		{object}	ops.Envelope{data=InvitePreview}
// @Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504		{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/client-users/{id}/resend-invite [post]
func clientUsersResend(ctx context.Context, c *ops.Call, in *ResendInput) (InvitePreview, error) {
	if err := ownerOnly(c); err != nil {
		return InvitePreview{}, err
	}
	x, err := loadClientUser(ctx, c, in.ID, false)
	if err != nil {
		return InvitePreview{}, err
	}
	if x.Status != "invited" {
		return InvitePreview{}, apperr.E(apperr.Conflict, "errors.invite_not_pending")
	}
	return InvitePreview{ID: uuid.New(), TenantID: x.TenantID, Version: 1, CreatedAt: c.Now, UpdatedAt: c.Now, Type: "invite", Target: map[string]string{"kind": "client_user", "id": x.ID.String()},
		TemplateKey: "invite", Params: map[string]any{"targetName": x.Email, "at": c.Now, "status": "invited", "reason": nil, "amountMinor": nil, "currency": nil, "method": nil, "message": nil},
		Channel: "email", DeliveryState: "preview", Severity: "normal", OccurredAt: c.Now}, nil
}

// RegisterClientUsers binds clientUsers.*.
func RegisterClientUsers(r *ops.Registry) {
	ops.Register(r, "clientUsers.list", clientUsersList)
	ops.Register(r, "clientUsers.save", clientUsersSave)
	ops.Register(r, "clientUsers.remove", clientUsersRemove)
	ops.Register(r, "clientUsers.resendInvite", clientUsersResend)
}
