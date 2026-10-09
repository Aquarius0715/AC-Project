package energy

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// ExportInput is energy.exportReport input (DD-C16).
type ExportInput struct {
	Month       string      `json:"month"`
	PropertyIDs []uuid.UUID `json:"propertyIds"`
	Sections    []string    `json:"sections"`
	Format      string      `json:"format"`
}

var (
	monthRe  = regexp.MustCompile(`^[0-9]{4}-(0[1-9]|1[0-2])$`)
	sections = []string{"energy_cost", "month_comparison", "co2_offsets", "alerts_maintenance"}
)

// Validate implements ops.Validator.
func (in *ExportInput) Validate() map[string]string {
	fe := map[string]string{}
	if !monthRe.MatchString(in.Month) {
		fe["month"] = "error.invalid"
	}
	seen := map[uuid.UUID]bool{}
	for _, p := range in.PropertyIDs {
		if p == uuid.Nil || seen[p] {
			fe["propertyIds"] = "error.invalid"
		}
		seen[p] = true
	}
	if len(in.PropertyIDs) == 0 || len(in.PropertyIDs) > 100 {
		fe["propertyIds"] = "error.invalid"
	}
	ss := map[string]bool{}
	for _, s := range in.Sections {
		if !slices.Contains(sections, s) || ss[s] {
			fe["sections"] = "error.invalid"
		}
		ss[s] = true
	}
	if len(in.Sections) == 0 {
		fe["sections"] = "error.required"
	}
	if in.Format != "pdf" && in.Format != "csv" {
		fe["format"] = "error.invalid"
	}
	return fe
}

// ReportFile is ReportFile of service-contracts.ts.
type ReportFile struct {
	FileName    string    `json:"fileName"`
	Mime        string    `json:"mime"`
	Size        int       `json:"size"`
	GeneratedAt time.Time `json:"generatedAt"`
	IsDemo      bool      `json:"isDemo"`
}

func f1(p *float64) string {
	if p == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.1f", *p)
}

// exportReport renders the demo monthly report for the client's properties (IR110, IR153): the month must have ended
// in the caller's timezone; other customers' properties are NOT_FOUND. Only the file metadata is returned.
//
//	@Summary		energy.exportReport (read)
//	@ID				energy.exportReport
//	@Description	Authorization: client:self
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot; IR110 month YYYY-MM already ended; at least one section
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-C16
//	@Tags			energy
//	@Accept			json
//	@Produce		json
//	@Param			month		query		string		false	"input field month"
//	@Param			propertyIds	query		[]string	false	"input field propertyIds (repeat the parameter or separate values with commas; an empty value is the empty list)"	collectionFormat(multi)
//	@Param			sections	query		[]string	false	"input field sections (repeat the parameter or separate values with commas; an empty value is the empty list)"		collectionFormat(multi)
//	@Param			format		query		string		false	"input field format"
//	@Success		200			{object}	ops.Envelope{data=ReportFile}
//	@Failure		401			{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403			{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404			{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409			{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422			{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429			{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503			{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504			{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/energy/report [get]
func exportReport(ctx context.Context, c *ops.Call, in *ExportInput) (ReportFile, error) {
	tz := "Asia/Kuala_Lumpur"
	if c.Principal.Timezone != "" { // from identity with the principal (IR189)
		tz = c.Principal.Timezone
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	start, _ := time.ParseInLocation("2006-01", in.Month, loc)
	end := start.AddDate(0, 1, 0)
	if end.After(c.Now) {
		return ReportFile{}, apperr.Fields(map[string]string{"month": "errors.month_not_ended"})
	}
	var own int
	if err := c.Tx.QueryRow(ctx, `SELECT count(*) FROM energy.ref_properties WHERE id = ANY($1) AND customer_org_id = $2 AND NOT archived`, in.PropertyIDs, c.Principal.OrgID).Scan(&own); err != nil {
		return ReportFile{}, err
	}
	if own != len(in.PropertyIDs) {
		return ReportFile{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	rows, err := c.Tx.Query(ctx, `SELECT id FROM energy.ref_units WHERE property_id = ANY($1) AND NOT archived ORDER BY id`, in.PropertyIDs)
	if err != nil {
		return ReportFile{}, err
	}
	units, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return ReportFile{}, err
	}
	var b strings.Builder
	b.WriteString("section,metric,value\n")
	f, err := DefaultFactor(ctx, c)
	if err != nil {
		return ReportFile{}, err
	}
	month, _, err := Compute(ctx, c, units, start.UTC(), end.UTC(), f, nil)
	if err != nil {
		return ReportFile{}, err
	}
	for _, s := range in.Sections {
		switch s {
		case "energy_cost":
			amount := "n/a"
			if month.Totals.AmountMinor != nil {
				amount = fmt.Sprintf("%.2f MYR", float64(*month.Totals.AmountMinor)/100)
			}
			fmt.Fprintf(&b, "energy_cost,kWh,%s\nenergy_cost,cost,%s\n", f1(month.Totals.KWh), amount)
		case "month_comparison":
			prev, _, err := Compute(ctx, c, units, start.AddDate(0, -1, 0).UTC(), start.UTC(), f, nil)
			if err != nil {
				return ReportFile{}, err
			}
			fmt.Fprintf(&b, "month_comparison,previous_kWh,%s\nmonth_comparison,current_kWh,%s\n", f1(prev.Totals.KWh), f1(month.Totals.KWh))
		case "co2_offsets":
			var retired float64
			if err := c.Tx.QueryRow(ctx, `SELECT COALESCE(sum(r.amount_kg), 0)::float8 FROM energy.offset_records r JOIN energy.ref_customers cu ON cu.id = r.customer_id
				WHERE cu.organization_id = $1 AND r.state = 'demo_retired' AND r.updated_at >= $2 AND r.updated_at < $3`, c.Principal.OrgID, start, end).Scan(&retired); err != nil {
				return ReportFile{}, err
			}
			fmt.Fprintf(&b, "co2_offsets,emissions_kg,%s\nco2_offsets,demo_retired_kg,%.1f\n", f1(month.Totals.EmissionsKg), retired)
		case "alerts_maintenance":
			var alerts, jobs int
			if err := c.Tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM energy.ref_alerts WHERE unit_id = ANY($1) AND detected_at >= $2 AND detected_at < $3),
				(SELECT count(*) FROM energy.ref_jobs WHERE unit_id = ANY($1) AND status = 'completed' AND updated_at >= $2 AND updated_at < $3)`, units, start, end).Scan(&alerts, &jobs); err != nil {
				return ReportFile{}, err
			}
			fmt.Fprintf(&b, "alerts_maintenance,alerts,%d\nalerts_maintenance,completed_jobs,%d\n", alerts, jobs)
		}
	}
	content, mime := b.String(), "text/csv"
	if in.Format == "pdf" { // demo PDF: the same figures in a minimal one-page document
		content, mime = "%PDF-1.4\n% Demo energy report "+in.Month+" (fictional values)\n"+content+"%%EOF\n", "application/pdf"
	}
	return ReportFile{FileName: "energy-report-" + in.Month + "." + in.Format, Mime: mime, Size: len(content), GeneratedAt: c.Now, IsDemo: true}, nil
}
