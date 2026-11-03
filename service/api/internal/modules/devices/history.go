package devices

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
	"github.com/pradita/ac-project/service/api/internal/platform/unitscope"
)

// TechUnitAccess is the IR94 check for technician operations without jobId.
type TechUnitAccess interface {
	TechnicianUnit(
		ctx context.Context,
		c *ops.Call,
		unit uuid.UUID,
		inScope bool,
		internal bool,
	) error
}

// HistoryInput is the input of devices.operations and devices.calibrations: {deviceId, query}.
type HistoryInput struct {
	DeviceID uuid.UUID    `json:"deviceId"`
	Query    paging.Query `json:"query"`
}

// Validate implements ops.Validator.
func (in *HistoryInput) Validate() map[string]string {
	if in.DeviceID == uuid.Nil {
		return map[string]string{"deviceId": apperr.KeyRequired}
	}

	return nil // the filters are checked per operation (decodeHistory)
}

func (in *HistoryInput) device() uuid.UUID { return in.DeviceID }

// EventsInput is the input of devices.events: {id, query}. Each history read takes only its own contract's ID field,
// so the other one is an unknown field (IR391).
type EventsInput struct {
	ID    uuid.UUID    `json:"id"`
	Query paging.Query `json:"query"`
}

// Validate implements ops.Validator.
func (in *EventsInput) Validate() map[string]string {
	if in.ID == uuid.Nil {
		return map[string]string{"id": apperr.KeyRequired}
	}

	return nil
}

func (in *EventsInput) device() uuid.UUID { return in.ID }

// historyAccess checks the current-device part of SR24 (device visible to the caller) and, for technicians, an
// active assignment on the device's current unit. It returns the binding-time scope predicate for history rows:
// clients only see rows whose binding belonged to their organization (occurrence scope).
func (m *Module) historyAccess(
	ctx context.Context,
	call *ops.Call,
	device uuid.UUID,
) (string, []any, error) {
	args := []any{device}

	var unit *uuid.UUID

	err := call.Tx.QueryRow(ctx, "SELECT d.unit_id FROM devices.devices d WHERE d.id = $1 AND "+
		deviceScope(call, &args, unitscope.List), args...).
		Scan(&unit)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return "", nil, apperr.Wrap(err, "select devices.devices")
	}

	switch call.Principal.Role {
	case roleTechnician:
		if unit == nil {
			return "", nil, apperr.E(apperr.NotFound, apperr.KeyNotFound)
		}

		err := m.UnitAccess.TechnicianUnit(
			ctx,
			call,
			*unit,
			true,
			call.Principal.Employment == "internal",
		)
		if err != nil {
			return "", nil, apperr.Wrap(err, "devices.TechUnitAccess.TechnicianUnit")
		}

		return "b.unit_id = $2", []any{*unit}, nil // only history of the current binding's unit
	case roleClient:
		return "b.customer_org_id = $2", []any{call.Principal.OrgID}, nil
	}

	return "$2::uuid IS NULL", []any{nil}, nil // every history row (HQ); keeps the $2 slot
}

// historyFilter is the devices.events period of the query catalog ([from, to) on occurredAt); devices.operations and
// devices.calibrations take no filters, so any filter key there is VALIDATION (SR06).
type historyFilter struct {
	From *time.Time `json:"from,omitempty"`
	To   *time.Time `json:"to,omitempty"`
}

func decodeHistory(query paging.Query, period bool) (historyFilter, error) {
	var filter historyFilter
	if !query.HasFilters() {
		return filter, nil
	}

	dec := json.NewDecoder(strings.NewReader(string(query.Filters)))
	dec.DisallowUnknownFields()

	if !period || dec.Decode(&filter) != nil {
		return filter, apperr.Fields(map[string]string{"query.filters": apperr.KeyInvalid})
	}

	if filter.From != nil && filter.To != nil && !filter.From.Before(*filter.To) {
		return filter, apperr.Fields(map[string]string{"query.filters.to": apperr.KeyRange})
	}

	return filter, nil
}

func historyPage[T any](
	ctx context.Context,
	call *ops.Call,
	query paging.Query,
	sel, from, where string,
	args []any,
	order string,
	scan func(pgx.Row) (T, error),
) (paging.Page[T], error) {
	f, err := decodeHistory(query, false)
	if err != nil {
		return paging.Page[T]{}, err
	}

	return historyPageOf(ctx, call, query, f, sel, from, where, args, order, scan)
}

func historyPageOf[T any](
	ctx context.Context,
	call *ops.Call,
	query paging.Query,
	filter historyFilter,
	sel, from, where string,
	args []any,
	order string,
	scan func(pgx.Row) (T, error),
) (paging.Page[T], error) {
	window, err := paging.Resolve(query, filter, call.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[T]{}, apperr.Wrap(err, "paging.Resolve")
	}

	var total int

	scanErr := call.Tx.QueryRow(ctx, "SELECT count(*) "+from+" WHERE "+where, args...).
		Scan(&total)
	if scanErr != nil {
		return paging.Page[T]{}, apperr.Wrap(scanErr, "select count")
	}

	rows, err := call.Tx.Query(
		ctx,
		fmt.Sprintf(
			"SELECT %s %s WHERE %s ORDER BY %s LIMIT %d OFFSET %d",
			sel,
			from,
			where,
			order,
			window.Limit,
			window.Offset,
		),
		args...)
	if err != nil {
		return paging.Page[T]{}, apperr.Wrap(err, "select (historyPageOf)")
	}
	defer rows.Close()

	items := []T{}

	for rows.Next() {
		x, err := scan(rows)
		if err != nil {
			return paging.Page[T]{}, err
		}

		items = append(items, x)
	}

	return paging.Page[T]{
		Items:           items,
		NextCursor:      window.Next(total),
		Total:           total,
		SnapshotVersion: window.Snapshot,
	}, apperr.Wrap(rows.Err(), "select (historyPageOf)")
}

func orderFor(q paging.Query, prefix string) (string, error) {
	out, err := paging.OrderBy(
		q.Sort,
		map[string]string{
			"id":           prefix + "id",
			fieldCreatedAt: prefix + colCreatedAt,
			fieldUpdatedAt: prefix + colCreatedAt,
		},
		prefix+"created_at DESC, "+prefix+"id ASC",
	)

	return out, apperr.Wrap(err, "paging.OrderBy")
}

// operations serves devices.operations.
//
//	@Summary		devices.operations (read)
//	@ID				devices.operations
//	@Description	Authorization: technician:device.maintain:assigned-valid-job | admin:device.read; SR24 \
//	@Description	current-device AND occurrence-scope
//	@Description	Validation: D01; SR08/SR09
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-T11, DD-A04 · Query: filters none · sort id,createdAt,updatedAt (default createdAt \
//	@Description	desc;id asc)
//	@Tags			devices
//	@Accept			json
//	@Produce		json
//	@Param			deviceId	path		string	true	"input field deviceId"
//	@Param			cursor		query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit		query		integer	false	"page size 1–100, default 25"
//	@Param			sort		query		string	false	"field:direction — fields id,createdAt,updatedAt; default createdAt desc;id asc"
//	@Success		200			{object}	ops.Envelope{data=DeviceOperationPage}
//	@Failure		401			{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403			{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404			{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409			{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422			{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429			{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503			{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504			{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/devices/{deviceId}/operations [get]
func (m *Module) operations(
	ctx context.Context,
	call *ops.Call,
	input *HistoryInput,
) (paging.Page[DeviceOperation], error) {
	scope, sargs, err := m.historyAccess(ctx, call, input.device())
	if err != nil {
		return paging.Page[DeviceOperation]{}, err
	}

	order, err := orderFor(input.Query, "o.")
	if err != nil {
		return paging.Page[DeviceOperation]{}, err
	}

	from := ` FROM devices.device_operations o LEFT JOIN devices.device_bindings b ON b.device_id = o.device_id AND
		b.bound_at <= o.created_at
		AND (b.unbound_at IS NULL OR b.unbound_at > o.created_at)`

	return historyPage(
		ctx,
		call,
		input.Query,
		opCols,
		from,
		"o.device_id = $1 AND "+scope,
		append([]any{input.device()}, sargs...),
		order,
		scanOp,
	)
}

// calibrations serves devices.calibrations.
//
//	@Summary		devices.calibrations (read)
//	@ID				devices.calibrations
//	@Description	Authorization: technician:device.maintain:assigned-valid-job | admin:device.read; SR24 \
//	@Description	current-device AND occurrence-scope
//	@Description	Validation: D01; SR08/SR09
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-T11, DD-A04 · Query: filters none · sort id,createdAt,updatedAt (default createdAt \
//	@Description	desc;id asc)
//	@Tags			devices
//	@Accept			json
//	@Produce		json
//	@Param			deviceId	path		string	true	"input field deviceId"
//	@Param			cursor		query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit		query		integer	false	"page size 1–100, default 25"
//	@Param			sort		query		string	false	"field:direction — fields id,createdAt,updatedAt; default createdAt desc;id asc"
//	@Success		200			{object}	ops.Envelope{data=CalibrationRecordPage}
//	@Failure		401			{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403			{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404			{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409			{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422			{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429			{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503			{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504			{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/devices/{deviceId}/calibrations [get]
func (m *Module) calibrations(
	ctx context.Context,
	call *ops.Call,
	input *HistoryInput,
) (paging.Page[CalibrationRecord], error) {
	scope, sargs, err := m.historyAccess(ctx, call, input.device())
	if err != nil {
		return paging.Page[CalibrationRecord]{}, err
	}

	order, err := orderFor(input.Query, "r.")
	if err != nil {
		return paging.Page[CalibrationRecord]{}, err
	}

	return historyPage(
		ctx,
		call,
		input.Query,
		calCols,
		calFrom,
		"r.device_id = $1 AND "+scope,
		append([]any{input.device()}, sargs...),
		order,
		scanCal,
	)
}

// ResponseNote is one DeviceEvent.responseNotes entry.
type ResponseNote struct {
	ActorID uuid.UUID `json:"actorId"`
	Message string    `json:"message"`
	At      time.Time `json:"at"`
}

// DeviceEvent is DeviceEvent of service-contracts.ts.
type DeviceEvent struct {
	ID                        uuid.UUID      `json:"id"`
	TenantID                  uuid.UUID      `json:"tenantId"`
	Version                   int            `json:"version"`
	CreatedAt                 time.Time      `json:"createdAt"`
	UpdatedAt                 time.Time      `json:"updatedAt"`
	BindingID                 *uuid.UUID     `json:"bindingId"`
	UnitIDAtOccurrence        *uuid.UUID     `json:"unitIdAtOccurrence"`
	CustomerOrgIDAtOccurrence *uuid.UUID     `json:"customerOrgIdAtOccurrence"`
	AlertIDs                  []uuid.UUID    `json:"alertIds"`
	DeviceID                  uuid.UUID      `json:"deviceId"`
	EventType                 string         `json:"eventType"`
	Recovery                  []byte         `json:"-"`
	RecoveryJSON              any            `json:"recovery"`
	EvidenceSource            string         `json:"evidenceSource"`
	Sequence                  int64          `json:"sequence"`
	OccurredAt                time.Time      `json:"occurredAt"`
	RestoredAt                *time.Time     `json:"restoredAt"`
	ResponseNotes             []ResponseNote `json:"responseNotes"`
}

const evCols = `e.id, e.tenant_id, e.version, e.created_at, e.created_at AS updated_at, b.id, e.unit_id, ` +
	`b.customer_org_id, e.alert_ids, e.device_id, e.event_type, e.recovery,
	e.evidence_source, e.sequence, e.occurred_at, e.restored_at`

const evFrom = ` FROM devices.device_events e LEFT JOIN devices.device_bindings b ON b.device_id = e.device_id AND
	b.bound_at <= e.occurred_at
	AND (b.unbound_at IS NULL OR b.unbound_at > e.occurred_at)`

func (m *Module) scanEvent(_ context.Context, _ *ops.Call) func(pgx.Row) (DeviceEvent, error) {
	return func(r pgx.Row) (DeviceEvent, error) {
		var deviceEvent DeviceEvent

		err := r.Scan(
			&deviceEvent.ID,
			&deviceEvent.TenantID,
			&deviceEvent.Version,
			&deviceEvent.CreatedAt,
			&deviceEvent.UpdatedAt,
			&deviceEvent.BindingID,
			&deviceEvent.UnitIDAtOccurrence,
			&deviceEvent.CustomerOrgIDAtOccurrence,
			&deviceEvent.AlertIDs,
			&deviceEvent.DeviceID,
			&deviceEvent.EventType,
			&deviceEvent.Recovery,
			&deviceEvent.EvidenceSource,
			&deviceEvent.Sequence,
			&deviceEvent.OccurredAt,
			&deviceEvent.RestoredAt,
		)

		deviceEvent.ResponseNotes = []ResponseNote{}
		if len(deviceEvent.Recovery) > 0 {
			deviceEvent.RecoveryJSON = jsonRaw(deviceEvent.Recovery)
		}

		return deviceEvent, apperr.Wrap(err, "scan (scanEvent)")
	}
}

type jsonRaw []byte

func (j jsonRaw) MarshalJSON() ([]byte, error) { return j, nil }

// withNotes reads the response notes of a page of events in one query, not one per event (IR497).
func (m *Module) withNotes(ctx context.Context, call *ops.Call, evs []DeviceEvent) error {
	if len(evs) == 0 {
		return nil
	}

	ids, indexOf := make([]uuid.UUID, len(evs)), make(map[uuid.UUID]int, len(evs))
	for i := range evs {
		ids[i], indexOf[evs[i].ID] = evs[i].ID, i
	}

	rows, err := call.Tx.Query(
		ctx,
		`SELECT event_id, actor_id, message, created_at FROM devices.device_event_notes WHERE event_id = ANY($1) ORDER BY
			event_id, created_at, id`,
		ids,
	)
	if err != nil {
		return apperr.Wrap(err, "select devices.device_event_notes")
	}

	for rows.Next() {
		var (
			event        uuid.UUID
			responseNote ResponseNote
		)

		err := rows.Scan(&event, &responseNote.ActorID, &responseNote.Message, &responseNote.At)
		if err != nil {
			rows.Close()

			return apperr.Wrap(err, "select devices.device_event_notes")
		}

		if i, ok := indexOf[event]; ok {
			evs[i].ResponseNotes = append(evs[i].ResponseNotes, responseNote)
		}
	}

	rows.Close()

	return apperr.Wrap(rows.Err(), "select devices.device_event_notes")
}

// events serves devices.events.
//
//	@Summary		devices.events (read)
//	@ID				devices.events
//	@Description	Authorization: client:self | contractor:accepted-valid-offer | technician:assigned | \
//	@Description	admin:device.read | admin:audit.read; SR24 current-device AND occurrence-scope
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A04, DD-A16, DD-T12, DD-T03 · Query: filters from,to · sort id,createdAt,updatedAt \
//	@Description	(default occurredAt desc;sequence desc;id asc)
//	@Tags			devices
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string	true	"input field id"
//	@Param			cursor	query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit	query		integer	false	"page size 1–100, default 25"
//	@Param			sort	query		string	false	"field:direction — fields id,createdAt,updatedAt; default occurredAt desc;sequence desc;id asc"
//	@Param			from	query		string	false	"filter → [from,to) on occurredAt"
//	@Param			to		query		string	false	"filter → [from,to) on occurredAt"
//	@Success		200		{object}	ops.Envelope{data=DeviceEventPage}
//	@Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504		{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/devices/{id}/events [get]
func (m *Module) events(
	ctx context.Context,
	call *ops.Call,
	input *EventsInput,
) (paging.Page[DeviceEvent], error) {
	scope, sargs, err := m.historyAccess(ctx, call, input.device())
	if err != nil {
		return paging.Page[DeviceEvent]{}, err
	}

	order, err := paging.OrderBy(
		input.Query.Sort,
		map[string]string{
			"id":           "e.id",
			fieldCreatedAt: "e.created_at",
			fieldUpdatedAt: "e.created_at",
		},
		"e.occurred_at DESC, e.sequence DESC, e.id ASC",
	)
	if err != nil {
		return paging.Page[DeviceEvent]{}, apperr.Wrap(err, "paging.OrderBy")
	}

	filter, err := decodeHistory(input.Query, true)
	if err != nil {
		return paging.Page[DeviceEvent]{}, err
	}

	where, args := "e.device_id = $1 AND "+scope, append([]any{input.device()}, sargs...)
	if filter.From != nil {
		args = append(args, *filter.From)
		where += fmt.Sprintf(" AND e.occurred_at >= $%d", len(args))
	}

	if filter.To != nil {
		args = append(args, *filter.To)
		where += fmt.Sprintf(" AND e.occurred_at < $%d", len(args))
	}

	page, err := historyPageOf(
		ctx,
		call,
		input.Query,
		filter,
		evCols,
		evFrom,
		where,
		args,
		order,
		m.scanEvent(ctx, call),
	)
	if err != nil {
		return page, err
	}

	return page, m.withNotes(ctx, call, page.Items)
}

// NoteInput is devices.addResponseNote input.
type NoteInput struct {
	DeviceID     uuid.UUID `json:"deviceId"`
	EventID      uuid.UUID `json:"eventId"`
	ResponseNote string    `json:"responseNote"`
}

// Validate implements ops.Validator (DD-T12: 1–1000 after trimming).
func (in *NoteInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.DeviceID == uuid.Nil {
		fieldErrs["deviceId"] = apperr.KeyRequired
	}

	if in.EventID == uuid.Nil {
		fieldErrs["eventId"] = apperr.KeyRequired
	}

	in.ResponseNote = strings.TrimSpace(in.ResponseNote)
	if n := utf8.RuneCountInString(in.ResponseNote); n < 1 || n > 1000 {
		fieldErrs["responseNote"] = apperr.KeyLength
	}

	return fieldErrs
}

// addResponseNote appends a note to a device event; the version is DeviceEvent.version (strict review).
//
//	@Summary		devices.addResponseNote (write)
//	@ID				devices.addResponseNote
//	@Description	Authorization: technician:device.maintain:assigned-valid-job | admin:device.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR94 technician write table (assignment and work window, jobId required when typed)
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-T12
//	@Tags			devices
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer		true	"all: required (target device event, read devices.events)"
//	@Param			deviceId			path		string		true	"input field deviceId"
//	@Param			eventId				path		string		true	"input field eventId"
//	@Param			request				body		NoteInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=DeviceEvent}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/devices/{deviceId}/events/{eventId}/response-note [post]
func (m *Module) addResponseNote(
	ctx context.Context,
	call *ops.Call,
	input *NoteInput,
) (DeviceEvent, error) {
	scope, sargs, err := m.historyAccess(ctx, call, input.DeviceID)
	if err != nil {
		return DeviceEvent{}, err
	}

	err = lockEvent(ctx, call, input, scope, sargs)
	if err != nil {
		return DeviceEvent{}, err
	}

	version, err := insertNote(ctx, call, input)
	if err != nil {
		return DeviceEvent{}, err
	}

	call.Audit(
		ops.AuditEntry{
			Action:          "devices.addResponseNote",
			TargetKind:      "device_event",
			TargetID:        input.EventID.String(),
			PreviousVersion: call.ExpectedVersion,
			NextVersion:     &version, Reason: "", Result: "", Before: nil, After: nil,
		},
	)
	call.Emit(
		ops.Event{
			AggregateType: entityDevice,
			AggregateID:   input.DeviceID,
			Type:          "DeviceEventNoted",
			Payload:       map[string]any{"eventId": input.EventID},
		},
	)

	deviceEvent, err := m.scanEvent(
		ctx,
		call,
	)(
		call.Tx.QueryRow(ctx, "SELECT "+evCols+evFrom+" WHERE e.id = $1", input.EventID),
	)
	if err != nil {
		return DeviceEvent{}, err
	}

	evs := []DeviceEvent{deviceEvent}

	return evs[0], m.withNotes(ctx, call, evs)
}

// lockEvent locks the device event of a response note inside the caller's occurrence scope (NOT_FOUND, SR24) and
// checks its expected version.
func lockEvent(
	ctx context.Context,
	call *ops.Call,
	input *NoteInput,
	scope string,
	sargs []any,
) error {
	var version int

	err := call.Tx.QueryRow(ctx, "SELECT e.version "+evFrom+" WHERE e.id = $3 AND e.device_id = $1 AND "+scope+
		" FOR UPDATE OF e",
		append(append([]any{input.DeviceID}, sargs...), input.EventID)...).
		Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.E(
			apperr.NotFound,
			apperr.KeyNotFound,
		) // event outside the occurrence scope (SR24)
	}

	if err != nil {
		return apperr.Wrap(err, "select devices.device_events")
	}

	if version != *call.ExpectedVersion {
		return apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	return nil
}

// insertNote stores the response note and returns the event's new version.
func insertNote(ctx context.Context, call *ops.Call, input *NoteInput) (int, error) {
	_, execErr := call.Tx.Exec(
		ctx,
		`INSERT INTO devices.device_event_notes (tenant_id, event_id, actor_id, message) VALUES
			(current_setting('app.tenant_id')::uuid, $1, $2, $3)`,
		input.EventID,
		call.Principal.UserID,
		input.ResponseNote,
	)
	if execErr != nil {
		return 0, apperr.Wrap(execErr, "insert devices.device_event_notes")
	}

	var version int

	scanErr := call.Tx.QueryRow(ctx, `UPDATE devices.device_events SET version = version + 1 WHERE id = $1 RETURNING
		version`, input.EventID).
		Scan(&version)
	if scanErr != nil {
		return 0, apperr.Wrap(scanErr, "update devices.device_events")
	}

	return version, nil
}
