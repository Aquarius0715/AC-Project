package energy

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

// kindMRVReport is the aggregate and audit target kind of an MRV report.
const kindMRVReport = "mrv_report"

// DeleteInput is the input of baselines.delete, factors.delete and mrv.deleteDraft.
type DeleteInput struct {
	ID     uuid.UUID `json:"id"`
	Reason string    `json:"reason"`
}

// Validate implements ops.Validator.
func (in *DeleteInput) Validate() map[string]string {
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

// removal is one delete of an energy record: the table of its versions, the kind it is audited as, the SQL that says
// whether an MRV report still holds it, and the key of that refusal.
type removal struct {
	action  string
	kind    string
	table   string
	event   string
	citedBy string
	cited   string
}

// remove locks the current version of the record, checks the expected version and the refusals, and deletes every
// version of it.
func (r removal) remove(
	ctx context.Context,
	call *ops.Call,
	input *DeleteInput,
	refuse func(context.Context, *ops.Call) error,
) (DeletedResource, error) {
	var version int

	err := call.Tx.QueryRow(
		ctx,
		`SELECT version FROM `+r.table+` WHERE id = $1 AND is_current FOR UPDATE`,
		input.ID,
	).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return DeletedResource{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return DeletedResource{}, apperr.Wrap(err, "select "+r.table)
	}

	if call.ExpectedVersion != nil && version != *call.ExpectedVersion {
		return DeletedResource{}, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	if refuse != nil {
		refusal := refuse(ctx, call)
		if refusal != nil {
			return DeletedResource{}, refusal
		}
	}

	var held bool

	err = call.Tx.QueryRow(ctx, r.citedBy, input.ID.String()).Scan(&held)
	if err != nil {
		return DeletedResource{}, apperr.Wrap(err, "select energy.mrv_reports")
	}

	if held {
		return DeletedResource{}, apperr.E(apperr.Conflict, r.cited)
	}

	_, err = call.Tx.Exec(ctx, `DELETE FROM `+r.table+` WHERE id = $1`, input.ID)
	if err != nil {
		return DeletedResource{}, apperr.Wrap(err, "delete "+r.table)
	}

	call.Emit(ops.Event{AggregateType: r.kind, AggregateID: input.ID, Type: r.event, Payload: nil})
	call.Audit(
		ops.AuditEntry{
			Action:          r.action,
			TargetKind:      r.kind,
			TargetID:        input.ID.String(),
			PreviousVersion: call.ExpectedVersion,
			Reason:          input.Reason, NextVersion: nil, Result: "", Before: nil, After: nil,
		},
	)

	return DeletedResource{ID: input.ID, Deleted: true}, nil
}

// deleteBaseline deletes a baseline, every version, that no MRV report cites (DEC-103, IR656): CONFLICT
// error.baselineCited otherwise.
//
//	@Summary		baselines.delete (write)
//	@ID				baselines.delete
//	@Description	Authorization: admin:energy.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR656 only a baseline no MRV report cites (CONFLICT error.baselineCited); deletes every version
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A13
//	@Tags			baselines
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string	true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer	true	"all: required (target baselines, read baselines.list)"
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
//	@Router			/v1/baselines/{id} [delete]
func deleteBaseline(
	ctx context.Context,
	call *ops.Call,
	input *DeleteInput,
) (DeletedResource, error) {
	return removal{
		action:  "baselines.delete",
		kind:    "baseline",
		table:   "energy.baselines",
		event:   "BaselineDeleted",
		citedBy: `SELECT EXISTS (SELECT 1 FROM energy.mrv_reports WHERE conditions->>'baselineId' = $1)`,
		cited:   "error.baselineCited",
	}.remove(ctx, call, input, nil)
}

// deleteFactor deletes an emission factor, every version, that no MRV report cites and that is not the default factor
// energy.summary uses (DEC-103, IR656): CONFLICT error.factorCited or error.factorDefault otherwise.
//
//	@Summary		factors.delete (write)
//	@ID				factors.delete
//	@Description	Authorization: admin:mrv.factors
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR656 only a factor no MRV report cites (CONFLICT error.factorCited) and not the default factor of \
//	@Description	energy.summary (error.factorDefault); deletes every version
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A14
//	@Tags			factors
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string	true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer	true	"all: required (target factors, read factors.list)"
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
//	@Router			/v1/factors/{id} [delete]
func deleteFactor(
	ctx context.Context,
	call *ops.Call,
	input *DeleteInput,
) (DeletedResource, error) {
	return removal{
		action:  "factors.delete",
		kind:    "emission_factor",
		table:   "energy.emission_factors",
		event:   "EmissionFactorDeleted",
		citedBy: `SELECT EXISTS (SELECT 1 FROM energy.mrv_reports WHERE conditions->>'factorId' = $1)`,
		cited:   "error.factorCited",
	}.remove(ctx, call, input, func(_ context.Context, _ *ops.Call) error {
		if input.ID == DefaultFactorID() {
			return apperr.E(apperr.Conflict, "error.factorDefault")
		}

		return nil
	})
}

// deleteDraft deletes an MRV report, every version, that no review touched (DEC-103, IR656): CONFLICT
// error.reportReviewed when a version was reviewed.
//
//	@Summary		mrv.deleteDraft (write)
//	@ID				mrv.deleteDraft
//	@Description	Authorization: admin:mrv.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR656 only a report that no review touched (every version a draft; CONFLICT error.reportReviewed); \
//	@Description	deletes every version
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A14
//	@Tags			mrv
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string	true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer	true	"all: required (target mrv, read mrv.get)"
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
//	@Router			/v1/mrv/{id} [delete]
func deleteDraft(ctx context.Context, call *ops.Call, input *DeleteInput) (DeletedResource, error) {
	latest, err := latestVersion(ctx, call, input.ID)
	if err != nil {
		return DeletedResource{}, err
	}

	if call.ExpectedVersion != nil && latest != *call.ExpectedVersion {
		return DeletedResource{}, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	var reviewed bool

	err = call.Tx.QueryRow(
		ctx,
		`SELECT EXISTS (SELECT 1 FROM energy.mrv_reports WHERE id = $1 AND status <> 'draft')
		OR EXISTS (SELECT 1 FROM energy.mrv_reviews WHERE report_id = $1)`,
		input.ID,
	).Scan(&reviewed)
	if err != nil {
		return DeletedResource{}, apperr.Wrap(err, "select energy.mrv_reports")
	}

	if reviewed {
		return DeletedResource{}, apperr.E(apperr.Conflict, "error.reportReviewed")
	}

	_, err = call.Tx.Exec(ctx, `DELETE FROM energy.mrv_reports WHERE id = $1`, input.ID)
	if err != nil {
		return DeletedResource{}, apperr.Wrap(err, "delete energy.mrv_reports")
	}

	call.Emit(
		ops.Event{
			AggregateType: kindMRVReport,
			AggregateID:   input.ID,
			Type:          "MRVReportDeleted",
			Payload:       nil,
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:          "mrv.deleteDraft",
			TargetKind:      kindMRVReport,
			TargetID:        input.ID.String(),
			PreviousVersion: call.ExpectedVersion,
			Reason:          input.Reason, NextVersion: nil, Result: "", Before: nil, After: nil,
		},
	)

	return DeletedResource{ID: input.ID, Deleted: true}, nil
}
