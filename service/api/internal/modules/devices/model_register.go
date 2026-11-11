package devices

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

// modelRegister is a versioned model table: the aircon models (devices.capabilities) and the IoT models
// (devices.iot_models) are listed and versioned the same way (DD-A04, DEC-105). Each row is one version; the current
// one is listed.
type modelRegister[T any] struct {
	table   string
	cols    string
	scan    func(pgx.Row) (T, error)
	fields  func(T) map[string]any
	version func(T) int
}

// list serves the register's list operation: the current versions, no filters, sorted by id, createdAt or updatedAt.
func (r modelRegister[T]) list(
	ctx context.Context,
	call *ops.Call,
	query *paging.Query,
) (paging.Page[T], error) {
	var none paging.Page[T]

	if len(query.Filters) > 0 && string(query.Filters) != "{}" && string(query.Filters) != "null" {
		return none, apperr.Fields(
			map[string]string{fieldFilters: apperr.KeyInvalid},
		) // query catalog: no filters
	}

	order, err := paging.OrderBy(
		query.Sort,
		map[string]string{"id": "id", fieldCreatedAt: colCreatedAt, fieldUpdatedAt: colCreatedAt},
		"id ASC",
	)
	if err != nil {
		return none, apperr.Wrap(err, "paging.OrderBy")
	}

	window, err := paging.Resolve(*query, nil, call.Principal.ScopeVersion, 1)
	if err != nil {
		return none, apperr.Wrap(err, "paging.Resolve")
	}

	var total int

	scanErr := call.Tx.QueryRow(ctx, `SELECT count(*) FROM `+r.table+` WHERE is_current`).
		Scan(&total)
	if scanErr != nil {
		return none, apperr.Wrap(scanErr, "select "+r.table)
	}

	rows, err := call.Tx.Query(
		ctx,
		fmt.Sprintf(
			`SELECT %s FROM %s WHERE is_current ORDER BY %s LIMIT %d OFFSET %d`,
			r.cols,
			r.table,
			order,
			window.Limit,
			window.Offset,
		),
	)
	if err != nil {
		return none, apperr.Wrap(err, "select "+r.table)
	}

	items, err := r.scanAll(rows)

	return paging.Page[T]{
		Items:           items,
		NextCursor:      window.Next(total),
		Total:           total,
		SnapshotVersion: window.Snapshot,
	}, err
}

// scanAll reads a page of rows.
func (r modelRegister[T]) scanAll(rows pgx.Rows) ([]T, error) {
	defer rows.Close()

	items := []T{}

	for rows.Next() {
		item, err := r.scan(rows)
		if err != nil {
			return nil, err
		}

		items = append(items, item)
	}

	return items, apperr.Wrap(rows.Err(), "select "+r.table)
}

// revise is version 1 of a new model or, for an update of model id, the next version of the current row, which it
// retires after the expected-version check (the change reason is taken on update only).
func (r modelRegister[T]) revise(
	ctx context.Context,
	call *ops.Call,
	id *uuid.UUID,
	reason *string,
) (capabilityRevision, error) {
	revision := capabilityRevision{
		modelID: uuid.Must(uuid.NewV7()),
		version: 1,
		reason:  "",
		before:  nil,
	}
	if id == nil {
		return revision, nil
	}

	lockErr := r.lock(ctx, call, *id)
	if lockErr != nil {
		return revision, lockErr
	}

	old, err := r.scan(
		call.Tx.QueryRow(
			ctx,
			`SELECT `+r.cols+` FROM `+r.table+` WHERE id = $1 AND is_current FOR UPDATE`,
			*id,
		),
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return revision, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return revision, err
	}

	revision.before = r.fields(old)

	if r.version(old) != *call.ExpectedVersion {
		return revision, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE `+r.table+` SET is_current = false WHERE id = $1 AND version = $2`,
		*id,
		r.version(old),
	)
	if execErr != nil {
		return revision, apperr.Wrap(execErr, "update "+r.table)
	}

	revision.modelID, revision.version = *id, r.version(old)+1
	if reason != nil {
		revision.reason = *reason
	}

	return revision, nil
}

// lock locks every version row of a model FOR UPDATE before a write reads its current version (IR766): a write that
// waited for a new version saved meanwhile then reads that version and answers a version conflict, instead of finding
// the row it waited for no longer current and answering NOT_FOUND.
func (r modelRegister[T]) lock(ctx context.Context, call *ops.Call, id uuid.UUID) error {
	_, err := call.Tx.Exec(
		ctx,
		`SELECT 1 FROM `+r.table+` WHERE id = $1 ORDER BY version FOR UPDATE`,
		id,
	)

	return apperr.Wrap(err, "select "+r.table)
}

// capabilityRegister is the register of the aircon models.
func capabilityRegister() modelRegister[Capability] {
	return modelRegister[Capability]{
		table:   "devices.capabilities",
		cols:    capCols,
		scan:    scanCap,
		fields:  Capability.fields,
		version: func(c Capability) int { return c.Version },
	}
}

// iotModelRegister is the register of the IoT models (DEC-105).
func iotModelRegister() modelRegister[IoTModel] {
	return modelRegister[IoTModel]{
		table:   "devices.iot_models",
		cols:    iotCols,
		scan:    scanIoTModel,
		fields:  IoTModel.fields,
		version: func(m IoTModel) int { return m.Version },
	}
}
