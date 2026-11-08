package ops

import (
	"context"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// The internal queries that report whether a domain still holds records of an organization (DEC-103, IR654): a
// customer or a contractor company is deleted only when none of them does. Each domain checks its own tables and
// registers its query under the name here.
const (
	QueryOrgRecordsBilling      = "billing.orgRecords"
	QueryOrgRecordsEnergy       = "energy.orgRecords"
	QueryOrgRecordsMaintenance  = "maintenance.orgRecords"
	QueryOrgRecordsRestrictions = "restrictions.orgRecords"
	QueryOrgRecordsIdentity     = "identity.orgUsers"
)

// The internal queries that report whether an organization still has a customer or a contractor profile (DEC-103):
// the organization is deleted after its profile.
const (
	QueryOrgHasCustomer   = "assets.orgHasCustomer"
	QueryOrgHasContractor = "maintenance.orgHasContractor"
)

// QueryMembershipRecords is the internal query that reports an assignment or a certificate of a membership
// (maintenance): an account with one is not deleted (DEC-103, IR657).
const QueryMembershipRecords = "maintenance.membershipRecords"

// MembershipRecordsInput names a membership.
type MembershipRecordsInput struct {
	MembershipID uuid.UUID `json:"membershipId"`
}

// OrgRecordsInput names an organization and, for a customer, its customer ID (some records are keyed by it).
type OrgRecordsInput struct {
	OrgID      uuid.UUID  `json:"orgId"`
	CustomerID *uuid.UUID `json:"customerId"`
}

// OrgHasRecords asks every domain whether it holds records of the organization (jobs, offers, contracts, invoices,
// payouts, restrictions, reports, certificates, accounts) and is true when one does.
func OrgHasRecords(ctx context.Context, call *Call, input OrgRecordsInput) (bool, error) {
	for _, name := range []string{
		QueryOrgRecordsMaintenance,
		QueryOrgRecordsBilling,
		QueryOrgRecordsRestrictions,
		QueryOrgRecordsEnergy,
		QueryOrgRecordsIdentity,
	} {
		held, err := Query[bool](ctx, call, name, input)
		if err != nil {
			return false, apperr.Wrap(err, "ask "+name)
		}

		if held {
			return true, nil
		}
	}

	return false, nil
}
