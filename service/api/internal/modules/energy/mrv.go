package energy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

// Conditions is MRVConditions.
type Conditions struct {
	From            time.Time   `json:"from"`
	To              time.Time   `json:"to"`
	UnitIDs         []uuid.UUID `json:"unitIds"`
	BaselineID      uuid.UUID   `json:"baselineId"`
	BaselineVersion int         `json:"baselineVersion"`
	FactorID        uuid.UUID   `json:"factorId"`
	FactorVersion   int         `json:"factorVersion"`
	BoundaryID      string      `json:"boundaryId"`
	Boundary        string      `json:"boundary"`
	OrganizationID  uuid.UUID   `json:"organizationId"`
}

// Validate implements ops.Validator (mrv.preview input).
func (in *Conditions) Validate() map[string]string {
	fe := map[string]string{}
	in.check(fe, "")

	return fe
}

func (in *Conditions) check(fieldErrs map[string]string, prefix string) {
	if CheckRange(in.From, in.To) != nil {
		fieldErrs[prefix+"from"] = keyEnergyRange
	}

	seen := map[uuid.UUID]bool{}
	for _, u := range in.UnitIDs {
		if u == uuid.Nil || seen[u] {
			fieldErrs[prefix+keyUnitIDs] = apperr.KeyInvalid
		}

		seen[u] = true
	}

	if len(in.UnitIDs) == 0 || len(in.UnitIDs) > 100 {
		fieldErrs[prefix+keyUnitIDs] = apperr.KeyInvalid
	}

	if in.BaselineID == uuid.Nil || in.BaselineVersion < 1 {
		fieldErrs[prefix+"baselineId"] = apperr.KeyRequired
	}

	if in.FactorID == uuid.Nil || in.FactorVersion < 1 {
		fieldErrs[prefix+"factorId"] = apperr.KeyRequired
	}

	if !slices.Contains(boundaryIDs(), in.BoundaryID) {
		fieldErrs[prefix+"boundaryId"] = apperr.KeyInvalid
	}

	in.Boundary = strings.TrimSpace(in.Boundary)
	if n := utf8.RuneCountInString(in.Boundary); n < 1 || n > 500 {
		fieldErrs[prefix+"boundary"] = apperr.KeyLength
	}

	if in.OrganizationID == uuid.Nil {
		fieldErrs[prefix+"organizationId"] = apperr.KeyRequired
	}
}

// Preview is MRVPreview.
type Preview struct {
	Conditions Conditions `json:"conditions"`
	Summary    Summary    `json:"summary"`
	Incomplete bool       `json:"incomplete"`
	Scope      string     `json:"scope"`
	IsDemo     bool       `json:"isDemo"`
}

// preview computes the MRV result for the conditions with the referenced baseline and factor versions (SR09,
// IR149): every unit must be a not archived unit of the customer organization (VALIDATION); unknown versions are
// NOT_FOUND. incomplete when the boundary differs from the actual boundary or the baseline, coverage < 1, or the
// baseline is not comparable.
//
//	@Summary		mrv.preview (read)
//	@ID				mrv.preview
//	@Description	Authorization: admin:mrv.read
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A14
//	@Tags			mrv
//	@Accept			json
//	@Produce		json
//	@Param			from			query		string		false	"input field from"
//	@Param			to				query		string		false	"input field to"
//	@Param			unitIds			query		[]string	false	"input field unitIds (repeat the parameter or separate values with commas; an empty value is the empty list)"	collectionFormat(multi)
//	@Param			baselineId		query		string		false	"input field baselineId"
//	@Param			baselineVersion	query		integer		false	"input field baselineVersion"
//	@Param			factorId		query		string		false	"input field factorId"
//	@Param			factorVersion	query		integer		false	"input field factorVersion"
//	@Param			boundaryId		query		string		false	"input field boundaryId"
//	@Param			boundary		query		string		false	"input field boundary"
//	@Param			organizationId	query		string		false	"input field organizationId"
//	@Success		200				{object}	ops.Envelope{data=Preview}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/mrv/preview [get]
func preview(ctx context.Context, call *ops.Call, input *Conditions) (Preview, error) {
	var kind string

	err := call.Tx.QueryRow(ctx, `SELECT kind FROM energy.ref_organizations WHERE id = $1`, input.OrganizationID).
		Scan(&kind)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && kind != "customer") {
		return Preview{}, apperr.Fields(map[string]string{"organizationId": apperr.KeyInvalid})
	}

	if err != nil {
		return Preview{}, apperr.Wrap(err, "select energy.ref_organizations")
	}

	var inOrg int

	scanErr := call.Tx.QueryRow(ctx, `SELECT count(*) FROM energy.ref_units WHERE id = ANY($1) AND customer_org_id = $2 AND
		NOT archived`, input.UnitIDs, input.OrganizationID).
		Scan(&inOrg)
	if scanErr != nil {
		return Preview{}, apperr.Wrap(scanErr, "select energy.ref_units")
	}

	if inOrg != len(input.UnitIDs) {
		return Preview{}, apperr.Fields(
			map[string]string{keyUnitIDs: "errors.units_outside_organization"},
		)
	}

	baseline, err := LoadBaseline(ctx, call, input.BaselineID, &input.BaselineVersion)
	if err != nil {
		return Preview{}, err
	}

	factor, err := LoadFactor(ctx, call, input.FactorID, &input.FactorVersion)
	if err != nil {
		return Preview{}, err
	}

	computed, integration, err := Compute(
		ctx,
		call,
		input.UnitIDs,
		input.From,
		input.To,
		&factor,
		&baseline,
	)
	if err != nil {
		return Preview{}, err
	}

	incomplete := input.BoundaryID != Boundary || baseline.BoundaryID != input.BoundaryID ||
		integration.ExpectedSlots == 0 ||
		integration.ValidSlots < integration.ExpectedSlots ||
		!Comparable(&baseline, input.UnitIDs, input.From, input.To, integration)

	return Preview{
		Conditions: *input,
		Summary:    computed,
		Incomplete: incomplete,
		Scope:      "scope_2",
		IsDemo:     true,
	}, nil
}

// Review is a reviewHistory entry.
type Review struct {
	UserID        uuid.UUID `json:"userId"`
	ReportVersion int       `json:"reportVersion"`
	Comment       string    `json:"comment"`
	At            time.Time `json:"at"`
}

// Report is MRVReport.
type Report struct {
	ID            uuid.UUID   `json:"id"`
	TenantID      uuid.UUID   `json:"tenantId"`
	Version       int         `json:"version"`
	CreatedAt     time.Time   `json:"createdAt"`
	UpdatedAt     time.Time   `json:"updatedAt"`
	Conditions    Conditions  `json:"conditions"`
	Summary       Summary     `json:"summary"`
	Incomplete    bool        `json:"incomplete"`
	Scope         string      `json:"scope"`
	IsDemo        bool        `json:"isDemo"`
	Status        string      `json:"status"`
	EvidenceIDs   []uuid.UUID `json:"evidenceIds"`
	ReviewHistory []Review    `json:"reviewHistory"`
}

type stored struct {
	Summary    Summary `json:"summary"`
	Incomplete bool    `json:"incomplete"`
}

const reportCols = `r.id, r.tenant_id, r.version, (SELECT min(created_at) FROM energy.mrv_reports o WHERE o.id = ` +
	`r.id), r.created_at, r.conditions, r.results, r.status, r.evidence_ids`

func scanReport(row pgx.Row) (Report, error) {
	var (
		report    Report
		cond, res []byte
	)

	err := row.Scan(
		&report.ID,
		&report.TenantID,
		&report.Version,
		&report.CreatedAt,
		&report.UpdatedAt,
		&cond,
		&res,
		&report.Status,
		&report.EvidenceIDs,
	)
	if err != nil {
		return report, apperr.Wrap(err, "scan (scanReport)")
	}

	var st stored

	_ = json.Unmarshal(cond, &report.Conditions)
	_ = json.Unmarshal(res, &st)
	report.Summary, report.Incomplete, report.Scope, report.IsDemo = st.Summary, st.Incomplete, "scope_2", true

	return report, nil
}

func reviews(ctx context.Context, c *ops.Call, report *Report) error {
	rows, err := c.Tx.Query(
		ctx,
		`SELECT user_id, report_version, comment, reviewed_at FROM energy.mrv_reviews WHERE report_id = $1 AND report_version
			< $2 ORDER BY reviewed_at, id`,
		report.ID,
		report.Version,
	)
	if err != nil {
		return apperr.Wrap(err, "select energy.mrv_reviews")
	}
	defer rows.Close()

	report.ReviewHistory = []Review{}

	for rows.Next() {
		var review Review

		err := rows.Scan(&review.UserID, &review.ReportVersion, &review.Comment, &review.At)
		if err != nil {
			return apperr.Wrap(err, "select energy.mrv_reviews")
		}

		report.ReviewHistory = append(report.ReviewHistory, review)
	}

	return apperr.Wrap(rows.Err(), "select energy.mrv_reviews")
}

func latestVersion(ctx context.Context, c *ops.Call, id uuid.UUID) (int, error) {
	var latest *int

	err := c.Tx.QueryRow(ctx, `SELECT max(version) FROM energy.mrv_reports WHERE id = $1`, id).
		Scan(&latest)
	if err != nil {
		return 0, apperr.Wrap(err, "select energy.mrv_reports")
	}

	if latest == nil {
		return 0, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	return *latest, nil
}

// LoadReport returns a report version (latest when version is nil).
func LoadReport(
	ctx context.Context,
	call *ops.Call,
	reportID uuid.UUID,
	version *int,
) (Report, error) {
	var reportVersion int
	if version != nil {
		reportVersion = *version
	} else {
		latest, err := latestVersion(ctx, call, reportID)
		if err != nil {
			return Report{}, err
		}

		reportVersion = latest
	}

	report, err := scanReport(
		call.Tx.QueryRow(
			ctx,
			"SELECT "+reportCols+" FROM energy.mrv_reports r WHERE r.id = $1 AND r.version = $2",
			reportID,
			reportVersion,
		),
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return report, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return report, err
	}

	err = reviews(ctx, call, &report)

	return report, err
}

// GetInput is mrv.get input.
type GetInput struct {
	ID            uuid.UUID `json:"id"`
	ReportVersion *int      `json:"reportVersion,omitempty"`
}

// Validate implements ops.Validator.
func (in *GetInput) Validate() map[string]string {
	if in.ID == uuid.Nil || (in.ReportVersion != nil && *in.ReportVersion < 1) {
		return map[string]string{"id": apperr.KeyInvalid}
	}

	return nil
}

// getReport serves mrv.get.
//
//	@Summary		mrv.get (read)
//	@ID				mrv.get
//	@Description	Authorization: admin:mrv.read
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A14
//	@Tags			mrv
//	@Accept			json
//	@Produce		json
//	@Param			id				path		string	true	"input field id"
//	@Param			reportVersion	query		integer	false	"input field reportVersion"
//	@Success		200				{object}	ops.Envelope{data=Report}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/mrv/{id} [get]
func getReport(ctx context.Context, c *ops.Call, in *GetInput) (Report, error) {
	return LoadReport(ctx, c, in.ID, in.ReportVersion)
}

func pageReports(
	ctx context.Context,
	call *ops.Call,
	query paging.Query,
	f any,
	where string,
	args []any,
	order string,
) (paging.Page[Report], error) {
	window, err := paging.Resolve(query, f, call.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Report]{}, apperr.Wrap(err, "paging.Resolve")
	}

	var total int

	scanErr := call.Tx.QueryRow(ctx, "SELECT count(*) FROM energy.mrv_reports r WHERE "+where, args...).
		Scan(&total)
	if scanErr != nil {
		return paging.Page[Report]{}, apperr.Wrap(scanErr, "select energy.mrv_reports")
	}

	rows, err := call.Tx.Query(
		ctx,
		fmt.Sprintf(
			"SELECT %s FROM energy.mrv_reports r WHERE %s ORDER BY %s LIMIT %d OFFSET %d",
			reportCols,
			where,
			order,
			window.Limit,
			window.Offset,
		),
		args...)
	if err != nil {
		return paging.Page[Report]{}, apperr.Wrap(err, "select energy.mrv_reports")
	}

	items := []Report{}

	for rows.Next() {
		report, err := scanReport(rows)
		if err != nil {
			rows.Close()

			return paging.Page[Report]{}, err
		}

		items = append(items, report)
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return paging.Page[Report]{}, apperr.Wrap(rowsErr, "select energy.mrv_reports")
	}

	for i := range items {
		err := reviews(ctx, call, &items[i])
		if err != nil {
			return paging.Page[Report]{}, err
		}
	}

	return paging.Page[Report]{
		Items:           items,
		NextCursor:      window.Next(total),
		Total:           total,
		SnapshotVersion: window.Snapshot,
	}, nil
}

// listReports serves mrv.list.
//
//	@Summary		mrv.list (read)
//	@ID				mrv.list
//	@Description	Authorization: admin:mrv.read
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A14 · Query: filters unitId,from,to,status,organizationId · sort \
//	@Description	id,createdAt,updatedAt,periodFrom (default createdAt desc;id desc)
//	@Tags			mrv
//	@Accept			json
//	@Produce		json
//	@Param			cursor			query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit			query		integer	false	"page size 1–100, default 25"
//	@Param			sort			query		string	false	"field:direction — fields id,createdAt,updatedAt,periodFrom; default createdAt desc;id desc"
//	@Param			unitId			query		string	false	"filter → conditions.unitIds contains"
//	@Param			from			query		string	false	"filter → [from,to) on conditions.from"
//	@Param			to				query		string	false	"filter → [from,to) on conditions.from"
//	@Param			status			query		string	false	"filter → status"
//	@Param			organizationId	query		string	false	"filter → conditions.organizationId"
//	@Success		200				{object}	ops.Envelope{data=ReportPage}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/mrv [get]
func listReports(
	ctx context.Context,
	call *ops.Call,
	query *paging.Query,
) (paging.Page[Report], error) {
	var filter struct {
		OrganizationID *uuid.UUID `json:"organizationId,omitempty"`
		UnitID         *uuid.UUID `json:"unitId,omitempty"`
		Status         *string    `json:"status,omitempty"`
		From           *time.Time `json:"from,omitempty"`
		To             *time.Time `json:"to,omitempty"`
	}

	if len(query.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(query.Filters)))
		dec.DisallowUnknownFields()

		if dec.Decode(&filter) != nil ||
			(filter.Status != nil && *filter.Status != "draft" && *filter.Status != "demo_reviewed") ||
			(filter.From != nil && filter.To != nil && !filter.From.Before(*filter.To)) {
			return paging.Page[Report]{}, apperr.Fields(
				map[string]string{keyFilters: apperr.KeyInvalid},
			)
		}
	}

	order, err := paging.OrderBy(
		query.Sort,
		map[string]string{
			"id":         "r.id",
			keyCreatedAt: colCreatedAt,
			keyUpdatedAt: colCreatedAt,
			"periodFrom": "lower(r.period)",
		},
		"r.created_at DESC, r.id DESC",
	)
	if err != nil {
		return paging.Page[Report]{}, apperr.Wrap(err, "paging.OrderBy")
	}

	var args []any

	add := func(v any) string {
		args = append(args, v)

		return fmt.Sprintf("$%d", len(args))
	}

	conds := []string{
		"r.version = (SELECT max(version) FROM energy.mrv_reports o WHERE o.id = r.id)",
	}
	if filter.OrganizationID != nil {
		conds = append(conds, "r.organization_id = "+add(*filter.OrganizationID))
	}

	if filter.UnitID != nil {
		conds = append(conds, "r.conditions->'unitIds' ? "+add(filter.UnitID.String()))
	}

	if filter.Status != nil {
		conds = append(conds, "r.status = "+add(*filter.Status))
	}

	if filter.From != nil {
		conds = append(conds, "upper(r.period) > "+add(*filter.From))
	}

	if filter.To != nil {
		conds = append(conds, "lower(r.period) < "+add(*filter.To))
	}

	return pageReports(ctx, call, *query, filter, strings.Join(conds, " AND "), args, order)
}

// VersionsInput is mrv.versions input.
type VersionsInput struct {
	ID    uuid.UUID    `json:"id"`
	Query paging.Query `json:"query"`
}

// Validate implements ops.Validator.
func (in *VersionsInput) Validate() map[string]string {
	if in.ID == uuid.Nil {
		return map[string]string{"id": apperr.KeyRequired}
	}

	return nil
}

// versions serves mrv.versions.
//
//	@Summary		mrv.versions (read)
//	@ID				mrv.versions
//	@Description	Authorization: admin:mrv.read
//	@Description	Validation: D01; SR08/SR09
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A14 · Query: filters none · sort version (default version asc)
//	@Tags			mrv
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string	true	"input field id"
//	@Param			cursor	query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit	query		integer	false	"page size 1–100, default 25"
//	@Param			sort	query		string	false	"field:direction — fields version; default version asc"
//	@Success		200		{object}	ops.Envelope{data=ReportPage}
//	@Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504		{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/mrv/{id}/versions [get]
func versions(
	ctx context.Context,
	call *ops.Call,
	input *VersionsInput,
) (paging.Page[Report], error) {
	if input.Query.HasFilters() { // no filters (query catalog)
		return paging.Page[Report]{}, apperr.Fields(
			map[string]string{"query.filters": apperr.KeyInvalid},
		)
	}

	order, err := paging.OrderBy(
		input.Query.Sort,
		map[string]string{"version": "r.version"},
		"r.version ASC",
	)
	if err != nil {
		return paging.Page[Report]{}, apperr.Wrap(err, "paging.OrderBy")
	}

	_, latestVersionErr := latestVersion(ctx, call, input.ID)
	if latestVersionErr != nil {
		return paging.Page[Report]{}, latestVersionErr
	}

	return pageReports(ctx, call, input.Query, input.ID, "r.id = $1", []any{input.ID}, order)
}

// SaveInput is mrv.saveDraft input.
type SaveInput struct {
	ID          *uuid.UUID  `json:"id,omitempty"`
	Conditions  Conditions  `json:"conditions"`
	EvidenceIDs []uuid.UUID `json:"evidenceIds"`
}

// Validate implements ops.Validator.
func (in *SaveInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	in.Conditions.check(fieldErrs, "conditions.")

	seen := map[uuid.UUID]bool{}
	for _, e := range in.EvidenceIDs {
		if e == uuid.Nil || seen[e] {
			fieldErrs["evidenceIds"] = apperr.KeyInvalid
		}

		seen[e] = true
	}

	if in.EvidenceIDs == nil || len(in.EvidenceIDs) > 20 {
		fieldErrs["evidenceIds"] = apperr.KeyInvalid
	}

	return fieldErrs
}

func insertReport(
	ctx context.Context,
	call *ops.Call,
	reportID uuid.UUID,
	version int,
	content Preview,
	status string,
	evidence []uuid.UUID,
) error {
	cond, marshalErr := json.Marshal(content.Conditions)
	if marshalErr != nil {
		return apperr.Wrap(marshalErr, "json.Marshal")
	}

	res, marshalErr := json.Marshal(
		stored{Summary: content.Summary, Incomplete: content.Incomplete},
	)
	if marshalErr != nil {
		return apperr.Wrap(marshalErr, "json.Marshal")
	}

	_, err := call.Tx.Exec(
		ctx,
		`INSERT INTO energy.mrv_reports (id, version, tenant_id, organization_id, period, status, conditions, results,
			evidence_ids, created_by, created_at)
		VALUES ($1, $2, current_setting('app.tenant_id')::uuid, $3, tstzrange($4, $5), $6, $7, $8, $9, $10, $11)`,
		reportID,
		version,
		content.Conditions.OrganizationID,
		content.Conditions.From,
		content.Conditions.To,
		status,
		cond,
		res,
		evidence,
		call.Principal.MembershipID,
		call.Now,
	)

	return apperr.Wrap(err, "insert energy.mrv_reports")
}

// saveDraft stores the current conditions and the result computed now as a new draft version (SR09). Evidence IDs
// must be existing attachments (VALIDATION).
//
//	@Summary		mrv.saveDraft (write)
//	@ID				mrv.saveDraft
//	@Description	Authorization: admin:mrv.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A14
//	@Tags			mrv
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		SaveInput	true	"input"
//	@Success		200				{object}	ops.Envelope{data=Report}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/mrv [post]
func saveDraft(ctx context.Context, call *ops.Call, input *SaveInput) (Report, error) {
	reportID, version := uuid.New(), 1

	if input.ID != nil {
		latest, err := latestVersion(ctx, call, *input.ID)
		if err != nil {
			return Report{}, err
		}

		if latest != *call.ExpectedVersion {
			return Report{}, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
		}

		reportID, version = *input.ID, latest+1
	}

	var found int

	err := call.Tx.QueryRow(ctx, `SELECT count(*) FROM energy.ref_attachments WHERE id = ANY($1)`, input.EvidenceIDs).
		Scan(&found)
	if err != nil {
		return Report{}, apperr.Wrap(err, "select energy.ref_attachments")
	}

	if found != len(input.EvidenceIDs) {
		return Report{}, apperr.Fields(map[string]string{"evidenceIds": "errors.evidence_unknown"})
	}

	p, err := preview(ctx, call, &input.Conditions)
	if err != nil {
		return Report{}, err
	}

	insertReportErr := insertReport(ctx, call, reportID, version, p, "draft", input.EvidenceIDs)
	if insertReportErr != nil {
		return Report{}, insertReportErr
	}

	call.Audit(
		ops.AuditEntry{
			Action:          "mrv.saveDraft",
			TargetKind:      "mrv_report",
			TargetID:        reportID.String(),
			PreviousVersion: call.ExpectedVersion,
			NextVersion:     &version, Reason: "", Result: "", Before: nil, After: nil,
		},
	)

	return LoadReport(ctx, call, reportID, &version)
}

// ReviewInput is mrv.recordReview input.
type ReviewInput struct {
	ReportID      uuid.UUID `json:"reportId"`
	ReportVersion int       `json:"reportVersion"`
	ReviewComment string    `json:"reviewComment"`
}

// Validate implements ops.Validator.
func (in *ReviewInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.ReportID == uuid.Nil {
		fieldErrs["reportId"] = apperr.KeyRequired
	}

	if in.ReportVersion < 1 {
		fieldErrs["reportVersion"] = apperr.KeyRequired
	}

	in.ReviewComment = strings.TrimSpace(in.ReviewComment)
	if n := utf8.RuneCountInString(in.ReviewComment); n < 1 || n > 1000 {
		fieldErrs["reviewComment"] = apperr.KeyLength
	}

	return fieldErrs
}

// recordReview records a demo review of the latest version (SR09): reportVersion and the expected version must both
// be the latest (CONFLICT); incomplete or already reviewed versions are CONFLICT. It stores a new demo_reviewed
// version with the same conditions and results; the reviewed version stays unchanged.
//
//	@Summary		mrv.recordReview (write)
//	@ID				mrv.recordReview
//	@Description	Authorization: admin:mrv.review
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR87 reviewComment 1-1000
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A14 · Input versions: reportVersion=MRVReport.version
//	@Tags			mrv
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer		true	"all: required (target mrv, read mrv.get)"
//	@Param			reportId			path		string		true	"input field reportId"
//	@Param			request				body		ReviewInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=Report}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/mrv/{reportId}/review [post]
func recordReview(ctx context.Context, call *ops.Call, input *ReviewInput) (Report, error) {
	latest, err := latestVersion(ctx, call, input.ReportID)
	if err != nil {
		return Report{}, err
	}

	if latest != input.ReportVersion || latest != *call.ExpectedVersion {
		return Report{}, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	report, err := LoadReport(ctx, call, input.ReportID, &latest)
	if err != nil {
		return report, err
	}

	if report.Incomplete {
		return report, apperr.E(apperr.Conflict, "errors.mrv_incomplete")
	}

	if report.Status == "demo_reviewed" {
		return report, apperr.E(apperr.Conflict, "errors.mrv_already_reviewed")
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`INSERT INTO energy.mrv_reviews (tenant_id, report_id, report_version, user_id, comment, reviewed_at)
		VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $5)`,
		report.ID,
		latest,
		call.Principal.UserID,
		input.ReviewComment,
		call.Now,
	)
	if execErr != nil {
		return report, apperr.Wrap(execErr, "insert energy.mrv_reviews")
	}

	next := latest + 1

	content := Preview{
		Conditions: report.Conditions,
		Summary:    report.Summary,
		Incomplete: report.Incomplete,
		Scope:      "",
		IsDemo:     false,
	}

	insertReportErr := insertReport(
		ctx,
		call,
		report.ID,
		next,
		content,
		"demo_reviewed",
		report.EvidenceIDs,
	)
	if insertReportErr != nil {
		return report, insertReportErr
	}

	call.Audit(
		ops.AuditEntry{
			Action:          "mrv.recordReview",
			TargetKind:      "mrv_report",
			TargetID:        report.ID.String(),
			PreviousVersion: &latest,
			NextVersion:     &next,
			Reason:          input.ReviewComment, Result: "", Before: nil, After: nil,
		},
	)

	return LoadReport(ctx, call, report.ID, &next)
}

// RegisterMRV binds mrv.*.
func RegisterMRV(reg *ops.Registry) {
	ops.Register(reg, "mrv.preview", preview)
	ops.Register(reg, "mrv.get", getReport)
	ops.Register(reg, "mrv.list", listReports)
	ops.Register(reg, "mrv.versions", versions)
	ops.Register(reg, "mrv.saveDraft", saveDraft)
	ops.Register(reg, "mrv.recordReview", recordReview)
}
