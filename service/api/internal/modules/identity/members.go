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
func fillMembers(ctx context.Context, call *ops.Call, members []Member) error {
	if len(members) == 0 {
		return nil
	}

	ids := make([]uuid.UUID, 0, len(members))
	positions := map[uuid.UUID][]int{}

	for idx := range members {
		members[idx].Permissions, members[idx].Scopes, members[idx].Qualifications = []string{}, []ScopeRef{}, []Grant{}
		if _, seen := positions[members[idx].ID]; !seen {
			ids = append(ids, members[idx].ID)
		}

		positions[members[idx].ID] = append(positions[members[idx].ID], idx)
	}

	var memberID uuid.UUID

	each := func(sql string, scans []any, add func(x *Member)) error {
		rows, err := call.Tx.Query(ctx, sql, ids)
		if err != nil {
			return apperr.Wrap(err, "query (fillMembers)")
		}

		_, err = pgx.ForEachRow(rows, append([]any{&memberID}, scans...), func() error {
			for _, i := range positions[memberID] {
				add(&members[i])
			}

			return nil
		})

		return apperr.Wrap(err, "query (fillMembers)")
	}

	if call.Principal.Role != authz.RoleContractor {
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

// memberFilter is the filters of members.list.
type memberFilter struct {
	Role           *string    `json:"role,omitempty"`
	OrganizationID *uuid.UUID `json:"organizationId,omitempty"`
	Qualification  *string    `json:"qualification,omitempty"` // a grant of that code valid now (SR13)
	// true: validFrom <= now < validUntil; false: no validity filter
	ActiveOnly *bool `json:"activeOnly,omitempty"`
}

// memberFilterOf decodes and checks the filters of members.list.
func memberFilterOf(raw json.RawMessage) (memberFilter, error) {
	var filter memberFilter

	if len(raw) == 0 {
		return filter, nil
	}

	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()

	if dec.Decode(&filter) != nil ||
		(filter.Role != nil && !slices.Contains(memberRoles(), *filter.Role)) ||
		(filter.Qualification != nil && !slices.Contains(qualificationCodes(), *filter.Qualification)) {
		return memberFilter{}, apperr.Fields(
			map[string]string{keyFilters: apperr.KeyInvalid},
		)
	}

	return filter, nil
}

// where is the filter's WHERE clause, with its arguments.
func (filter memberFilter) where(call *ops.Call) (string, []any) {
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

	if filter.Role != nil {
		conds = append(conds, "m.role = "+add(*filter.Role))
	} else {
		conds = append(conds, "m.role <> 'client'")
	}

	if filter.OrganizationID != nil {
		conds = append(conds, "m.organization_id = "+add(*filter.OrganizationID))
	}

	if filter.ActiveOnly != nil && *filter.ActiveOnly {
		now := add(call.Now)
		conds = append(
			conds,
			"(m.valid_from <= "+now+" AND (m.valid_until IS NULL OR m.valid_until > "+now+"))",
		)
	}

	if filter.Qualification != nil {
		code, now := add(*filter.Qualification), add(call.Now)
		conds = append(
			conds,
			"EXISTS (SELECT 1 FROM identity.qualification_grants g WHERE g.membership_id = m.id AND g.code = "+code+
				" AND g.valid_from <= "+now+" AND g.valid_until > "+now+" AND (g.revoked_at IS NULL OR g.revoked_at > "+now+"))",
		)
	}

	return strings.Join(conds, " AND "), args
}

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
	filters, err := memberFilterOf(query.Filters)
	if err != nil {
		return paging.Page[Member]{}, err
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

	where, args := filters.where(call)

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

// memberRoles are the roles of a membership.
func memberRoles() []string {
	return []string{authz.RoleClient, authz.RoleContractor, authz.RoleTechnician, authz.RoleAdmin}
}

// qualificationCodes are the demo QualificationCode values (SR13).
func qualificationCodes() []string { return []string{"demo_indoor", "demo_outdoor", "demo_electrical"} }

// rolePermissions are the permissions a role may hold (IR132 item 1), and whether the role is limited to them (an
// admin may hold any of the contract's permissions).
func rolePermissions(role string) ([]string, bool) {
	switch role {
	case authz.RoleContractor:
		return []string{"partner.accept", "partner.assign", "partner.review"}, true
	case authz.RoleTechnician:
		return []string{"alert.read", "alert.resolve", "control.diagnose", "device.maintain"}, true
	case authz.RoleClient:
		return []string{}, true
	}

	return nil, false
}

// The lengths a member's texts take: an e-mail address (RFC 5321), a display name and a reason.
const (
	maxEmail       = 254
	maxDisplayName = 120
	maxReasonText  = 1000
)

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

	in.validatePerson(fieldErrs)

	if in.OrganizationID == uuid.Nil {
		fieldErrs["organizationId"] = apperr.KeyRequired
	}

	if !slices.Contains(memberRoles(), in.Role) {
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

	in.validatePermissions(fieldErrs)
	in.validateScopes(fieldErrs)

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
	if n := utf8.RuneCountInString(in.Reason); n < 1 || n > maxReasonText {
		fieldErrs[keyReason] = apperr.KeyLength
	}

	return fieldErrs
}

// validatePerson checks the person of a membership: an existing user, or a new one with an address and a display name
// (only for a new membership).
func (in *MemberInput) validatePerson(fieldErrs map[string]string) {
	if in.NewUser == nil {
		if in.UserID == uuid.Nil {
			fieldErrs["userId"] = apperr.KeyRequired
		}

		return
	}

	if in.UserID != uuid.Nil || in.ID != nil {
		fieldErrs["newUser"] = apperr.KeyNotAllowed
	}

	in.NewUser.Email, in.NewUser.DisplayName = strings.TrimSpace(
		in.NewUser.Email,
	), strings.TrimSpace(
		in.NewUser.DisplayName,
	)

	address, err := mail.ParseAddress(in.NewUser.Email)
	if err != nil || address.Address != in.NewUser.Email || len(in.NewUser.Email) > maxEmail {
		fieldErrs["newUser.email"] = apperr.KeyInvalid
	}

	if n := utf8.RuneCountInString(in.NewUser.DisplayName); n < 1 || n > maxDisplayName {
		fieldErrs["newUser.displayName"] = apperr.KeyLength
	}
}

// validatePermissions checks a membership's permissions: those of its role (any of the contract's for an admin), each
// once, and a write permission with its read one.
func (in *MemberInput) validatePermissions(fieldErrs map[string]string) {
	allowedForRole, limited := rolePermissions(in.Role)
	seen := map[string]bool{}

	for _, permission := range in.Permissions {
		allowed := authz.IsPermission(permission)
		if limited {
			allowed = slices.Contains(allowedForRole, permission)
		}

		if !allowed || seen[permission] {
			fieldErrs["permissions"] = apperr.KeyInvalid
		}

		seen[permission] = true
	}

	for permission := range seen {
		if area, ok := strings.CutSuffix(
			permission,
			".write",
		); ok &&
			authz.IsPermission(area+".read") &&
			!seen[area+".read"] {
			fieldErrs["permissions"] = "errors.write_needs_read"
		}
	}
}

// validateScopes checks a membership's scopes by role: an admin the tenant (one scope at most), a client or contractor
// an organization, a technician units, properties or organizations.
func (in *MemberInput) validateScopes(fieldErrs map[string]string) {
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
func membersSave(ctx context.Context, call *ops.Call, input *MemberInput) (Member, error) {
	newPerson, err := resolvePerson(ctx, call, input)
	if err != nil {
		return Member{}, err
	}

	err = checkMemberTarget(ctx, call, input)
	if err != nil {
		return Member{}, err
	}

	err = checkScopes(ctx, call, input)
	if err != nil {
		return Member{}, err
	}

	err = checkSelfPromotion(ctx, call, input)
	if err != nil {
		return Member{}, err
	}

	var (
		change membershipChange
		prev   *int
	)

	if input.ID == nil {
		change, err = createMembership(ctx, call, input, newPerson)
	} else {
		change, err = updateMembership(ctx, call, input)
		prev = call.ExpectedVersion
	}

	if err != nil {
		return Member{}, err
	}

	memberID := change.id

	before, err := memberBefore(ctx, call, memberID, prev)
	if err != nil {
		return Member{}, err
	}

	err = replaceGrants(ctx, call, memberID, input)
	if err != nil {
		return Member{}, err
	}

	if prev != nil {
		err = bumpScopeVersion(ctx, call, memberID, input, before)
		if err != nil {
			return Member{}, err
		}
	}

	saved, err := reloadMember(ctx, call, memberID)
	if err != nil {
		return Member{}, err
	}

	call.Emit(
		ops.Event{
			AggregateType: kindMembership,
			AggregateID:   memberID,
			Type:          "MembershipSaved",
			Payload:       map[string]any{keyRole: input.Role},
		},
	)

	if change.account {
		requestAccount(call, input.UserID, memberID, input.Role)
	}

	if change.revoke || change.restore {
		setAccountEnabled(call, memberID, change.restore)
	}

	call.Audit(
		ops.AuditEntry{
			Action:          "members.save",
			TargetKind:      kindMembership,
			TargetID:        memberID.String(),
			PreviousVersion: prev,
			NextVersion:     &saved.Version,
			Reason:          input.Reason, Result: "", Before: nil, After: nil,
		},
	)

	return saved, nil
}

// reloadMember reads a saved membership back with its permissions, scopes and qualifications.
func reloadMember(ctx context.Context, call *ops.Call, memberID uuid.UUID) (Member, error) {
	saved, err := scanMember(
		call.Tx.QueryRow(
			ctx,
			"SELECT "+memberCols+" FROM identity.memberships m WHERE m.id = $1",
			memberID,
		),
	)
	if err != nil {
		return saved, err
	}

	memberList := []Member{saved}

	err = LoadMembers(ctx, call, memberList)

	return memberList[0], err
}

// resolvePerson finds or creates the person of a membership given as a new user (IR461, IR462), sets input.UserID, and
// reports whether the person is new to this save.
func resolvePerson(ctx context.Context, call *ops.Call, input *MemberInput) (bool, error) {
	if input.NewUser == nil {
		return false, nil
	}

	var err error

	input.UserID, err = personFor(ctx, call, input.NewUser.Email, input.NewUser.DisplayName)
	if errors.Is(err, errEmailTaken) {
		return true, apperr.Fields(map[string]string{"newUser.email": keyEmailTaken})
	}

	return true, err
}

// checkMemberTarget checks the person and organization of a membership: both exist, and the organization is of the
// kind the role needs (HQ for admins and internal technicians, a customer for clients, a contractor for contractors and
// external technicians).
func checkMemberTarget(ctx context.Context, call *ops.Call, input *MemberInput) error {
	var exists bool

	scanErr := call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.users WHERE id = $1)`, input.UserID).
		Scan(&exists)
	if scanErr != nil {
		return apperr.Wrap(scanErr, "select identity.users")
	}

	var kind string

	err := call.Tx.QueryRow(ctx, `SELECT kind FROM identity.organizations WHERE id = $1`, input.OrganizationID).
		Scan(&kind)
	if !exists || errors.Is(err, pgx.ErrNoRows) {
		return apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return apperr.Wrap(err, "select identity.organizations")
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
		return apperr.Fields(
			map[string]string{"organizationId": "errors.organization_kind"},
		)
	}

	return nil
}

// checkScopes checks the scope references of a membership: the tenant itself, a customer organization (or the
// membership's own; a technician may also take customers), or a known property or unit.
func checkScopes(ctx context.Context, call *ops.Call, input *MemberInput) error {
	for _, scopeRef := range input.Scopes {
		var (
			valid bool
			err   error
		)

		switch scopeRef.Kind {
		case "tenant":
			valid = scopeRef.ID == call.Principal.TenantID
		case scopeOrganization:
			err = call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.organizations WHERE id = $1 AND (kind = 'customer'
				OR
				id = $2))`, scopeRef.ID, input.OrganizationID).
				Scan(&valid)
			if input.Role != authz.RoleTechnician {
				valid = valid && scopeRef.ID == input.OrganizationID
			}
		case "property":
			err = call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM notify.ref_properties WHERE id = $1)`, scopeRef.ID).
				Scan(&valid)
		case "unit":
			err = call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM notify.ref_units WHERE id = $1)`, scopeRef.ID).
				Scan(&valid)
		}

		if err != nil {
			return apperr.Wrap(err, "select notify.ref_units")
		}

		if !valid {
			return apperr.Fields(map[string]string{"scopes": "errors.unknown_scope"})
		}
	}

	return nil
}

// checkSelfPromotion refuses a user granting themselves identity.write or restriction.override they did not hold
// already (DD-A03).
func checkSelfPromotion(ctx context.Context, call *ops.Call, input *MemberInput) error {
	if input.UserID != call.Principal.UserID {
		return nil
	}

	for _, permission := range []string{"identity.write", "restriction.override"} {
		if !slices.Contains(input.Permissions, permission) {
			continue
		}

		had, err := heldBefore(ctx, call, input.ID, permission)
		if err != nil {
			return err
		}

		if !had {
			return apperr.E(apperr.Forbidden, "errors.self_promotion")
		}
	}

	return nil
}

// heldBefore is whether the membership being saved held a permission already; a new membership held none.
func heldBefore(
	ctx context.Context,
	call *ops.Call,
	membership *uuid.UUID,
	permission string,
) (bool, error) {
	if membership == nil {
		return false, nil
	}

	var had bool

	err := call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.membership_permissions
		WHERE membership_id = $1 AND permission = $2)`, *membership, permission).
		Scan(&had)

	return had, apperr.Wrap(err, "select identity.membership_permissions")
}

// membershipChange is what saving a membership changed: its ID, whether its sign-in account is opened or pointed at it
// (IR461, IR462), and whether HQ turned its sign-in off or on with the access (IR508).
type membershipChange struct {
	id                       uuid.UUID
	account, revoke, restore bool
}

// createMembership stores a new membership; a new person, or an existing one whose memberships here have all ended,
// signs in with it from now on (IR462), while one whose sign-in belongs to another tenant keeps it. The membership is
// then added to the customer's or company's user list.
func createMembership(
	ctx context.Context,
	call *ops.Call,
	input *MemberInput,
	newPerson bool,
) (membershipChange, error) {
	var (
		memberID uuid.UUID
		account  bool
	)

	var clientRole *string

	if input.Role == authz.RoleClient { // HQ adds a member; a customer owner adds owners in the customer app (IR417)
		cr := clientRoleMember
		clientRole = &cr
	}

	err := call.Tx.QueryRow(ctx, `INSERT INTO identity.memberships (tenant_id, user_id, organization_id, role, employment,
		client_role, valid_from, valid_until, created_at, updated_at)
		VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $5, $6, $7, $8, $8) RETURNING id`,
		input.UserID, input.OrganizationID, input.Role, input.Employment, clientRole, input.ValidFrom, input.ValidUntil,
		call.Now).
		Scan(&memberID)
	if err != nil {
		return membershipChange{}, apperr.Wrap(err, "insert identity.memberships")
	}
	// an existing person whose memberships here have all ended signs in with this one from now on (IR462); one
	// whose sign-in belongs to another tenant keeps it
	account = newPerson
	if !newPerson {
		err := call.Tx.QueryRow(ctx, `SELECT (u.sign_in_membership_id IS NULL OR EXISTS (SELECT 1 FROM identity.memberships s
			WHERE s.id = u.sign_in_membership_id))
			AND NOT EXISTS (SELECT 1 FROM identity.memberships m WHERE m.user_id = u.id AND m.id <> $2 AND (m.valid_until IS
			NULL OR m.valid_until > $3))
			FROM identity.users u WHERE u.id = $1`, input.UserID, memberID, call.Now).
			Scan(&account)
		if err != nil {
			return membershipChange{}, apperr.Wrap(err, "select identity.users")
		}
	}

	listed := statusActive // the status the customer's or contractor's user list shows: invited until the first sign-in
	if account {
		listed = statusInvited
	}

	err = addToUserLists(ctx, call, input, memberID, clientRole, listed)
	if err != nil {
		return membershipChange{}, err
	}

	return membershipChange{id: memberID, account: account, revoke: false, restore: false}, nil
}

// addToUserLists gives a new client membership its initial consent (IR84) and a row in the customer's user list
// (IR144), and a new contractor or technician membership a row in the company's (FR-P11); listed is the status the
// row shows, invited until the first sign-in when an account is opened (IR461). An address already on a list keeps its
// row.
func addToUserLists(
	ctx context.Context,
	call *ops.Call,
	input *MemberInput,
	memberID uuid.UUID,
	clientRole *string,
	listed string,
) error {
	if input.Role == authz.RoleClient { // IR84 initial Consent
		_, err := call.Tx.Exec(
			ctx,
			`INSERT INTO identity.consents (tenant_id, membership_id, purpose, granted) VALUES
				(current_setting('app.tenant_id')::uuid, $1, 'location_automation', false)`,
			memberID,
		)
		if err != nil {
			return apperr.Wrap(err, "insert identity.consents")
		}
		// IR144: the customer's user list includes the new member, listed as not signed in yet when the person was
		// created with it (IR461); an address already on the list keeps its row
		_, err = call.Tx.Exec(
			ctx,
			`INSERT INTO identity.client_users (tenant_id, customer_id, membership_id, email, display_name, client_role, status,
				invited_at, invited_by_membership_id, created_at, updated_at)
			SELECT current_setting('app.tenant_id')::uuid, cu.id, $1, u.email, u.display_name, $6, $7, $3, $4, $3, $3
			FROM notify.ref_customers cu, identity.users u WHERE cu.organization_id = $2 AND u.id = $5
			ON CONFLICT (tenant_id, customer_id, email) DO NOTHING`,
			memberID,
			input.OrganizationID,
			call.Now,
			call.Principal.MembershipID,
			input.UserID,
			*clientRole,
			listed,
		)
		if err != nil {
			return apperr.Wrap(err, "insert identity.client_users")
		}
	}
	// FR-P11: the contractor company's user list includes the new account, listed as not signed in yet when the
	// person was created with it (IR461); an address already on the list keeps its row
	if input.Role == authz.RoleContractor || input.Role == authz.RoleTechnician {
		_, err := call.Tx.Exec(
			ctx,
			`INSERT INTO identity.partner_users (tenant_id, organization_id, membership_id, email, display_name, partner_role,
				status, invited_at, invited_by_membership_id, created_at, updated_at)
			SELECT current_setting('app.tenant_id')::uuid, o.id, $1, u.email, u.display_name, $5, $7, $3, $4, $3, $3
			FROM identity.organizations o, identity.users u WHERE o.id = $2 AND o.kind = 'contractor' AND u.id = $6
			ON CONFLICT (tenant_id, organization_id, email) DO NOTHING`,
			memberID,
			input.OrganizationID,
			call.Now,
			call.Principal.MembershipID,
			input.Role,
			input.UserID,
			listed,
		)
		if err != nil {
			return apperr.Wrap(err, "insert identity.partner_users")
		}
	}

	return nil
}

// updateMembership stores a new version of a membership at the version shown, whose person, organization and role
// stay as they were. The last active identity.write holder keeps the permission (IR472); an account its company or
// customer disabled keeps its end (IR416, IR417, IR420); and HQ's new end decides the sign-in (IR508).
func updateMembership(
	ctx context.Context,
	call *ops.Call,
	input *MemberInput,
) (membershipChange, error) {
	member, err := scanMember(
		call.Tx.QueryRow(
			ctx,
			"SELECT "+memberCols+" FROM identity.memberships m WHERE m.id = $1 FOR UPDATE",
			*input.ID,
		),
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return membershipChange{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return membershipChange{}, err
	}

	if member.UserID != input.UserID || member.OrganizationID != input.OrganizationID ||
		member.Role != input.Role {
		return membershipChange{}, apperr.Fields(
			map[string]string{keyRole: "errors.membership_fixed"},
		)
	}

	if member.Version != *call.ExpectedVersion {
		return membershipChange{}, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	err = checkLastIdentityAdmin(ctx, call, input, member)
	if err != nil {
		return membershipChange{}, err
	}

	until, disabled, err := keptEnd(ctx, call, input, member)
	if err != nil {
		return membershipChange{}, err
	}

	revoked, revoke, restore := revocation(input, member, disabled, call.Now)

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE identity.memberships SET employment = $2, valid_from = $3, valid_until = $4, revoked_at = $6, version =
			version + 1, updated_at = $5 WHERE id = $1`,
		member.ID,
		input.Employment,
		input.ValidFrom,
		until,
		call.Now,
		revoked,
	)
	if execErr != nil {
		return membershipChange{}, apperr.Wrap(execErr, "update identity.memberships")
	}

	return membershipChange{id: member.ID, account: false, revoke: revoke, restore: restore}, nil
}

// checkLastIdentityAdmin keeps the tenant's last active identity.write holder: a membership that loses the permission
// or stops being valid now (ended, or its start moved later) is refused while it holds the permission and no other
// active membership does. The tenant's changes to holders wait for each other here, so two at once, each taking the
// permission from or ending the other of the last two, cannot both see the other still active (IR472).
func checkLastIdentityAdmin(
	ctx context.Context,
	call *ops.Call,
	input *MemberInput,
	member Member,
) error {
	ending := (input.ValidUntil != nil && !input.ValidUntil.After(call.Now)) ||
		input.ValidFrom.After(call.Now)
	if !slices.Contains(input.Permissions, "identity.write") || ending {
		_, err := call.Tx.Exec(
			ctx,
			`SELECT pg_advisory_xact_lock(hashtext('identity.identity_admins:' || $1::text))`,
			call.Principal.TenantID,
		)
		if err != nil {
			return apperr.Wrap(err, "select pg_advisory_xact_lock")
		}

		var (
			holds  bool
			others int
		)

		err = call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.membership_permissions WHERE membership_id = $1 AND
			permission = 'identity.write')`, member.ID).
			Scan(&holds)
		if err != nil {
			return apperr.Wrap(err, "select identity.membership_permissions")
		}

		err = call.Tx.QueryRow(ctx, `SELECT count(*) FROM identity.memberships m JOIN identity.membership_permissions p ON
			p.membership_id = m.id AND p.permission = 'identity.write'
			WHERE m.id <> $1 AND m.valid_from <= $2 AND (m.valid_until IS NULL OR m.valid_until > $2)`, member.ID, call.Now).
			Scan(&others)
		if err != nil {
			return apperr.Wrap(err, "select identity.memberships")
		}

		if holds && others == 0 {
			return apperr.E(apperr.Conflict, "errors.last_identity_admin")
		}
	}

	return nil
}

// keptEnd is the end a membership gets, and whether its company or customer disabled the account. A disabled account
// (IR416, IR417) stays closed: the membership keeps its end, and a contractor account keeps HQ's new end for when it is
// enabled again (IR420).
func keptEnd(
	ctx context.Context,
	call *ops.Call,
	input *MemberInput,
	member Member,
) (*time.Time, bool, error) {
	until := input.ValidUntil

	var disabled bool

	scanErr := call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.partner_users WHERE membership_id = $1 AND
		status
		= 'disabled')
		OR EXISTS (SELECT 1 FROM identity.client_users WHERE membership_id = $1 AND status = 'disabled')`, member.ID).
		Scan(&disabled)
	if scanErr != nil {
		return nil, false, apperr.Wrap(scanErr, "select identity.partner_users")
	}

	if disabled && member.ValidUntil != nil &&
		(until == nil || until.After(*member.ValidUntil)) {
		_, err := call.Tx.Exec(
			ctx,
			`UPDATE identity.partner_users SET access_until = $2, version = version + 1, updated_at = $3 WHERE membership_id =
				$1`,
			member.ID,
			until,
			call.Now,
		)
		if err != nil {
			return nil, false, apperr.Wrap(err, "update identity.partner_users")
		}

		until = member.ValidUntil
	}

	return until, disabled, nil
}

// revocation is the membership's revocation time after the save, and whether HQ turns the sign-in off or on (IR508,
// DEC-83). A new end at or before now, on an open membership or on one its company or customer disabled, turns the
// sign-in off, and only HQ gives the access back; the first revoke keeps its time. A later end, or none, clears it and
// turns the sign-in on, unless the company or the customer disabled the account. HQ's form sends whole minutes, so an
// end it did not move (an access that ended by itself, or the time the company disabled the account) is no revoke when
// HQ saves other changes (IR514).
func revocation(
	input *MemberInput,
	member Member,
	disabled bool,
	now time.Time,
) (*time.Time, bool, bool) {
	revoked := member.RevokedAt
	moved := input.ValidUntil != nil &&
		(member.ValidUntil == nil || !input.ValidUntil.Truncate(time.Minute).Equal(member.ValidUntil.Truncate(time.Minute)))

	switch {
	case moved && !input.ValidUntil.After(now):
		if revoked == nil {
			return &now, true, false
		}
	case member.RevokedAt != nil && (input.ValidUntil == nil || input.ValidUntil.After(now)):
		return nil, false, !disabled
	}

	return revoked, false, false
}

// memberBefore is the membership as it was before an update (prev is the version the update replaced), for the scope
// version check; a new membership has none.
func memberBefore(
	ctx context.Context,
	call *ops.Call,
	memberID uuid.UUID,
	prev *int,
) (Member, error) {
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

		err := LoadMembers(ctx, call, beforeList)
		if err != nil {
			return before, err
		}

		before = beforeList[0]
	}

	return before, nil
}

// replaceGrants replaces the membership's permissions and scopes with the input's.
func replaceGrants(
	ctx context.Context,
	call *ops.Call,
	memberID uuid.UUID,
	input *MemberInput,
) error {
	_, execErr := call.Tx.Exec(
		ctx,
		`DELETE FROM identity.membership_permissions WHERE membership_id = $1`,
		memberID,
	)
	if execErr != nil {
		return apperr.Wrap(execErr, "delete identity.membership_permissions")
	}

	for _, permission := range input.Permissions {
		_, err := call.Tx.Exec(
			ctx,
			`INSERT INTO identity.membership_permissions (tenant_id, membership_id, permission) VALUES
				(current_setting('app.tenant_id')::uuid, $1, $2)`,
			memberID,
			permission,
		)
		if err != nil {
			return apperr.Wrap(err, "insert identity.membership_permissions")
		}
	}

	_, execErr = call.Tx.Exec(
		ctx,
		`DELETE FROM identity.membership_scopes WHERE membership_id = $1`,
		memberID,
	)
	if execErr != nil {
		return apperr.Wrap(execErr, "delete identity.membership_scopes")
	}

	for _, scopeRef := range input.Scopes {
		_, err := call.Tx.Exec(
			ctx,
			`INSERT INTO identity.membership_scopes (tenant_id, membership_id, kind, ref_id) VALUES
				(current_setting('app.tenant_id')::uuid, $1, $2, $3)
			ON CONFLICT DO NOTHING`,
			memberID,
			scopeRef.Kind,
			scopeRef.ID,
		)
		if err != nil {
			return apperr.Wrap(err, "insert identity.membership_scopes")
		}
	}

	return nil
}

// bumpScopeVersion moves the membership's scope version on when an update changed its permissions or scopes, so the
// sessions and cursors scoped to the old ones end.
func bumpScopeVersion(
	ctx context.Context,
	call *ops.Call,
	memberID uuid.UUID,
	input *MemberInput,
	before Member,
) error {
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
		_, err := call.Tx.Exec(
			ctx,
			`UPDATE identity.memberships SET scope_version = scope_version + 1 WHERE id = $1`,
			memberID,
		)
		if err != nil {
			return apperr.Wrap(err, "update identity.memberships")
		}
	}

	return nil
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
// time, so the demo scenario clock would not compare (IR384).
func wallClock() time.Time { return time.Now().UTC() }

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
	ops.Register(r, "members.delete", membersDelete)
	ops.Register(r, "members.revokeSessions", membersRevokeSessions)
}
