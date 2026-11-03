package identity

// The values the identity module compares and writes by name: statuses, kinds, field and payload keys.
const (
	appClientID        = "ac-web"
	clientRoleMember   = "member"
	employmentExternal = "external"
	everyRow           = "TRUE"
	keyCreatedAt       = "createdAt"
	keyEmail           = "email"
	keyEmailTaken      = "errors.email_taken"
	keyFilters         = "filters"
	keyKind            = "kind"
	keyMembershipID    = "membershipId"
	keyReason          = "reason"
	keyRole            = "role"
	keyStatus          = "status"
	keyUpdatedAt       = "updatedAt"
	kindMembership     = "membership"
	noticeInvite       = "invite"
	orgKindContractor  = "contractor"
	orgKindOperator    = "operator"
	scopeOrganization  = "organization"
	statusActive       = "active"
	statusInvited      = "invited"
	targetClientUser   = "client_user"
	targetPartnerUser  = "partner_user"
)

// The values this package compares and writes by name.
const (
	clientRoleOwner    = "owner"
	employmentInternal = "internal"
	orgKindCustomer    = "customer"
	statusDisabled     = "disabled"
)
