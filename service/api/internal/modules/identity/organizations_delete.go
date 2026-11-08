package identity

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// OrganizationDeleteInput is organizations.delete input.
type OrganizationDeleteInput struct {
	ID     uuid.UUID `json:"id"`
	Reason string    `json:"reason"`
}

// Validate implements ops.Validator.
func (in *OrganizationDeleteInput) Validate() map[string]string {
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

// organizationsDelete deletes a customer or contractor organization that has lost its profile and has no account
// (DEC-103, IR654): customers.delete or contractors.delete comes first (error.orgHasProfile), accounts are removed
// first (error.orgHasUsers), and an operator organization is never deleted (error.orgOperator).
//
//	@Summary		organizations.delete (write)
//	@ID				organizations.delete
//	@Description	Authorization: admin:asset.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR654 only a customer or contractor organization without a profile (CONFLICT error.orgHasProfile) \
//	@Description	and without an account (error.orgHasUsers); an operator organization is never deleted \
//	@Description	(error.orgOperator)
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A02, DD-A21
//	@Tags			organizations
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string	true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer	true	"all: required (target organizations, read organizations.list)"
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
//	@Router			/v1/organizations/{id} [delete]
func organizationsDelete(
	ctx context.Context,
	call *ops.Call,
	input *OrganizationDeleteInput,
) (Deleted, error) {
	var (
		kind    string
		version int
	)

	err := call.Tx.QueryRow(
		ctx,
		`SELECT kind, version FROM identity.organizations WHERE id = $1 FOR UPDATE`,
		input.ID,
	).Scan(&kind, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return Deleted{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return Deleted{}, apperr.Wrap(err, "select identity.organizations")
	}

	if call.ExpectedVersion != nil && version != *call.ExpectedVersion {
		return Deleted{}, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	refusal := refuseOrganizationDelete(ctx, call, input.ID, kind)
	if refusal != nil {
		return Deleted{}, refusal
	}

	_, err = call.Tx.Exec(ctx, `DELETE FROM identity.organizations WHERE id = $1`, input.ID)
	if err != nil {
		return Deleted{}, apperr.Wrap(err, "delete identity.organizations")
	}

	call.Emit(
		ops.Event{
			AggregateType: scopeOrganization,
			AggregateID:   input.ID,
			Type:          "OrganizationDeleted",
			Payload:       nil,
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:          "organizations.delete",
			TargetKind:      scopeOrganization,
			TargetID:        input.ID.String(),
			PreviousVersion: call.ExpectedVersion,
			Reason:          input.Reason, NextVersion: nil, Result: "", Before: nil, After: nil,
		},
	)

	return Deleted{ID: input.ID, Deleted: true}, nil
}

// refuseOrganizationDelete is the CONFLICT for an organization that is not deleted yet, or nil.
func refuseOrganizationDelete(
	ctx context.Context,
	call *ops.Call,
	org uuid.UUID,
	kind string,
) error {
	if kind == "operator" {
		return apperr.E(apperr.Conflict, "error.orgOperator")
	}

	profile := ops.QueryOrgHasCustomer
	if kind == "contractor" {
		profile = ops.QueryOrgHasContractor
	}

	has, err := ops.Query[bool](
		ctx,
		call,
		profile,
		ops.OrgRecordsInput{OrgID: org, CustomerID: nil},
	)
	if err != nil {
		return apperr.Wrap(err, "ask "+profile)
	}

	if has {
		return apperr.E(apperr.Conflict, "error.orgHasProfile")
	}

	var users bool

	err = call.Tx.QueryRow(
		ctx,
		`SELECT EXISTS (SELECT 1 FROM identity.memberships WHERE organization_id = $1)
		OR EXISTS (SELECT 1 FROM identity.partner_users WHERE organization_id = $1)`,
		org,
	).Scan(&users)
	if err != nil {
		return apperr.Wrap(err, "select identity.memberships")
	}

	if users {
		return apperr.E(apperr.Conflict, "error.orgHasUsers")
	}

	return nil
}
