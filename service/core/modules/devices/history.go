package devices

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/core/ops"
	"github.com/pradita/ac-project/service/core/platform/apperr"
	"github.com/pradita/ac-project/service/core/platform/paging"
)

// TechUnitAccess is the IR94 check for technician operations without jobId.
type TechUnitAccess interface {
	TechnicianUnit(ctx context.Context, c *ops.Call, unit uuid.UUID, inScope bool, internal bool) error
}

// HistoryInput is {deviceId, query} / {id, query}.
type HistoryInput struct {
	DeviceID uuid.UUID    `json:"deviceId"`
	ID       uuid.UUID    `json:"id"`
	Query    paging.Query `json:"query"`
}

func (in *HistoryInput) device() uuid.UUID {
	if in.DeviceID != uuid.Nil {
		return in.DeviceID
	}
	return in.ID
}

// Validate implements ops.Validator.
func (in *HistoryInput) Validate() map[string]string {
	if in.device() == uuid.Nil {
		return map[string]string{"deviceId": "error.required"}
	}
	if len(in.Query.Filters) > 0 && string(in.Query.Filters) != "{}" && string(in.Query.Filters) != "null" {
		return map[string]string{"query.filters": "error.invalid"}
	}
	return nil
}

// historyAccess checks the current-device part of SR24 (device visible to the caller) and, for technicians, an
// active assignment on the device's current unit. It returns the binding-time scope predicate for history rows:
// clients only see rows whose binding belonged to their organization (occurrence scope).
func (m *Module) historyAccess(ctx context.Context, c *ops.Call, device uuid.UUID) (string, []any, error) {
	args := []any{device}
	var unit *uuid.UUID
	err := c.Tx.QueryRow(ctx, "SELECT d.unit_id FROM devices.devices d WHERE d.id = $1 AND "+deviceScope(c.Principal, &args), args...).Scan(&unit)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return "", nil, err
	}
	switch c.Principal.Role {
	case "technician":
		if unit == nil {
			return "", nil, apperr.E(apperr.NotFound, "error.notFound")
		}
		if err := m.UnitAccess.TechnicianUnit(ctx, c, *unit, true, c.Principal.Employment == "internal"); err != nil {
			return "", nil, err
		}
		return "b.unit_id = $2", []any{*unit}, nil // only history of the current binding's unit
	case "client":
		return "b.customer_org_id = $2", []any{c.Principal.OrgID}, nil
	}
	return "$2::uuid IS NULL", []any{nil}, nil // every history row (HQ); keeps the $2 slot
}

func historyPage[T any](ctx context.Context, c *ops.Call, q paging.Query, sel, from, where string, args []any, order string, scan func(pgx.Row) (T, error)) (paging.Page[T], error) {
	w, err := paging.Resolve(q, nil, c.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[T]{}, err
	}
	var total int
	if err := c.Tx.QueryRow(ctx, "SELECT count(*) "+from+" WHERE "+where, args...).Scan(&total); err != nil {
		return paging.Page[T]{}, err
	}
	rows, err := c.Tx.Query(ctx, fmt.Sprintf("SELECT %s %s WHERE %s ORDER BY %s LIMIT %d OFFSET %d", sel, from, where, order, w.Limit, w.Offset), args...)
	if err != nil {
		return paging.Page[T]{}, err
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
	return paging.Page[T]{Items: items, NextCursor: w.Next(total), Total: total, SnapshotVersion: w.Snapshot}, rows.Err()
}

func orderFor(q paging.Query, prefix string) (string, error) {
	return paging.OrderBy(q.Sort, map[string]string{"id": prefix + "id", "createdAt": prefix + "created_at", "updatedAt": prefix + "created_at"}, prefix+"created_at DESC, "+prefix+"id ASC")
}

func (m *Module) operations(ctx context.Context, c *ops.Call, in *HistoryInput) (paging.Page[DeviceOperation], error) {
	scope, sargs, err := m.historyAccess(ctx, c, in.device())
	if err != nil {
		return paging.Page[DeviceOperation]{}, err
	}
	order, err := orderFor(in.Query, "o.")
	if err != nil {
		return paging.Page[DeviceOperation]{}, err
	}
	from := ` FROM devices.device_operations o LEFT JOIN devices.device_bindings b ON b.device_id = o.device_id AND b.bound_at <= o.created_at
		AND (b.unbound_at IS NULL OR b.unbound_at > o.created_at)`
	return historyPage(ctx, c, in.Query, opCols, from, "o.device_id = $1 AND "+scope, append([]any{in.device()}, sargs...), order, scanOp)
}

func (m *Module) calibrations(ctx context.Context, c *ops.Call, in *HistoryInput) (paging.Page[CalibrationRecord], error) {
	scope, sargs, err := m.historyAccess(ctx, c, in.device())
	if err != nil {
		return paging.Page[CalibrationRecord]{}, err
	}
	order, err := orderFor(in.Query, "r.")
	if err != nil {
		return paging.Page[CalibrationRecord]{}, err
	}
	return historyPage(ctx, c, in.Query, calCols, calFrom, "r.device_id = $1 AND "+scope, append([]any{in.device()}, sargs...), order, scanCal)
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

const evCols = `e.id, e.tenant_id, e.version, e.created_at, e.created_at, b.id, e.unit_id, b.customer_org_id, e.alert_ids, e.device_id, e.event_type, e.recovery,
	e.evidence_source, e.sequence, e.occurred_at, e.restored_at`
const evFrom = ` FROM devices.device_events e LEFT JOIN devices.device_bindings b ON b.device_id = e.device_id AND b.bound_at <= e.occurred_at
	AND (b.unbound_at IS NULL OR b.unbound_at > e.occurred_at)`

func (m *Module) scanEvent(ctx context.Context, c *ops.Call) func(pgx.Row) (DeviceEvent, error) {
	return func(r pgx.Row) (DeviceEvent, error) {
		var e DeviceEvent
		err := r.Scan(&e.ID, &e.TenantID, &e.Version, &e.CreatedAt, &e.UpdatedAt, &e.BindingID, &e.UnitIDAtOccurrence, &e.CustomerOrgIDAtOccurrence, &e.AlertIDs,
			&e.DeviceID, &e.EventType, &e.Recovery, &e.EvidenceSource, &e.Sequence, &e.OccurredAt, &e.RestoredAt)
		e.ResponseNotes = []ResponseNote{}
		if len(e.Recovery) > 0 {
			e.RecoveryJSON = jsonRaw(e.Recovery)
		}
		return e, err
	}
}

type jsonRaw []byte

func (j jsonRaw) MarshalJSON() ([]byte, error) { return j, nil }

func (m *Module) withNotes(ctx context.Context, c *ops.Call, evs []DeviceEvent) error {
	for i := range evs {
		rows, err := c.Tx.Query(ctx, `SELECT actor_id, message, created_at FROM devices.device_event_notes WHERE event_id = $1 ORDER BY created_at, id`, evs[i].ID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var n ResponseNote
			if err := rows.Scan(&n.ActorID, &n.Message, &n.At); err != nil {
				rows.Close()
				return err
			}
			evs[i].ResponseNotes = append(evs[i].ResponseNotes, n)
		}
		rows.Close()
	}
	return nil
}

func (m *Module) events(ctx context.Context, c *ops.Call, in *HistoryInput) (paging.Page[DeviceEvent], error) {
	scope, sargs, err := m.historyAccess(ctx, c, in.device())
	if err != nil {
		return paging.Page[DeviceEvent]{}, err
	}
	order, err := paging.OrderBy(in.Query.Sort, map[string]string{"id": "e.id", "createdAt": "e.created_at", "updatedAt": "e.created_at"}, "e.occurred_at DESC, e.sequence DESC, e.id ASC")
	if err != nil {
		return paging.Page[DeviceEvent]{}, err
	}
	p, err := historyPage(ctx, c, in.Query, evCols, evFrom, "e.device_id = $1 AND "+scope, append([]any{in.device()}, sargs...), order, m.scanEvent(ctx, c))
	if err != nil {
		return p, err
	}
	return p, m.withNotes(ctx, c, p.Items)
}

// NoteInput is devices.addResponseNote input.
type NoteInput struct {
	DeviceID     uuid.UUID `json:"deviceId"`
	EventID      uuid.UUID `json:"eventId"`
	ResponseNote string    `json:"responseNote"`
}

// Validate implements ops.Validator (DD-T12: 1–1000 after trimming).
func (in *NoteInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.DeviceID == uuid.Nil {
		fe["deviceId"] = "error.required"
	}
	if in.EventID == uuid.Nil {
		fe["eventId"] = "error.required"
	}
	in.ResponseNote = strings.TrimSpace(in.ResponseNote)
	if n := utf8.RuneCountInString(in.ResponseNote); n < 1 || n > 1000 {
		fe["responseNote"] = "error.length"
	}
	return fe
}

// addResponseNote appends a note to a device event; the version is DeviceEvent.version (strict review).
func (m *Module) addResponseNote(ctx context.Context, c *ops.Call, in *NoteInput) (DeviceEvent, error) {
	scope, sargs, err := m.historyAccess(ctx, c, in.DeviceID)
	if err != nil {
		return DeviceEvent{}, err
	}
	var v int
	err = c.Tx.QueryRow(ctx, "SELECT e.version "+evFrom+" WHERE e.id = $3 AND e.device_id = $1 AND "+scope+" FOR UPDATE OF e",
		append(append([]any{in.DeviceID}, sargs...), in.EventID)...).Scan(&v)
	if errors.Is(err, pgx.ErrNoRows) {
		return DeviceEvent{}, apperr.E(apperr.NotFound, "error.notFound") // event outside the occurrence scope (SR24)
	}
	if err != nil {
		return DeviceEvent{}, err
	}
	if v != *c.ExpectedVersion {
		return DeviceEvent{}, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	if _, err := c.Tx.Exec(ctx, `INSERT INTO devices.device_event_notes (tenant_id, event_id, actor_id, message) VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3)`,
		in.EventID, c.Principal.UserID, in.ResponseNote); err != nil {
		return DeviceEvent{}, err
	}
	if err := c.Tx.QueryRow(ctx, `UPDATE devices.device_events SET version = version + 1 WHERE id = $1 RETURNING version`, in.EventID).Scan(&v); err != nil {
		return DeviceEvent{}, err
	}
	c.Audit(ops.AuditEntry{Action: "devices.addResponseNote", TargetKind: "device_event", TargetID: in.EventID.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &v})
	c.Emit(ops.Event{AggregateType: "device", AggregateID: in.DeviceID, Type: "DeviceEventNoted", Payload: map[string]any{"eventId": in.EventID}})
	e, err := m.scanEvent(ctx, c)(c.Tx.QueryRow(ctx, "SELECT "+evCols+evFrom+" WHERE e.id = $1", in.EventID))
	if err != nil {
		return DeviceEvent{}, err
	}
	evs := []DeviceEvent{e}
	return evs[0], m.withNotes(ctx, c, evs)
}
