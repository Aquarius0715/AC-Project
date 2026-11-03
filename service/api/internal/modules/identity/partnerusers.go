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
	// an active technician who holds an assigned job: not disabled or removed until reassigned (IR421, IR432)
	Booked     bool       `json:"booked"`
	RevokedAt  *time.Time `json:"revokedAt"` // HQ ended the access: only HQ gives it back (IR508)
	accessKept *time.Time // the access end kept while disabled
}

// accessUntil: the membership's access end, or the one kept while the account is disabled.
const partnerUserCols = `x.id, x.tenant_id, x.version, x.created_at, x.updated_at, x.organization_id, ` +
	`x.membership_id, x.email::text, x.display_name, x.partner_role, x.status,
	u.last_sign_in_at, CASE WHEN x.status = 'disabled' THEN x.access_until ELSE m.valid_until END, x.invited_at, ` +
	`x.invited_by_membership_id, m.revoked_at, x.access_until`

const partnerUserFrom = `identity.partner_users x LEFT JOIN identity.memberships m ON m.id = x.membership_id LEFT ` +
	`JOIN identity.users u ON u.id = m.user_id`

func scanPartnerUser(r pgx.Row) (PartnerUser, error) {
	var partnerUser PartnerUser

	err := r.Scan(
		&partnerUser.ID,
		&partnerUser.TenantID,
		&partnerUser.Version,
		&partnerUser.CreatedAt,
		&partnerUser.UpdatedAt,
		&partnerUser.OrganizationID,
		&partnerUser.MembershipID,
		&partnerUser.Email,
		&partnerUser.DisplayName,
		&partnerUser.PartnerRole,
		&partnerUser.Status,
		&partnerUser.LastSignInAt,
		&partnerUser.AccessUntil,
		&partnerUser.InvitedAt,
		&partnerUser.InvitedByMembershipID,
		&partnerUser.RevokedAt,
		&partnerUser.accessKept,
	)

	return partnerUser, apperr.Wrap(err, "scan (scanPartnerUser)")
}

// partnerUsersList lists the caller's contractor company accounts and invites.
//
//	@Summary		partnerUsers.list (read)
//	@ID				partnerUsers.list
//	@Description	Authorization: contractor:own-company
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot; own company only
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-P11, DD-P06 · Query: filters status,partnerRole,search · sort \
//	@Description	id,name,email,invitedAt,createdAt,updatedAt (default name asc;id asc)
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
func partnerUsersList(
	ctx context.Context,
	call *ops.Call,
	query *paging.Query,
) (paging.Page[PartnerUser], error) {
	var f struct {
		Status      *string `json:"status,omitempty"`
		PartnerRole *string `json:"partnerRole,omitempty"`
		Search      *string `json:"search,omitempty"` // email or display name contains, case-insensitive
	}

	if len(query.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(query.Filters)))
		dec.DisallowUnknownFields()

		if dec.Decode(&f) != nil ||
			(f.Status != nil && !slices.Contains([]string{"invited", "active", "disabled"}, *f.Status)) ||
			(f.PartnerRole != nil && *f.PartnerRole != "contractor" && *f.PartnerRole != "technician") {
			return paging.Page[PartnerUser]{}, apperr.Fields(
				map[string]string{"filters": "error.invalid"},
			)
		}
	}

	order, err := paging.OrderBy(
		query.Sort,
		map[string]string{
			"id":        "x.id",
			"name":      "lower(COALESCE(x.display_name, x.email::text))",
			"email":     "x.email",
			"invitedAt": "x.invited_at",
			"createdAt": "x.created_at",
			"updatedAt": "x.updated_at",
		},
		"lower(COALESCE(x.display_name, x.email::text)) ASC, x.id ASC",
	)
	if err != nil {
		return paging.Page[PartnerUser]{}, apperr.Wrap(err, "paging.OrderBy")
	}

	window, err := paging.Resolve(*query, f, call.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[PartnerUser]{}, apperr.Wrap(err, "paging.Resolve")
	}

	args := []any{call.Principal.OrgID}
	add := func(v any) string {
		args = append(args, v)

		return fmt.Sprintf("$%d", len(args))
	}

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

	scanErr := call.Tx.QueryRow(ctx, "SELECT count(*) FROM identity.partner_users x WHERE "+where, args...).
		Scan(&total)
	if scanErr != nil {
		return paging.Page[PartnerUser]{}, apperr.Wrap(scanErr, "select identity.partner_users")
	}

	rows, err := call.Tx.Query(
		ctx,
		fmt.Sprintf(
			"SELECT %s FROM %s WHERE %s ORDER BY %s LIMIT %d OFFSET %d",
			partnerUserCols,
			partnerUserFrom,
			where,
			order,
			window.Limit,
			window.Offset,
		),
		args...)
	if err != nil {
		return paging.Page[PartnerUser]{}, apperr.Wrap(err, "select (partnerUsersList)")
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

	rowsErr := rows.Err()
	if rowsErr != nil {
		return paging.Page[PartnerUser]{}, apperr.Wrap(rowsErr, "select (partnerUsersList)")
	}

	rows.Close()

	markBookedErr := markBooked(ctx, call, items)
	if markBookedErr != nil {
		return paging.Page[PartnerUser]{}, markBookedErr
	}

	return paging.Page[PartnerUser]{
		Items:           items,
		NextCursor:      window.Next(total),
		Total:           total,
		SnapshotVersion: window.Snapshot,
	}, nil
}

// markBooked sets Booked on the active technicians of the page that hold an assigned job (one question to the
// maintenance service for the page, IR432).
func markBooked(ctx context.Context, call *ops.Call, items []PartnerUser) error {
	var ids []uuid.UUID

	for _, x := range items {
		if x.PartnerRole == "technician" && x.Status == "active" && x.MembershipID != nil {
			ids = append(ids, *x.MembershipID)
		}
	}

	if len(ids) == 0 {
		return nil
	}

	booked, err := ops.Query[[]uuid.UUID](
		ctx,
		call,
		"maintenance.bookedTechnicians",
		map[string]any{"membershipIds": ids},
	)
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
func loadPartnerUser(
	ctx context.Context,
	call *ops.Call,
	id uuid.UUID,
	lock bool,
) (PartnerUser, error) {
	q := "SELECT " + partnerUserCols + " FROM " + partnerUserFrom + " WHERE x.id = $1"
	if lock {
		q += " FOR UPDATE OF x"
	}

	x, err := scanPartnerUser(call.Tx.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && x.OrganizationID != call.Principal.OrgID) {
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
	fieldErrs := map[string]string{}

	in.Email = strings.TrimSpace(in.Email)

	a, err := mail.ParseAddress(
		in.Email,
	)
	if err != nil || a.Address != in.Email ||
		len(in.Email) > 254 {
		fieldErrs["email"] = "error.invalid"
	}

	if in.PartnerRole != "contractor" && in.PartnerRole != "technician" {
		fieldErrs["partnerRole"] = "error.invalid"
	}

	if in.Status != nil && *in.Status != "active" && *in.Status != "disabled" {
		fieldErrs["status"] = "error.invalid"
	}

	if in.Reason != nil {
		*in.Reason = strings.TrimSpace(*in.Reason)
		if n := utf8.RuneCountInString(*in.Reason); n < 1 || n > 1000 {
			fieldErrs["reason"] = "error.length"
		}
	}

	if in.ID == nil { // a new user is named (IR461)
		if in.DisplayName == nil {
			fieldErrs["displayName"] = "error.required"
		} else if *in.DisplayName = strings.TrimSpace(*in.DisplayName); utf8.RuneCountInString(*in.DisplayName) < 1 ||
			utf8.RuneCountInString(*in.DisplayName) > 120 {
			fieldErrs["displayName"] = "error.length"
		}

		if in.PartnerRole == "technician" && in.AccessUntil == nil {
			fieldErrs["accessUntil"] = "error.required"
		}
	} else if in.DisplayName != nil {
		fieldErrs["displayName"] = "error.notAllowed"
	}

	if in.AccessUntil != nil && in.PartnerRole != "technician" {
		fieldErrs["accessUntil"] = "error.notAllowed"
	}

	return fieldErrs
}

// accessUntilOK: after now and at most TechnicianAccessMax ahead (IR461).
func accessUntilOK(now, until time.Time) bool {
	return until.After(now) && !until.After(now.Add(TechnicianAccessMax))
}

// otherActiveContractor reports whether the company keeps another active contractor account besides the given one.
// The company's changes to its contractors wait for each other here, so two at once — each disabling or removing the
// other contractor — cannot both see the other still active (IR471).
func otherActiveContractor(
	ctx context.Context,
	call *ops.Call,
	org, except uuid.UUID,
) (bool, error) {
	_, execErr := call.Tx.Exec(
		ctx,
		`SELECT pg_advisory_xact_lock(hashtext('identity.active_contractors:' || $1::text))`,
		org,
	)
	if execErr != nil {
		return false, apperr.Wrap(execErr, "select pg_advisory_xact_lock")
	}

	var ok bool

	err := call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.partner_users WHERE organization_id = $1 AND id <>
		$2
		AND partner_role = 'contractor' AND status = 'active')`, org, except).
		Scan(&ok)

	return ok, apperr.Wrap(err, "select identity.partner_users")
}

// InviteResendGap is the least time between two invitation e-mails to one invite (IR423): the write limit per member
// alone would let 60 e-mails a minute reach an address that has not agreed to anything.
const InviteResendGap = 5 * time.Minute

// inviteResendWait locks the invite row of table and refuses a resend sooner than InviteResendGap after the last
// invitation e-mail (RATE_LIMITED with the seconds to wait).
func inviteResendWait(ctx context.Context, call *ops.Call, table string, id uuid.UUID) error {
	var last *time.Time

	err := call.Tx.QueryRow(ctx, `SELECT last_invite_sent_at FROM identity.`+table+` WHERE id = $1 FOR UPDATE`, id).
		Scan(&last)
	if err != nil {
		return apperr.Wrap(err, "select identity")
	}

	if last == nil {
		return nil
	}

	if wait := last.Add(InviteResendGap).Sub(call.Now); wait > 0 {
		return apperr.E(apperr.RateLimited, "errors.invite_recently_sent").
			RetryAfter(int((wait + time.Second - 1) / time.Second))
	}

	return nil
}

// notSelfOrLast refuses a change to the caller's own account, one that would leave the company without an active
// contractor, and closing a technician who still holds an active assignment (IR421: reassign the job first).
func notSelfOrLast(ctx context.Context, c *ops.Call, partnerUser PartnerUser) error {
	if partnerUser.MembershipID != nil && *partnerUser.MembershipID == c.Principal.MembershipID {
		return apperr.E(apperr.Conflict, "errors.own_account")
	}

	if partnerUser.PartnerRole == "technician" && partnerUser.Status == "active" &&
		partnerUser.MembershipID != nil {
		booked, err := ops.Query[bool](
			ctx,
			c,
			"maintenance.technicianBooked",
			map[string]any{"membershipId": *partnerUser.MembershipID},
		)
		if err != nil {
			return err
		}

		if booked {
			return apperr.E(apperr.Conflict, "errors.technician_booked")
		}
	}

	if partnerUser.PartnerRole == "contractor" && partnerUser.Status == "active" {
		ok, err := otherActiveContractor(ctx, c, partnerUser.OrganizationID, partnerUser.ID)
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
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	email unique per company and one account per person (errors.email_taken; a person this tenant \
//	@Description	removed is added again; IR462); at most 30 accounts a contractor or customer owner adds to the \
//	@Description	organization in 24 hours (RATE_LIMITED errors.too_many_accounts with retryAfterSeconds; HQ not \
//	@Description	limited; IR464); a new user needs displayName 1–120 and is created at once with a set-up e-mail; a \
//	@Description	technician needs accessUntil at most a year ahead (IR461); email and role fixed; not the caller's \
//	@Description	own account; not the last active contractor; not a technician with an active assignment (CONFLICT; \
//	@Description	IR421)
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
func partnerUsersSave(
	ctx context.Context,
	c *ops.Call,
	input *PartnerUserInput,
) (PartnerUser, error) {
	var self uuid.UUID
	if input.ID != nil {
		self = *input.ID
	}

	var dup bool

	err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.partner_users WHERE organization_id = $1 AND email =
		$2::citext AND id <> $3)`, c.Principal.OrgID, input.Email, self).
		Scan(&dup)
	if err != nil {
		return PartnerUser{}, apperr.Wrap(err, "select identity.partner_users")
	}

	if dup {
		return PartnerUser{}, apperr.Fields(map[string]string{"email": "errors.email_taken"})
	}

	if input.ID == nil { // a new user: the account is created now and gets the set-up e-mail (IR461)
		if input.Status != nil {
			return PartnerUser{}, apperr.Fields(map[string]string{"status": "error.notAllowed"})
		}

		if input.Reason != nil {
			return PartnerUser{}, apperr.Fields(map[string]string{"reason": "error.notAllowed"})
		}

		acct := newAccount{
			Email:       input.Email,
			DisplayName: *input.DisplayName,
			Role:        input.PartnerRole,
			OrgID:       c.Principal.OrgID,
			Permissions: defaultPermissions[input.PartnerRole],
			Employment:  nil,
			ClientRole:  nil,
			ValidUntil:  nil,
			Scopes:      nil,
		}
		if input.PartnerRole == "technician" {
			if !accessUntilOK(c.Now, *input.AccessUntil) {
				return PartnerUser{}, apperr.Fields(map[string]string{"accessUntil": "error.range"})
			}

			ext := "external"
			acct.Employment, acct.ValidUntil, acct.Scopes = &ext, input.AccessUntil, []ScopeRef{}
		} else {
			acct.Scopes = []ScopeRef{{Kind: "organization", ID: c.Principal.OrgID}}
		}

		_, membership, err := createAccount(ctx, c, acct)
		if err != nil {
			return PartnerUser{}, err
		}

		id := uuid.New()

		_, execErr := c.Tx.Exec(
			ctx,
			`INSERT INTO identity.partner_users (id, tenant_id, organization_id, membership_id, email, display_name,
				partner_role, status, invited_at, invited_by_membership_id, last_invite_sent_at, created_at, updated_at)
			VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, 'invited', $7, $8, $7, $7, $7)`,
			id,
			c.Principal.OrgID,
			membership,
			input.Email,
			*input.DisplayName,
			input.PartnerRole,
			c.Now,
			c.Principal.MembershipID,
		)
		if execErr != nil {
			return PartnerUser{}, apperr.Wrap(execErr, "insert identity.partner_users")
		}

		v := 1
		c.Audit(
			ops.AuditEntry{
				Action:          "partnerUsers.save",
				TargetKind:      "partner_user",
				TargetID:        id.String(),
				NextVersion:     &v,
				Reason:          "account created",
				PreviousVersion: nil,
				Result:          "",
				Before:          nil,
				After:           nil,
			},
		)

		return loadPartnerUser(ctx, c, id, false)
	}

	partnerUser, err := loadPartnerUser(ctx, c, *input.ID, true)
	if err != nil {
		return partnerUser, err
	}

	if partnerUser.Version != *c.ExpectedVersion {
		return partnerUser, apperr.E(apperr.Conflict, "error.versionConflict")
	}

	if input.Reason == nil {
		return partnerUser, apperr.Fields(map[string]string{"reason": "error.required"})
	}

	status := partnerUser.Status
	if input.Status != nil {
		status = *input.Status
	}

	if partnerUser.Status == "invited" && status != "invited" &&
		input.Status != nil { // active at the first sign-in (IR461)
		return partnerUser, apperr.E(apperr.Conflict, "errors.invite_pending")
	}

	if partnerUser.RevokedAt != nil &&
		(partnerUser.Status == "disabled" && status != "disabled" || input.AccessUntil != nil) {
		// HQ ended the access: only HQ gives it back (IR508)
		return partnerUser, apperr.E(apperr.Conflict, "errors.revoked_by_hq")
	}
	// the account exists from the start (IR461): its e-mail and role are fixed; remove the user and add them again
	if !strings.EqualFold(input.Email, partnerUser.Email) {
		return partnerUser, apperr.Fields(map[string]string{"email": "error.immutable"})
	} else if input.PartnerRole != partnerUser.PartnerRole {
		return partnerUser, apperr.Fields(map[string]string{"partnerRole": "error.immutable"})
	} else {
		input.Email = partnerUser.Email // the same address in other letter case keeps the stored spelling
	}

	if partnerUser.Status != "invited" && status != partnerUser.Status ||
		partnerUser.PartnerRole != input.PartnerRole {
		err := notSelfOrLast(ctx, c, partnerUser)
		if err != nil {
			return partnerUser, err
		}
	}

	kept := partnerUser.accessKept
	if partnerUser.MembershipID != nil && status != partnerUser.Status {
		if status == "disabled" { // keep the access end, and close the membership now
			kept = partnerUser.AccessUntil

			_, err := c.Tx.Exec(
				ctx,
				`UPDATE identity.memberships SET valid_until = GREATEST($2::timestamptz, valid_from + interval '1 second'),
					scope_version = scope_version + 1,
				version = version + 1, updated_at = $2 WHERE id = $1`,
				*partnerUser.MembershipID,
				c.Now,
			)
			if err != nil {
				return partnerUser, apperr.Wrap(err, "update identity.memberships")
			}
		} else { // re-enabled: the kept access end comes back, if it has not passed
			if kept != nil && !kept.After(c.Now) {
				return partnerUser, apperr.E(apperr.Conflict, "errors.access_expired")
			}

			_, err := c.Tx.Exec(
				ctx,
				`UPDATE identity.memberships SET valid_until = $2, scope_version = scope_version + 1, version = version + 1,
					updated_at = $3 WHERE id = $1`,
				*partnerUser.MembershipID,
				kept,
				c.Now,
			)
			if err != nil {
				return partnerUser, apperr.Wrap(err, "update identity.memberships")
			}

			kept = nil
		}
	}

	if input.AccessUntil != nil { // a technician's access end, set by the contractor (IR461)
		if !accessUntilOK(c.Now, *input.AccessUntil) {
			return partnerUser, apperr.Fields(map[string]string{"accessUntil": "error.range"})
		}

		if partnerUser.MembershipID == nil {
			return partnerUser, apperr.E(apperr.Conflict, "error.invalidState")
		}

		if partnerUser.AccessUntil != nil &&
			input.AccessUntil.Before(
				*partnerUser.AccessUntil,
			) { // shortening: not while booked (IR421)
			booked, err := ops.Query[bool](
				ctx,
				c,
				"maintenance.technicianBooked",
				map[string]any{"membershipId": *partnerUser.MembershipID},
			)
			if err != nil {
				return partnerUser, err
			}

			if booked {
				return partnerUser, apperr.E(apperr.Conflict, "errors.technician_booked")
			}
		}

		if status == "disabled" {
			kept = input.AccessUntil // comes back when re-enabled
		} else {
			_, err := c.Tx.Exec(
				ctx,
				`UPDATE identity.memberships SET valid_until = $2, scope_version = scope_version + 1, version = version + 1,
					updated_at = $3 WHERE id = $1`,
				*partnerUser.MembershipID,
				*input.AccessUntil,
				c.Now,
			)
			if err != nil {
				return partnerUser, apperr.Wrap(err, "update identity.memberships")
			}
		}
	}

	_, execErr := c.Tx.Exec(
		ctx,
		`UPDATE identity.partner_users SET email = $2, partner_role = $3, status = $4, access_until = $5, version = version +
			1, updated_at = $6 WHERE id = $1`,
		partnerUser.ID,
		input.Email,
		input.PartnerRole,
		status,
		kept,
		c.Now,
	)
	if execErr != nil {
		return partnerUser, apperr.Wrap(execErr, "update identity.partner_users")
	}

	if partnerUser.MembershipID != nil && status != partnerUser.Status &&
		(status == "disabled" || partnerUser.Status == "disabled") { // the sign-in follows (IR461)
		setAccountEnabled(c, *partnerUser.MembershipID, status != "disabled")
	}

	next := partnerUser.Version + 1
	c.Audit(
		ops.AuditEntry{
			Action:          "partnerUsers.save",
			TargetKind:      "partner_user",
			TargetID:        partnerUser.ID.String(),
			PreviousVersion: c.ExpectedVersion,
			NextVersion:     &next,
			Reason:          *input.Reason, Result: "", Before: nil, After: nil,
		},
	)

	return loadPartnerUser(ctx, c, partnerUser.ID, false)
}

// PartnerRemoveInput is partnerUsers.remove input.
type PartnerRemoveInput struct {
	ID     uuid.UUID `json:"id"`
	Reason string    `json:"reason"`
}

// Validate implements ops.Validator.
func (in *PartnerRemoveInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.ID == uuid.Nil {
		fieldErrs["id"] = "error.required"
	}

	in.Reason = strings.TrimSpace(in.Reason)
	if n := utf8.RuneCountInString(in.Reason); n < 1 || n > 1000 {
		fieldErrs["reason"] = "error.length"
	}

	return fieldErrs
}

// partnerUsersRemove removes an invite or an account of the caller's company; an account's membership ends now.
//
//	@Summary		partnerUsers.remove (write)
//	@ID				partnerUsers.remove
//	@Description	Authorization: contractor:own-company
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	reason 1–1000; not the caller's own account; not the last active contractor; not a technician with \
//	@Description	an active assignment (CONFLICT; IR421)
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
func partnerUsersRemove(
	ctx context.Context,
	call *ops.Call,
	in *PartnerRemoveInput,
) (Deleted, error) {
	partnerUser, err := loadPartnerUser(ctx, call, in.ID, true)
	if err != nil {
		return Deleted{}, err
	}

	if partnerUser.Version != *call.ExpectedVersion {
		return Deleted{}, apperr.E(apperr.Conflict, "error.versionConflict")
	}

	notSelfOrLastErr := notSelfOrLast(ctx, call, partnerUser)
	if notSelfOrLastErr != nil {
		return Deleted{}, notSelfOrLastErr
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`DELETE FROM identity.partner_users WHERE id = $1`,
		partnerUser.ID,
	)
	if execErr != nil {
		return Deleted{}, apperr.Wrap(execErr, "delete identity.partner_users")
	}

	if partnerUser.MembershipID != nil {
		_, err := call.Tx.Exec(
			ctx,
			`UPDATE identity.memberships SET valid_until = GREATEST($2::timestamptz, valid_from + interval '1 second'),
				scope_version = scope_version + 1,
			version = version + 1, updated_at = $2 WHERE id = $1 AND (valid_until IS NULL OR valid_until > $2)`,
			*partnerUser.MembershipID,
			call.Now,
		)
		if err != nil {
			return Deleted{}, apperr.Wrap(err, "update identity.memberships")
		}

		setAccountEnabled(call, *partnerUser.MembershipID, false) // the sign-in ends too (IR461)
	}

	call.Audit(
		ops.AuditEntry{
			Action:          "partnerUsers.remove",
			TargetKind:      "partner_user",
			TargetID:        partnerUser.ID.String(),
			PreviousVersion: call.ExpectedVersion,
			Reason:          in.Reason, NextVersion: nil, Result: "", Before: nil, After: nil,
		},
	)

	return Deleted{ID: partnerUser.ID, Deleted: true}, nil
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
//	@Description	Validation: D01; input constraints in the corresponding DD; sends the set-up e-mail again (IR461; an \
//	@Description	invite from before IR461 queues the invitation e-mail; IR418); users not signed in yet only \
//	@Description	(CONFLICT); not once the access has ended (CONFLICT errors.access_expired; IR461); at most every 5 \
//	@Description	minutes per user (RATE_LIMITED retryAfterSeconds; IR423)
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
func partnerUsersResend(
	ctx context.Context,
	call *ops.Call,
	in *PartnerResendInput,
) (InvitePreview, error) {
	partnerUser, err := loadPartnerUser(ctx, call, in.ID, false)
	if err != nil {
		return InvitePreview{}, err
	}

	if partnerUser.Status != "invited" {
		return InvitePreview{}, apperr.E(apperr.Conflict, "errors.invite_not_pending")
	}

	if partnerUser.AccessUntil != nil &&
		!partnerUser.AccessUntil.After(
			call.Now,
		) { // no set-up e-mail once the access has ended (IR461)
		return InvitePreview{}, apperr.E(apperr.Conflict, "errors.access_expired")
	}

	inviteResendWaitErr := inviteResendWait(ctx, call, "partner_users", partnerUser.ID)
	if inviteResendWaitErr != nil {
		return InvitePreview{}, inviteResendWaitErr
	}

	var user uuid.UUID // the set-up e-mail again (IR461)

	scanErr := call.Tx.QueryRow(ctx, `SELECT user_id FROM identity.memberships WHERE id = $1`, *partnerUser.MembershipID).
		Scan(&user)
	if scanErr != nil {
		return InvitePreview{}, apperr.Wrap(scanErr, "select identity.memberships")
	}

	requestAccount(call, user, *partnerUser.MembershipID, partnerUser.PartnerRole)

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE identity.partner_users SET last_invite_sent_at = $2 WHERE id = $1`,
		partnerUser.ID,
		call.Now,
	)
	if execErr != nil {
		return InvitePreview{}, apperr.Wrap(execErr, "update identity.partner_users")
	}

	call.Audit(
		ops.AuditEntry{
			Action:          "partnerUsers.resendInvite",
			TargetKind:      "partner_user",
			TargetID:        partnerUser.ID.String(),
			Reason:          "set-up e-mail sent again",
			PreviousVersion: nil,
			NextVersion:     nil,
			Result:          "",
			Before:          nil,
			After:           nil,
		},
	)

	return InvitePreview{
		ID:          uuid.New(),
		TenantID:    partnerUser.TenantID,
		Version:     1,
		CreatedAt:   call.Now,
		UpdatedAt:   call.Now,
		TargetOpen:  true,
		Type:        "invite",
		Target:      map[string]string{"kind": "partner_user", "id": partnerUser.ID.String()},
		TemplateKey: "invite",
		Params: map[string]any{
			"targetName":  partnerUser.Email,
			"at":          call.Now,
			"status":      "invited",
			"reason":      nil,
			"amountMinor": nil,
			"currency":    nil,
			"method":      nil,
			"message":     nil,
		},
		Channel:                "email",
		DeliveryState:          "queued",
		Severity:               "normal",
		OccurredAt:             call.Now,
		SourceAlertID:          nil,
		RecipientMembershipID:  nil,
		ScopeVersionAtCreation: 0,
		ReadAt:                 nil,
	}, nil
}

// RegisterPartnerUsers binds partnerUsers.*.
func RegisterPartnerUsers(r *ops.Registry) {
	ops.Register(r, "partnerUsers.list", partnerUsersList)
	ops.Register(r, "partnerUsers.save", partnerUsersSave)
	ops.Register(r, "partnerUsers.remove", partnerUsersRemove)
	ops.Register(r, "partnerUsers.resendInvite", partnerUsersResend)
}
