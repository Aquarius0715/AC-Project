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

// ScopeRef is ScopeRef of service-contracts.ts.
type ScopeRef struct {
	Kind string    `json:"kind"`
	ID   uuid.UUID `json:"id"`
}

// Grant is QualificationGrant.
type Grant struct {
	Code       string     `json:"code"`
	ValidFrom  time.Time  `json:"validFrom"`
	ValidUntil time.Time  `json:"validUntil"`
	RevokedAt  *time.Time `json:"revokedAt"`
}

// Member is Membership of service-contracts.ts.
type Member struct {
	ID             uuid.UUID  `json:"id"`
	TenantID       uuid.UUID  `json:"tenantId"`
	Version        int        `json:"version"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
	UserID         uuid.UUID  `json:"userId"`
	DisplayName    string     `json:"displayName"` // the user's display name (IR172)
	OrganizationID uuid.UUID  `json:"organizationId"`
	Role           string     `json:"role"`
	Employment     *string    `json:"employment"`
	Permissions    []string   `json:"permissions"`
	Scopes         []ScopeRef `json:"scopes"`
	ScopeVersion   int        `json:"scopeVersion"`
	Qualifications []Grant    `json:"qualifications"`
	ValidFrom      time.Time  `json:"validFrom"`
	ValidUntil     *time.Time `json:"validUntil"`
	ClientRole     *string    `json:"clientRole"`
	RevokedAt      *time.Time `json:"revokedAt"` // HQ ended the access: the sign-in is off, only HQ gives it back (IR508)
}

const memberCols = `m.id, m.tenant_id, m.version, m.created_at, m.updated_at, m.user_id, m.organization_id, m.role, ` +
	`m.employment, m.scope_version, m.valid_from,
	m.valid_until, m.client_role, m.revoked_at, COALESCE((SELECT u.display_name FROM identity.users u WHERE u.id = ` +
	`m.user_id), '')`

func scanMember(r pgx.Row) (Member, error) {
	var member Member

	err := r.Scan(
		&member.ID,
		&member.TenantID,
		&member.Version,
		&member.CreatedAt,
		&member.UpdatedAt,
		&member.UserID,
		&member.OrganizationID,
		&member.Role,
		&member.Employment,
		&member.ScopeVersion,
		&member.ValidFrom,
		&member.ValidUntil,
		&member.ClientRole,
		&member.RevokedAt,
		&member.DisplayName,
	)

	return member, apperr.Wrap(err, "scan (scanMember)")
}

// QueryLoadMembers completes members for other domains (IR194: maintenance's technician lists).
const QueryLoadMembers = "identity.loadMembers"

// LoadMembersInput is QueryLoadMembers input.
type LoadMembersInput struct {
	Members []Member `json:"members"`
}

// LoadMembers fills permissions, scopes and qualifications (IR42: contractors get no permissions or scopes) and a
// missing display name (IR172); callers in other domains ask identity-api.
func LoadMembers(ctx context.Context, call *ops.Call, memberList []Member) error {
	if len(memberList) == 0 {
		return nil
	}

	out, err := ops.Delegate(
		ctx,
		call,
		QueryLoadMembers,
		LoadMembersInput{Members: memberList},
		loadMembers,
	)
	if err != nil {
		return apperr.Wrap(err, "delegate identity.loadMembers")
	}

	copy(memberList, out)

	return nil
}

func loadMembers(ctx context.Context, call *ops.Call, in *LoadMembersInput) ([]Member, error) {
	memberList := in.Members
	for i := range memberList {
		if memberList[i].DisplayName == "" {
			err := call.Tx.QueryRow(ctx, `SELECT COALESCE((SELECT display_name FROM identity.users WHERE id = $1),
				'')`, memberList[i].UserID).
				Scan(&memberList[i].DisplayName)
			if err != nil {
				return nil, apperr.Wrap(err, "select identity.users")
			}
		}
	}

	return memberList, fillMembers(ctx, call, memberList)
}

// fillMembers loads the permissions, scopes and qualification grants of the members with one query per table for all
// of them (IR496: a query per member and table made a page of 100 members 300 round trips).
func fillMembers(ctx context.Context, c *ops.Call, ms []Member) error {
	if len(ms) == 0 {
		return nil
	}

	ids := make([]uuid.UUID, 0, len(ms))
	positions := map[uuid.UUID][]int{}

	for idx := range ms {
		ms[idx].Permissions, ms[idx].Scopes, ms[idx].Qualifications = []string{}, []ScopeRef{}, []Grant{}
		if _, seen := positions[ms[idx].ID]; !seen {
			ids = append(ids, ms[idx].ID)
		}

		positions[ms[idx].ID] = append(positions[ms[idx].ID], idx)
	}

	var memberID uuid.UUID

	each := func(sql string, scans []any, add func(x *Member)) error {
		rows, err := c.Tx.Query(ctx, sql, ids)
		if err != nil {
			return apperr.Wrap(err, "query (fillMembers)")
		}

		_, err = pgx.ForEachRow(rows, append([]any{&memberID}, scans...), func() error {
			for _, i := range positions[memberID] {
				add(&ms[i])
			}

			return nil
		})

		return apperr.Wrap(err, "query (fillMembers)")
	}

	if c.Principal.Role != authz.RoleContractor {
		var permission string

		err := each(
			`SELECT membership_id, permission FROM identity.membership_permissions WHERE membership_id = ANY($1) ORDER BY
				membership_id, permission`,
			[]any{&permission},
			func(x *Member) { x.Permissions = append(x.Permissions, permission) },
		)
		if err != nil {
			return err
		}

		var scopeRef ScopeRef

		err = each(
			`SELECT membership_id, kind, ref_id FROM identity.membership_scopes WHERE membership_id = ANY($1) ORDER BY
				membership_id, kind, ref_id`,
			[]any{&scopeRef.Kind, &scopeRef.ID},
			func(x *Member) { x.Scopes = append(x.Scopes, scopeRef) },
		)
		if err != nil {
			return err
		}
	}

	var scannedGrant Grant

	return each(
		`SELECT membership_id, code, valid_from, valid_until, revoked_at FROM identity.qualification_grants WHERE
			membership_id = ANY($1) ORDER BY membership_id, code`,
		[]any{
			&scannedGrant.Code,
			&scannedGrant.ValidFrom,
			&scannedGrant.ValidUntil,
			&scannedGrant.RevokedAt,
		},
		func(member *Member) {
			grant := scannedGrant
			if scannedGrant.RevokedAt != nil {
				// the scan target is shared by every row: each grant keeps its own revocation time
				at := *scannedGrant.RevokedAt
				grant.RevokedAt = &at
			}

			member.Qualifications = append(member.Qualifications, grant)
		},
	)
}

// ---- members.list ----

// membersList serves members.list.
//
//	@Summary		members.list (read)
//	@ID				members.list
//	@Description	Authorization: admin:identity.read | admin:job.read | contractor:partner.assign:own-company
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot; IR94 contractor \
//	@Description	sees own-company technicians only
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A03, DD-P06, DD-P05, DD-P01, DD-P02, DD-P03, DD-P07, DD-A06 · Query: filters \
//	@Description	role,organizationId,qualification,activeOnly · sort id,validFrom,createdAt,updatedAt (default id \
//	@Description	asc)
//	@Tags			members
//	@Accept			json
//	@Produce		json
//	@Param			cursor			query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit			query		integer	false	"page size 1–100, default 25"
//	@Param			sort			query		string	false	"field:direction — fields id,validFrom,createdAt,updatedAt; default id asc"
//	@Param			role			query		string	false	"filter → Membership.role (omitted: every role except client)"
//	@Param			organizationId	query		string	false	"filter → Membership.organizationId"
//	@Param			qualification	query		string	false	"filter → matching valid Membership.qualifications[].code (SR13)"
//	@Param			activeOnly		query		boolean	false	"filter → true => validFrom<=now<validUntil (null end infinite)"
//	@Success		200				{object}	ops.Envelope{data=MemberPage}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/members [get]
func membersList(
	ctx context.Context,
	call *ops.Call,
	query *paging.Query,
) (paging.Page[Member], error) {
	var filters struct {
		Role           *string    `json:"role,omitempty"`
		OrganizationID *uuid.UUID `json:"organizationId,omitempty"`
		Qualification  *string    `json:"qualification,omitempty"` // a grant of that code valid now (SR13)
		// true: validFrom <= now < validUntil; false: no validity filter
		ActiveOnly *bool `json:"activeOnly,omitempty"`
	}

	if len(query.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(query.Filters)))
		dec.DisallowUnknownFields()

		if dec.Decode(&filters) != nil ||
			(filters.Role != nil && !slices.Contains(roles, *filters.Role)) ||
			(filters.Qualification != nil && !slices.Contains(qualificationCodes, *filters.Qualification)) {
			return paging.Page[Member]{}, apperr.Fields(
				map[string]string{keyFilters: apperr.KeyInvalid},
			)
		}
	}

	order, err := paging.OrderBy(
		query.Sort,
		map[string]string{
			"id":         "m.id",
			"validFrom":  "m.valid_from",
			keyCreatedAt: "m.created_at",
			keyUpdatedAt: "m.updated_at",
		},
		"m.id ASC",
	)
	if err != nil {
		return paging.Page[Member]{}, apperr.Wrap(err, "paging.OrderBy")
	}

	window, err := paging.Resolve(*query, filters, call.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Member]{}, apperr.Wrap(err, "paging.Resolve")
	}

	var args []any

	add := func(v any) string {
		args = append(args, v)

		return fmt.Sprintf("$%d", len(args))
	}

	conds := []string{everyRow}
	if call.Principal.Role == authz.RoleContractor {
		conds = append(
			conds,
			"m.role = 'technician' AND m.organization_id = "+add(call.Principal.OrgID),
		)
	}

	if filters.Role != nil {
		conds = append(conds, "m.role = "+add(*filters.Role))
	} else {
		conds = append(conds, "m.role <> 'client'")
	}

	if filters.OrganizationID != nil {
		conds = append(conds, "m.organization_id = "+add(*filters.OrganizationID))
	}

	if filters.ActiveOnly != nil && *filters.ActiveOnly {
		now := add(call.Now)
		conds = append(
			conds,
			"(m.valid_from <= "+now+" AND (m.valid_until IS NULL OR m.valid_until > "+now+"))",
		)
	}

	if filters.Qualification != nil {
		code, now := add(*filters.Qualification), add(call.Now)
		conds = append(
			conds,
			"EXISTS (SELECT 1 FROM identity.qualification_grants g WHERE g.membership_id = m.id AND g.code = "+code+
				" AND g.valid_from <= "+now+" AND g.valid_until > "+now+" AND (g.revoked_at IS NULL OR g.revoked_at > "+now+"))",
		)
	}

	where := strings.Join(conds, " AND ")

	var total int

	scanErr := call.Tx.QueryRow(ctx, "SELECT count(*) FROM identity.memberships m WHERE "+where, args...).
		Scan(&total)
	if scanErr != nil {
		return paging.Page[Member]{}, apperr.Wrap(scanErr, "select identity.memberships")
	}

	rows, err := call.Tx.Query(
		ctx,
		fmt.Sprintf(
			"SELECT %s FROM identity.memberships m WHERE %s ORDER BY %s LIMIT %d OFFSET %d",
			memberCols,
			where,
			order,
			window.Limit,
			window.Offset,
		),
		args...)
	if err != nil {
		return paging.Page[Member]{}, apperr.Wrap(err, "select identity.memberships")
	}

	items := []Member{}

	for rows.Next() {
		member, err := scanMember(rows)
		if err != nil {
			rows.Close()

			return paging.Page[Member]{}, err
		}

		items = append(items, member)
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return paging.Page[Member]{}, apperr.Wrap(rowsErr, "select identity.memberships")
	}

	loadMembersErr := LoadMembers(ctx, call, items)
	if loadMembersErr != nil {
		return paging.Page[Member]{}, loadMembersErr
	}

	return paging.Page[Member]{
		Items:           items,
		NextCursor:      window.Next(total),
		Total:           total,
		SnapshotVersion: window.Snapshot,
	}, nil
}

// ---- members.save ----

var roles = []string{authz.RoleClient, authz.RoleContractor, authz.RoleTechnician, authz.RoleAdmin}

// qualificationCodes are the demo QualificationCode values (SR13).
var qualificationCodes = []string{"demo_indoor", "demo_outdoor", "demo_electrical"}

// RolePermissions are the permissions each role may hold (IR132 item 1).
var RolePermissions = map[string][]string{
	authz.RoleContractor: {"partner.accept", "partner.assign", "partner.review"},
	authz.RoleTechnician: {"alert.read", "alert.resolve", "control.diagnose", "device.maintain"},
	authz.RoleClient:     {},
}

// MemberInput is members.save input. The display name is the user's own (IR172): a Membership result carries it, the
// input does not (IR391).
type MemberInput struct {
	ID             *uuid.UUID `json:"id,omitempty"`
	UserID         uuid.UUID  `json:"userId"`
	OrganizationID uuid.UUID  `json:"organizationId"`
	Role           string     `json:"role"`
	Employment     *string    `json:"employment"`
	Permissions    []string   `json:"permissions"`
	Scopes         []ScopeRef `json:"scopes"`
	ValidFrom      time.Time  `json:"validFrom"`
	ValidUntil     *time.Time `json:"validUntil"`
	Reason         string     `json:"reason"`
	// NewUser creates the person with the membership instead of choosing an existing userId; the account then gets
	// the set-up e-mail (IR461)
	NewUser *NewUserInput `json:"newUser,omitempty"`
}

// NewUserInput names the person members.save creates (IR461).
type NewUserInput struct {
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
}

// Validate implements ops.Validator for the lookup-free rules.
func (in *MemberInput) Validate() map[string]string {
	fieldErrs := map[string]string{}

	if in.NewUser != nil {
		if in.UserID != uuid.Nil || in.ID != nil {
			fieldErrs["newUser"] = apperr.KeyNotAllowed
		}

		in.NewUser.Email, in.NewUser.DisplayName = strings.TrimSpace(
			in.NewUser.Email,
		), strings.TrimSpace(
			in.NewUser.DisplayName,
		)

		a, err := mail.ParseAddress(
			in.NewUser.Email,
		)
		if err != nil || a.Address != in.NewUser.Email ||
			len(in.NewUser.Email) > 254 {
			fieldErrs["newUser.email"] = apperr.KeyInvalid
		}

		if n := utf8.RuneCountInString(in.NewUser.DisplayName); n < 1 || n > 120 {
			fieldErrs["newUser.displayName"] = apperr.KeyLength
		}
	} else if in.UserID == uuid.Nil {
		fieldErrs["userId"] = apperr.KeyRequired
	}

	if in.OrganizationID == uuid.Nil {
		fieldErrs["organizationId"] = apperr.KeyRequired
	}

	if !slices.Contains(roles, in.Role) {
		fieldErrs[keyRole] = apperr.KeyInvalid
	}

	switch {
	case in.Role == authz.RoleTechnician &&
		(in.Employment == nil || (*in.Employment != employmentInternal && *in.Employment != employmentExternal)):
		fieldErrs["employment"] = apperr.KeyRequired
	case in.Role != authz.RoleTechnician && in.Employment != nil:
		fieldErrs["employment"] = apperr.KeyNotAllowed
	}

	if in.Permissions == nil || in.Scopes == nil {
		fieldErrs["permissions"] = apperr.KeyRequired
	}

	seen := map[string]bool{}

	for _, permission := range in.Permissions {
		allowed := authz.IsPermission(permission)
		if list, ok := RolePermissions[in.Role]; ok {
			allowed = slices.Contains(list, permission)
		}

		if !allowed || seen[permission] {
			fieldErrs["permissions"] = apperr.KeyInvalid
		}

		seen[permission] = true
	}

	for p := range seen {
		if r, ok := strings.CutSuffix(
			p,
			".write",
		); ok && authz.IsPermission(r+".read") &&
			!seen[r+".read"] {
			fieldErrs["permissions"] = "errors.write_needs_read"
		}
	}

	kinds := map[string][]string{
		authz.RoleAdmin: {"tenant"}, authz.RoleClient: {scopeOrganization}, authz.RoleContractor: {scopeOrganization},
		authz.RoleTechnician: {"unit", "property", scopeOrganization},
	}[in.Role]
	for _, sc := range in.Scopes {
		if !slices.Contains(kinds, sc.Kind) {
			fieldErrs["scopes"] = apperr.KeyInvalid
		}
	}

	if in.Role == authz.RoleAdmin && len(in.Scopes) > 1 {
		fieldErrs["scopes"] = apperr.KeyInvalid
	}

	if in.ValidFrom.IsZero() {
		fieldErrs["validFrom"] = apperr.KeyRequired
	}

	if in.ValidUntil != nil && !in.ValidFrom.Before(*in.ValidUntil) {
		fieldErrs["validUntil"] = apperr.KeyRange
	}

	if in.Role == authz.RoleTechnician && in.Employment != nil &&
		*in.Employment == employmentExternal &&
		in.ValidUntil == nil {
		fieldErrs["validUntil"] = "errors.external_needs_end"
	}

	in.Reason = strings.TrimSpace(in.Reason)
	if n := utf8.RuneCountInString(in.Reason); n < 1 || n > 1000 {
		fieldErrs[keyReason] = apperr.KeyLength
	}

	return fieldErrs
}

// membersSave serves members.save.
//
//	@Summary		members.save (write)
//	@ID				members.save
//	@Description	Authorization: admin:identity.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR84 new client membership creates initial Consent granted=false; newUser creates the person with \
//	@Description	the membership instead of userId and sends a set-up e-mail (IR461; a person this tenant removed is \
//	@Description	taken back; for an existing user whose memberships here have all ended the sign-in account is \
//	@Description	pointed at the new one; IR462); the last active identity.write holder keeps the permission and stays \
//	@Description	valid — no end now and no later start (CONFLICT errors.last_identity_admin; IR472)
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A03
//	@Tags			members
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		MemberInput	true	"input"
//	@Success		200				{object}	ops.Envelope{data=Member}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/members [post]
func membersSave(ctx context.Context, c *ops.Call, input *MemberInput) (Member, error) {
	newPerson := input.NewUser != nil
	if newPerson { // the person is created with the membership, or taken back if this tenant removed them (IR461, IR462)
		var err error

		input.UserID, err = personFor(ctx, c, input.NewUser.Email, input.NewUser.DisplayName)
		if errors.Is(err, errEmailTaken) {
			return Member{}, apperr.Fields(map[string]string{"newUser.email": keyEmailTaken})
		}

		if err != nil {
			return Member{}, err
		}
	}

	var exists bool

	scanErr := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.users WHERE id = $1)`, input.UserID).
		Scan(&exists)
	if scanErr != nil {
		return Member{}, apperr.Wrap(scanErr, "select identity.users")
	}

	var kind string

	err := c.Tx.QueryRow(ctx, `SELECT kind FROM identity.organizations WHERE id = $1`, input.OrganizationID).
		Scan(&kind)
	if !exists || errors.Is(err, pgx.ErrNoRows) {
		return Member{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return Member{}, apperr.Wrap(err, "select identity.organizations")
	}

	want := map[string]string{
		authz.RoleAdmin:      orgKindOperator,
		authz.RoleClient:     orgKindCustomer,
		authz.RoleContractor: orgKindContractor,
	}[input.Role]
	if input.Role == authz.RoleTechnician {
		want = map[string]string{
			employmentInternal: orgKindOperator,
			employmentExternal: orgKindContractor,
		}[*input.Employment]
	}

	if kind != want {
		return Member{}, apperr.Fields(
			map[string]string{"organizationId": "errors.organization_kind"},
		)
	}
	// scope references
	for _, scopeRef := range input.Scopes {
		var valid bool

		switch scopeRef.Kind {
		case "tenant":
			valid = scopeRef.ID == c.Principal.TenantID
		case scopeOrganization:
			err = c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.organizations WHERE id = $1 AND (kind = 'customer' OR
				id = $2))`, scopeRef.ID, input.OrganizationID).
				Scan(&valid)
			if input.Role != authz.RoleTechnician {
				valid = valid && scopeRef.ID == input.OrganizationID
			}
		case "property":
			err = c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM notify.ref_properties WHERE id = $1)`, scopeRef.ID).
				Scan(&valid)
		case "unit":
			err = c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM notify.ref_units WHERE id = $1)`, scopeRef.ID).
				Scan(&valid)
		}

		if err != nil {
			return Member{}, apperr.Wrap(err, "select notify.ref_units")
		}

		if !valid {
			return Member{}, apperr.Fields(map[string]string{"scopes": "errors.unknown_scope"})
		}
	}
	// self promotion
	if input.UserID == c.Principal.UserID { // granting identity.write / restriction.override to oneself (DD-A03)
		for _, permission := range []string{"identity.write", "restriction.override"} {
			if !slices.Contains(input.Permissions, permission) {
				continue
			}

			had := false
			if input.ID != nil {
				err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.membership_permissions WHERE membership_id = $1 AND
					permission = $2)`, *input.ID, permission).
					Scan(&had)
				if err != nil {
					return Member{}, apperr.Wrap(err, "select identity.membership_permissions")
				}
			}

			if !had {
				return Member{}, apperr.E(apperr.Forbidden, "errors.self_promotion")
			}
		}
	}

	var (
		memberID uuid.UUID
		prev     *int
	)

	account := false // the sign-in account is opened or pointed at this new membership (IR461, IR462)
	// HQ turns the sign-in off with the access, or on when it gives the access back (IR508)
	revoke, restore := false, false

	if input.ID == nil {
		var clientRole *string

		if input.Role == authz.RoleClient { // HQ adds a member; a customer owner adds owners in the customer app (IR417)
			cr := clientRoleMember
			clientRole = &cr
		}

		err := c.Tx.QueryRow(ctx, `INSERT INTO identity.memberships (tenant_id, user_id, organization_id, role, employment,
			client_role, valid_from, valid_until, created_at, updated_at)
			VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $5, $6, $7, $8, $8) RETURNING id`,
			input.UserID, input.OrganizationID, input.Role, input.Employment, clientRole, input.ValidFrom, input.ValidUntil,
			c.Now).
			Scan(&memberID)
		if err != nil {
			return Member{}, apperr.Wrap(err, "insert identity.memberships")
		}
		// an existing person whose memberships here have all ended signs in with this one from now on (IR462); one
		// whose sign-in belongs to another tenant keeps it
		account = newPerson
		if !newPerson {
			err := c.Tx.QueryRow(ctx, `SELECT (u.sign_in_membership_id IS NULL OR EXISTS (SELECT 1 FROM identity.memberships s
				WHERE s.id = u.sign_in_membership_id))
				AND NOT EXISTS (SELECT 1 FROM identity.memberships m WHERE m.user_id = u.id AND m.id <> $2 AND (m.valid_until IS
				NULL OR m.valid_until > $3))
				FROM identity.users u WHERE u.id = $1`, input.UserID, memberID, c.Now).
				Scan(&account)
			if err != nil {
				return Member{}, apperr.Wrap(err, "select identity.users")
			}
		}

		listed := statusActive // the status the customer's or contractor's user list shows: invited until the first sign-in
		if account {
			listed = statusInvited
		}

		if input.Role == authz.RoleClient { // IR84 initial Consent
			_, err := c.Tx.Exec(
				ctx,
				`INSERT INTO identity.consents (tenant_id, membership_id, purpose, granted) VALUES
					(current_setting('app.tenant_id')::uuid, $1, 'location_automation', false)`,
				memberID,
			)
			if err != nil {
				return Member{}, apperr.Wrap(err, "insert identity.consents")
			}
			// IR144: the customer's user list includes the new member, listed as not signed in yet when the person was
			// created with it (IR461); an address already on the list keeps its row
			_, err = c.Tx.Exec(
				ctx,
				`INSERT INTO identity.client_users (tenant_id, customer_id, membership_id, email, display_name, client_role, status,
					invited_at, invited_by_membership_id, created_at, updated_at)
				SELECT current_setting('app.tenant_id')::uuid, cu.id, $1, u.email, u.display_name, $6, $7, $3, $4, $3, $3
				FROM notify.ref_customers cu, identity.users u WHERE cu.organization_id = $2 AND u.id = $5
				ON CONFLICT (tenant_id, customer_id, email) DO NOTHING`,
				memberID,
				input.OrganizationID,
				c.Now,
				c.Principal.MembershipID,
				input.UserID,
				*clientRole,
				listed,
			)
			if err != nil {
				return Member{}, apperr.Wrap(err, "insert identity.client_users")
			}
		}
		// FR-P11: the contractor company's user list includes the new account, listed as not signed in yet when the
		// person was created with it (IR461); an address already on the list keeps its row
		if input.Role == authz.RoleContractor || input.Role == authz.RoleTechnician {
			_, err := c.Tx.Exec(
				ctx,
				`INSERT INTO identity.partner_users (tenant_id, organization_id, membership_id, email, display_name, partner_role,
					status, invited_at, invited_by_membership_id, created_at, updated_at)
				SELECT current_setting('app.tenant_id')::uuid, o.id, $1, u.email, u.display_name, $5, $7, $3, $4, $3, $3
				FROM identity.organizations o, identity.users u WHERE o.id = $2 AND o.kind = 'contractor' AND u.id = $6
				ON CONFLICT (tenant_id, organization_id, email) DO NOTHING`,
				memberID,
				input.OrganizationID,
				c.Now,
				c.Principal.MembershipID,
				input.Role,
				input.UserID,
				listed,
			)
			if err != nil {
				return Member{}, apperr.Wrap(err, "insert identity.partner_users")
			}
		}
	} else {
		member, err := scanMember(
			c.Tx.QueryRow(
				ctx,
				"SELECT "+memberCols+" FROM identity.memberships m WHERE m.id = $1 FOR UPDATE",
				*input.ID,
			),
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return Member{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
		}

		if err != nil {
			return Member{}, err
		}

		if member.UserID != input.UserID || member.OrganizationID != input.OrganizationID ||
			member.Role != input.Role {
			return Member{}, apperr.Fields(
				map[string]string{keyRole: "errors.membership_fixed"},
			)
		}

		if member.Version != *c.ExpectedVersion {
			return Member{}, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
		}
		// last active identity.write holder: a membership that loses the permission or stops being valid now (ended, or
		// its start moved later). The tenant's changes to holders wait for each other here, so two at once — each taking
		// the permission from, or ending, the other of the last two — cannot both see the other still active (IR472).
		ending := (input.ValidUntil != nil && !input.ValidUntil.After(c.Now)) ||
			input.ValidFrom.After(c.Now)
		if !slices.Contains(input.Permissions, "identity.write") || ending {
			_, err := c.Tx.Exec(
				ctx,
				`SELECT pg_advisory_xact_lock(hashtext('identity.identity_admins:' || $1::text))`,
				c.Principal.TenantID,
			)
			if err != nil {
				return Member{}, apperr.Wrap(err, "select pg_advisory_xact_lock")
			}

			var (
				holds  bool
				others int
			)

			err = c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.membership_permissions WHERE membership_id = $1 AND
				permission = 'identity.write')`, member.ID).
				Scan(&holds)
			if err != nil {
				return Member{}, apperr.Wrap(err, "select identity.membership_permissions")
			}

			err = c.Tx.QueryRow(ctx, `SELECT count(*) FROM identity.memberships m JOIN identity.membership_permissions p ON
				p.membership_id = m.id AND p.permission = 'identity.write'
				WHERE m.id <> $1 AND m.valid_from <= $2 AND (m.valid_until IS NULL OR m.valid_until > $2)`, member.ID, c.Now).
				Scan(&others)
			if err != nil {
				return Member{}, apperr.Wrap(err, "select identity.memberships")
			}

			if holds && others == 0 {
				return Member{}, apperr.E(apperr.Conflict, "errors.last_identity_admin")
			}
		}
		// an account its company or customer disabled (IR416, IR417) stays closed: the membership keeps its end, and a
		// contractor account keeps HQ's new end for when it is enabled again (IR420)
		until := input.ValidUntil

		var disabled bool

		scanErr := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.partner_users WHERE membership_id = $1 AND status
			= 'disabled')
			OR EXISTS (SELECT 1 FROM identity.client_users WHERE membership_id = $1 AND status = 'disabled')`, member.ID).
			Scan(&disabled)
		if scanErr != nil {
			return Member{}, apperr.Wrap(scanErr, "select identity.partner_users")
		}

		if disabled && member.ValidUntil != nil &&
			(until == nil || until.After(*member.ValidUntil)) {
			_, err := c.Tx.Exec(
				ctx,
				`UPDATE identity.partner_users SET access_until = $2, version = version + 1, updated_at = $3 WHERE membership_id =
					$1`,
				member.ID,
				until,
				c.Now,
			)
			if err != nil {
				return Member{}, apperr.Wrap(err, "update identity.partner_users")
			}

			until = member.ValidUntil
		}
		// HQ's revoke (IR508, DEC-83): a new end at or before now — on an open membership, or on one its company or
		// customer disabled — turns the sign-in off, and only HQ gives the access back. The first revoke keeps its time.
		// A later end (or none) clears it and turns the sign-in on, unless the company or the customer disabled it.
		revoked := member.RevokedAt
		// HQ's form sends whole minutes: an end it did not move (an access that ended by itself, or the time the company
		// disabled the account) is no revoke when HQ saves other changes (IR514)
		moved := input.ValidUntil != nil &&
			(member.ValidUntil == nil || !input.ValidUntil.Truncate(time.Minute).Equal(member.ValidUntil.Truncate(time.Minute)))
		switch {
		case moved && !input.ValidUntil.After(c.Now):
			if revoked == nil {
				revoked, revoke = &c.Now, true
			}
		case member.RevokedAt != nil && (input.ValidUntil == nil || input.ValidUntil.After(c.Now)):
			revoked, restore = nil, !disabled
		}

		_, execErr := c.Tx.Exec(
			ctx,
			`UPDATE identity.memberships SET employment = $2, valid_from = $3, valid_until = $4, revoked_at = $6, version =
				version + 1, updated_at = $5 WHERE id = $1`,
			member.ID,
			input.Employment,
			input.ValidFrom,
			until,
			c.Now,
			revoked,
		)
		if execErr != nil {
			return Member{}, apperr.Wrap(execErr, "update identity.memberships")
		}

		memberID, prev = member.ID, c.ExpectedVersion
	}

	before := Member{
		ID:             memberID,
		TenantID:       uuid.UUID{},
		Version:        0,
		CreatedAt:      time.Time{},
		UpdatedAt:      time.Time{},
		UserID:         uuid.UUID{},
		DisplayName:    "",
		OrganizationID: uuid.UUID{},
		Role:           "",
		Employment:     nil,
		Permissions:    nil,
		Scopes:         nil,
		ScopeVersion:   0,
		Qualifications: nil,
		ValidFrom:      time.Time{},
		ValidUntil:     nil,
		ClientRole:     nil,
		RevokedAt:      nil,
	}
	if prev != nil {
		beforeList := []Member{before}

		err := LoadMembers(ctx, c, beforeList)
		if err != nil {
			return Member{}, err
		}

		before = beforeList[0]
	}

	_, execErr := c.Tx.Exec(
		ctx,
		`DELETE FROM identity.membership_permissions WHERE membership_id = $1`,
		memberID,
	)
	if execErr != nil {
		return Member{}, apperr.Wrap(execErr, "delete identity.membership_permissions")
	}

	for _, permission := range input.Permissions {
		_, err := c.Tx.Exec(
			ctx,
			`INSERT INTO identity.membership_permissions (tenant_id, membership_id, permission) VALUES
				(current_setting('app.tenant_id')::uuid, $1, $2)`,
			memberID,
			permission,
		)
		if err != nil {
			return Member{}, apperr.Wrap(err, "insert identity.membership_permissions")
		}
	}

	_, execErr = c.Tx.Exec(
		ctx,
		`DELETE FROM identity.membership_scopes WHERE membership_id = $1`,
		memberID,
	)
	if execErr != nil {
		return Member{}, apperr.Wrap(execErr, "delete identity.membership_scopes")
	}

	for _, scopeRef := range input.Scopes {
		_, err := c.Tx.Exec(
			ctx,
			`INSERT INTO identity.membership_scopes (tenant_id, membership_id, kind, ref_id) VALUES
				(current_setting('app.tenant_id')::uuid, $1, $2, $3)
			ON CONFLICT DO NOTHING`,
			memberID,
			scopeRef.Kind,
			scopeRef.ID,
		)
		if err != nil {
			return Member{}, apperr.Wrap(err, "insert identity.membership_scopes")
		}
	}

	if prev != nil {
		perms := slices.Clone(input.Permissions)
		slices.Sort(perms)

		changed := !slices.Equal(perms, before.Permissions) ||
			len(input.Scopes) != len(before.Scopes)
		if !changed {
			for _, sc := range input.Scopes {
				changed = changed || !slices.Contains(before.Scopes, sc)
			}
		}

		if changed {
			_, err := c.Tx.Exec(
				ctx,
				`UPDATE identity.memberships SET scope_version = scope_version + 1 WHERE id = $1`,
				memberID,
			)
			if err != nil {
				return Member{}, apperr.Wrap(err, "update identity.memberships")
			}
		}
	}

	saved, err := scanMember(
		c.Tx.QueryRow(
			ctx,
			"SELECT "+memberCols+" FROM identity.memberships m WHERE m.id = $1",
			memberID,
		),
	)
	if err != nil {
		return saved, err
	}

	memberList := []Member{saved}

	loadMembersErr := LoadMembers(ctx, c, memberList)
	if loadMembersErr != nil {
		return saved, loadMembersErr
	}

	c.Emit(
		ops.Event{
			AggregateType: kindMembership,
			AggregateID:   memberID,
			Type:          "MembershipSaved",
			Payload:       map[string]any{keyRole: input.Role},
		},
	)

	if account {
		requestAccount(c, input.UserID, memberID, input.Role)
	}

	if revoke || restore {
		setAccountEnabled(c, memberID, restore)
	}

	c.Audit(
		ops.AuditEntry{
			Action:          "members.save",
			TargetKind:      kindMembership,
			TargetID:        memberID.String(),
			PreviousVersion: prev,
			NextVersion:     &memberList[0].Version,
			Reason:          input.Reason, Result: "", Before: nil, After: nil,
		},
	)

	return memberList[0], nil
}

// RevokeSessionsInput is members.revokeSessions input (IR384).
type RevokeSessionsInput struct {
	MembershipID uuid.UUID `json:"membershipId"`
	Reason       string    `json:"reason"`
}

// Validate implements ops.Validator: a membership and a reason of 1–1000 characters.
func (in *RevokeSessionsInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.MembershipID == uuid.Nil {
		fieldErrs[keyMembershipID] = apperr.KeyRequired
	}

	in.Reason = strings.TrimSpace(in.Reason)
	if n := utf8.RuneCountInString(in.Reason); n < 1 || n > 1000 {
		fieldErrs[keyReason] = apperr.KeyLength
	}

	return fieldErrs
}

// SessionsRevoked is SessionsRevoked of service-contracts.ts.
type SessionsRevoked struct {
	MembershipID  uuid.UUID   `json:"membershipId"`
	UserID        uuid.UUID   `json:"userId"`
	RevokedAt     time.Time   `json:"revokedAt"`
	MembershipIDs []uuid.UUID `json:"membershipIds"`
}

// wallClock is the time a revocation starts from: tokens carry the identity provider's auth_time, which is wall-clock
// time, so the demo scenario clock would not compare (IR384). A variable, so tests can move it.
var wallClock = func() time.Time { return time.Now().UTC() }

// membersRevokeSessions signs the membership's user out everywhere in this tenant (IR384, Figma Admin 03-8): every
// membership of the user records the revocation time, and a token signed in before it is refused by every service.
// Access itself stays; the user signs in again. An HQ user with asset.write may do it for a client membership (the
// customer's Users tab), any other membership needs identity.write (Access & roles).
//
//	@Summary		members.revokeSessions (write)
//	@ID				members.revokeSessions
//	@Description	Authorization: admin:identity.write | admin:asset.write:client-membership
//	@Description	Validation: D01; input constraints in the corresponding DD; IR384 reason 1–1000; every membership of \
//	@Description	the target's user in the tenant records sessionsRevokedAt (wall clock); a token signed in before it \
//	@Description	is UNAUTHENTICATED errors.session_revoked; a client membership needs asset.write, any other \
//	@Description	identity.write
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A03, DD-A17
//	@Tags			members
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string				true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			membershipId	path		string				true	"input field membershipId"
//	@Param			request			body		RevokeSessionsInput	true	"input; the path parameters come from the route"
//	@Success		200				{object}	ops.Envelope{data=SessionsRevoked}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/members/{membershipId}/revoke-sessions [post]
func membersRevokeSessions(
	ctx context.Context,
	call *ops.Call,
	input *RevokeSessionsInput,
) (SessionsRevoked, error) {
	var (
		user uuid.UUID
		role string
	)

	err := call.Tx.QueryRow(ctx, `SELECT user_id, role FROM identity.memberships WHERE id = $1`, input.MembershipID).
		Scan(&user, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		return SessionsRevoked{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return SessionsRevoked{}, apperr.Wrap(err, "select identity.memberships")
	}

	requireErr := call.Require(authz.Facts{"client-membership": role == authz.RoleClient})
	if requireErr != nil {
		return SessionsRevoked{}, apperr.Wrap(requireErr, "ops.Call.Require")
	}

	revokedAt := wallClock()

	rows, err := call.Tx.Query(
		ctx,
		`UPDATE identity.memberships SET sessions_revoked_at = $2 WHERE user_id = $1 RETURNING id`,
		user,
		revokedAt,
	)
	if err != nil {
		return SessionsRevoked{}, apperr.Wrap(err, "update identity.memberships")
	}

	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return SessionsRevoked{}, apperr.Wrap(err, "update identity.memberships")
	}

	slices.SortFunc(
		ids,
		func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) },
	)

	when := revokedAt.Format(time.RFC3339)
	call.Audit(ops.AuditEntry{
		Action:     "members.revokeSessions",
		TargetKind: kindMembership,
		TargetID:   input.MembershipID.String(),
		Reason:     input.Reason,
		After: map[string]*string{
			"sessionsRevokedAt": &when,
		},
		PreviousVersion: nil,
		NextVersion:     nil,
		Result:          "",
		Before:          nil,
	})
	call.Emit(
		ops.Event{
			AggregateType: kindMembership,
			AggregateID:   input.MembershipID,
			Type:          EventSessionsRevoked,
			Payload:       map[string]any{"userId": user, "revokedAt": revokedAt},
		},
	) // the identity provider's sessions end too (IR386)

	return SessionsRevoked{
		MembershipID:  input.MembershipID,
		UserID:        user,
		RevokedAt:     revokedAt,
		MembershipIDs: ids,
	}, nil
}

// RegisterMembers binds members.list / members.save / members.revokeSessions.
func RegisterMembers(r *ops.Registry) {
	ops.Register(r, "members.list", membersList)
	ops.Register(r, "members.save", membersSave)
	ops.Register(r, "members.revokeSessions", membersRevokeSessions)
}
