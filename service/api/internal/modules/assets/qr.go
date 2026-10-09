package assets

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// QrInput is units.resolveQr input.
type QrInput struct {
	Code string `json:"code"`
}

// Validate implements ops.Validator.
func (in *QrInput) Validate() map[string]string {
	in.Code = strings.TrimSpace(in.Code)
	if in.Code == "" || len(in.Code) > 200 {
		return map[string]string{"code": "error.invalid"}
	}
	return nil
}

// QrResolution is QrResolution of service-contracts.ts.
type QrResolution struct {
	UnitID uuid.UUID  `json:"unitId"`
	JobID  *uuid.UUID `json:"jobId"`
}

// QrAssignments reports whether the calling technician holds an active Assignment on the unit and the open job to
// open first (Maintenance, IR194).
type QrAssignments interface {
	QrAssignment(ctx context.Context, c *ops.Call, unit uuid.UUID) (assigned bool, job *uuid.UUID, err error)
}

// RegisterQr binds units.resolveQr.
func RegisterQr(r *ops.Registry, a QrAssignments) {
	q := qrOps{a}
	ops.Register(r, "units.resolveQr", q.resolve)
}

// qrOps binds units.resolveQr to the assignment source.
type qrOps struct{ a QrAssignments }

// @Summary		units.resolveQr (read)
// @ID				units.resolveQr
// @Description	Authorization: technician:assigned
// @Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
// @Description	Recovery: D04: retry only UNAVAILABLE, at most twice
// @Description	Design: DD-T13
// @Tags			units
// @Accept			json
// @Produce		json
// @Param			code	path		string	true	"input field code"
// @Success		200		{object}	ops.Envelope{data=QrResolution}
// @Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504		{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/units/qr/{code} [get]
func (q qrOps) resolve(ctx context.Context, c *ops.Call, in *QrInput) (QrResolution, error) {
	return resolveQr(ctx, c, in, q.a)
}

// resolveQr maps a label code to a unit (IR145): the unit ID, `ac-unit:<unit ID>` or the bound device serial
// (case-insensitive). Units without an active Assignment of the technician are NOT_FOUND (IR111); jobId is the
// technician's open job on the unit whose scheduled window ends first after now, or null.
func resolveQr(ctx context.Context, c *ops.Call, in *QrInput, a QrAssignments) (QrResolution, error) {
	code := in.Code
	if len(code) > 8 && strings.EqualFold(code[:8], "ac-unit:") {
		code = code[8:]
	}
	var unit uuid.UUID
	if id, err := uuid.Parse(code); err == nil {
		unit = id
	} else if err := c.Tx.QueryRow(ctx, `SELECT unit_id FROM devices.devices WHERE upper(btrim(serial)) = upper($1) AND unit_id IS NOT NULL`, in.Code).Scan(&unit); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return QrResolution{}, err
	}
	nf := apperr.E(apperr.NotFound, "error.notFound")
	if unit == uuid.Nil {
		return QrResolution{}, nf
	}
	assigned, job, err := a.QrAssignment(ctx, c, unit)
	if err != nil {
		return QrResolution{}, err
	}
	if !assigned {
		return QrResolution{}, nf
	}
	return QrResolution{UnitID: unit, JobID: job}, nil
}
