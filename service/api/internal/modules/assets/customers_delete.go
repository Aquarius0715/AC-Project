package assets

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/events"
)

// CustomerDeleteInput is customers.delete input: the customer, the reason and, for a holder of data.forceDelete, that
// the accounts of its organization do not stop the delete (IR690): the caller deletes them afterwards.
type CustomerDeleteInput struct {
	ID     uuid.UUID `json:"id"`
	Reason string    `json:"reason"`
	Force  bool      `json:"force,omitempty"`
}

// Validate implements ops.Validator.
func (in *CustomerDeleteInput) Validate() map[string]string {
	archive := ArchiveInput{ID: in.ID, Reason: in.Reason}

	return archive.Validate()
}

// customersDelete deletes a customer that has no unit and no record in any domain (DEC-103, IR654): its locations
// (properties and spaces) and unit imports go with it; a customer with units, jobs, contracts, invoices, restrictions,
// reports or accounts is CONFLICT with the reason. The billing organization stays until organizations.delete.
//
//	@Summary		customers.delete (write)
//	@ID				customers.delete
//	@Description	Authorization: admin:asset.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR654 only a customer with no unit (CONFLICT error.customerHasUnits) and no job, contract, invoice, \
//	@Description	inquiry, restriction, report or account (CONFLICT error.customerInUse); deletes its properties, \
//	@Description	spaces and unit imports; IR690 force (data.forceDelete, else FORBIDDEN \
//	@Description	errors.force_delete_not_allowed): the accounts of its organization do not stop the delete, the \
//	@Description	caller deletes them afterwards with members.delete force, every other record still stops it; the \
//	@Description	reason is recorded as [forced]
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A02
//	@Tags			customers
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string	true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer	true	"all: required (target customers, read customers.list)"
//	@Param			id					path		string	true	"input field id"
//	@Param			reason				query		string	false	"input field reason"
//	@Param			force				query		boolean	false	"input field force"
//	@Success		200					{object}	ops.Envelope{data=DeletedResource}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/customers/{id} [delete]
func (*Module) customersDelete(
	ctx context.Context,
	call *ops.Call,
	input *CustomerDeleteInput,
) (DeletedResource, error) {
	forbidden := ops.CheckForce(call, input.Force)
	if forbidden != nil {
		return DeletedResource{}, apperr.Wrap(forbidden, "ops.CheckForce")
	}

	var (
		org     uuid.UUID
		version int
	)

	err := call.Tx.QueryRow(
		ctx,
		`SELECT organization_id, version FROM assets.customers WHERE id = $1 FOR UPDATE`,
		input.ID,
	).Scan(&org, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return DeletedResource{}, notFound()
	}

	if err != nil {
		return DeletedResource{}, apperr.Wrap(err, "select assets.customers")
	}

	if call.ExpectedVersion != nil && version != *call.ExpectedVersion {
		return DeletedResource{}, conflict(apperr.KeyVersionConflict)
	}

	var units bool

	err = call.Tx.QueryRow(
		ctx,
		`SELECT EXISTS (SELECT 1 FROM assets.units WHERE customer_org_id = $1)`,
		org,
	).Scan(&units)
	if err != nil {
		return DeletedResource{}, apperr.Wrap(err, "select assets.units")
	}

	if units {
		return DeletedResource{}, conflict("error.customerHasUnits")
	}

	held, err := ops.OrgHasRecords(
		ctx,
		call,
		ops.OrgRecordsInput{OrgID: org, CustomerID: &input.ID, SkipAccounts: input.Force},
	)
	if err != nil {
		return DeletedResource{}, apperr.Wrap(err, "ops.OrgHasRecords")
	}

	if held {
		return DeletedResource{}, conflict("error.customerInUse")
	}

	err = deleteCustomerRows(ctx, call, org, input.ID)
	if err != nil {
		return DeletedResource{}, err
	}

	call.Emit(
		ops.Event{
			AggregateType: kindCustomer,
			AggregateID:   input.ID,
			Type:          events.CustomerDeleted,
			Payload:       nil,
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:          "customers.delete",
			TargetKind:      kindCustomer,
			TargetID:        input.ID.String(),
			PreviousVersion: call.ExpectedVersion,
			Reason: ops.ForcedReason(
				input.Reason,
				input.Force,
			),
			NextVersion: nil,
			Result:      "",
			Before:      nil,
			After:       nil,
		},
	)

	return DeletedResource{ID: input.ID, Deleted: true}, nil
}

// deleteCustomerRows removes the customer row and the property, space and import rows that hang off its organization.
func deleteCustomerRows(ctx context.Context, call *ops.Call, org, id uuid.UUID) error {
	for _, stmt := range []struct {
		sql string
		arg uuid.UUID
	}{
		{`DELETE FROM assets.spaces WHERE property_id IN (SELECT id FROM assets.properties WHERE customer_org_id = $1)`, org},
		{`DELETE FROM assets.properties WHERE customer_org_id = $1`, org},
		{`DELETE FROM assets.unit_import_previews WHERE customer_id = $1`, id},
		{`DELETE FROM assets.unit_imports WHERE customer_id = $1`, id},
		{`DELETE FROM assets.customers WHERE id = $1`, id},
	} {
		_, execErr := call.Tx.Exec(ctx, stmt.sql, stmt.arg)
		if execErr != nil {
			return apperr.Wrap(execErr, "delete assets.customers")
		}
	}

	return nil
}
