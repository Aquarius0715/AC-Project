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
	RevokedAt             *time.Time `json:"revokedAt"` // HQ ended the access: only HQ gives it back (IR508)
	customerOrg           uuid.UUID
}

const clientUserCols = `x.id, x.tenant_id, x.version, x.created_at, x.updated_at, x.customer_id, x.membership_id, ` +
	`x.email::text, x.display_name, x.client_role, x.status,
	(SELECT u.last_sign_in_at FROM identity.memberships m JOIN identity.users u ON u.id = m.user_id WHERE m.id = ` +
	`x.membership_id), x.allowed_channels, x.invited_at,
	x.invited_by_membership_id, (SELECT m.revoked_at FROM identity.memberships m WHERE m.id = x.membership_id), ` +
	`cu.organization_id`

const clientUserFrom = `identity.client_users x JOIN notify.ref_customers cu ON cu.id = x.customer_id`

func scanClientUser(r pgx.Row) (ClientUser, error) {
	var clientUser ClientUser

	err := r.Scan(
		&clientUser.ID,
		&clientUser.TenantID,
		&clientUser.Version,
		&clientUser.CreatedAt,
		&clientUser.UpdatedAt,
		&clientUser.CustomerID,
		&clientUser.MembershipID,
		&clientUser.Email,
		&clientUser.DisplayName,
		&clientUser.ClientRole,
		&clientUser.Status,
		&clientUser.LastSignInAt,
		&clientUser.AllowedChannels,
		&clientUser.InvitedAt,
		&clientUser.InvitedByMembershipID,
		&clientUser.RevokedAt,
		&clientUser.customerOrg,
	)

	return clientUser, apperr.Wrap(err, "scan (scanClientUser)")
}

// ownerOnly enforces the client:self-customer:owner alternative: clients must be owners (FORBIDDEN).
func ownerOnly(c *ops.Call) error {
	if c.Principal.Role == "client" && c.Principal.ClientRole != "owner" {
		return apperr.E(apperr.Forbidden, apperr.KeyForbidden)
	}

	return nil
}

// clientUsersList serves clientUsers.list.
//
//	@Summary		clientUsers.list (read)
//	@ID				clientUsers.list
//	@Description	Authorization: client:self-customer:owner | admin:asset.read
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A17, DD-C19 · Query: filters customerId,status,clientRole,search · sort \
//	@Description	id,name,email,invitedAt,createdAt,updatedAt (default name asc;id asc)
//	@Tags			clientUsers
//	@Accept			json
//	@Produce		json
//	@Param			cursor		query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit		query		integer	false	"page size 1–100, default 25"
//	@Param			sort		query		string	false	"field:direction — fields id,name,email,invitedAt,createdAt,updatedAt; default name asc;id asc"
//	@Param			customerId	query		string	false	"filter → customerId"
//	@Param			status		query		string	false	"filter → status"
//	@Param			clientRole	query		string	false	"filter → clientRole"
//	@Param			search		query		string	false	"filter → email or displayName contains (case-insensitive)"
//	@Success		200			{object}	ops.Envelope{data=ClientUserPage}
//	@Failure		401			{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403			{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404			{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409			{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422			{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429			{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503			{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504			{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/client-users [get]
func clientUsersList(
	ctx context.Context,
	call *ops.Call,
	query *paging.Query,
) (paging.Page[ClientUser], error) {
	err := ownerOnly(call)
	if err != nil {
		return paging.Page[ClientUser]{}, err
	}

	var f struct {
		CustomerID *uuid.UUID `json:"customerId,omitempty"`
		Status     *string    `json:"status,omitempty"`
		ClientRole *string    `json:"clientRole,omitempty"`
		Search     *string    `json:"search,omitempty"` // email or display name contains, case-insensitive
	}

	if len(query.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(query.Filters)))
		dec.DisallowUnknownFields()

		if dec.Decode(&f) != nil ||
			(f.Status != nil && !slices.Contains([]string{"invited", "active", "disabled"}, *f.Status)) ||
			(f.ClientRole != nil && *f.ClientRole != "owner" && *f.ClientRole != "member") {
			return paging.Page[ClientUser]{}, apperr.Fields(
				map[string]string{"filters": apperr.KeyInvalid},
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
		return paging.Page[ClientUser]{}, apperr.Wrap(err, "paging.OrderBy")
	}

	window, err := paging.Resolve(*query, f, call.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[ClientUser]{}, apperr.Wrap(err, "paging.Resolve")
	}

	var args []any

	add := func(v any) string {
		args = append(args, v)

		return fmt.Sprintf("$%d", len(args))
	}

	conds := []string{"TRUE"}
	if call.Principal.Role == "client" {
		conds = append(conds, "cu.organization_id = "+add(call.Principal.OrgID))
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
		p := add(paging.Contains(*f.Search))
		conds = append(conds, "(x.email ILIKE "+p+" OR x.display_name ILIKE "+p+")")
	}

	where := strings.Join(conds, " AND ")

	var total int

	scanErr := call.Tx.QueryRow(ctx, "SELECT count(*) FROM "+clientUserFrom+" WHERE "+where, args...).
		Scan(&total)
	if scanErr != nil {
		return paging.Page[ClientUser]{}, apperr.Wrap(scanErr, "select identity.client_users")
	}

	rows, err := call.Tx.Query(
		ctx,
		fmt.Sprintf(
			"SELECT %s FROM %s WHERE %s ORDER BY %s LIMIT %d OFFSET %d",
			clientUserCols,
			clientUserFrom,
			where,
			order,
			window.Limit,
			window.Offset,
		),
		args...)
	if err != nil {
		return paging.Page[ClientUser]{}, apperr.Wrap(err, "select (clientUsersList)")
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

	rowsErr := rows.Err()
	if rowsErr != nil {
		return paging.Page[ClientUser]{}, apperr.Wrap(rowsErr, "select (clientUsersList)")
	}

	return paging.Page[ClientUser]{
		Items:           items,
		NextCursor:      window.Next(total),
		Total:           total,
		SnapshotVersion: window.Snapshot,
	}, nil
}

func loadClientUser(
	ctx context.Context,
	call *ops.Call,
	id uuid.UUID,
	lock bool,
) (ClientUser, error) {
	q := "SELECT " + clientUserCols + " FROM " + clientUserFrom + " WHERE x.id = $1"
	if lock {
		q += " FOR UPDATE OF x"
	}

	clientUser, err := scanClientUser(call.Tx.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) ||
		(err == nil && call.Principal.Role == "client" && clientUser.customerOrg != call.Principal.OrgID) {
		return ClientUser{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	return clientUser, err
}

// ClientUserInput is clientUsers.save input.
type ClientUserInput struct {
	ID         *uuid.UUID `json:"id,omitempty"`
	CustomerID uuid.UUID  `json:"customerId"`
	Email      string     `json:"email"`
	ClientRole string     `json:"clientRole"`
	Status     *string    `json:"status,omitempty"`
	Reason     *string    `json:"reason,omitempty"`
	// DisplayName names a new user (IR461)
	DisplayName *string `json:"displayName,omitempty"`
}

// Validate implements ops.Validator.
func (in *ClientUserInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.CustomerID == uuid.Nil {
		fieldErrs["customerId"] = apperr.KeyRequired
	}

	in.Email = strings.TrimSpace(in.Email)

	a, err := mail.ParseAddress(
		in.Email,
	)
	if err != nil || a.Address != in.Email ||
		len(in.Email) > 254 {
		fieldErrs["email"] = apperr.KeyInvalid
	}

	if in.ClientRole != "owner" && in.ClientRole != "member" {
		fieldErrs["clientRole"] = apperr.KeyInvalid
	}

	if in.Status != nil && *in.Status != "active" && *in.Status != "disabled" {
		fieldErrs["status"] = apperr.KeyInvalid
	}

	if in.Reason != nil {
		*in.Reason = strings.TrimSpace(*in.Reason)
		if n := utf8.RuneCountInString(*in.Reason); n < 1 || n > 1000 {
			fieldErrs["reason"] = apperr.KeyLength
		}
	}

	if in.ID == nil { // a new user is named (IR461)
		if in.DisplayName == nil {
			fieldErrs["displayName"] = apperr.KeyRequired
		} else if *in.DisplayName = strings.TrimSpace(*in.DisplayName); utf8.RuneCountInString(*in.DisplayName) < 1 ||
			utf8.RuneCountInString(*in.DisplayName) > 120 {
			fieldErrs["displayName"] = apperr.KeyLength
		}
	} else if in.DisplayName != nil {
		fieldErrs["displayName"] = apperr.KeyNotAllowed
	}

	return fieldErrs
}

// otherActiveOwner reports whether the customer keeps another active owner besides the given client user. The
// customer's changes to its owners wait for each other here, so two at once — each demoting, disabling or removing
// the other owner — cannot both see the other still active (IR471).
func otherActiveOwner(
	ctx context.Context,
	call *ops.Call,
	customer, except uuid.UUID,
) (bool, error) {
	_, execErr := call.Tx.Exec(
		ctx,
		`SELECT pg_advisory_xact_lock(hashtext('identity.active_owners:' || $1::text))`,
		customer,
	)
	if execErr != nil {
		return false, apperr.Wrap(execErr, "select pg_advisory_xact_lock")
	}

	var ok bool

	err := call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.client_users WHERE customer_id = $1 AND id <> $2
		AND
		client_role = 'owner' AND status = 'active')`, customer, except).
		Scan(&ok)

	return ok, apperr.Wrap(err, "select identity.client_users")
}

// clientUsersSave serves clientUsers.save.
//
//	@Summary		clientUsers.save (write)
//	@ID				clientUsers.save
//	@Description	Authorization: client:self-customer:owner | admin:asset.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR111 email unique per customer and one account per person (errors.email_taken; a person this tenant \
//	@Description	removed is added again; IR462); at most 30 accounts a contractor or customer owner adds to the \
//	@Description	organization in 24 hours (RATE_LIMITED errors.too_many_accounts with retryAfterSeconds; HQ not \
//	@Description	limited; IR464); a new user needs displayName 1–120 and is created at once with a set-up e-mail \
//	@Description	(IR461); last active owner cannot be demoted or disabled (CONFLICT); an owner never changes the own \
//	@Description	account (IR417); the e-mail stays (IR422)
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A17, DD-C19
//	@Tags			clientUsers
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		ClientUserInput	true	"input"
//	@Success		200				{object}	ops.Envelope{data=ClientUser}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/client-users [post]
func clientUsersSave(ctx context.Context, call *ops.Call, in *ClientUserInput) (ClientUser, error) {
	ownerOnlyErr := ownerOnly(call)
	if ownerOnlyErr != nil {
		return ClientUser{}, ownerOnlyErr
	}

	var org uuid.UUID

	err := call.Tx.QueryRow(ctx, `SELECT organization_id FROM notify.ref_customers WHERE id = $1`, in.CustomerID).
		Scan(&org)
	if errors.Is(err, pgx.ErrNoRows) ||
		(err == nil && call.Principal.Role == "client" && org != call.Principal.OrgID) {
		return ClientUser{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return ClientUser{}, apperr.Wrap(err, "select notify.ref_customers")
	}

	var (
		dup  bool
		self uuid.UUID
	)
	if in.ID != nil {
		self = *in.ID
	}

	scanErr := call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.client_users WHERE customer_id = $1 AND email =
		$2::citext AND id <> $3)`, in.CustomerID, in.Email, self).
		Scan(&dup)
	if scanErr != nil {
		return ClientUser{}, apperr.Wrap(scanErr, "select identity.client_users")
	}

	if dup {
		return ClientUser{}, apperr.Fields(map[string]string{"email": "errors.email_taken"})
	}

	if in.ID == nil {
		// a new user: an owner (or HQ) adds members and owners, whose account is created now and gets the set-up e-mail
		// (IR417, IR461)
		if in.Status != nil {
			return ClientUser{}, apperr.Fields(map[string]string{"status": apperr.KeyNotAllowed})
		}

		role := in.ClientRole

		_, membership, err := createAccount(ctx, call, newAccount{
			Email:       in.Email,
			DisplayName: *in.DisplayName,
			Role:        "client",
			OrgID:       org,
			ClientRole:  &role,
			Permissions: defaultPermissions["client"],
			Scopes: []ScopeRef{
				{Kind: "organization", ID: org},
			},
			Employment: nil,
			ValidUntil: nil,
		})
		if err != nil {
			return ClientUser{}, err
		}

		id := uuid.New()

		_, execErr := call.Tx.Exec(
			ctx,
			`INSERT INTO identity.client_users (id, tenant_id, customer_id, membership_id, email, display_name, client_role,
				status, invited_at, invited_by_membership_id, last_invite_sent_at, created_at, updated_at)
			VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, 'invited', $7, $8, $7, $7, $7)`,
			id,
			in.CustomerID,
			membership,
			in.Email,
			*in.DisplayName,
			in.ClientRole,
			call.Now,
			call.Principal.MembershipID,
		)
		if execErr != nil {
			return ClientUser{}, apperr.Wrap(execErr, "insert identity.client_users")
		}

		v := 1
		call.Audit(
			ops.AuditEntry{
				Action:          "clientUsers.save",
				TargetKind:      "client_user",
				TargetID:        id.String(),
				NextVersion:     &v,
				Reason:          "account created",
				PreviousVersion: nil,
				Result:          "",
				Before:          nil,
				After:           nil,
			},
		)

		return loadClientUser(ctx, call, id, false)
	}

	clientUser, err := loadClientUser(ctx, call, *in.ID, true)
	if err != nil {
		return clientUser, err
	}

	if clientUser.Version != *call.ExpectedVersion {
		return clientUser, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	if call.Principal.Role == "client" && clientUser.MembershipID != nil &&
		*clientUser.MembershipID == call.Principal.MembershipID { // an owner never changes the own account (IR417)
		return clientUser, apperr.E(apperr.Conflict, "errors.own_account")
	}
	// the e-mail is the sign-in account's, created with the user (IR461): it stays, for HQ too (IR422)
	if !strings.EqualFold(in.Email, clientUser.Email) {
		return clientUser, apperr.Fields(map[string]string{"email": apperr.KeyImmutable})
	}

	in.Email = clientUser.Email // the same address in other letter case keeps the stored spelling
	if clientUser.CustomerID != in.CustomerID {
		return clientUser, apperr.Fields(map[string]string{"customerId": apperr.KeyImmutable})
	}

	if in.Reason == nil {
		return clientUser, apperr.Fields(map[string]string{"reason": apperr.KeyRequired})
	}

	status := clientUser.Status
	if in.Status != nil {
		status = *in.Status
	}

	if status == "active" && clientUser.Status == "invited" ||
		status == "active" && clientUser.Status != "active" &&
			clientUser.MembershipID == nil { // an invite becomes active only on first sign-in
		return clientUser, apperr.E(apperr.Conflict, "errors.invite_pending")
	}

	if clientUser.RevokedAt != nil &&
		status != "disabled" { // HQ ended the access: a save would open the membership again, only HQ gives it back (IR508)
		return clientUser, apperr.E(apperr.Conflict, "errors.revoked_by_hq")
	}

	if clientUser.ClientRole == "owner" && clientUser.Status == "active" &&
		(in.ClientRole != "owner" || status != "active") {
		ok, err := otherActiveOwner(ctx, call, clientUser.CustomerID, clientUser.ID)
		if err != nil {
			return clientUser, err
		}

		if !ok {
			return clientUser, apperr.E(apperr.Conflict, "errors.last_owner")
		}
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE identity.client_users SET email = $2, client_role = $3, status = $4, version = version + 1, updated_at = $5
			WHERE id = $1`,
		clientUser.ID,
		in.Email,
		in.ClientRole,
		status,
		call.Now,
	)
	if execErr != nil {
		return clientUser, apperr.Wrap(execErr, "update identity.client_users")
	}

	if clientUser.MembershipID != nil { // keep the membership in step: role and access window
		// $4 is cast: inside CASE … ELSE NULL the parameter's type would be taken as text
		// an account created this instant (IR461) ends a second after it starts, as the membership's check requires
		_, err := call.Tx.Exec(
			ctx,
			`UPDATE identity.memberships SET client_role = $2, valid_until = CASE WHEN $3 = 'disabled' THEN
				GREATEST($4::timestamptz, valid_from + interval '1 second') ELSE NULL END,
			scope_version = scope_version + 1, version = version + 1, updated_at = $4 WHERE id = $1`,
			*clientUser.MembershipID,
			in.ClientRole,
			status,
			call.Now,
		)
		if err != nil {
			return clientUser, apperr.Wrap(err, "update identity.memberships")
		}

		if status != clientUser.Status &&
			(status == "disabled" || clientUser.Status == "disabled") { // the sign-in follows (IR461)
			setAccountEnabled(call, *clientUser.MembershipID, status != "disabled")
		}
	}

	next := clientUser.Version + 1
	call.Audit(
		ops.AuditEntry{
			Action:          "clientUsers.save",
			TargetKind:      "client_user",
			TargetID:        clientUser.ID.String(),
			PreviousVersion: call.ExpectedVersion,
			NextVersion:     &next,
			Reason:          *in.Reason, Result: "", Before: nil, After: nil,
		},
	)

	return loadClientUser(ctx, call, clientUser.ID, false)
}

// RemoveInput is clientUsers.remove input.
type RemoveInput struct {
	ID     uuid.UUID `json:"id"`
	Reason string    `json:"reason"`
}

// Validate implements ops.Validator.
func (in *RemoveInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.ID == uuid.Nil {
		fieldErrs["id"] = apperr.KeyRequired
	}

	in.Reason = strings.TrimSpace(in.Reason)
	if n := utf8.RuneCountInString(in.Reason); n < 1 || n > 1000 {
		fieldErrs["reason"] = apperr.KeyLength
	}

	return fieldErrs
}

// Deleted is DeletedResource.
type Deleted struct {
	ID      uuid.UUID `json:"id"`
	Deleted bool      `json:"deleted"`
}

// clientUsersRemove serves clientUsers.remove.
//
//	@Summary		clientUsers.remove (write)
//	@ID				clientUsers.remove
//	@Description	Authorization: client:self-customer:owner | admin:asset.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR111 reason 1–1000; last active owner cannot be removed (CONFLICT); an owner never removes the own \
//	@Description	account (IR417)
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A17, DD-C19
//	@Tags			clientUsers
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string	true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer	true	"all: required (target client_user, read clientUsers.list)"
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
//	@Router			/v1/client-users/{id} [delete]
func clientUsersRemove(ctx context.Context, c *ops.Call, input *RemoveInput) (Deleted, error) {
	err := ownerOnly(c)
	if err != nil {
		return Deleted{}, err
	}

	clientUser, err := loadClientUser(ctx, c, input.ID, true)
	if err != nil {
		return Deleted{}, err
	}

	if clientUser.Version != *c.ExpectedVersion {
		return Deleted{}, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	if c.Principal.Role == "client" && clientUser.MembershipID != nil &&
		*clientUser.MembershipID == c.Principal.MembershipID { // IR417
		return Deleted{}, apperr.E(apperr.Conflict, "errors.own_account")
	}

	if clientUser.ClientRole == "owner" && clientUser.Status == "active" {
		ok, err := otherActiveOwner(ctx, c, clientUser.CustomerID, clientUser.ID)
		if err != nil {
			return Deleted{}, err
		}

		if !ok {
			return Deleted{}, apperr.E(apperr.Conflict, "errors.last_owner")
		}
	}

	_, execErr := c.Tx.Exec(
		ctx,
		`DELETE FROM identity.client_users WHERE id = $1`,
		clientUser.ID,
	)
	if execErr != nil {
		return Deleted{}, apperr.Wrap(execErr, "delete identity.client_users")
	}

	if clientUser.MembershipID != nil {
		_, err := c.Tx.Exec(
			ctx,
			`UPDATE identity.memberships SET valid_until = GREATEST($2::timestamptz, valid_from + interval '1 second'),
				scope_version = scope_version + 1, version = version + 1, updated_at = $2
			WHERE id = $1 AND (valid_until IS NULL OR valid_until > $2)`,
			*clientUser.MembershipID,
			c.Now,
		)
		if err != nil {
			return Deleted{}, apperr.Wrap(err, "update identity.memberships")
		}

		setAccountEnabled(c, *clientUser.MembershipID, false) // the sign-in ends too (IR461)
	}

	c.Audit(
		ops.AuditEntry{
			Action:          "clientUsers.remove",
			TargetKind:      "client_user",
			TargetID:        clientUser.ID.String(),
			PreviousVersion: c.ExpectedVersion,
			Reason:          input.Reason, NextVersion: nil, Result: "", Before: nil, After: nil,
		},
	)

	return Deleted{ID: clientUser.ID, Deleted: true}, nil
}

// ResendInput is clientUsers.resendInvite input.
type ResendInput struct {
	ID uuid.UUID `json:"id"`
}

// Validate implements ops.Validator.
func (in *ResendInput) Validate() map[string]string {
	if in.ID == uuid.Nil {
		return map[string]string{"id": apperr.KeyRequired}
	}

	return nil
}

// InvitePreview is the NotificationPreview returned by resendInvite: the invitation e-mail is queued for the
// notification worker (IR418; before, a preview only, IR111).
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
	// Notification.targetOpen (IR602): the invitee's page opens
	TargetOpen bool `json:"targetOpen"`
}

// clientUsersResend serves clientUsers.resendInvite.
//
//	@Summary		clientUsers.resendInvite (write)
//	@ID				clientUsers.resendInvite
//	@Description	Authorization: client:self-customer:owner | admin:asset.write
//	@Description	Validation: D01; input constraints in the corresponding DD; sends the set-up e-mail again (IR461; an \
//	@Description	invite from before IR461 queues the invitation e-mail; IR418); users not signed in yet only \
//	@Description	(CONFLICT); at most every 5 minutes per user (RATE_LIMITED retryAfterSeconds; IR423)
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A17, DD-C19
//	@Tags			clientUsers
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			id				path		string		true	"input field id"
//	@Param			request			body		ResendInput	true	"input; the path parameters come from the route"
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
//	@Router			/v1/client-users/{id}/resend-invite [post]
func clientUsersResend(
	ctx context.Context,
	call *ops.Call,
	in *ResendInput,
) (InvitePreview, error) {
	err := ownerOnly(call)
	if err != nil {
		return InvitePreview{}, err
	}

	clientUser, err := loadClientUser(ctx, call, in.ID, false)
	if err != nil {
		return InvitePreview{}, err
	}

	if clientUser.Status != "invited" {
		return InvitePreview{}, apperr.E(apperr.Conflict, "errors.invite_not_pending")
	}

	inviteResendWaitErr := inviteResendWait(ctx, call, "client_users", clientUser.ID)
	if inviteResendWaitErr != nil {
		return InvitePreview{}, inviteResendWaitErr
	}

	var user uuid.UUID // the set-up e-mail again (IR461)

	scanErr := call.Tx.QueryRow(ctx, `SELECT user_id FROM identity.memberships WHERE id = $1`, *clientUser.MembershipID).
		Scan(&user)
	if scanErr != nil {
		return InvitePreview{}, apperr.Wrap(scanErr, "select identity.memberships")
	}

	requestAccount(call, user, *clientUser.MembershipID, "client")

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE identity.client_users SET last_invite_sent_at = $2 WHERE id = $1`,
		clientUser.ID,
		call.Now,
	)
	if execErr != nil {
		return InvitePreview{}, apperr.Wrap(execErr, "update identity.client_users")
	}

	call.Audit(
		ops.AuditEntry{
			Action:          "clientUsers.resendInvite",
			TargetKind:      "client_user",
			TargetID:        clientUser.ID.String(),
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
		TenantID:    clientUser.TenantID,
		Version:     1,
		CreatedAt:   call.Now,
		UpdatedAt:   call.Now,
		TargetOpen:  true,
		Type:        "invite",
		Target:      map[string]string{"kind": "client_user", "id": clientUser.ID.String()},
		TemplateKey: "invite",
		Params: map[string]any{
			"targetName":  clientUser.Email,
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

// RegisterClientUsers binds clientUsers.*.
func RegisterClientUsers(r *ops.Registry) {
	ops.Register(r, "clientUsers.list", clientUsersList)
	ops.Register(r, "clientUsers.save", clientUsersSave)
	ops.Register(r, "clientUsers.remove", clientUsersRemove)
	ops.Register(r, "clientUsers.resendInvite", clientUsersResend)
}
