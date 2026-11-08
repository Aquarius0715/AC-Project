package maintenance

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

// ContractorDeleteInput is contractors.delete input.
type ContractorDeleteInput struct {
	ID     uuid.UUID `json:"id"`
	Reason string    `json:"reason"`
}

// Validate implements ops.Validator.
func (in *ContractorDeleteInput) Validate() map[string]string {
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

// DeletedResource is DeletedResource of service-contracts.ts.
type DeletedResource struct {
	ID      uuid.UUID `json:"id"`
	Deleted bool      `json:"deleted"`
}

// contractorsDelete deletes a contractor company's profile that has no record in any domain (DEC-103, IR654): its
// rate cards and unavailability go with it; a company with jobs, offers, history, certificates, payouts or accounts
// is CONFLICT error.contractorInUse. The organization stays until organizations.delete.
//
//	@Summary		contractors.delete (write)
//	@ID				contractors.delete
//	@Description	Authorization: admin:job.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR654 only a contractor company with no job, offer, history, certificate, payout or account \
//	@Description	(CONFLICT error.contractorInUse); deletes its rate cards and unavailability
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A21
//	@Tags			contractors
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string	true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer	true	"all: required (target contractor, read contractors.list)"
//	@Param			id					path		string	true	"input field id"
//	@Param			reason				query		string	false	"input field reason"
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
//	@Router			/v1/contractors/{id} [delete]
func (Partners) delete(
	ctx context.Context,
	call *ops.Call,
	input *ContractorDeleteInput,
) (DeletedResource, error) {
	var (
		org     uuid.UUID
		version int
	)

	err := call.Tx.QueryRow(
		ctx,
		`SELECT organization_id, version FROM maintenance.contractors WHERE id = $1 FOR UPDATE`,
		input.ID,
	).Scan(&org, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return DeletedResource{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return DeletedResource{}, apperr.Wrap(err, "select maintenance.contractors")
	}

	if call.ExpectedVersion != nil && version != *call.ExpectedVersion {
		return DeletedResource{}, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	held, err := ops.OrgHasRecords(ctx, call, ops.OrgRecordsInput{OrgID: org, CustomerID: nil})
	if err != nil {
		return DeletedResource{}, apperr.Wrap(err, "ops.OrgHasRecords")
	}

	if held {
		return DeletedResource{}, apperr.E(apperr.Conflict, "error.contractorInUse")
	}

	for _, stmt := range []struct {
		sql string
		arg uuid.UUID
	}{
		{`DELETE FROM maintenance.rate_cards WHERE contractor_org_id = $1`, org},
		{`DELETE FROM maintenance.unavailability WHERE organization_id = $1`, org},
		{`DELETE FROM maintenance.contractors WHERE id = $1`, input.ID},
	} {
		_, execErr := call.Tx.Exec(ctx, stmt.sql, stmt.arg)
		if execErr != nil {
			return DeletedResource{}, apperr.Wrap(execErr, "delete maintenance.contractors")
		}
	}

	call.Emit(
		outboxEvent(
			"contractor",
			input.ID,
			"ContractorDeleted",
			map[string]any{fieldOrganizationID: org},
		),
	)
	call.Audit(
		auditEntry(
			"contractors.delete",
			"contractor",
			input.ID,
			call.ExpectedVersion,
			nil,
			input.Reason,
		),
	)

	return DeletedResource{ID: input.ID, Deleted: true}, nil
}
