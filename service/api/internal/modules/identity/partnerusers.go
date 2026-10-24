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

	"github.com/pradita/ac-project/service/api/internal/modules/notify"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

// PartnerUser is PartnerUser of service-contracts.ts: an account of a contractor company (a contractor or one of its
// technicians), or an invite to one (FR-P11).
type PartnerUser struct {
	ID                    uuid.UUID  `json:"id"`
	TenantID              uuid.UUID  `json:"tenantId"`
	Version               int        `json:"version"`
	CreatedAt             time.Time  `json:"createdAt"`
	UpdatedAt             time.Time  `json:"updatedAt"`
	OrganizationID        uuid.UUID  `json:"organizationId"`
	MembershipID          *uuid.UUID `json:"membershipId"`
	Email                 string     `json:"email"`
	DisplayName           *string    `json:"displayName"`
	PartnerRole           string     `json:"partnerRole"`
	Status                string     `json:"status"`
	LastSignInAt          *time.Time `json:"lastSignInAt"`
	AccessUntil           *time.Time `json:"accessUntil"`
	InvitedAt             time.Time  `json:"invitedAt"`
	InvitedByMembershipID uuid.UUID  `json:"invitedByMembershipId"`
	Booked                bool       `json:"booked"` // an active technician who holds an assigned job: not disabled or removed until reassigned (IR421, IR432)
	accessKept            *time.Time // the access end kept while disabled
}

// accessUntil: the membership's access end, or the one kept while the account is disabled
const partnerUserCols = `x.id, x.tenant_id, x.version, x.created_at, x.updated_at, x.organization_id, x.membership_id, x.email::text, x.display_name, x.partner_role, x.status,
	u.last_sign_in_at, CASE WHEN x.status = 'disabled' THEN x.access_until ELSE m.valid_until END, x.invited_at, x.invited_by_membership_id, x.access_until`

const partnerUserFrom = `identity.partner_users x LEFT JOIN identity.memberships m ON m.id = x.membership_id LEFT JOIN identity.users u ON u.id = m.user_id`

func scanPartnerUser(r pgx.Row) (PartnerUser, error) {
	var x PartnerUser
	err := r.Scan(&x.ID, &x.TenantID, &x.Version, &x.CreatedAt, &x.UpdatedAt, &x.OrganizationID, &x.MembershipID, &x.Email, &x.DisplayName, &x.PartnerRole, &x.Status,
		&x.LastSignInAt, &x.AccessUntil, &x.InvitedAt, &x.InvitedByMembershipID, &x.accessKept)
	return x, err
}

// partnerUsersList lists the caller's contractor company accounts and invites.
//
//	@Summary		partnerUsers.list (read)
//	@ID				partnerUsers.list
//	@Description	Authorization: contractor:own-company
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot; own company only
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-P11 · Query: filters status,partnerRole,search · sort id,name,email,invitedAt,createdAt,updatedAt (default name asc;id asc)
//	@Tags			partnerUsers
//	@Accept			json
//	@Produce		json
//	@Param			cursor		query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit		query		integer	false	"page size 1–100, default 25"
//	@Param			sort		query		string	false	"field:direction — fields id,name,email,invitedAt,createdAt,updatedAt; default name asc;id asc"
//	@Param			status		query		string	false	"filter → status"
//	@Param			partnerRole	query		string	false	"filter → partnerRole"
//	@Param			search		query		string	false	"filter → email or displayName contains (case-insensitive)"
//	@Success		200			{object}	ops.Envelope{data=PartnerUserPage}
//	@Failure		401			{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403			{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404			{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409			{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422			{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429			{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503			{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504			{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/partner-users [get]
func partnerUsersList(ctx context.Context, c *ops.Call, in *paging.Query) (paging.Page[PartnerUser], error) {
	var f struct {
		Status      *string `json:"status,omitempty"`
		PartnerRole *string `json:"partnerRole,omitempty"`
		Search      *string `json:"search,omitempty"` // email or display name contains, case-insensitive
	}
	if len(in.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(in.Filters)))
		dec.DisallowUnknownFields()
		if dec.Decode(&f) != nil || (f.Status != nil && !slices.Contains([]string{"invited", "active", "disabled"}, *f.Status)) ||
			(f.PartnerRole != nil && *f.PartnerRole != "contractor" && *f.PartnerRole != "technician") {
			return paging.Page[PartnerUser]{}, apperr.Fields(map[string]string{"filters": "error.invalid"})
		}
	}
	order, err := paging.OrderBy(in.Sort, map[string]string{"id": "x.id", "name": "lower(COALESCE(x.display_name, x.email::text))", "email": "x.email", "invitedAt": "x.invited_at", "createdAt": "x.created_at", "updatedAt": "x.updated_at"}, "lower(COALESCE(x.display_name, x.email::text)) ASC, x.id ASC")
	if err != nil {
		return paging.Page[PartnerUser]{}, err
	}
	w, err := paging.Resolve(*in, f, c.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[PartnerUser]{}, err
	}
	args := []any{c.Principal.OrgID}
	add := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	conds := []string{"x.organization_id = $1"}
	if f.Status != nil {
		conds = append(conds, "x.status = "+add(*f.Status))
	}
	if f.PartnerRole != nil {
		conds = append(conds, "x.partner_role = "+add(*f.PartnerRole))
	}
	if f.Search != nil && strings.TrimSpace(*f.Search) != "" {
		p := add(paging.Contains(*f.Search))
		conds = append(conds, "(x.email ILIKE "+p+" OR x.display_name ILIKE "+p+")")
	}
	where := strings.Join(conds, " AND ")
	var total int
	if err := c.Tx.QueryRow(ctx, "SELECT count(*) FROM identity.partner_users x WHERE "+where, args...).Scan(&total); err != nil {
		return paging.Page[PartnerUser]{}, err
	}
	rows, err := c.Tx.Query(ctx, fmt.Sprintf("SELECT %s FROM %s WHERE %s ORDER BY %s LIMIT %d OFFSET %d", partnerUserCols, partnerUserFrom, where, order, w.Limit, w.Offset), args...)
	if err != nil {
		return paging.Page[PartnerUser]{}, err
	}
	defer rows.Close()
	items := []PartnerUser{}
	for rows.Next() {
		x, err := scanPartnerUser(rows)
		if err != nil {
			return paging.Page[PartnerUser]{}, err
		}
		items = append(items, x)
	}
	if err := rows.Err(); err != nil {
		return paging.Page[PartnerUser]{}, err
	}
	rows.Close()
	if err := markBooked(ctx, c, items); err != nil {
		return paging.Page[PartnerUser]{}, err
	}
	return paging.Page[PartnerUser]{Items: items, NextCursor: w.Next(total), Total: total, SnapshotVersion: w.Snapshot}, nil
}

// markBooked sets Booked on the active technicians of the page that hold an assigned job (one question to the
// maintenance service for the page, IR432).
func markBooked(ctx context.Context, c *ops.Call, items []PartnerUser) error {
	var ids []uuid.UUID
	for _, x := range items {
		if x.PartnerRole == "technician" && x.Status == "active" && x.MembershipID != nil {
			ids = append(ids, *x.MembershipID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	booked, err := ops.Query[[]uuid.UUID](ctx, c, "maintenance.bookedTechnicians", map[string]any{"membershipIds": ids})
	if err != nil {
		return err
	}
	for i := range items {
		if items[i].MembershipID != nil && slices.Contains(booked, *items[i].MembershipID) {
			items[i].Booked = true
		}
	}
	return nil
}

// loadPartnerUser loads an account of the caller's company; another company's is NOT_FOUND.
func loadPartnerUser(ctx context.Context, c *ops.Call, id uuid.UUID, lock bool) (PartnerUser, error) {
	q := "SELECT " + partnerUserCols + " FROM " + partnerUserFrom + " WHERE x.id = $1"
	if lock {
		q += " FOR UPDATE OF x"
	}
	x, err := scanPartnerUser(c.Tx.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && x.OrganizationID != c.Principal.OrgID) {
		return PartnerUser{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	return x, err
}

// PartnerUserInput is partnerUsers.save input.
type PartnerUserInput struct {
	ID          *uuid.UUID `json:"id,omitempty"`
	Email       string     `json:"email"`
	PartnerRole string     `json:"partnerRole"`
	Status      *string    `json:"status,omitempty"`
	Reason      *string    `json:"reason,omitempty"`
	// DisplayName names a new user (IR461); AccessUntil is a technician's access end, set by the contractor (at most
	// TechnicianAccessMax ahead) when it adds or extends them
	DisplayName *string    `json:"displayName,omitempty"`
	AccessUntil *time.Time `json:"accessUntil,omitempty"`
}

// Validate implements ops.Validator.
func (in *PartnerUserInput) Validate() map[string]string {
	fe := map[string]string{}
	in.Email = strings.TrimSpace(in.Email)
	if a, err := mail.ParseAddress(in.Email); err != nil || a.Address != in.Email || len(in.Email) > 254 {
		fe["email"] = "error.invalid"
	}
	if in.PartnerRole != "contractor" && in.PartnerRole != "technician" {
		fe["partnerRole"] = "error.invalid"
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
	if in.ID == nil { // a new user is named (IR461)
		if in.DisplayName == nil {
			fe["displayName"] = "error.required"
		} else if *in.DisplayName = strings.TrimSpace(*in.DisplayName); utf8.RuneCountInString(*in.DisplayName) < 1 || utf8.RuneCountInString(*in.DisplayName) > 120 {
			fe["displayName"] = "error.length"
		}
		if in.PartnerRole == "technician" && in.AccessUntil == nil {
			fe["accessUntil"] = "error.required"
		}
	} else if in.DisplayName != nil {
		fe["displayName"] = "error.notAllowed"
	}
	if in.AccessUntil != nil && in.PartnerRole != "technician" {
		fe["accessUntil"] = "error.notAllowed"
	}
	return fe
}

// accessUntilOK: after now and at most TechnicianAccessMax ahead (IR461).
func accessUntilOK(now, until time.Time) bool {
	return until.After(now) && !until.After(now.Add(TechnicianAccessMax))
}

// otherActiveContractor reports whether the company keeps another active contractor account besides the given one.
func otherActiveContractor(ctx context.Context, c *ops.Call, org, except uuid.UUID) (bool, error) {
	var ok bool
	err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.partner_users WHERE organization_id = $1 AND id <> $2 AND partner_role = 'contractor' AND status = 'active')`, org, except).Scan(&ok)
	return ok, err
}

// InviteResendGap is the least time between two invitation e-mails to one invite (IR423): the write limit per member
// alone would let 60 e-mails a minute reach an address that has not agreed to anything.
const InviteResendGap = 5 * time.Minute

// inviteResendWait locks the invite row of table and refuses a resend sooner than InviteResendGap after the last
// invitation e-mail (RATE_LIMITED with the seconds to wait).
func inviteResendWait(ctx context.Context, c *ops.Call, table string, id uuid.UUID) error {
	var last *time.Time
	if err := c.Tx.QueryRow(ctx, `SELECT last_invite_sent_at FROM identity.`+table+` WHERE id = $1 FOR UPDATE`, id).Scan(&last); err != nil {
		return err
	}
	if last == nil {
		return nil
	}
	if wait := last.Add(InviteResendGap).Sub(c.Now); wait > 0 {
		return apperr.E(apperr.RateLimited, "errors.invite_recently_sent").RetryAfter(int((wait + time.Second - 1) / time.Second))
	}
	return nil
}

// reinvite sends the invitation of a pending invite whose e-mail changed to the new address: the e-mails not yet sent
// to the old one are cancelled, and the resend pause counts from now (IR426).
func reinvite(ctx context.Context, c *ops.Call, table string, id uuid.UUID, queue func() error) error {
	if err := notify.CancelInvites(ctx, c.Tx, id); err != nil {
		return err
	}
	if err := queue(); err != nil {
		return err
	}
	_, err := c.Tx.Exec(ctx, `UPDATE identity.`+table+` SET last_invite_sent_at = $2 WHERE id = $1`, id, c.Now)
	return err
}

// notSelfOrLast refuses a change to the caller's own account, one that would leave the company without an active
// contractor, and closing a technician who still holds an active assignment (IR421: reassign the job first).
func notSelfOrLast(ctx context.Context, c *ops.Call, x PartnerUser) error {
	if x.MembershipID != nil && *x.MembershipID == c.Principal.MembershipID {
		return apperr.E(apperr.Conflict, "errors.own_account")
	}
	if x.PartnerRole == "technician" && x.Status == "active" && x.MembershipID != nil {
		booked, err := ops.Query[bool](ctx, c, "maintenance.technicianBooked", map[string]any{"membershipId": *x.MembershipID})
		if err != nil {
			return err
		}
		if booked {
			return apperr.E(apperr.Conflict, "errors.technician_booked")
		}
	}
	if x.PartnerRole == "contractor" && x.Status == "active" {
		ok, err := otherActiveContractor(ctx, c, x.OrganizationID, x.ID)
		if err != nil {
			return err
		}
		if !ok {
			return apperr.E(apperr.Conflict, "errors.last_contractor")
		}
	}
	return nil
}

// partnerUsersSave invites an account to the caller's company (id omitted), or changes one: a pending invite's email
// and role, or an account's access (disable / re-enable). The role and email of an accepted account stay as they are.
//
//	@Summary		partnerUsers.save (write)
//	@ID				partnerUsers.save
//	@Description	Authorization: contractor:own-company
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; email unique per company and one account per person (errors.email_taken); a new user needs displayName 1–120 and is created at once with a set-up e-mail; a technician needs accessUntil at most a year ahead (IR461); email and role fixed; not the caller's own account; not the last active contractor; not a technician with an active assignment (CONFLICT; IR421)
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-P11
//	@Tags			partnerUsers
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string				true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		PartnerUserInput	true	"input"
//	@Success		200				{object}	ops.Envelope{data=PartnerUser}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/partner-users [post]
func partnerUsersSave(ctx context.Context, c *ops.Call, in *PartnerUserInput) (PartnerUser, error) {
	var self uuid.UUID
	if in.ID != nil {
		self = *in.ID
	}
	var dup bool
	if err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.partner_users WHERE organization_id = $1 AND email = $2::citext AND id <> $3)`, c.Principal.OrgID, in.Email, self).Scan(&dup); err != nil {
		return PartnerUser{}, err
	}
	if dup {
		return PartnerUser{}, apperr.Fields(map[string]string{"email": "errors.email_taken"})
	}
	if in.ID == nil { // a new user: the account is created now and gets the set-up e-mail (IR461)
		if in.Status != nil {
			return PartnerUser{}, apperr.Fields(map[string]string{"status": "error.notAllowed"})
		}
		if in.Reason != nil {
			return PartnerUser{}, apperr.Fields(map[string]string{"reason": "error.notAllowed"})
		}
		acct := newAccount{Email: in.Email, DisplayName: *in.DisplayName, Role: in.PartnerRole, OrgID: c.Principal.OrgID, Permissions: defaultPermissions[in.PartnerRole]}
		if in.PartnerRole == "technician" {
			if !accessUntilOK(c.Now, *in.AccessUntil) {
				return PartnerUser{}, apperr.Fields(map[string]string{"accessUntil": "error.range"})
			}
			ext := "external"
			acct.Employment, acct.ValidUntil, acct.Scopes = &ext, in.AccessUntil, []ScopeRef{}
		} else {
			acct.Scopes = []ScopeRef{{Kind: "organization", ID: c.Principal.OrgID}}
		}
		_, membership, err := createAccount(ctx, c, acct)
		if err != nil {
			return PartnerUser{}, err
		}
		id := uuid.New()
		if _, err := c.Tx.Exec(ctx, `INSERT INTO identity.partner_users (id, tenant_id, organization_id, membership_id, email, display_name, partner_role, status, invited_at, invited_by_membership_id, last_invite_sent_at, created_at, updated_at)
			VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, 'invited', $7, $8, $7, $7, $7)`, id, c.Principal.OrgID, membership, in.Email, *in.DisplayName, in.PartnerRole, c.Now, c.Principal.MembershipID); err != nil {
			return PartnerUser{}, err
		}
		v := 1
		c.Audit(ops.AuditEntry{Action: "partnerUsers.save", TargetKind: "partner_user", TargetID: id.String(), NextVersion: &v, Reason: "account created"})
		return loadPartnerUser(ctx, c, id, false)
	}
	x, err := loadPartnerUser(ctx, c, *in.ID, true)
	if err != nil {
		return x, err
	}
	if x.Version != *c.ExpectedVersion {
		return x, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	if in.Reason == nil {
		return x, apperr.Fields(map[string]string{"reason": "error.required"})
	}
	status := x.Status
	if in.Status != nil {
		status = *in.Status
	}
	if x.Status == "invited" && status != "invited" && in.Status != nil { // active at the first sign-in (IR461)
		return x, apperr.E(apperr.Conflict, "errors.invite_pending")
	}
	// the account exists from the start (IR461): its e-mail and role are fixed; remove the user and add them again
	if !strings.EqualFold(in.Email, x.Email) {
		return x, apperr.Fields(map[string]string{"email": "error.immutable"})
	} else if in.PartnerRole != x.PartnerRole {
		return x, apperr.Fields(map[string]string{"partnerRole": "error.immutable"})
	} else {
		in.Email = x.Email // the same address in other letter case keeps the stored spelling
	}
	if x.Status != "invited" && status != x.Status || x.PartnerRole != in.PartnerRole {
		if err := notSelfOrLast(ctx, c, x); err != nil {
			return x, err
		}
	}
	kept := x.accessKept
	if x.MembershipID != nil && status != x.Status {
		if status == "disabled" { // keep the access end, and close the membership now
			kept = x.AccessUntil
			if _, err := c.Tx.Exec(ctx, `UPDATE identity.memberships SET valid_until = GREATEST($2::timestamptz, valid_from + interval '1 second'), scope_version = scope_version + 1,
				version = version + 1, updated_at = $2 WHERE id = $1`, *x.MembershipID, c.Now); err != nil {
				return x, err
			}
		} else { // re-enabled: the kept access end comes back, if it has not passed
			if kept != nil && !kept.After(c.Now) {
				return x, apperr.E(apperr.Conflict, "errors.access_expired")
			}
			if _, err := c.Tx.Exec(ctx, `UPDATE identity.memberships SET valid_until = $2, scope_version = scope_version + 1, version = version + 1, updated_at = $3 WHERE id = $1`,
				*x.MembershipID, kept, c.Now); err != nil {
				return x, err
			}
			kept = nil
		}
	}
	if in.AccessUntil != nil { // a technician's access end, set by the contractor (IR461)
		if !accessUntilOK(c.Now, *in.AccessUntil) {
			return x, apperr.Fields(map[string]string{"accessUntil": "error.range"})
		}
		if x.MembershipID == nil {
			return x, apperr.E(apperr.Conflict, "error.invalidState")
		}
		if x.AccessUntil != nil && in.AccessUntil.Before(*x.AccessUntil) { // shortening: not while booked (IR421)
			booked, err := ops.Query[bool](ctx, c, "maintenance.technicianBooked", map[string]any{"membershipId": *x.MembershipID})
			if err != nil {
				return x, err
			}
			if booked {
				return x, apperr.E(apperr.Conflict, "errors.technician_booked")
			}
		}
		if status == "disabled" {
			kept = in.AccessUntil // comes back when re-enabled
		} else if _, err := c.Tx.Exec(ctx, `UPDATE identity.memberships SET valid_until = $2, scope_version = scope_version + 1, version = version + 1, updated_at = $3 WHERE id = $1`,
			*x.MembershipID, *in.AccessUntil, c.Now); err != nil {
			return x, err
		}
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE identity.partner_users SET email = $2, partner_role = $3, status = $4, access_until = $5, version = version + 1, updated_at = $6 WHERE id = $1`,
		x.ID, in.Email, in.PartnerRole, status, kept, c.Now); err != nil {
		return x, err
	}
	if x.MembershipID != nil && status != x.Status && (status == "disabled" || x.Status == "disabled") { // the sign-in follows (IR461)
		setAccountEnabled(c, *x.MembershipID, status != "disabled")
	}
	next := x.Version + 1
	c.Audit(ops.AuditEntry{Action: "partnerUsers.save", TargetKind: "partner_user", TargetID: x.ID.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &next, Reason: *in.Reason})
	return loadPartnerUser(ctx, c, x.ID, false)
}

// PartnerRemoveInput is partnerUsers.remove input.
type PartnerRemoveInput struct {
	ID     uuid.UUID `json:"id"`
	Reason string    `json:"reason"`
}

// Validate implements ops.Validator.
func (in *PartnerRemoveInput) Validate() map[string]string {
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

// partnerUsersRemove removes an invite or an account of the caller's company; an account's membership ends now.
//
//	@Summary		partnerUsers.remove (write)
//	@ID				partnerUsers.remove
//	@Description	Authorization: contractor:own-company
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; reason 1–1000; not the caller's own account; not the last active contractor; not a technician with an active assignment (CONFLICT; IR421)
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-P11
//	@Tags			partnerUsers
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string	true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer	true	"all: required (target partner_user, read partnerUsers.list)"
//	@Param			id					path		string	true	"input field id"
//	@Param			reason				query		string	false	"input field reason"
//	@Success		200					{object}	ops.Envelope{data=Deleted}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/partner-users/{id} [delete]
func partnerUsersRemove(ctx context.Context, c *ops.Call, in *PartnerRemoveInput) (Deleted, error) {
	x, err := loadPartnerUser(ctx, c, in.ID, true)
	if err != nil {
		return Deleted{}, err
	}
	if x.Version != *c.ExpectedVersion {
		return Deleted{}, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	if err := notSelfOrLast(ctx, c, x); err != nil {
		return Deleted{}, err
	}
	if err := notify.CancelInvites(ctx, c.Tx, x.ID); err != nil { // nothing more goes to a removed invite (IR426)
		return Deleted{}, err
	}
	if _, err := c.Tx.Exec(ctx, `DELETE FROM identity.partner_users WHERE id = $1`, x.ID); err != nil {
		return Deleted{}, err
	}
	if x.MembershipID != nil {
		if _, err := c.Tx.Exec(ctx, `UPDATE identity.memberships SET valid_until = GREATEST($2::timestamptz, valid_from + interval '1 second'), scope_version = scope_version + 1,
			version = version + 1, updated_at = $2 WHERE id = $1 AND (valid_until IS NULL OR valid_until > $2)`, *x.MembershipID, c.Now); err != nil {
			return Deleted{}, err
		}
		setAccountEnabled(c, *x.MembershipID, false) // the sign-in ends too (IR461)
	}
	c.Audit(ops.AuditEntry{Action: "partnerUsers.remove", TargetKind: "partner_user", TargetID: x.ID.String(), PreviousVersion: c.ExpectedVersion, Reason: in.Reason})
	return Deleted{ID: x.ID, Deleted: true}, nil
}

// PartnerResendInput is partnerUsers.resendInvite input.
type PartnerResendInput struct {
	ID uuid.UUID `json:"id"`
}

// Validate implements ops.Validator.
func (in *PartnerResendInput) Validate() map[string]string {
	if in.ID == uuid.Nil {
		return map[string]string{"id": "error.required"}
	}
	return nil
}

// partnerUsersResend queues the invitation e-mail again (IR418).
//
//	@Summary		partnerUsers.resendInvite (write)
//	@ID				partnerUsers.resendInvite
//	@Description	Authorization: contractor:own-company
//	@Description	Validation: D01; input constraints in the corresponding DD; sends the set-up e-mail again (IR461; an invite from before IR461 queues the invitation e-mail; IR418); users not signed in yet only (CONFLICT); not once the access has ended (CONFLICT errors.access_expired; IR461); at most every 5 minutes per user (RATE_LIMITED retryAfterSeconds; IR423)
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-P11
//	@Tags			partnerUsers
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string				true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			id				path		string				true	"input field id"
//	@Param			request			body		PartnerResendInput	true	"input; the path parameters come from the route"
//	@Success		200				{object}	ops.Envelope{data=InvitePreview}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/partner-users/{id}/resend-invite [post]
func partnerUsersResend(ctx context.Context, c *ops.Call, in *PartnerResendInput) (InvitePreview, error) {
	x, err := loadPartnerUser(ctx, c, in.ID, false)
	if err != nil {
		return InvitePreview{}, err
	}
	if x.Status != "invited" {
		return InvitePreview{}, apperr.E(apperr.Conflict, "errors.invite_not_pending")
	}
	if x.AccessUntil != nil && !x.AccessUntil.After(c.Now) { // no set-up e-mail once the access has ended (IR461)
		return InvitePreview{}, apperr.E(apperr.Conflict, "errors.access_expired")
	}
	if err := inviteResendWait(ctx, c, "partner_users", x.ID); err != nil {
		return InvitePreview{}, err
	}
	if x.MembershipID == nil { // an invite from before IR461: HQ creates its account
		if err := queuePartnerInvite(ctx, c, x.ID, x.Email, x.PartnerRole); err != nil {
			return InvitePreview{}, err
		}
	} else { // the set-up e-mail again (IR461)
		var user uuid.UUID
		if err := c.Tx.QueryRow(ctx, `SELECT user_id FROM identity.memberships WHERE id = $1`, *x.MembershipID).Scan(&user); err != nil {
			return InvitePreview{}, err
		}
		requestAccount(c, user, *x.MembershipID, x.PartnerRole)
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE identity.partner_users SET last_invite_sent_at = $2 WHERE id = $1`, x.ID, c.Now); err != nil {
		return InvitePreview{}, err
	}
	c.Audit(ops.AuditEntry{Action: "partnerUsers.resendInvite", TargetKind: "partner_user", TargetID: x.ID.String(), Reason: "invite e-mail queued"})
	return InvitePreview{ID: uuid.New(), TenantID: x.TenantID, Version: 1, CreatedAt: c.Now, UpdatedAt: c.Now, Type: "invite", Target: map[string]string{"kind": "partner_user", "id": x.ID.String()},
		TemplateKey: "invite", Params: map[string]any{"targetName": x.Email, "at": c.Now, "status": "invited", "reason": nil, "amountMinor": nil, "currency": nil, "method": nil, "message": nil},
		Channel: "email", DeliveryState: "queued", Severity: "normal", OccurredAt: c.Now}, nil
}

// queuePartnerInvite queues the invitation e-mail to the partner app (IR418); a technician's account opens the
// technician app, which HQ creates with the membership.
func queuePartnerInvite(ctx context.Context, c *ops.Call, invite uuid.UUID, email, role string) error {
	var org string
	if err := c.Tx.QueryRow(ctx, `SELECT name FROM identity.organizations WHERE id = $1`, c.Principal.OrgID).Scan(&org); err != nil {
		return err
	}
	app := "partner"
	if role == "technician" {
		app = "technician"
	}
	return notify.QueueInvite(ctx, c.Tx, invite, email, org, role, app)
}

// RegisterPartnerUsers binds partnerUsers.*.
func RegisterPartnerUsers(r *ops.Registry) {
	ops.Register(r, "partnerUsers.list", partnerUsersList)
	ops.Register(r, "partnerUsers.save", partnerUsersSave)
	ops.Register(r, "partnerUsers.remove", partnerUsersRemove)
	ops.Register(r, "partnerUsers.resendInvite", partnerUsersResend)
}
