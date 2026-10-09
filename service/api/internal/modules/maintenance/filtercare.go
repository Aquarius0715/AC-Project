package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

// FilterUnits lists units for filter care (Assets).
type FilterUnits interface {
	BriefUnits(ctx context.Context, c *ops.Call, customer, property, space, unit *uuid.UUID) ([]ops.UnitBrief, error)
	CustomerOfOrg(ctx context.Context, c *ops.Call, org uuid.UUID) (uuid.UUID, bool, error)
	UnitState(ctx context.Context, c *ops.Call, unit uuid.UUID) (uuid.UUID, bool, bool, error)
}

// RunTime computes run hours since a cleaning (Monitoring).
type RunTime interface {
	RunHours(ctx context.Context, c *ops.Call, unit uuid.UUID, since *time.Time) (*float64, error)
}

// FilterCare is the filter care operation set (IR134).
type FilterCare struct {
	Units FilterUnits
	Run   RunTime
}

// DefaultFilterHours is the model default threshold (DD-C18).
const DefaultFilterHours = 250

// FilterSettings is FilterCareSettings.
type FilterSettings struct {
	ID             uuid.UUID `json:"id"`
	TenantID       uuid.UUID `json:"tenantId"`
	Version        int       `json:"version"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
	CustomerID     uuid.UUID `json:"customerId"`
	ThresholdHours *int      `json:"thresholdHours"`
	FallbackDays   int       `json:"fallbackDays"`
	Recipients     string    `json:"recipients"`
	Channels       []string  `json:"channels"`
}

// FilterStatus is FilterCareStatus.
type FilterStatus struct {
	UnitID                uuid.UUID  `json:"unitId"`
	RunHoursSinceCleaning *float64   `json:"runHoursSinceCleaning"`
	ThresholdHours        int        `json:"thresholdHours"`
	FallbackDays          int        `json:"fallbackDays"`
	LastCleanedAt         *time.Time `json:"lastCleanedAt"`
	LastCleanedBy         *string    `json:"lastCleanedBy"`
	LastCleaningJobID     *uuid.UUID `json:"lastCleaningJobId"`
	Status                string     `json:"status"`
}

func (m FilterCare) settings(ctx context.Context, c *ops.Call, customer uuid.UUID) (FilterSettings, error) {
	s := FilterSettings{TenantID: c.Principal.TenantID, Version: 0, CustomerID: customer, FallbackDays: 30, Recipients: "owners", Channels: []string{"inApp"}}
	err := c.Tx.QueryRow(ctx, `SELECT id, version, updated_at, threshold_hours, fallback_days, recipients, channels FROM maintenance.filter_care_settings WHERE customer_id = $1`, customer).
		Scan(&s.ID, &s.Version, &s.UpdatedAt, &s.ThresholdHours, &s.FallbackDays, &s.Recipients, &s.Channels)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, nil
	}
	s.CreatedAt = s.UpdatedAt
	return s, err
}

// status computes FilterCareStatus for one unit (IR134 items 1–3).
func (m FilterCare) status(ctx context.Context, c *ops.Call, u ops.UnitBrief, set FilterSettings) (FilterStatus, error) {
	st := FilterStatus{UnitID: u.ID, ThresholdHours: DefaultFilterHours, FallbackDays: set.FallbackDays, Status: "unknown"}
	if set.ThresholdHours != nil {
		st.ThresholdHours = *set.ThresholdHours
	}
	var at time.Time
	if err := c.Tx.QueryRow(ctx, `SELECT max(cleaned_at) FROM maintenance.filter_cleanings WHERE unit_id = $1`, u.ID).Scan(&st.LastCleanedAt); err != nil {
		return st, err
	}
	if st.LastCleanedAt != nil {
		by := "customer"
		st.LastCleanedBy = &by
	}
	var job uuid.UUID
	err := c.Tx.QueryRow(ctx, `SELECT j.id, j.completed_at FROM maintenance.jobs j WHERE j.unit_id = $1 AND j.status = 'completed' AND EXISTS (
		SELECT 1 FROM maintenance.work_reports w WHERE w.job_id = j.id AND w.state = 'accepted' AND w.items @> '[{"componentKey":"filter","result":"normal"}]')
		ORDER BY j.completed_at DESC LIMIT 1`, u.ID).Scan(&job, &at)
	switch {
	case err == nil && (st.LastCleanedAt == nil || at.After(*st.LastCleanedAt)):
		by := "technician"
		st.LastCleanedAt, st.LastCleanedBy, st.LastCleaningJobID = &at, &by, &job
	case err != nil && !errors.Is(err, pgx.ErrNoRows):
		return st, err
	}
	if u.Connection == "online" {
		if st.RunHoursSinceCleaning, err = m.Run.RunHours(ctx, c, u.ID, st.LastCleanedAt); err != nil {
			return st, err
		}
	}
	ratio := -1.0
	switch {
	case st.RunHoursSinceCleaning != nil:
		ratio = *st.RunHoursSinceCleaning / float64(st.ThresholdHours)
	case st.LastCleanedAt != nil:
		ratio = c.Now.Sub(*st.LastCleanedAt).Hours() / 24 / float64(st.FallbackDays)
	}
	switch {
	case ratio >= 1:
		st.Status = "overdue"
	case ratio >= 0.8:
		st.Status = "due_soon"
	case ratio >= 0:
		st.Status = "ok"
	}
	return st, nil
}

var filterRank = map[string]int{"unknown": 0, "ok": 1, "due_soon": 2, "overdue": 3}

// @Summary		filterCare.list (read)
// @ID				filterCare.list
// @Description	Authorization: client:self | technician:assigned
// @Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
// @Description	Recovery: D04: retry only UNAVAILABLE, at most twice
// @Description	Design: DD-C18 · Query: filters customerId,propertyId,spaceId,unitId,status · sort id,status (default status desc;id asc)
// @Tags			filterCare
// @Accept			json
// @Produce		json
// @Param			request	body		paging.Query	true	"input"
// @Success		200		{object}	ops.Envelope{data=FilterStatusPage}
// @Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504		{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/ops/filterCare.list [post]
func (m FilterCare) list(ctx context.Context, c *ops.Call, in *paging.Query) (paging.Page[FilterStatus], error) {
	var f struct {
		CustomerID *uuid.UUID `json:"customerId,omitempty"`
		PropertyID *uuid.UUID `json:"propertyId,omitempty"`
		SpaceID    *uuid.UUID `json:"spaceId,omitempty"`
		UnitID     *uuid.UUID `json:"unitId,omitempty"`
		Status     *string    `json:"status,omitempty"`
	}
	if len(in.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(in.Filters)))
		dec.DisallowUnknownFields()
		if dec.Decode(&f) != nil || (f.Status != nil && filterRank[*f.Status] == 0 && *f.Status != "unknown") {
			return paging.Page[FilterStatus]{}, apperr.Fields(map[string]string{"filters": "error.invalid"})
		}
	}
	desc := true
	if in.Sort != nil {
		if in.Sort.Field != "status" && in.Sort.Field != "id" {
			return paging.Page[FilterStatus]{}, apperr.Fields(map[string]string{"sort.field": "error.invalid"})
		}
		desc = in.Sort.Direction == "desc"
	}
	units, err := m.Units.BriefUnits(ctx, c, f.CustomerID, f.PropertyID, f.SpaceID, f.UnitID)
	if err != nil {
		return paging.Page[FilterStatus]{}, err
	}
	cache := map[uuid.UUID]FilterSettings{}
	out := []FilterStatus{}
	for _, u := range units {
		set, ok := cache[u.OrgID]
		if !ok {
			cust, _, err := m.Units.CustomerOfOrg(ctx, c, u.OrgID)
			if err != nil {
				return paging.Page[FilterStatus]{}, err
			}
			if set, err = m.settings(ctx, c, cust); err != nil {
				return paging.Page[FilterStatus]{}, err
			}
			cache[u.OrgID] = set
		}
		st, err := m.status(ctx, c, u, set)
		if err != nil {
			return paging.Page[FilterStatus]{}, err
		}
		if f.Status == nil || st.Status == *f.Status {
			out = append(out, st)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if in.Sort == nil || in.Sort.Field == "status" {
			if a, b := filterRank[out[i].Status], filterRank[out[j].Status]; a != b {
				return (a > b) == desc
			}
		}
		less := out[i].UnitID.String() < out[j].UnitID.String()
		if in.Sort != nil && in.Sort.Field == "id" && desc {
			return !less
		}
		return less
	})
	return pageOf(out, *in, f, c.Principal.ScopeVersion)
}

// MarkInput is filterCare.markCleaned input.
type MarkInput struct {
	UnitID uuid.UUID `json:"unitId"`
}

// Validate implements ops.Validator.
func (in *MarkInput) Validate() map[string]string {
	if in.UnitID == uuid.Nil {
		return map[string]string{"unitId": "error.required"}
	}
	return nil
}

// @Summary		filterCare.markCleaned (write)
// @ID				filterCare.markCleaned
// @Description	Authorization: client:self
// @Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates
// @Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
// @Description	Design: DD-C18
// @Tags			filterCare
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key	header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Param			request			body		MarkInput	true	"input"
// @Success		200				{object}	ops.Envelope{data=FilterStatus}
// @Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504				{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/ops/filterCare.markCleaned [post]
func (m FilterCare) markCleaned(ctx context.Context, c *ops.Call, in *MarkInput) (FilterStatus, error) {
	org, archived, found, err := m.Units.UnitState(ctx, c, in.UnitID)
	if err != nil {
		return FilterStatus{}, err
	}
	if !found || org != c.Principal.OrgID {
		return FilterStatus{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	if archived {
		return FilterStatus{}, apperr.Fields(map[string]string{"unitId": "error.unitArchived"})
	}
	if _, err := c.Tx.Exec(ctx, `INSERT INTO maintenance.filter_cleanings (tenant_id, unit_id, cleaned_at, marked_by_membership_id) VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3)`,
		in.UnitID, c.Now, c.Principal.MembershipID); err != nil {
		return FilterStatus{}, err
	}
	units, err := m.Units.BriefUnits(ctx, c, nil, nil, nil, &in.UnitID)
	if err != nil || len(units) == 0 {
		return FilterStatus{}, err
	}
	cust, _, err := m.Units.CustomerOfOrg(ctx, c, org)
	if err != nil {
		return FilterStatus{}, err
	}
	set, err := m.settings(ctx, c, cust)
	if err != nil {
		return FilterStatus{}, err
	}
	st, err := m.status(ctx, c, units[0], set)
	if err != nil {
		return st, err
	}
	c.Emit(ops.Event{AggregateType: "unit", AggregateID: in.UnitID, Type: "FilterCleaned"})
	c.Audit(ops.AuditEntry{Action: "filterCare.markCleaned", TargetKind: "unit", TargetID: in.UnitID.String()})
	return st, nil
}

// SettingsInput is filterCare.saveSettings input.
type SettingsInput struct {
	ThresholdHours *int     `json:"thresholdHours"`
	FallbackDays   int      `json:"fallbackDays"`
	Recipients     string   `json:"recipients"`
	Channels       []string `json:"channels"`
}

// Validate implements ops.Validator (IR134 item 4).
func (in *SettingsInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.ThresholdHours != nil && (*in.ThresholdHours < 50 || *in.ThresholdHours > 2000) {
		fe["thresholdHours"] = "error.range"
	}
	if in.FallbackDays < 7 || in.FallbackDays > 180 {
		fe["fallbackDays"] = "error.range"
	}
	if in.Recipients != "owners" && in.Recipients != "all_users" {
		fe["recipients"] = "error.invalid"
	}
	seen := map[string]bool{}
	ok := len(in.Channels) >= 1 && len(in.Channels) <= 2
	for _, ch := range in.Channels {
		ok = ok && (ch == "inApp" || ch == "email") && !seen[ch]
		seen[ch] = true
	}
	if !ok || !slices.Contains(in.Channels, "inApp") {
		fe["channels"] = "error.invalid"
	}
	return fe
}

// @Summary		filterCare.saveSettings (write)
// @ID				filterCare.saveSettings
// @Description	Authorization: client:self-customer:owner
// @Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; IR110 thresholdHours 50–2000 or null (model default); fallbackDays 7–180
// @Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
// @Description	Design: DD-C18
// @Tags			filterCare
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key	header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Param			request			body		SettingsInput	true	"input"
// @Success		200				{object}	ops.Envelope{data=FilterSettings}
// @Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504				{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/ops/filterCare.saveSettings [post]
func (m FilterCare) saveSettings(ctx context.Context, c *ops.Call, in *SettingsInput) (FilterSettings, error) {
	if c.Principal.ClientRole != "owner" {
		return FilterSettings{}, apperr.E(apperr.Forbidden, "error.ownerOnly")
	}
	cust, ok, err := m.Units.CustomerOfOrg(ctx, c, c.Principal.OrgID)
	if err != nil {
		return FilterSettings{}, err
	}
	if !ok {
		return FilterSettings{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	if _, err := c.Tx.Exec(ctx, `INSERT INTO maintenance.filter_care_settings (tenant_id, customer_id, threshold_hours, fallback_days, recipients, channels, updated_at)
		VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $5, $6)
		ON CONFLICT (customer_id) DO UPDATE SET threshold_hours = EXCLUDED.threshold_hours, fallback_days = EXCLUDED.fallback_days, recipients = EXCLUDED.recipients,
			channels = EXCLUDED.channels, version = maintenance.filter_care_settings.version + 1, updated_at = EXCLUDED.updated_at`,
		cust, in.ThresholdHours, in.FallbackDays, in.Recipients, in.Channels, c.Now); err != nil {
		return FilterSettings{}, err
	}
	s, err := m.settings(ctx, c, cust)
	if err != nil {
		return s, err
	}
	c.Emit(ops.Event{AggregateType: "filter_care_settings", AggregateID: s.ID, Type: "FilterCareSettingsSaved"})
	c.Audit(ops.AuditEntry{Action: "filterCare.saveSettings", TargetKind: "filter_care_settings", TargetID: s.ID.String(), NextVersion: &s.Version})
	return s, nil
}

// RegisterFilterCare binds the filter care operations.
func RegisterFilterCare(r *ops.Registry, m FilterCare) {
	ops.Register(r, "filterCare.list", m.list)
	ops.Register(r, "filterCare.markCleaned", m.markCleaned)
	ops.Register(r, "filterCare.saveSettings", m.saveSettings)
}
