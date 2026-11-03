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
	"github.com/pradita/ac-project/service/api/internal/platform/authz"
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
	var filters struct {
		Status      *string `json:"status,omitempty"`
		PartnerRole *string `json:"partnerRole,omitempty"`
		Search      *string `json:"search,omitempty"` // email or display name contains, case-insensitive
	}

	if len(query.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(query.Filters)))
		dec.DisallowUnknownFields()

		if dec.Decode(&filters) != nil ||
			(filters.Status != nil &&
				!slices.Contains([]string{statusInvited, statusActive, statusDisabled}, *filters.Status)) ||
			(filters.PartnerRole != nil &&
				*filters.PartnerRole != authz.RoleContractor && *filters.PartnerRole != authz.RoleTechnician) {
			return paging.Page[PartnerUser]{}, apperr.Fields(
				map[string]string{keyFilters: apperr.KeyInvalid},
			)
		}
	}

	order, err := paging.OrderBy(
		query.Sort,
		map[string]string{
			"id":         "x.id",
			"name":       "lower(COALESCE(x.display_name, x.email::text))",
			keyEmail:     "x.email",
			"invitedAt":  "x.invited_at",
			keyCreatedAt: "x.created_at",
			keyUpdatedAt: "x.updated_at",
		},
		"lower(COALESCE(x.display_name, x.email::text)) ASC, x.id ASC",
	)
	if err != nil {
		return paging.Page[PartnerUser]{}, apperr.Wrap(err, "paging.OrderBy")
	}

	window, err := paging.Resolve(*query, filters, call.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[PartnerUser]{}, apperr.Wrap(err, "paging.Resolve")
	}

	args := []any{call.Principal.OrgID}
	add := func(v any) string {
		args = append(args, v)

		return fmt.Sprintf("$%d", len(args))
	}

	conds := []string{"x.organization_id = $1"}
	if filters.Status != nil {
		conds = append(conds, "x.status = "+add(*filters.Status))
	}

	if filters.PartnerRole != nil {
		conds = append(conds, "x.partner_role = "+add(*filters.PartnerRole))
	}

	if filters.Search != nil && strings.TrimSpace(*filters.Search) != "" {
		p := add(paging.Contains(*filters.Search))
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
		if x.PartnerRole == authz.RoleTechnician && x.Status == statusActive &&
			x.MembershipID != nil {
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
	partnerUserID uuid.UUID,
	lock bool,
) (PartnerUser, error) {
	q := "SELECT " + partnerUserCols + " FROM " + partnerUserFrom + " WHERE x.id = $1"
	if lock {
		q += " FOR UPDATE OF x"
	}

	x, err := scanPartnerUser(call.Tx.QueryRow(ctx, q, partnerUserID))
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && x.OrganizationID != call.Principal.OrgID) {
		return PartnerUser{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
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
		fieldErrs[keyEmail] = apperr.KeyInvalid
	}

	if in.PartnerRole != authz.RoleContractor && in.PartnerRole != authz.RoleTechnician {
		fieldErrs["partnerRole"] = apperr.KeyInvalid
	}

	if in.Status != nil && *in.Status != statusActive && *in.Status != statusDisabled {
		fieldErrs[keyStatus] = apperr.KeyInvalid
	}

	if in.Reason != nil {
		*in.Reason = strings.TrimSpace(*in.Reason)
		if n := utf8.RuneCountInString(*in.Reason); n < 1 || n > 1000 {
			fieldErrs[keyReason] = apperr.KeyLength
		}
	}

	if in.ID == nil { // a new user is named (IR461)
		if in.DisplayName == nil {
			fieldErrs["displayName"] = apperr.KeyRequired
		} else if *in.DisplayName = strings.TrimSpace(*in.DisplayName); utf8.RuneCountInString(*in.DisplayName) < 1 ||
			utf8.RuneCountInString(*in.DisplayName) > 120 {
			fieldErrs["displayName"] = apperr.KeyLength
		}

		if in.PartnerRole == authz.RoleTechnician && in.AccessUntil == nil {
			fieldErrs["accessUntil"] = apperr.KeyRequired
		}
	} else if in.DisplayName != nil {
		fieldErrs["displayName"] = apperr.KeyNotAllowed
	}

	if in.AccessUntil != nil && in.PartnerRole != authz.RoleTechnician {
		fieldErrs["accessUntil"] = apperr.KeyNotAllowed
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

	var found bool

	err := call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.partner_users WHERE organization_id = $1 AND id <>
		$2
		AND partner_role = 'contractor' AND status = 'active')`, org, except).
		Scan(&found)

	return found, apperr.Wrap(err, "select identity.partner_users")
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

	if partnerUser.PartnerRole == authz.RoleTechnician && partnerUser.Status == statusActive &&
		partnerUser.MembershipID != nil {
		booked, err := ops.Query[bool](
			ctx,
			c,
			"maintenance.technicianBooked",
			map[string]any{keyMembershipID: *partnerUser.MembershipID},
		)
		if err != nil {
			return err
		}

		if booked {
			return apperr.E(apperr.Conflict, "errors.technician_booked")
		}
	}

	if partnerUser.PartnerRole == authz.RoleContractor && partnerUser.Status == statusActive {
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
	call *ops.Call,
	input *PartnerUserInput,
) (PartnerUser, error) {
	var self uuid.UUID
	if input.ID != nil {
		self = *input.ID
	}

	var dup bool

	err := call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.partner_users WHERE organization_id = $1 AND email
		=
		$2::citext AND id <> $3)`, call.Principal.OrgID, input.Email, self).
		Scan(&dup)
	if err != nil {
		return PartnerUser{}, apperr.Wrap(err, "select identity.partner_users")
	}

	if dup {
		return PartnerUser{}, apperr.Fields(map[string]string{keyEmail: keyEmailTaken})
	}

	if input.ID == nil { // a new user: the account is created now and gets the set-up e-mail (IR461)
		if input.Status != nil {
			return PartnerUser{}, apperr.Fields(
				map[string]string{keyStatus: apperr.KeyNotAllowed},
			)
		}

		if input.Reason != nil {
			return PartnerUser{}, apperr.Fields(
				map[string]string{keyReason: apperr.KeyNotAllowed},
			)
		}

		acct := newAccount{
			Email:       input.Email,
			DisplayName: *input.DisplayName,
			Role:        input.PartnerRole,
			OrgID:       call.Principal.OrgID,
			Permissions: defaultPermissions[input.PartnerRole],
			Employment:  nil,
			ClientRole:  nil,
			ValidUntil:  nil,
			Scopes:      nil,
		}
		if input.PartnerRole == authz.RoleTechnician {
			if !accessUntilOK(call.Now, *input.AccessUntil) {
				return PartnerUser{}, apperr.Fields(
					map[string]string{"accessUntil": apperr.KeyRange},
				)
			}

			ext := employmentExternal
			acct.Employment, acct.ValidUntil, acct.Scopes = &ext, input.AccessUntil, []ScopeRef{}
		} else {
			acct.Scopes = []ScopeRef{{Kind: scopeOrganization, ID: call.Principal.OrgID}}
		}

		_, membership, err := createAccount(ctx, call, acct)
		if err != nil {
			return PartnerUser{}, err
		}

		partnerUserID := uuid.New()

		_, execErr := call.Tx.Exec(
			ctx,
			`INSERT INTO identity.partner_users (id, tenant_id, organization_id, membership_id, email, display_name,
				partner_role, status, invited_at, invited_by_membership_id, last_invite_sent_at, created_at, updated_at)
			VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, 'invited', $7, $8, $7, $7, $7)`,
			partnerUserID,
			call.Principal.OrgID,
			membership,
			input.Email,
			*input.DisplayName,
			input.PartnerRole,
			call.Now,
			call.Principal.MembershipID,
		)
		if execErr != nil {
			return PartnerUser{}, apperr.Wrap(execErr, "insert identity.partner_users")
		}

		version := 1
		call.Audit(
			ops.AuditEntry{
				Action:          "partnerUsers.save",
				TargetKind:      targetPartnerUser,
				TargetID:        partnerUserID.String(),
				NextVersion:     &version,
				Reason:          "account created",
				PreviousVersion: nil,
				Result:          "",
				Before:          nil,
				After:           nil,
			},
		)

		return loadPartnerUser(ctx, call, partnerUserID, false)
	}

	partnerUser, err := loadPartnerUser(ctx, call, *input.ID, true)
	if err != nil {
		return partnerUser, err
	}

	if partnerUser.Version != *call.ExpectedVersion {
		return partnerUser, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	if input.Reason == nil {
		return partnerUser, apperr.Fields(map[string]string{keyReason: apperr.KeyRequired})
	}

	status := partnerUser.Status
	if input.Status != nil {
		status = *input.Status
	}

	if partnerUser.Status == statusInvited && status != statusInvited &&
		input.Status != nil { // active at the first sign-in (IR461)
		return partnerUser, apperr.E(apperr.Conflict, "errors.invite_pending")
	}

	if partnerUser.RevokedAt != nil &&
		(partnerUser.Status == statusDisabled && status != statusDisabled || input.AccessUntil != nil) {
		// HQ ended the access: only HQ gives it back (IR508)
		return partnerUser, apperr.E(apperr.Conflict, "errors.revoked_by_hq")
	}
	// the account exists from the start (IR461): its e-mail and role are fixed; remove the user and add them again
	if !strings.EqualFold(input.Email, partnerUser.Email) {
		return partnerUser, apperr.Fields(map[string]string{keyEmail: apperr.KeyImmutable})
	} else if input.PartnerRole != partnerUser.PartnerRole {
		return partnerUser, apperr.Fields(map[string]string{"partnerRole": apperr.KeyImmutable})
	} else {
		input.Email = partnerUser.Email // the same address in other letter case keeps the stored spelling
	}

	if partnerUser.Status != statusInvited && status != partnerUser.Status ||
		partnerUser.PartnerRole != input.PartnerRole {
		err := notSelfOrLast(ctx, call, partnerUser)
		if err != nil {
			return partnerUser, err
		}
	}

	kept := partnerUser.accessKept
	if partnerUser.MembershipID != nil && status != partnerUser.Status {
		if status == statusDisabled { // keep the access end, and close the membership now
			kept = partnerUser.AccessUntil

			_, err := call.Tx.Exec(
				ctx,
				`UPDATE identity.memberships SET valid_until = GREATEST($2::timestamptz, valid_from + interval '1 second'),
					scope_version = scope_version + 1,
				version = version + 1, updated_at = $2 WHERE id = $1`,
				*partnerUser.MembershipID,
				call.Now,
			)
			if err != nil {
				return partnerUser, apperr.Wrap(err, "update identity.memberships")
			}
		} else { // re-enabled: the kept access end comes back, if it has not passed
			if kept != nil && !kept.After(call.Now) {
				return partnerUser, apperr.E(apperr.Conflict, "errors.access_expired")
			}

			_, err := call.Tx.Exec(
				ctx,
				`UPDATE identity.memberships SET valid_until = $2, scope_version = scope_version + 1, version = version + 1,
					updated_at = $3 WHERE id = $1`,
				*partnerUser.MembershipID,
				kept,
				call.Now,
			)
			if err != nil {
				return partnerUser, apperr.Wrap(err, "update identity.memberships")
			}

			kept = nil
		}
	}

	if input.AccessUntil != nil { // a technician's access end, set by the contractor (IR461)
		if !accessUntilOK(call.Now, *input.AccessUntil) {
			return partnerUser, apperr.Fields(map[string]string{"accessUntil": apperr.KeyRange})
		}

		if partnerUser.MembershipID == nil {
			return partnerUser, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
		}

		if partnerUser.AccessUntil != nil &&
			input.AccessUntil.Before(
				*partnerUser.AccessUntil,
			) { // shortening: not while booked (IR421)
			booked, err := ops.Query[bool](
				ctx,
				call,
				"maintenance.technicianBooked",
				map[string]any{keyMembershipID: *partnerUser.MembershipID},
			)
			if err != nil {
				return partnerUser, err
			}

			if booked {
				return partnerUser, apperr.E(apperr.Conflict, "errors.technician_booked")
			}
		}

		if status == statusDisabled {
			kept = input.AccessUntil // comes back when re-enabled
		} else {
			_, err := call.Tx.Exec(
				ctx,
				`UPDATE identity.memberships SET valid_until = $2, scope_version = scope_version + 1, version = version + 1,
					updated_at = $3 WHERE id = $1`,
				*partnerUser.MembershipID,
				*input.AccessUntil,
				call.Now,
			)
			if err != nil {
				return partnerUser, apperr.Wrap(err, "update identity.memberships")
			}
		}
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE identity.partner_users SET email = $2, partner_role = $3, status = $4, access_until = $5, version = version +
			1, updated_at = $6 WHERE id = $1`,
		partnerUser.ID,
		input.Email,
		input.PartnerRole,
		status,
		kept,
		call.Now,
	)
	if execErr != nil {
		return partnerUser, apperr.Wrap(execErr, "update identity.partner_users")
	}

	if partnerUser.MembershipID != nil && status != partnerUser.Status &&
		(status == statusDisabled || partnerUser.Status == statusDisabled) { // the sign-in follows (IR461)
		setAccountEnabled(call, *partnerUser.MembershipID, status != statusDisabled)
	}

	next := partnerUser.Version + 1
	call.Audit(
		ops.AuditEntry{
			Action:          "partnerUsers.save",
			TargetKind:      targetPartnerUser,
			TargetID:        partnerUser.ID.String(),
			PreviousVersion: call.ExpectedVersion,
			NextVersion:     &next,
			Reason:          *input.Reason, Result: "", Before: nil, After: nil,
		},
	)

	return loadPartnerUser(ctx, call, partnerUser.ID, false)
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
		fieldErrs["id"] = apperr.KeyRequired
	}

	in.Reason = strings.TrimSpace(in.Reason)
	if n := utf8.RuneCountInString(in.Reason); n < 1 || n > 1000 {
		fieldErrs[keyReason] = apperr.KeyLength
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
	input *PartnerRemoveInput,
) (Deleted, error) {
	partnerUser, err := loadPartnerUser(ctx, call, input.ID, true)
	if err != nil {
		return Deleted{}, err
	}

	if partnerUser.Version != *call.ExpectedVersion {
		return Deleted{}, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
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
			TargetKind:      targetPartnerUser,
			TargetID:        partnerUser.ID.String(),
			PreviousVersion: call.ExpectedVersion,
			Reason:          input.Reason, NextVersion: nil, Result: "", Before: nil, After: nil,
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
		return map[string]string{"id": apperr.KeyRequired}
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

	if partnerUser.Status != statusInvited {
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
			TargetKind:      targetPartnerUser,
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
		Type:        noticeInvite,
		Target:      map[string]string{keyKind: targetPartnerUser, "id": partnerUser.ID.String()},
		TemplateKey: noticeInvite,
		Params: map[string]any{
			"targetName":  partnerUser.Email,
			"at":          call.Now,
			keyStatus:     statusInvited,
			keyReason:     nil,
			"amountMinor": nil,
			"currency":    nil,
			"method":      nil,
			"message":     nil,
		},
		Channel:                keyEmail,
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
