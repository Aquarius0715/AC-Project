package monitoring

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
	"github.com/pradita/ac-project/service/api/internal/platform/unitscope"
)

// DeviceRecovery is a recovery event of a unit's device linked to an alert (a tamper cleared, a connection or power
// signal back).
type DeviceRecovery struct {
	ID         uuid.UUID
	EventType  string
	OccurredAt time.Time
}

// DeviceRecoveries reads the recovery events of the devices linked to an alert since a time (devices, IR327).
type DeviceRecoveries interface {
	AlertRecoveries(ctx context.Context, c *ops.Call, alert uuid.UUID, since time.Time) ([]DeviceRecovery, error)
}

// EvidenceCandidate is EvidenceCandidate of service-contracts.ts: a record that may support an alert's resolution.
type EvidenceCandidate struct {
	ID         uuid.UUID `json:"id"`
	Kind       string    `json:"kind"` // detection | remeasurement | device_event
	ObservedAt time.Time `json:"observedAt"`
	Metric     *string   `json:"metric"`
	Value      *float64  `json:"value"`
	Unit       *string   `json:"unit"`
	Origin     *string   `json:"origin"`
	Quality    *string   `json:"quality"`
	EventType  *string   `json:"eventType"`
}

// maxRemeasurements bounds the remeasurements one candidate list carries (the newest first).
const maxRemeasurements = 100

// remeasurementSQL selects the valid readings of the alert's unit measured after its detection — measured or recorded
// on site, never estimated; $3 is the rule's metric, or null for an alert without a rule (any metric).
const remeasurementSQL = `FROM monitoring.measurements m WHERE m.unit_id = $1 AND m.observed_at > $2 AND m.quality = 'valid'
	AND m.origin IN ('measured', 'inspection') AND m.value IS NOT NULL AND ($3::text IS NULL OR m.metric = $3)`

func ruleMetric(a Alert) *string {
	if a.Rule == nil || a.Rule.Metric == "" {
		return nil
	}
	return &a.Rule.Metric
}

// candidates lists an alert's evidence candidates (IR327, DD-A05 item 6): its own evidence at detection, the unit's
// valid measured or inspection remeasurements after detection (of the rule's metric when it has one) and the recovery events of the unit's
// device linked to the alert, the newest first.
func (m Alerts) candidates(ctx context.Context, c *ops.Call, a Alert) ([]EvidenceCandidate, error) {
	out := []EvidenceCandidate{}
	for _, id := range a.EvidenceIDs {
		out = append(out, EvidenceCandidate{ID: id, Kind: "detection", ObservedAt: a.ObservedAt})
	}
	rows, err := c.Tx.Query(ctx, `SELECT m.event_id, m.observed_at, m.metric, m.value, m.unit, m.origin, m.quality `+remeasurementSQL+`
		ORDER BY m.observed_at DESC, m.sequence DESC, m.event_id LIMIT `+strconv.Itoa(maxRemeasurements), a.UnitID, a.DetectedAt, ruleMetric(a))
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var x EvidenceCandidate
		var metric, unit, origin, quality string
		if err := rows.Scan(&x.ID, &x.ObservedAt, &metric, &x.Value, &unit, &origin, &quality); err != nil {
			rows.Close()
			return nil, err
		}
		x.Kind, x.Metric, x.Unit, x.Origin, x.Quality = "remeasurement", &metric, &unit, &origin, &quality
		out = append(out, x)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if m.Devices != nil {
		recs, err := m.Devices.AlertRecoveries(ctx, c, a.ID, a.DetectedAt)
		if err != nil {
			return nil, err
		}
		for _, r := range recs {
			typ := r.EventType
			out = append(out, EvidenceCandidate{ID: r.ID, Kind: "device_event", ObservedAt: r.OccurredAt, EventType: &typ})
		}
	}
	slices.SortStableFunc(out, func(x, y EvidenceCandidate) int {
		if c := y.ObservedAt.Compare(x.ObservedAt); c != 0 {
			return c
		}
		return strings.Compare(x.ID.String(), y.ID.String())
	})
	return out, nil
}

// isCandidate reports whether one ID is an evidence candidate of the alert, without the list's bound.
func (m Alerts) isCandidate(ctx context.Context, c *ops.Call, a Alert, id uuid.UUID) (bool, error) {
	if slices.Contains(a.EvidenceIDs, id) {
		return true, nil
	}
	var ok bool
	if err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 `+remeasurementSQL+` AND m.event_id = $4)`, a.UnitID, a.DetectedAt, ruleMetric(a), id).Scan(&ok); err != nil || ok {
		return ok, err
	}
	if m.Devices == nil {
		return false, nil
	}
	recs, err := m.Devices.AlertRecoveries(ctx, c, a.ID, a.DetectedAt)
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(recs, func(r DeviceRecovery) bool { return r.ID == id }), nil
}

func distinct(ids []uuid.UUID) bool {
	seen := map[uuid.UUID]bool{}
	for _, id := range ids {
		if seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}

// checkEvidence applies IR66 / IR327 to a resolution: an alert without a policy needs at least one evidence ID, and
// every ID given must be one of the alert's candidates.
func (m Alerts) checkEvidence(ctx context.Context, c *ops.Call, a Alert, ids []uuid.UUID) error {
	if len(ids) == 0 {
		if a.PolicyID == nil {
			return apperr.Fields(map[string]string{"resolutionEvidenceIds": "errors.evidence_required"})
		}
		return nil
	}
	for _, id := range ids {
		ok, err := m.isCandidate(ctx, c, a, id)
		if err != nil {
			return err
		}
		if !ok {
			return apperr.Fields(map[string]string{"resolutionEvidenceIds": "errors.evidence_unknown"})
		}
	}
	return nil
}

// EvidenceInput is alerts.evidence input.
type EvidenceInput struct {
	AlertID uuid.UUID    `json:"alertId"`
	Query   paging.Query `json:"query"`
}

// Validate implements ops.Validator.
func (in *EvidenceInput) Validate() map[string]string {
	if in.AlertID == uuid.Nil {
		return map[string]string{"alertId": "error.required"}
	}
	return nil
}

// @Summary		alerts.evidence (read)
// @ID				alerts.evidence
// @Description	Authorization: technician:alert.resolve:assigned | admin:alert.resolve
// @Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
// @Description	Recovery: D04: retry only UNAVAILABLE, at most twice
// @Description	Design: DD-A05, DD-T07 · Query: filters none · sort id,observedAt (default observedAt desc;id asc)
// @Tags			alerts
// @Accept			json
// @Produce		json
// @Param			alertId	path		string	true	"input field alertId"
// @Param			cursor	query		string	false	"page cursor: nextCursor of the previous page (D12)"
// @Param			limit	query		integer	false	"page size 1–100, default 25"
// @Param			sort	query		string	false	"field:direction — fields id,observedAt; default observedAt desc;id asc"
// @Success		200		{object}	ops.Envelope{data=EvidenceCandidatePage}
// @Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504		{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/alerts/{alertId}/evidence [get]
func (m Alerts) evidence(ctx context.Context, c *ops.Call, in *EvidenceInput) (paging.Page[EvidenceCandidate], error) {
	if err := paging.NoFilters(in.Query, "query.filters"); err != nil {
		return paging.Page[EvidenceCandidate]{}, err
	}
	args := []any{in.AlertID}
	a, err := scanAlert(c.Tx.QueryRow(ctx, "SELECT "+alertCols+" FROM monitoring.alerts a WHERE a.id = $1 AND "+alertScope(c, &args, unitscope.List), args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return paging.Page[EvidenceCandidate]{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return paging.Page[EvidenceCandidate]{}, err
	}
	if err := unitscope.Gate(ctx, c, []uuid.UUID{a.UnitID}); err != nil {
		return paging.Page[EvidenceCandidate]{}, err
	}
	all, err := m.candidates(ctx, c, a)
	if err != nil {
		return paging.Page[EvidenceCandidate]{}, err
	}
	if err := paging.SortSlice(all, in.Query.Sort, map[string]func(x, y EvidenceCandidate) int{
		"id":         func(x, y EvidenceCandidate) int { return strings.Compare(x.ID.String(), y.ID.String()) },
		"observedAt": func(x, y EvidenceCandidate) int { return x.ObservedAt.Compare(y.ObservedAt) },
	}); err != nil {
		return paging.Page[EvidenceCandidate]{}, err
	}
	w, err := paging.Resolve(in.Query, in.AlertID, c.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[EvidenceCandidate]{}, err
	}
	items := []EvidenceCandidate{}
	for i := w.Offset; i < len(all) && i < w.Offset+w.Limit; i++ {
		items = append(items, all[i])
	}
	return paging.Page[EvidenceCandidate]{Items: items, NextCursor: w.Next(len(all)), Total: len(all), SnapshotVersion: w.Snapshot}, nil
}
