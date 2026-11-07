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

var monthRe = regexp.MustCompile(`^\d{4}-(0[1-9]|1[012])$`)

// reportSections are the sections an energy report can have.
func reportSections() []string {
	return []string{"energy_cost", "month_comparison", "co2_offsets", "alerts_maintenance"}
}

// Validate implements ops.Validator.
func (in *ExportInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if !monthRe.MatchString(in.Month) {
		fieldErrs["month"] = apperr.KeyInvalid
	}

	seen := map[uuid.UUID]bool{}
	for _, p := range in.PropertyIDs {
		if p == uuid.Nil || seen[p] {
			fieldErrs["propertyIds"] = apperr.KeyInvalid
		}

		seen[p] = true
	}

	if len(in.PropertyIDs) == 0 || len(in.PropertyIDs) > 100 {
		fieldErrs["propertyIds"] = apperr.KeyInvalid
	}

	seenSections := map[string]bool{}
	for _, s := range in.Sections {
		if !slices.Contains(reportSections(), s) || seenSections[s] {
			fieldErrs["sections"] = apperr.KeyInvalid
		}

		seenSections[s] = true
	}

	if len(in.Sections) == 0 {
		fieldErrs["sections"] = apperr.KeyRequired
	}

	if in.Format != "pdf" && in.Format != "csv" {
		fieldErrs["format"] = apperr.KeyInvalid
	}

	return fieldErrs
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
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot; IR110 month \
//	@Description	YYYY-MM already ended; at least one section
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
func exportReport(ctx context.Context, call *ops.Call, input *ExportInput) (ReportFile, error) {
	start, end, err := reportMonth(call, input.Month)
	if err != nil {
		return ReportFile{}, err
	}

	units, err := reportUnits(ctx, call, input.PropertyIDs)
	if err != nil {
		return ReportFile{}, err
	}

	factor, err := DefaultFactor(ctx, call)
	if err != nil {
		return ReportFile{}, err
	}

	month, _, err := Compute(ctx, call, units, start.UTC(), end.UTC(), factor, nil)
	if err != nil {
		return ReportFile{}, err
	}

	scope := reportScope{units: units, start: start, end: end, factor: factor, month: month}

	var builder strings.Builder
	builder.WriteString("section,metric,value\n")

	for _, section := range input.Sections {
		err := scope.write(ctx, call, &builder, section)
		if err != nil {
			return ReportFile{}, err
		}
	}

	content, mime := builder.String(), "text/csv"
	if input.Format == "pdf" { // demo PDF: the same figures in a minimal one-page document
		content, mime = "%PDF-1.4\n% Demo energy report "+input.Month+" (fictional values)\n"+content+
			"%%EOF\n", "application/pdf"
	}

	return ReportFile{
		FileName:    "energy-report-" + input.Month + "." + input.Format,
		Mime:        mime,
		Size:        len(content),
		GeneratedAt: call.Now,
		IsDemo:      true,
	}, nil
}

// reportMonth is the month a report covers in the user's timezone (from identity with the principal, IR189; Kuala
// Lumpur by default, UTC for a zone this server does not know): its start and its end, once it has ended.
func reportMonth(call *ops.Call, month string) (time.Time, time.Time, error) {
	tz := "Asia/Kuala_Lumpur"
	if call.Principal.Timezone != "" {
		tz = call.Principal.Timezone
	}

	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}

	start, err := time.ParseInLocation("2006-01", month, loc)
	if err != nil {
		return time.Time{}, time.Time{}, apperr.Fields(
			map[string]string{"month": apperr.KeyInvalid},
		)
	}

	end := start.AddDate(0, 1, 0)
	if end.After(call.Now) {
		return time.Time{}, time.Time{}, apperr.Fields(
			map[string]string{"month": "errors.month_not_ended"},
		)
	}

	return start, end, nil
}

// reportUnits are the units of a report's properties, which must all be the caller's own and not archived.
func reportUnits(ctx context.Context, call *ops.Call, properties []uuid.UUID) ([]uuid.UUID, error) {
	var own int

	err := call.Tx.QueryRow(ctx, `SELECT count(*) FROM energy.ref_properties WHERE id = ANY($1) AND customer_org_id =
		$2
		AND NOT archived`, properties, call.Principal.OrgID).
		Scan(&own)
	if err != nil {
		return nil, apperr.Wrap(err, "select energy.ref_properties")
	}

	if own != len(properties) {
		return nil, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	rows, err := call.Tx.Query(
		ctx,
		`SELECT id FROM energy.ref_units WHERE property_id = ANY($1) AND NOT archived ORDER BY id`,
		properties,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select energy.ref_units")
	}

	units, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])

	return units, apperr.Wrap(err, "select energy.ref_units")
}

// reportScope is what a report's sections read: its units and month, the factor, and the month's summary.
type reportScope struct {
	units      []uuid.UUID
	start, end time.Time
	factor     *Factor
	month      Summary
}

// write writes the rows of one section.
func (scope reportScope) write(
	ctx context.Context,
	call *ops.Call,
	builder *strings.Builder,
	section string,
) error {
	switch section {
	case "energy_cost":
		amount := "n/a"
		if scope.month.Totals.AmountMinor != nil {
			amount = fmt.Sprintf("%.2f MYR", float64(*scope.month.Totals.AmountMinor)/senPerRinggit)
		}

		fmt.Fprintf(
			builder,
			"energy_cost,kWh,%s\nenergy_cost,cost,%s\n",
			f1(scope.month.Totals.KWh),
			amount,
		)
	case "month_comparison":
		prev, _, err := Compute(
			ctx,
			call,
			scope.units,
			scope.start.AddDate(0, -1, 0).UTC(),
			scope.start.UTC(),
			scope.factor,
			nil,
		)
		if err != nil {
			return err
		}

		fmt.Fprintf(
			builder,
			"month_comparison,previous_kWh,%s\nmonth_comparison,current_kWh,%s\n",
			f1(prev.Totals.KWh),
			f1(scope.month.Totals.KWh),
		)
	case "co2_offsets":
		var retired float64

		err := call.Tx.QueryRow(ctx, `SELECT COALESCE(sum(r.amount_kg), 0)::float8 FROM energy.offset_records r JOIN
			energy.ref_customers cu ON cu.id = r.customer_id
			WHERE cu.organization_id = $1 AND r.state = 'demo_retired' AND r.updated_at >= $2 AND r.updated_at <
			$3`, call.Principal.OrgID, scope.start, scope.end).
			Scan(&retired)
		if err != nil {
			return apperr.Wrap(err, "select energy.offset_records")
		}

		fmt.Fprintf(
			builder,
			"co2_offsets,emissions_kg,%s\nco2_offsets,demo_retired_kg,%.1f\n",
			f1(scope.month.Totals.EmissionsKg),
			retired,
		)
	case "alerts_maintenance":
		var alerts, jobs int

		err := call.Tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM energy.ref_alerts WHERE unit_id = ANY($1) AND detected_at
			>=
			$2 AND detected_at < $3),
			(SELECT count(*) FROM energy.ref_jobs WHERE unit_id = ANY($1) AND status = 'completed' AND updated_at >= $2 AND
			updated_at < $3)`, scope.units, scope.start, scope.end).
			Scan(&alerts, &jobs)
		if err != nil {
			return apperr.Wrap(err, "select energy.ref_alerts")
		}

		fmt.Fprintf(
			builder,
			"alerts_maintenance,alerts,%d\nalerts_maintenance,completed_jobs,%d\n",
			alerts,
			jobs,
		)
	}

	return nil
}
