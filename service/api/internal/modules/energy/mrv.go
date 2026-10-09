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

func (in *Conditions) check(fe map[string]string, prefix string) {
	if CheckRange(in.From, in.To) != nil {
		fe[prefix+"from"] = "errors.energy_range"
	}
	seen := map[uuid.UUID]bool{}
	for _, u := range in.UnitIDs {
		if u == uuid.Nil || seen[u] {
			fe[prefix+"unitIds"] = "error.invalid"
		}
		seen[u] = true
	}
	if len(in.UnitIDs) == 0 || len(in.UnitIDs) > 100 {
		fe[prefix+"unitIds"] = "error.invalid"
	}
	if in.BaselineID == uuid.Nil || in.BaselineVersion < 1 {
		fe[prefix+"baselineId"] = "error.required"
	}
	if in.FactorID == uuid.Nil || in.FactorVersion < 1 {
		fe[prefix+"factorId"] = "error.required"
	}
	if !slices.Contains(boundaries, in.BoundaryID) {
		fe[prefix+"boundaryId"] = "error.invalid"
	}
	in.Boundary = strings.TrimSpace(in.Boundary)
	if n := utf8.RuneCountInString(in.Boundary); n < 1 || n > 500 {
		fe[prefix+"boundary"] = "error.length"
	}
	if in.OrganizationID == uuid.Nil {
		fe[prefix+"organizationId"] = "error.required"
	}
}

// Validate implements ops.Validator (mrv.preview input).
func (in *Conditions) Validate() map[string]string {
	fe := map[string]string{}
	in.check(fe, "")
	return fe
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
func preview(ctx context.Context, c *ops.Call, in *Conditions) (Preview, error) {
	var kind string
	err := c.Tx.QueryRow(ctx, `SELECT kind FROM energy.ref_organizations WHERE id = $1`, in.OrganizationID).Scan(&kind)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && kind != "customer") {
		return Preview{}, apperr.Fields(map[string]string{"organizationId": "error.invalid"})
	}
	if err != nil {
		return Preview{}, err
	}
	var inOrg int
	if err := c.Tx.QueryRow(ctx, `SELECT count(*) FROM energy.ref_units WHERE id = ANY($1) AND customer_org_id = $2 AND NOT archived`, in.UnitIDs, in.OrganizationID).Scan(&inOrg); err != nil {
		return Preview{}, err
	}
	if inOrg != len(in.UnitIDs) {
		return Preview{}, apperr.Fields(map[string]string{"unitIds": "errors.units_outside_organization"})
	}
	b, err := LoadBaseline(ctx, c, in.BaselineID, &in.BaselineVersion)
	if err != nil {
		return Preview{}, err
	}
	f, err := LoadFactor(ctx, c, in.FactorID, &in.FactorVersion)
	if err != nil {
		return Preview{}, err
	}
	s, n, err := Compute(ctx, c, in.UnitIDs, in.From, in.To, &f, &b)
	if err != nil {
		return Preview{}, err
	}
	incomplete := in.BoundaryID != Boundary || b.BoundaryID != in.BoundaryID || n.ExpectedSlots == 0 || n.ValidSlots < n.ExpectedSlots || !Comparable(&b, in.UnitIDs, in.From, in.To, n)
	return Preview{Conditions: *in, Summary: s, Incomplete: incomplete, Scope: "scope_2", IsDemo: true}, nil
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

const reportCols = `r.id, r.tenant_id, r.version, (SELECT min(created_at) FROM energy.mrv_reports o WHERE o.id = r.id), r.created_at, r.conditions, r.results, r.status, r.evidence_ids`

func scanReport(row pgx.Row) (Report, error) {
	var x Report
	var cond, res []byte
	if err := row.Scan(&x.ID, &x.TenantID, &x.Version, &x.CreatedAt, &x.UpdatedAt, &cond, &res, &x.Status, &x.EvidenceIDs); err != nil {
		return x, err
	}
	var st stored
	_ = json.Unmarshal(cond, &x.Conditions)
	_ = json.Unmarshal(res, &st)
	x.Summary, x.Incomplete, x.Scope, x.IsDemo = st.Summary, st.Incomplete, "scope_2", true
	return x, nil
}

func reviews(ctx context.Context, c *ops.Call, x *Report) error {
	rows, err := c.Tx.Query(ctx, `SELECT user_id, report_version, comment, reviewed_at FROM energy.mrv_reviews WHERE report_id = $1 AND report_version < $2 ORDER BY reviewed_at, id`, x.ID, x.Version)
	if err != nil {
		return err
	}
	defer rows.Close()
	x.ReviewHistory = []Review{}
	for rows.Next() {
		var r Review
		if err := rows.Scan(&r.UserID, &r.ReportVersion, &r.Comment, &r.At); err != nil {
			return err
		}
		x.ReviewHistory = append(x.ReviewHistory, r)
	}
	return rows.Err()
}

func latestVersion(ctx context.Context, c *ops.Call, id uuid.UUID) (int, error) {
	var v *int
	err := c.Tx.QueryRow(ctx, `SELECT max(version) FROM energy.mrv_reports WHERE id = $1`, id).Scan(&v)
	if err != nil {
		return 0, err
	}
	if v == nil {
		return 0, apperr.E(apperr.NotFound, "error.notFound")
	}
	return *v, nil
}

// LoadReport returns a report version (latest when version is nil).
func LoadReport(ctx context.Context, c *ops.Call, id uuid.UUID, version *int) (Report, error) {
	v := 0
	if version != nil {
		v = *version
	} else {
		var err error
		if v, err = latestVersion(ctx, c, id); err != nil {
			return Report{}, err
		}
	}
	x, err := scanReport(c.Tx.QueryRow(ctx, "SELECT "+reportCols+" FROM energy.mrv_reports r WHERE r.id = $1 AND r.version = $2", id, v))
	if errors.Is(err, pgx.ErrNoRows) {
		return x, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return x, err
	}
	return x, reviews(ctx, c, &x)
}

// GetInput is mrv.get input.
type GetInput struct {
	ID            uuid.UUID `json:"id"`
	ReportVersion *int      `json:"reportVersion,omitempty"`
}

// Validate implements ops.Validator.
func (in *GetInput) Validate() map[string]string {
	if in.ID == uuid.Nil || (in.ReportVersion != nil && *in.ReportVersion < 1) {
		return map[string]string{"id": "error.invalid"}
	}
	return nil
}

// @Summary		mrv.get (read)
// @ID				mrv.get
// @Description	Authorization: admin:mrv.read
// @Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
// @Description	Recovery: D04: retry only UNAVAILABLE, at most twice
// @Description	Design: DD-A14
// @Tags			mrv
// @Accept			json
// @Produce		json
// @Param			id				path		string	true	"input field id"
// @Param			reportVersion	query		integer	false	"input field reportVersion"
// @Success		200				{object}	ops.Envelope{data=Report}
// @Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504				{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/mrv/{id} [get]
func getReport(ctx context.Context, c *ops.Call, in *GetInput) (Report, error) {
	return LoadReport(ctx, c, in.ID, in.ReportVersion)
}

func pageReports(ctx context.Context, c *ops.Call, q paging.Query, f any, where string, args []any, order string) (paging.Page[Report], error) {
	w, err := paging.Resolve(q, f, c.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Report]{}, err
	}
	var total int
	if err := c.Tx.QueryRow(ctx, "SELECT count(*) FROM energy.mrv_reports r WHERE "+where, args...).Scan(&total); err != nil {
		return paging.Page[Report]{}, err
	}
	rows, err := c.Tx.Query(ctx, fmt.Sprintf("SELECT %s FROM energy.mrv_reports r WHERE %s ORDER BY %s LIMIT %d OFFSET %d", reportCols, where, order, w.Limit, w.Offset), args...)
	if err != nil {
		return paging.Page[Report]{}, err
	}
	items := []Report{}
	for rows.Next() {
		x, err := scanReport(rows)
		if err != nil {
			rows.Close()
			return paging.Page[Report]{}, err
		}
		items = append(items, x)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return paging.Page[Report]{}, err
	}
	for i := range items {
		if err := reviews(ctx, c, &items[i]); err != nil {
			return paging.Page[Report]{}, err
		}
	}
	return paging.Page[Report]{Items: items, NextCursor: w.Next(total), Total: total, SnapshotVersion: w.Snapshot}, nil
}

// @Summary		mrv.list (read)
// @ID				mrv.list
// @Description	Authorization: admin:mrv.read
// @Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
// @Description	Recovery: D04: retry only UNAVAILABLE, at most twice
// @Description	Design: DD-A14 · Query: filters unitId,from,to,status,organizationId · sort id,createdAt,updatedAt,periodFrom (default createdAt desc;id desc)
// @Tags			mrv
// @Accept			json
// @Produce		json
// @Param			cursor			query		string	false	"page cursor: nextCursor of the previous page (D12)"
// @Param			limit			query		integer	false	"page size 1–100, default 25"
// @Param			sort			query		string	false	"field:direction — fields id,createdAt,updatedAt,periodFrom; default createdAt desc;id desc"
// @Param			unitId			query		string	false	"filter → conditions.unitIds contains"
// @Param			from			query		string	false	"filter → [from,to) on conditions.from"
// @Param			to				query		string	false	"filter → [from,to) on conditions.from"
// @Param			status			query		string	false	"filter → status"
// @Param			organizationId	query		string	false	"filter → conditions.organizationId"
// @Success		200				{object}	ops.Envelope{data=ReportPage}
// @Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504				{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/mrv [get]
func listReports(ctx context.Context, c *ops.Call, in *paging.Query) (paging.Page[Report], error) {
	var f struct {
		OrganizationID *uuid.UUID `json:"organizationId,omitempty"`
		UnitID         *uuid.UUID `json:"unitId,omitempty"`
		Status         *string    `json:"status,omitempty"`
		From           *time.Time `json:"from,omitempty"`
		To             *time.Time `json:"to,omitempty"`
	}
	if len(in.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(in.Filters)))
		dec.DisallowUnknownFields()
		if dec.Decode(&f) != nil || (f.Status != nil && *f.Status != "draft" && *f.Status != "demo_reviewed") || (f.From != nil && f.To != nil && !f.From.Before(*f.To)) {
			return paging.Page[Report]{}, apperr.Fields(map[string]string{"filters": "error.invalid"})
		}
	}
	order, err := paging.OrderBy(in.Sort, map[string]string{"id": "r.id", "createdAt": "r.created_at", "updatedAt": "r.created_at", "periodFrom": "lower(r.period)"}, "r.created_at DESC, r.id DESC")
	if err != nil {
		return paging.Page[Report]{}, err
	}
	var args []any
	add := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	conds := []string{"r.version = (SELECT max(version) FROM energy.mrv_reports o WHERE o.id = r.id)"}
	if f.OrganizationID != nil {
		conds = append(conds, "r.organization_id = "+add(*f.OrganizationID))
	}
	if f.UnitID != nil {
		conds = append(conds, "r.conditions->'unitIds' ? "+add(f.UnitID.String()))
	}
	if f.Status != nil {
		conds = append(conds, "r.status = "+add(*f.Status))
	}
	if f.From != nil {
		conds = append(conds, "upper(r.period) > "+add(*f.From))
	}
	if f.To != nil {
		conds = append(conds, "lower(r.period) < "+add(*f.To))
	}
	return pageReports(ctx, c, *in, f, strings.Join(conds, " AND "), args, order)
}

// VersionsInput is mrv.versions input.
type VersionsInput struct {
	ID    uuid.UUID    `json:"id"`
	Query paging.Query `json:"query"`
}

// Validate implements ops.Validator.
func (in *VersionsInput) Validate() map[string]string {
	if in.ID == uuid.Nil {
		return map[string]string{"id": "error.required"}
	}
	return nil
}

// @Summary		mrv.versions (read)
// @ID				mrv.versions
// @Description	Authorization: admin:mrv.read
// @Description	Validation: D01; SR08/SR09
// @Description	Recovery: D04: retry only UNAVAILABLE, at most twice
// @Description	Design: DD-A14 · Query: filters none · sort version (default version asc)
// @Tags			mrv
// @Accept			json
// @Produce		json
// @Param			id		path		string	true	"input field id"
// @Param			cursor	query		string	false	"page cursor: nextCursor of the previous page (D12)"
// @Param			limit	query		integer	false	"page size 1–100, default 25"
// @Param			sort	query		string	false	"field:direction — fields version; default version asc"
// @Success		200		{object}	ops.Envelope{data=ReportPage}
// @Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504		{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/mrv/{id}/versions [get]
func versions(ctx context.Context, c *ops.Call, in *VersionsInput) (paging.Page[Report], error) {
	if in.Query.HasFilters() { // no filters (query catalog)
		return paging.Page[Report]{}, apperr.Fields(map[string]string{"query.filters": "error.invalid"})
	}
	order, err := paging.OrderBy(in.Query.Sort, map[string]string{"version": "r.version"}, "r.version ASC")
	if err != nil {
		return paging.Page[Report]{}, err
	}
	if _, err := latestVersion(ctx, c, in.ID); err != nil {
		return paging.Page[Report]{}, err
	}
	return pageReports(ctx, c, in.Query, in.ID, "r.id = $1", []any{in.ID}, order)
}

// SaveInput is mrv.saveDraft input.
type SaveInput struct {
	ID          *uuid.UUID  `json:"id,omitempty"`
	Conditions  Conditions  `json:"conditions"`
	EvidenceIDs []uuid.UUID `json:"evidenceIds"`
}

// Validate implements ops.Validator.
func (in *SaveInput) Validate() map[string]string {
	fe := map[string]string{}
	in.Conditions.check(fe, "conditions.")
	seen := map[uuid.UUID]bool{}
	for _, e := range in.EvidenceIDs {
		if e == uuid.Nil || seen[e] {
			fe["evidenceIds"] = "error.invalid"
		}
		seen[e] = true
	}
	if in.EvidenceIDs == nil || len(in.EvidenceIDs) > 20 {
		fe["evidenceIds"] = "error.invalid"
	}
	return fe
}

func insertReport(ctx context.Context, c *ops.Call, id uuid.UUID, version int, p Preview, status string, evidence []uuid.UUID) error {
	cond, _ := json.Marshal(p.Conditions)
	res, _ := json.Marshal(stored{Summary: p.Summary, Incomplete: p.Incomplete})
	_, err := c.Tx.Exec(ctx, `INSERT INTO energy.mrv_reports (id, version, tenant_id, organization_id, period, status, conditions, results, evidence_ids, created_by, created_at)
		VALUES ($1, $2, current_setting('app.tenant_id')::uuid, $3, tstzrange($4, $5), $6, $7, $8, $9, $10, $11)`,
		id, version, p.Conditions.OrganizationID, p.Conditions.From, p.Conditions.To, status, cond, res, evidence, c.Principal.MembershipID, c.Now)
	return err
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
func saveDraft(ctx context.Context, c *ops.Call, in *SaveInput) (Report, error) {
	id, version := uuid.New(), 1
	if in.ID != nil {
		latest, err := latestVersion(ctx, c, *in.ID)
		if err != nil {
			return Report{}, err
		}
		if latest != *c.ExpectedVersion {
			return Report{}, apperr.E(apperr.Conflict, "error.versionConflict")
		}
		id, version = *in.ID, latest+1
	}
	var found int
	if err := c.Tx.QueryRow(ctx, `SELECT count(*) FROM energy.ref_attachments WHERE id = ANY($1)`, in.EvidenceIDs).Scan(&found); err != nil {
		return Report{}, err
	}
	if found != len(in.EvidenceIDs) {
		return Report{}, apperr.Fields(map[string]string{"evidenceIds": "errors.evidence_unknown"})
	}
	p, err := preview(ctx, c, &in.Conditions)
	if err != nil {
		return Report{}, err
	}
	if err := insertReport(ctx, c, id, version, p, "draft", in.EvidenceIDs); err != nil {
		return Report{}, err
	}
	c.Audit(ops.AuditEntry{Action: "mrv.saveDraft", TargetKind: "mrv_report", TargetID: id.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &version})
	return LoadReport(ctx, c, id, &version)
}

// ReviewInput is mrv.recordReview input.
type ReviewInput struct {
	ReportID      uuid.UUID `json:"reportId"`
	ReportVersion int       `json:"reportVersion"`
	ReviewComment string    `json:"reviewComment"`
}

// Validate implements ops.Validator.
func (in *ReviewInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.ReportID == uuid.Nil {
		fe["reportId"] = "error.required"
	}
	if in.ReportVersion < 1 {
		fe["reportVersion"] = "error.required"
	}
	in.ReviewComment = strings.TrimSpace(in.ReviewComment)
	if n := utf8.RuneCountInString(in.ReviewComment); n < 1 || n > 1000 {
		fe["reviewComment"] = "error.length"
	}
	return fe
}

// recordReview records a demo review of the latest version (SR09): reportVersion and the expected version must both
// be the latest (CONFLICT); incomplete or already reviewed versions are CONFLICT. It stores a new demo_reviewed
// version with the same conditions and results; the reviewed version stays unchanged.
//
//	@Summary		mrv.recordReview (write)
//	@ID				mrv.recordReview
//	@Description	Authorization: admin:mrv.review
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; IR87 reviewComment 1-1000
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
func recordReview(ctx context.Context, c *ops.Call, in *ReviewInput) (Report, error) {
	latest, err := latestVersion(ctx, c, in.ReportID)
	if err != nil {
		return Report{}, err
	}
	if latest != in.ReportVersion || latest != *c.ExpectedVersion {
		return Report{}, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	x, err := LoadReport(ctx, c, in.ReportID, &latest)
	if err != nil {
		return x, err
	}
	if x.Incomplete {
		return x, apperr.E(apperr.Conflict, "errors.mrv_incomplete")
	}
	if x.Status == "demo_reviewed" {
		return x, apperr.E(apperr.Conflict, "errors.mrv_already_reviewed")
	}
	if _, err := c.Tx.Exec(ctx, `INSERT INTO energy.mrv_reviews (tenant_id, report_id, report_version, user_id, comment, reviewed_at)
		VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $5)`, x.ID, latest, c.Principal.UserID, in.ReviewComment, c.Now); err != nil {
		return x, err
	}
	next := latest + 1
	p := Preview{Conditions: x.Conditions, Summary: x.Summary, Incomplete: x.Incomplete}
	if err := insertReport(ctx, c, x.ID, next, p, "demo_reviewed", x.EvidenceIDs); err != nil {
		return x, err
	}
	c.Audit(ops.AuditEntry{Action: "mrv.recordReview", TargetKind: "mrv_report", TargetID: x.ID.String(), PreviousVersion: &latest, NextVersion: &next, Reason: in.ReviewComment})
	return LoadReport(ctx, c, x.ID, &next)
}

// RegisterMRV binds mrv.*.
func RegisterMRV(r *ops.Registry) {
	ops.Register(r, "mrv.preview", preview)
	ops.Register(r, "mrv.get", getReport)
	ops.Register(r, "mrv.list", listReports)
	ops.Register(r, "mrv.versions", versions)
	ops.Register(r, "mrv.saveDraft", saveDraft)
	ops.Register(r, "mrv.recordReview", recordReview)
}
