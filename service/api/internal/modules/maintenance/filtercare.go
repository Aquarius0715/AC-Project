package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/identity"
	"github.com/pradita/ac-project/service/api/internal/modules/notify"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/events"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

// FilterUnits lists units for filter care (Assets).
type FilterUnits interface {
	BriefUnits(
		ctx context.Context,
		c *ops.Call,
		customer, property, space, unit *uuid.UUID,
	) ([]ops.UnitBrief, error)
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

// The filter statuses of a unit (FilterCareStatus.status), from unknown (no run time and no cleaning) to overdue.
const (
	filterUnknown = "unknown"
	filterOK      = "ok"
	filterDueSoon = "due_soon"
	filterOverdue = "overdue"
)

// reminderChannels are the channels a cleaning reminder goes on.
func reminderChannels() []string { return []string{"inApp", "email"} }

// The defaults of the filter care settings: a cleaning is due 30 days after the last one when the run time is unknown,
// and due soon at 80 % of the threshold or the days.
const (
	defaultFallbackDays = 30
	dueSoonShare        = 0.8
	hoursPerDay         = 24
)

// recipientsOwners sends the cleaning reminders to the customer's owners only (FilterCareSettings.recipients); the
// other choice is all_users.
const recipientsOwners = "owners"

// The ranks of the filter statuses in the status sort, unknown first.
const (
	rankFilterUnknown = iota
	rankFilterOK
	rankFilterDueSoon
	rankFilterOverdue
)

// filterRank is the rank of a filter status in the status sort; a status that is not one ranks with unknown.
func filterRank(status string) int {
	switch status {
	case filterOK:
		return rankFilterOK
	case filterDueSoon:
		return rankFilterDueSoon
	case filterOverdue:
		return rankFilterOverdue
	}

	return rankFilterUnknown
}

// MarkInput is filterCare.markCleaned input.
type MarkInput struct {
	UnitID uuid.UUID `json:"unitId"`
}

// Validate implements ops.Validator.
func (in *MarkInput) Validate() map[string]string {
	if in.UnitID == uuid.Nil {
		return map[string]string{fieldUnitID: apperr.KeyRequired}
	}

	return nil
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
	fieldErrs := map[string]string{}
	if in.ThresholdHours != nil && (*in.ThresholdHours < 50 || *in.ThresholdHours > 2000) {
		fieldErrs["thresholdHours"] = apperr.KeyRange
	}

	if in.FallbackDays < 7 || in.FallbackDays > 180 {
		fieldErrs["fallbackDays"] = apperr.KeyRange
	}

	if in.Recipients != recipientsOwners && in.Recipients != "all_users" {
		fieldErrs["recipients"] = apperr.KeyInvalid
	}

	seen := map[string]bool{}

	valid := len(in.Channels) >= 1 && len(in.Channels) <= len(reminderChannels())
	for _, ch := range in.Channels {
		valid = valid && slices.Contains(reminderChannels(), ch) && !seen[ch]
		seen[ch] = true
	}

	if !valid || !slices.Contains(in.Channels, "inApp") {
		fieldErrs["channels"] = apperr.KeyInvalid
	}

	return fieldErrs
}

// ---- filterCare.getSettings ----

// SettingsGetInput is filterCare.getSettings input: none, the caller's customer.
type SettingsGetInput struct{}

// ---- reminders (scheduler job) ----

// FilterRemindEvery is how often the maintenance scheduler evaluates the filter statuses for reminders (IR239).
const FilterRemindEvery = 15 * time.Minute

// Remind is the maintenance scheduler job of IR134 item 5 (IR239). Every FilterRemindEvery of business time it
// evaluates the filter status of the tenant's units: a cleaning recorded since a reminder ends that cleaning cycle and
// resolves its Alert (FilterCleaningCleared — Mark cleaned does so at once, a technician cleaning here); an overdue
// unit without a reminder in its cycle gets one — the maintenance Alert (FilterCleaningDue, opened by equipment) and
// one cleaning_due notification per recipient and channel of the customer's settings.
func (m FilterCare) Remind(ctx context.Context, call *ops.Call) (int, error) {
	var when time.Time

	err := call.Tx.QueryRow(ctx, `SELECT evaluated_at FROM maintenance.filter_reminder_watermarks WHERE tenant_id = $1 FOR
		UPDATE`, call.Principal.TenantID).
		Scan(&when)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return 0, apperr.Wrap(err, "select maintenance.filter_reminder_watermarks")
	case call.Now.Sub(when) < FilterRemindEvery:
		return 0, nil
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`INSERT INTO maintenance.filter_reminder_watermarks (tenant_id, evaluated_at) VALUES ($1, $2)
		ON CONFLICT (tenant_id) DO UPDATE SET evaluated_at = EXCLUDED.evaluated_at`,
		call.Principal.TenantID,
		call.Now,
	)
	if execErr != nil {
		return 0, apperr.Wrap(execErr, "insert maintenance.filter_reminder_watermarks")
	}

	units, err := m.Units.BriefUnits(ctx, call, nil, nil, nil, nil)
	if err != nil {
		return 0, apperr.Wrap(err, "maintenance.FilterUnits.BriefUnits")
	}

	cache := map[uuid.UUID]FilterSettings{}
	count := 0

	for _, unitBrief := range units {
		set, ok := cache[unitBrief.OrgID]
		if !ok {
			cust, found, err := m.Units.CustomerOfOrg(ctx, call, unitBrief.OrgID)
			if err != nil {
				return count, apperr.Wrap(err, "maintenance.FilterUnits.CustomerOfOrg")
			}

			if !found {
				continue
			}

			set, err = m.settings(ctx, call, cust)
			if err != nil {
				return count, err
			}

			cache[unitBrief.OrgID] = set
		}

		filterStatus, err := m.status(ctx, call, unitBrief, set)
		if err != nil {
			return count, err
		}

		k, err := clearReminders(ctx, call, unitBrief.ID, filterStatus.LastCleanedAt)
		if count += k; err != nil {
			return count, err
		}

		if filterStatus.Status == filterOverdue {
			k, err := remind(ctx, call, unitBrief, set, filterStatus)
			if count += k; err != nil {
				return count, err
			}
		}
	}

	return count, nil
}

func (FilterCare) settings(
	ctx context.Context,
	call *ops.Call,
	customer uuid.UUID,
) (FilterSettings, error) {
	filterSettings := FilterSettings{
		TenantID:     call.Principal.TenantID,
		Version:      0,
		CustomerID:   customer,
		FallbackDays: defaultFallbackDays,
		Recipients:   recipientsOwners,
		Channels: []string{
			"inApp",
		},
		ID:             uuid.UUID{},
		CreatedAt:      time.Time{},
		UpdatedAt:      time.Time{},
		ThresholdHours: nil,
	}

	err := call.Tx.QueryRow(ctx, `SELECT id, version, updated_at, threshold_hours, fallback_days, recipients, channels FROM
		maintenance.filter_care_settings WHERE customer_id = $1`, customer).
		Scan(&filterSettings.ID, &filterSettings.Version, &filterSettings.UpdatedAt, &filterSettings.ThresholdHours,
			&filterSettings.FallbackDays, &filterSettings.Recipients, &filterSettings.Channels)
	if errors.Is(err, pgx.ErrNoRows) {
		return filterSettings, nil
	}

	filterSettings.CreatedAt = filterSettings.UpdatedAt

	return filterSettings, apperr.Wrap(err, "select maintenance.filter_care_settings")
}

// status computes FilterCareStatus for one unit (IR134 items 1–3).
func (m FilterCare) status(
	ctx context.Context,
	call *ops.Call,
	unitBrief ops.UnitBrief,
	set FilterSettings,
) (FilterStatus, error) {
	filterStatus := FilterStatus{
		UnitID:                unitBrief.ID,
		ThresholdHours:        DefaultFilterHours,
		FallbackDays:          set.FallbackDays,
		Status:                filterUnknown,
		RunHoursSinceCleaning: nil,
		LastCleanedAt:         nil,
		LastCleanedBy:         nil,
		LastCleaningJobID:     nil,
	}
	if set.ThresholdHours != nil {
		filterStatus.ThresholdHours = *set.ThresholdHours
	}

	var when time.Time

	scanErr := call.Tx.QueryRow(ctx, `SELECT max(cleaned_at) FROM maintenance.filter_cleanings WHERE unit_id =
		$1`, unitBrief.ID).
		Scan(&filterStatus.LastCleanedAt)
	if scanErr != nil {
		return filterStatus, apperr.Wrap(scanErr, "select maintenance.filter_cleanings")
	}

	if filterStatus.LastCleanedAt != nil {
		by := "customer"
		filterStatus.LastCleanedBy = &by
	}

	var job uuid.UUID

	err := call.Tx.QueryRow(ctx, `SELECT j.id, j.completed_at FROM maintenance.jobs j WHERE j.unit_id = $1 AND j.status =
		'completed' AND EXISTS (
		SELECT 1 FROM maintenance.work_reports w WHERE w.job_id = j.id AND w.state = 'accepted' AND w.items @>
		'[{"componentKey":"filter","result":"normal"}]')
		ORDER BY j.completed_at DESC LIMIT 1`, unitBrief.ID).
		Scan(&job, &when)
	switch {
	case err == nil && (filterStatus.LastCleanedAt == nil || when.After(*filterStatus.LastCleanedAt)):
		by := "technician"
		filterStatus.LastCleanedAt, filterStatus.LastCleanedBy, filterStatus.LastCleaningJobID = &when, &by, &job
	case err != nil && !errors.Is(err, pgx.ErrNoRows):
		return filterStatus, apperr.Wrap(err, "select maintenance.jobs")
	}

	if unitBrief.Connection == "online" {
		filterStatus.RunHoursSinceCleaning, err = m.Run.RunHours(
			ctx,
			call,
			unitBrief.ID,
			filterStatus.LastCleanedAt,
		)
		if err != nil {
			return filterStatus, apperr.Wrap(err, "maintenance.RunTime.RunHours")
		}
	}

	ratio := -1.0

	switch {
	case filterStatus.RunHoursSinceCleaning != nil:
		ratio = *filterStatus.RunHoursSinceCleaning / float64(filterStatus.ThresholdHours)
	case filterStatus.LastCleanedAt != nil:
		ratio = call.Now.Sub(*filterStatus.LastCleanedAt).
			Hours() /
			hoursPerDay / float64(
			filterStatus.FallbackDays,
		)
	}

	switch {
	case ratio >= 1:
		filterStatus.Status = filterOverdue
	case ratio >= dueSoonShare:
		filterStatus.Status = filterDueSoon
	case ratio >= 0:
		filterStatus.Status = filterOK
	}

	return filterStatus, nil
}

// list serves filterCare.list.
//
//	@Summary		filterCare.list (read)
//	@ID				filterCare.list
//	@Description	Authorization: client:self | technician:assigned
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-C18 · Query: filters customerId,propertyId,spaceId,unitId,status · sort id,status \
//	@Description	(default status desc;id asc)
//	@Tags			filterCare
//	@Accept			json
//	@Produce		json
//	@Param			cursor		query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit		query		integer	false	"page size 1–100, default 25"
//	@Param			sort		query		string	false	"field:direction — fields id,status; default status desc;id asc"
//	@Param			customerId	query		string	false	"filter → unit location"
//	@Param			propertyId	query		string	false	"filter → unit location"
//	@Param			spaceId		query		string	false	"filter → unit location"
//	@Param			unitId		query		string	false	"filter → unit location"
//	@Param			status		query		string	false	"filter → status"
//	@Success		200			{object}	ops.Envelope{data=FilterStatusPage}
//	@Failure		401			{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403			{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404			{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409			{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422			{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429			{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503			{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504			{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/filter-care [get]
func (m FilterCare) list(
	ctx context.Context,
	call *ops.Call,
	query *paging.Query,
) (paging.Page[FilterStatus], error) {
	var filters struct {
		CustomerID *uuid.UUID `json:"customerId,omitempty"`
		PropertyID *uuid.UUID `json:"propertyId,omitempty"`
		SpaceID    *uuid.UUID `json:"spaceId,omitempty"`
		UnitID     *uuid.UUID `json:"unitId,omitempty"`
		Status     *string    `json:"status,omitempty"`
	}

	if len(query.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(query.Filters)))
		dec.DisallowUnknownFields()

		if dec.Decode(&filters) != nil ||
			(filters.Status != nil && filterRank(*filters.Status) == rankFilterUnknown && *filters.Status != filterUnknown) {
			return paging.Page[FilterStatus]{}, apperr.Fields(
				map[string]string{fieldFilters: apperr.KeyInvalid},
			)
		}
	}

	desc := true

	if query.Sort != nil {
		if query.Sort.Field != fieldStatus && query.Sort.Field != "id" {
			return paging.Page[FilterStatus]{}, apperr.Fields(
				map[string]string{"sort.field": apperr.KeyInvalid},
			)
		}

		desc = query.Sort.Direction == sortDesc
	}

	units, err := m.Units.BriefUnits(
		ctx,
		call,
		filters.CustomerID,
		filters.PropertyID,
		filters.SpaceID,
		filters.UnitID,
	)
	if err != nil {
		return paging.Page[FilterStatus]{}, apperr.Wrap(err, "maintenance.FilterUnits.BriefUnits")
	}

	out, err := m.statuses(ctx, call, units, filters.Status)
	if err != nil {
		return paging.Page[FilterStatus]{}, err
	}

	sortFilterStatuses(out, query.Sort, desc)

	return pageOf(out, *query, filters, call.Principal.ScopeVersion)
}

// statuses are the units' filter statuses, with the status filter; the settings are read once per customer.
func (m FilterCare) statuses(
	ctx context.Context,
	call *ops.Call,
	units []ops.UnitBrief,
	status *string,
) ([]FilterStatus, error) {
	cache := map[uuid.UUID]FilterSettings{}
	out := []FilterStatus{}

	for _, unitBrief := range units {
		set, ok := cache[unitBrief.OrgID]
		if !ok {
			cust, _, err := m.Units.CustomerOfOrg(ctx, call, unitBrief.OrgID)
			if err != nil {
				return nil, apperr.Wrap(
					err,
					"maintenance.FilterUnits.CustomerOfOrg",
				)
			}

			set, err = m.settings(ctx, call, cust)
			if err != nil {
				return nil, err
			}

			cache[unitBrief.OrgID] = set
		}

		filterStatus, err := m.status(ctx, call, unitBrief, set)
		if err != nil {
			return nil, err
		}

		if status == nil || filterStatus.Status == *status {
			out = append(out, filterStatus)
		}
	}

	return out, nil
}

// sortFilterStatuses orders the statuses by rank (unknown, ok, due soon, overdue; desc unless asked otherwise) and then
// by unit, or by unit only for the id sort.
func sortFilterStatuses(out []FilterStatus, sortBy *paging.SortSpec, desc bool) {
	sort.SliceStable(out, func(left, right int) bool {
		if sortBy == nil || sortBy.Field == fieldStatus {
			if a, b := filterRank(out[left].Status), filterRank(out[right].Status); a != b {
				return (a > b) == desc
			}
		}

		less := out[left].UnitID.String() < out[right].UnitID.String()
		if sortBy != nil && sortBy.Field == "id" && desc {
			return !less
		}

		return less
	})
}

// markCleaned serves filterCare.markCleaned.
//
//	@Summary		filterCare.markCleaned (write)
//	@ID				filterCare.markCleaned
//	@Description	Authorization: client:self
//	@Description	Validation: D01; input constraints in the corresponding DD
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-C18
//	@Tags			filterCare
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			unitId			path		string		true	"input field unitId"
//	@Param			request			body		MarkInput	true	"input; the path parameters come from the route"
//	@Success		200				{object}	ops.Envelope{data=FilterStatus}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/filter-care/{unitId}/mark-cleaned [post]
func (m FilterCare) markCleaned(
	ctx context.Context,
	call *ops.Call,
	input *MarkInput,
) (FilterStatus, error) {
	org, archived, found, err := m.Units.UnitState(ctx, call, input.UnitID)
	if err != nil {
		return FilterStatus{}, apperr.Wrap(err, "maintenance.FilterUnits.UnitState")
	}

	if !found || org != call.Principal.OrgID {
		return FilterStatus{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if archived {
		return FilterStatus{}, apperr.Fields(map[string]string{fieldUnitID: apperr.KeyUnitArchived})
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`INSERT INTO maintenance.filter_cleanings (tenant_id, unit_id, cleaned_at, marked_by_membership_id) VALUES
			(current_setting('app.tenant_id')::uuid, $1, $2, $3)`,
		input.UnitID,
		call.Now,
		call.Principal.MembershipID,
	)
	if execErr != nil {
		return FilterStatus{}, apperr.Wrap(execErr, "insert maintenance.filter_cleanings")
	}

	units, err := m.Units.BriefUnits(ctx, call, nil, nil, nil, &input.UnitID)
	if err != nil || len(units) == 0 {
		return FilterStatus{}, apperr.Wrap(err, "maintenance.FilterUnits.BriefUnits")
	}

	cust, _, err := m.Units.CustomerOfOrg(ctx, call, org)
	if err != nil {
		return FilterStatus{}, apperr.Wrap(err, "maintenance.FilterUnits.CustomerOfOrg")
	}

	set, err := m.settings(ctx, call, cust)
	if err != nil {
		return FilterStatus{}, err
	}

	filterStatus, err := m.status(ctx, call, units[0], set)
	if err != nil {
		return filterStatus, err
	}

	_, clearRemindersErr := clearReminders(
		ctx,
		call,
		input.UnitID,
		filterStatus.LastCleanedAt,
	)
	if clearRemindersErr != nil { // the cleaning ends the reminder's cycle (IR239)
		return filterStatus, clearRemindersErr
	}

	call.Emit(outboxEvent("unit", input.UnitID, "FilterCleaned", nil))
	call.Audit(auditEntry("filterCare.markCleaned", "unit", input.UnitID, nil, nil, ""))

	return filterStatus, nil
}

// saveSettings serves filterCare.saveSettings.
//
//	@Summary		filterCare.saveSettings (write)
//	@ID				filterCare.saveSettings
//	@Description	Authorization: client:self-customer:owner
//	@Description	Validation: D01; input constraints in the corresponding DD; IR110 thresholdHours 50–2000 or null \
//	@Description	(model default); fallbackDays 7–180
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-C18
//	@Tags			filterCare
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		SettingsInput	true	"input"
//	@Success		200				{object}	ops.Envelope{data=FilterSettings}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/filter-care/settings [put]
func (m FilterCare) saveSettings(
	ctx context.Context,
	call *ops.Call,
	input *SettingsInput,
) (FilterSettings, error) {
	if call.Principal.ClientRole != "owner" {
		return FilterSettings{}, apperr.E(apperr.Forbidden, "error.ownerOnly")
	}

	cust, ok, err := m.Units.CustomerOfOrg(ctx, call, call.Principal.OrgID)
	if err != nil {
		return FilterSettings{}, apperr.Wrap(err, "maintenance.FilterUnits.CustomerOfOrg")
	}

	if !ok {
		return FilterSettings{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`INSERT INTO maintenance.filter_care_settings (tenant_id, customer_id, threshold_hours, fallback_days, recipients,
			channels, updated_at)
		VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $5, $6)
		ON CONFLICT (customer_id) DO UPDATE SET threshold_hours = EXCLUDED.threshold_hours, fallback_days =
		EXCLUDED.fallback_days, recipients = EXCLUDED.recipients,
			channels = EXCLUDED.channels, version = maintenance.filter_care_settings.version + 1, updated_at =
			EXCLUDED.updated_at`,
		cust,
		input.ThresholdHours,
		input.FallbackDays,
		input.Recipients,
		input.Channels,
		call.Now,
	)
	if execErr != nil {
		return FilterSettings{}, apperr.Wrap(execErr, "insert maintenance.filter_care_settings")
	}

	filterSettings, err := m.settings(ctx, call, cust)
	if err != nil {
		return filterSettings, err
	}

	call.Emit(
		outboxEvent("filter_care_settings", filterSettings.ID, "FilterCareSettingsSaved", nil),
	)
	call.Audit(
		auditEntry(
			"filterCare.saveSettings",
			"filter_care_settings",
			filterSettings.ID,
			nil,
			&filterSettings.Version,
			"",
		),
	)

	return filterSettings, nil
}

// getSettings serves filterCare.getSettings.
//
//	@Summary		filterCare.getSettings (read)
//	@ID				filterCare.getSettings
//	@Description	Authorization: client:self
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot; IR238 the \
//	@Description	customer's settings or the defaults (version 0)
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-C18
//	@Tags			filterCare
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	ops.Envelope{data=FilterSettings}
//	@Failure		401	{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403	{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404	{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409	{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422	{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429	{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503	{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504	{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/filter-care/settings [get]
func (m FilterCare) getSettings(
	ctx context.Context,
	call *ops.Call,
	_ *SettingsGetInput,
) (FilterSettings, error) {
	cust, ok, err := m.Units.CustomerOfOrg(ctx, call, call.Principal.OrgID)
	if err != nil {
		return FilterSettings{}, apperr.Wrap(err, "maintenance.FilterUnits.CustomerOfOrg")
	}

	if !ok {
		return FilterSettings{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	return m.settings(ctx, call, cust)
}

// clearReminders ends the open reminders of the unit whose cycle a later cleaning ended (last = the unit's last
// cleaning now) and asks equipment to resolve their Alerts.
func clearReminders(
	ctx context.Context,
	call *ops.Call,
	unit uuid.UUID,
	last *time.Time,
) (int, error) {
	rows, err := call.Tx.Query(
		ctx,
		`UPDATE maintenance.filter_reminders SET cleared_at = $3 WHERE unit_id = $1 AND cleared_at IS NULL AND cycle_from IS
			DISTINCT FROM $2
		RETURNING id, customer_org_id`,
		unit,
		last,
		call.Now,
	)
	if err != nil {
		return 0, apperr.Wrap(err, "update maintenance.filter_reminders")
	}

	type ended struct{ id, org uuid.UUID }

	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (ended, error) {
		var e ended

		return e, r.Scan(&e.id, &e.org)
	})
	for _, cleared := range list {
		call.Emit(
			outboxEvent("alert", cleared.id, events.FilterCleaningCleared, events.FilterReminder{
				AlertID:       cleared.id,
				UnitID:        unit,
				CustomerOrgID: cleared.org,
				At:            call.Now, Evidence: "", NoRecipient: false,
			}),
		)
	}

	return len(list), err
}

// remind raises the reminder of the unit's current cleaning cycle once: the Alert and the notifications to the owners
// (or all client users) of the customer by the settings' channels.
func remind(
	ctx context.Context,
	call *ops.Call,
	unitBrief ops.UnitBrief,
	set FilterSettings,
	filterStatus FilterStatus,
) (int, error) {
	alertID := uuid.Must(uuid.NewV7())

	tag, err := call.Tx.Exec(
		ctx,
		`INSERT INTO maintenance.filter_reminders (id, tenant_id, unit_id, customer_org_id, cycle_from, raised_at)
		VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5) ON CONFLICT DO NOTHING`,
		alertID,
		unitBrief.ID,
		unitBrief.OrgID,
		filterStatus.LastCleanedAt,
		call.Now,
	)
	if err != nil || tag.RowsAffected() == 0 { // this cycle was reminded already
		return 0, apperr.Wrap(err, "insert maintenance.filter_reminders")
	}

	membersInput := identity.MembersInput{
		Role:           roleClient,
		OrganizationID: &unitBrief.OrgID,
		IDs:            nil,
		Permission:     "",
		ClientRole:     "",
	}
	if set.Recipients == recipientsOwners {
		membersInput.ClientRole = "owner"
	}

	recipients, err := identity.Members(ctx, call, membersInput)
	if err != nil {
		return 0, apperr.Wrap(err, "identity.Members")
	}

	evidence := reminderEvidence(filterStatus, call.Now)
	sent := 0

	for _, r := range recipients {
		for _, ch := range set.Channels {
			_, err := (notify.Store{}).Create(ctx, call, notify.New{
				RecipientMembershipID: r,
				Channel:               ch,
				Type:                  "cleaning_due",
				TemplateKey:           "alert",
				TargetKind:            "unit",
				TargetID:              unitBrief.ID,
				Severity:              severityNormal,
				SourceAlertID:         &alertID,
				Params:                map[string]any{fieldStatus: "open", "message": evidence},
			})
			if err != nil {
				return 0, apperr.Wrap(err, "notify.Store.Create")
			}

			sent++
		}
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE maintenance.filter_reminders SET notifications = $2 WHERE id = $1`,
		alertID,
		sent,
	)
	if execErr != nil {
		return 0, apperr.Wrap(execErr, "update maintenance.filter_reminders")
	}

	call.Emit(outboxEvent("alert", alertID, events.FilterCleaningDue, events.FilterReminder{
		AlertID:       alertID,
		UnitID:        unitBrief.ID,
		CustomerOrgID: unitBrief.OrgID,
		Evidence:      evidence,
		NoRecipient:   sent == 0,
		At:            call.Now,
	}))
	call.Audit(auditEntry("filterCare.remind", "unit", unitBrief.ID, nil, nil, "cleaning due"))

	return 1, nil
}

// reminderEvidence is the Alert's evidence and the notification's message (Figma Client 06a).
func reminderEvidence(filterStatus FilterStatus, now time.Time) string {
	switch {
	case filterStatus.RunHoursSinceCleaning != nil && filterStatus.LastCleanedAt != nil:
		return fmt.Sprintf(
			"Cleaning due (%.0f h of run time since the last cleaning). Not a fault.",
			*filterStatus.RunHoursSinceCleaning,
		)
	case filterStatus.RunHoursSinceCleaning != nil:
		return fmt.Sprintf(
			"Cleaning due (%.0f h of run time, no cleaning recorded). Not a fault.",
			*filterStatus.RunHoursSinceCleaning,
		)
	case filterStatus.LastCleanedAt != nil:
		return fmt.Sprintf(
			"Cleaning due (%d days since the last cleaning, run time unknown). Not a fault.",
			int(now.Sub(*filterStatus.LastCleanedAt).Hours()/hoursPerDay),
		)
	}

	return "Cleaning due. Not a fault."
}

// RegisterFilterCare binds the filter care operations.
func RegisterFilterCare(r *ops.Registry, m FilterCare) {
	ops.Register(r, "filterCare.list", m.list)
	ops.Register(r, "filterCare.getSettings", m.getSettings)
	ops.Register(r, "filterCare.markCleaned", m.markCleaned)
	ops.Register(r, "filterCare.saveSettings", m.saveSettings)
}

// CustomerHandlers apply CustomerDeleted (consumer maintenance, IR739, DEC-103): a deleted customer's filter care
// settings go with it.
func CustomerHandlers() map[string]events.Handler {
	return map[string]events.Handler{
		events.CustomerDeleted: func(ctx context.Context, txn pgx.Tx, event events.Event) error {
			_, err := txn.Exec(
				ctx,
				`DELETE FROM maintenance.filter_care_settings WHERE customer_id = $1`,
				event.AggregateID,
			)

			return apperr.Wrap(err, "delete maintenance.filter_care_settings")
		},
	}
}
