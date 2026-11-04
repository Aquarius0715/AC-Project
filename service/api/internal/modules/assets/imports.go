package assets

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// ImportModels is what CSV import needs from Devices: the model register and the device behind a serial, which the
// import binds to the unit it creates and the undo releases again (DD-A18 serial column).
type ImportModels interface {
	ByCode(ctx context.Context, c *ops.Call, code string) (uuid.UUID, int, bool, error)
	SerialState(ctx context.Context, c *ops.Call, serial string) (uuid.UUID, string, error)
	BindImported(
		ctx context.Context,
		c *ops.Call,
		device, unit, org uuid.UUID,
		reason string,
	) (bool, error)
	UnbindUnits(ctx context.Context, c *ops.Call, units []uuid.UUID, reason string) error
}

// kualaLumpur is the tenant time zone the CSV dates (YYYY-MM-DD) are calendar dates of, like the unit form's; Malaysia
// Time when the time zone database is missing.
func kualaLumpur() *time.Location {
	zone, err := time.LoadLocation("Asia/Kuala_Lumpur")
	if err != nil {
		return time.FixedZone("MYT", malaysiaOffset)
	}

	return zone
}

// malaysiaOffset is Malaysia Time's offset east of UTC in seconds (UTC+8, no daylight saving).
const malaysiaOffset = 8 * 60 * 60

const (
	maxImportRows  = 1000
	previewTTL     = 30 * time.Minute
	importUndoTime = 24 * time.Hour
)

// importColumn reports whether key is one of the columns an import maps (DD-A18).
func importColumn(key string) bool {
	switch key {
	case kindProperty,
		levelFloor,
		levelRoom,
		"unit_name",
		"model_code",
		"serial",
		"installed_on",
		"warranty_end":
		return true
	}

	return false
}

// ImportRow is ImportRow of service-contracts.ts; the unexported fields are kept in the preview store.
type ImportRow struct {
	RowNumber    int     `json:"rowNumber"`
	PropertyName string  `json:"propertyName"`
	FloorName    *string `json:"floorName"`
	RoomName     *string `json:"roomName"`
	UnitName     string  `json:"unitName"`
	ModelCode    string  `json:"modelCode"`
	Serial       *string `json:"serial"`
	InstalledOn  *string `json:"installedOn"`
	WarrantyEnd  *string `json:"warrantyEnd"`
	Result       string  `json:"result"`
	MessageKey   *string `json:"messageKey"`
}

// storedRow is a checked row with what the commit needs: the IDs of its property, locations, model and device, or the
// stable placeholders of a property and locations the import creates.
type storedRow struct {
	ImportRow

	NewProperty bool       `json:"newProperty"` // the property does not exist yet: the import creates it
	DeviceID    *uuid.UUID `json:"deviceId"`
	PropertyID  uuid.UUID  `json:"propertyId"`
	FloorID     *uuid.UUID `json:"floorId"`
	RoomID      *uuid.UUID `json:"roomId"`
	ModelID     uuid.UUID  `json:"modelId"`
	CapVersion  int        `json:"capVersion"`
}

// ImportPreview is ImportPreview of service-contracts.ts.
type ImportPreview struct {
	PreviewID    uuid.UUID   `json:"previewId"`
	CustomerID   uuid.UUID   `json:"customerId"`
	FileName     string      `json:"fileName"`
	Rows         []ImportRow `json:"rows"`
	ReadyCount   int         `json:"readyCount"`
	WarningCount int         `json:"warningCount"`
	ErrorCount   int         `json:"errorCount"`
	ExpiresAt    time.Time   `json:"expiresAt"`
}

// ImportPreviewInput is units.importPreview input.
type ImportPreviewInput struct {
	CustomerID uuid.UUID         `json:"customerId"`
	FileName   string            `json:"fileName"`
	CSVText    string            `json:"csvText"`
	Mapping    map[string]string `json:"mapping"`
}

// Validate implements ops.Validator (DD-A18: property, unit_name, model_code mapping required).
func (in *ImportPreviewInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.CustomerID == uuid.Nil {
		fieldErrs["customerId"] = apperr.KeyRequired
	}

	if !strLen(in.FileName, 1, maxFileName) {
		fieldErrs["fileName"] = apperr.KeyLength
	}

	if strings.TrimSpace(in.CSVText) == "" {
		fieldErrs[keyCSVText] = apperr.KeyRequired
	}

	for k := range in.Mapping {
		if !importColumn(k) {
			fieldErrs["mapping."+k] = "error.unknownColumn"
		}
	}

	for _, k := range []string{kindProperty, "unit_name", "model_code"} {
		if strings.TrimSpace(in.Mapping[k]) == "" {
			fieldErrs["mapping."+k] = apperr.KeyRequired
		}
	}

	return fieldErrs
}

func cell(rec []string, idx map[string]int, key string) *string {
	i, ok := idx[key]
	if !ok || i >= len(rec) {
		return nil
	}

	v := strings.TrimSpace(rec[i])
	if v == "" {
		return nil
	}

	return &v
}

// unitsImportPreview serves units.importPreview.
//
//	@Summary		units.importPreview (read)
//	@ID				units.importPreview
//	@Description	Authorization: admin:asset.write
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot; IR111 CSV UTF-8, \
//	@Description	at most 1000 rows, 8 mapped columns; writes nothing
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A18
//	@Tags			units
//	@Accept			json
//	@Produce		json
//	@Param			request	body		ImportPreviewInput	true	"input"
//	@Success		200		{object}	ops.Envelope{data=ImportPreview}
//	@Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504		{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/units/imports/preview [post]
func (m *Module) unitsImportPreview(
	ctx context.Context,
	call *ops.Call,
	input *ImportPreviewInput,
) (ImportPreview, error) {
	var (
		org    uuid.UUID
		status string
	)

	err := call.Tx.QueryRow(ctx, `SELECT organization_id, status FROM assets.customers WHERE id = $1`, input.CustomerID).
		Scan(&org, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ImportPreview{}, notFound()
	}

	if err != nil {
		return ImportPreview{}, apperr.Wrap(err, "select assets.customers")
	}

	if status != statusActive {
		return ImportPreview{}, conflict(apperr.KeyCustomerInactive)
	}

	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(input.CSVText, "\ufeff")))
	reader.FieldsPerRecord = -1

	header, err := reader.Read()
	if err != nil {
		return ImportPreview{}, apperr.Fields(
			map[string]string{keyCSVText: apperr.KeyMalformedCsv},
		)
	}

	idx, err := columnIndex(header, input.Mapping)
	if err != nil {
		return ImportPreview{}, err
	}

	check := importCheck{
		models:    m.Import,
		call:      call,
		org:       org,
		zone:      kualaLumpur(),
		unitNames: map[string]bool{},
		serials:   map[string]bool{},
	}

	stored, err := check.rows(ctx, reader, idx)
	if err != nil {
		return ImportPreview{}, err
	}

	importPreview := previewOf(input, stored, call.Now)

	rows, marshalErr := json.Marshal(stored)
	if marshalErr != nil {
		return ImportPreview{}, apperr.Wrap(marshalErr, "json.Marshal")
	}
	// the preview is not business data: stored outside the operation's read-only transaction
	runErr := m.Store.Run(ctx, false, call.Principal, func(tx pgx.Tx) error {
		_, err := tx.Exec(
			ctx,
			`INSERT INTO assets.unit_import_previews (id, tenant_id, customer_id, membership_id, file_name, rows, expires_at)
			VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6)`,
			importPreview.PreviewID,
			input.CustomerID,
			call.Principal.MembershipID,
			input.FileName,
			rows,
			importPreview.ExpiresAt,
		)

		return apperr.Wrap(err, "insert assets.unit_import_previews")
	})
	if runErr != nil {
		return ImportPreview{}, apperr.Wrap(runErr, "ops.Runner.Run")
	}

	return importPreview, nil
}

// columnIndex is the position of each mapped column in the header; a mapped column the header does not have is a
// field error.
func columnIndex(header []string, mapping map[string]string) (map[string]int, error) {
	idx := map[string]int{}

	for key, col := range mapping {
		found := false

		for i, h := range header {
			if strings.EqualFold(strings.TrimSpace(h), strings.TrimSpace(col)) {
				idx[key], found = i, true
			}
		}

		if !found && strings.TrimSpace(col) != "" {
			return nil, apperr.Fields(
				map[string]string{"mapping." + key: "error.columnNotFound"},
			)
		}
	}

	return idx, nil
}

// previewOf is the preview of the checked rows, open for previewTTL from now, with the count of each result.
func previewOf(input *ImportPreviewInput, stored []storedRow, now time.Time) ImportPreview {
	importPreview := ImportPreview{
		PreviewID:    uuid.Must(uuid.NewV7()),
		CustomerID:   input.CustomerID,
		FileName:     input.FileName,
		ExpiresAt:    now.Add(previewTTL),
		Rows:         nil,
		ReadyCount:   0,
		WarningCount: 0,
		ErrorCount:   0,
	}
	for _, row := range stored {
		importPreview.Rows = append(importPreview.Rows, row.ImportRow)
		switch row.Result {
		case rowReady:
			importPreview.ReadyCount++
		case rowWarning:
			importPreview.WarningCount++
		default:
			importPreview.ErrorCount++
		}
	}

	return importPreview
}

// rowOf is a row's cells by the mapped columns, ready until a check says otherwise.
func rowOf(rowNumber int, rec []string, idx map[string]int) storedRow {
	get := func(k string) *string { return cell(rec, idx, k) }
	text := func(k string) string {
		if v := get(k); v != nil {
			return *v
		}

		return ""
	}

	return storedRow{
		ImportRow: ImportRow{
			RowNumber:    rowNumber,
			Result:       rowReady,
			PropertyName: text(kindProperty),
			FloorName:    get(levelFloor),
			RoomName:     get(levelRoom),
			UnitName:     text("unit_name"),
			ModelCode:    text("model_code"),
			Serial:       get("serial"),
			InstalledOn:  get("installed_on"),
			WarrantyEnd:  get("warranty_end"),
			MessageKey:   nil,
		},
		NewProperty: false,
		DeviceID:    nil,
		PropertyID:  uuid.UUID{},
		FloorID:     nil,
		RoomID:      nil,
		ModelID:     uuid.UUID{},
		CapVersion:  0,
	}
}

// fail makes the row an error with the message key.
func (row *storedRow) fail(key string) { row.Result, row.MessageKey = rowError, new(key) }

// warn makes a ready row a warning with the message key; an error or an earlier warning stays.
func (row *storedRow) warn(key string) {
	if row.Result == rowReady {
		row.Result, row.MessageKey = rowWarning, new(key)
	}
}

// importCheck checks the rows of one import file: against the customer's places, the model register and the devices,
// and against the rows before them (a serial or a unit name twice).
type importCheck struct {
	models    ImportModels
	call      *ops.Call
	org       uuid.UUID
	zone      *time.Location
	unitNames map[string]bool // the units of the rows so far, by location and lower-case name
	serials   map[string]bool // the serials of the rows so far, in upper case
}

// rows reads and checks the file's rows: at least one, at most maxImportRows.
func (check *importCheck) rows(
	ctx context.Context,
	reader *csv.Reader,
	idx map[string]int,
) ([]storedRow, error) {
	var stored []storedRow

	for rowNumber := 1; ; rowNumber++ {
		rec, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return nil, apperr.Fields(
				map[string]string{keyCSVText: apperr.KeyMalformedCsv},
			)
		}

		if rowNumber > maxImportRows {
			return nil, apperr.Fields(
				map[string]string{keyCSVText: "error.tooManyRows"},
			)
		}

		row, err := check.row(ctx, rowOf(rowNumber, rec, idx))
		if err != nil {
			return nil, err
		}

		stored = append(stored, row)
	}

	if len(stored) == 0 {
		return nil, apperr.Fields(map[string]string{keyCSVText: "error.noRows"})
	}

	return stored, nil
}

// row checks one row: its required fields and the unit name's length, then the property, model, dates, device and
// places, until a check makes the row an error.
func (check *importCheck) row(ctx context.Context, row storedRow) (storedRow, error) {
	switch {
	case row.PropertyName == "" || row.UnitName == "" || row.ModelCode == "":
		row.fail("error.importMissingField")
	case !strLen(row.UnitName, 1, maxName):
		row.fail(apperr.KeyLength)
	}

	for _, step := range []func(context.Context, *storedRow) error{
		check.property, check.model, check.dates, check.serial, check.places,
	} {
		if row.Result == rowError {
			break
		}

		err := step(ctx, &row)
		if err != nil {
			return row, err
		}
	}

	return row, nil
}

// property finds the row's property by name. One the customer does not have is created at import (Figma 02 import
// step 1: kind office, no address yet), under a stable placeholder ID.
func (check *importCheck) property(ctx context.Context, row *storedRow) error {
	err := check.call.Tx.QueryRow(ctx, `SELECT id FROM assets.properties WHERE customer_org_id = $1 AND NOT archived AND
		lower(name) = lower($2) ORDER BY id LIMIT 1`,
		check.org, row.PropertyName).
		Scan(&row.PropertyID)
	if errors.Is(err, pgx.ErrNoRows) {
		row.warn("warning.importCreatesProperty")

		row.NewProperty = true
		row.PropertyID = uuid.NewSHA1(
			check.org,
			[]byte("property/"+strings.ToLower(row.PropertyName)),
		)

		return nil
	}

	return apperr.Wrap(err, "select assets.properties")
}

// model finds the row's model in the register.
func (check *importCheck) model(ctx context.Context, row *storedRow) error {
	modelID, capVersion, ok, err := check.models.ByCode(ctx, check.call, row.ModelCode)
	if err != nil {
		return apperr.Wrap(err, "assets.ImportModels.ByCode")
	}

	if !ok {
		row.fail("error.unknownModel")
	}

	row.ModelID, row.CapVersion = modelID, capVersion

	return nil
}

// dates checks the row's dates: calendar dates of the tenant time zone, the installation not in the future.
func (check *importCheck) dates(_ context.Context, row *storedRow) error {
	for _, d := range []*string{row.InstalledOn, row.WarrantyEnd} {
		if row.Result != rowError && d != nil {
			_, err := time.ParseInLocation(time.DateOnly, *d, check.zone)
			if err != nil {
				row.fail("error.importInvalidDate")
			}
		}
	}

	if row.Result != rowError && row.InstalledOn != nil {
		if t, _ := time.ParseInLocation(
			time.DateOnly,
			*row.InstalledOn,
			check.zone,
		); t.After(
			check.call.Now,
		) {
			row.fail(apperr.KeyFuture)
		}
	}

	return nil
}

// serial checks the device the row's serial names: it must exist, be free and untampered, and have one row in the
// file.
func (check *importCheck) serial(ctx context.Context, row *storedRow) error {
	if row.Serial == nil {
		return nil
	}

	deviceID, state, err := check.models.SerialState(ctx, check.call, *row.Serial)
	if err != nil {
		return apperr.Wrap(err, "assets.ImportModels.SerialState")
	}

	switch key := strings.ToUpper(*row.Serial); {
	case check.serials[key]:
		row.fail(apperr.KeyDuplicateSerial)
	case state == stateUnknown:
		row.fail("error.unknownSerial")
	case state == "bound":
		row.fail("error.serialBound")
	case state == "tamper":
		row.fail(apperr.KeyTamperUnresolved)
	default:
		row.DeviceID = &deviceID
		check.serials[key] = true
	}

	return nil
}

// places finds the row's floor under the property and its room under the floor (or the property root): an existing
// location of any kind with the name under the parent is reused (names are unique per parent), and a new one gets a
// stable placeholder ID. The unit's name must then be new among its siblings and the rows before it.
func (check *importCheck) places(ctx context.Context, row *storedRow) error {
	parent := row.PropertyID

	var parentSpace *uuid.UUID

	for _, lvl := range []struct {
		name *string
		kind string
		dst  **uuid.UUID
	}{{row.FloorName, levelFloor, &row.FloorID}, {row.RoomName, levelRoom, &row.RoomID}} {
		if lvl.name == nil {
			continue
		}

		var spaceID uuid.UUID

		err := check.call.Tx.QueryRow(ctx, `SELECT id FROM assets.spaces WHERE property_id = $1 AND parent_space_id IS NOT
			DISTINCT FROM $2
			AND NOT archived AND lower(name) = lower($3) ORDER BY id LIMIT 1`, row.PropertyID, parentSpace, *lvl.name).
			Scan(&spaceID)
		if errors.Is(err, pgx.ErrNoRows) {
			row.warn("warning.importCreatesSpace")

			spaceID = uuid.NewSHA1(
				row.PropertyID,
				[]byte(lvl.kind+"/"+parent.String()+"/"+strings.ToLower(*lvl.name)),
			)
		} else if err != nil {
			return apperr.Wrap(err, "select assets.spaces")
		}

		*lvl.dst = &spaceID
		parent, parentSpace = spaceID, &spaceID
	}

	key := parent.String() + "/" + strings.ToLower(row.UnitName)

	var exists bool

	err := check.call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM assets.units WHERE property_id = $1 AND space_id
		IS NOT DISTINCT FROM $2
		AND NOT archived AND lower(display_name) = lower($3))`, row.PropertyID, parentSpace, row.UnitName).
		Scan(&exists)
	if err != nil {
		return apperr.Wrap(err, "select assets.units")
	}

	if exists || check.unitNames[key] {
		row.fail(apperr.KeyDuplicateSiblingName)
	}

	check.unitNames[key] = true

	return nil
}

// UnitImport is UnitImport of service-contracts.ts.
type UnitImport struct {
	ID                 uuid.UUID   `json:"id"`
	TenantID           uuid.UUID   `json:"tenantId"`
	Version            int         `json:"version"`
	CreatedAt          time.Time   `json:"createdAt"`
	UpdatedAt          time.Time   `json:"updatedAt"`
	CustomerID         uuid.UUID   `json:"customerId"`
	PreviewID          uuid.UUID   `json:"previewId"`
	CreatedPropertyIDs []uuid.UUID `json:"createdPropertyIds"`
	CreatedSpaceIDs    []uuid.UUID `json:"createdSpaceIds"`
	CreatedUnitIDs     []uuid.UUID `json:"createdUnitIds"`
	SkippedRowNumbers  []int32     `json:"skippedRowNumbers"`
	UndoUntil          time.Time   `json:"undoUntil"`
	State              string      `json:"state"`
}

const importCols = `id, tenant_id, version, created_at, updated_at, customer_id, preview_id, created_property_ids, ` +
	`created_space_ids, created_unit_ids, skipped_row_numbers, undo_until, state`

func scanImport(r pgx.Row) (UnitImport, error) {
	var unitImport UnitImport

	err := r.Scan(
		&unitImport.ID,
		&unitImport.TenantID,
		&unitImport.Version,
		&unitImport.CreatedAt,
		&unitImport.UpdatedAt,
		&unitImport.CustomerID,
		&unitImport.PreviewID,
		&unitImport.CreatedPropertyIDs,
		&unitImport.CreatedSpaceIDs,
		&unitImport.CreatedUnitIDs,
		&unitImport.SkippedRowNumbers,
		&unitImport.UndoUntil,
		&unitImport.State,
	)

	return unitImport, apperr.Wrap(err, "scan (scanImport)")
}

// ImportCommitInput is units.importCommit input.
type ImportCommitInput struct {
	PreviewID  uuid.UUID `json:"previewId"`
	CustomerID uuid.UUID `json:"customerId"`
}

// Validate implements ops.Validator.
func (in *ImportCommitInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.PreviewID == uuid.Nil {
		fieldErrs["previewId"] = apperr.KeyRequired
	}

	if in.CustomerID == uuid.Nil {
		fieldErrs["customerId"] = apperr.KeyRequired
	}

	return fieldErrs
}

// unitsImportCommit serves units.importCommit.
//
//	@Summary		units.importCommit (write)
//	@ID				units.importCommit
//	@Description	Authorization: admin:asset.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR111 preview must be unexpired and unchanged; imports ready and warning rows only, as one change \
//	@Description	set
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A18
//	@Tags			units
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string				true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		ImportCommitInput	true	"input"
//	@Success		200				{object}	ops.Envelope{data=UnitImport}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/units/imports [post]
func (m *Module) unitsImportCommit(
	ctx context.Context,
	call *ops.Call,
	input *ImportCommitInput,
) (UnitImport, error) {
	preview, err := takePreview(ctx, call, input)
	if err != nil {
		return UnitImport{}, err
	}

	commit := importCommit{
		models:     m.Import,
		call:       call,
		org:        preview.org,
		fileName:   preview.fileName,
		zone:       kualaLumpur(),
		created:    map[uuid.UUID]uuid.UUID{},
		properties: []uuid.UUID{},
		spaces:     []uuid.UUID{},
		units:      []uuid.UUID{},
	}
	skipped := []int32{}

	for _, row := range preview.rows {
		if row.Result == rowError {
			//nolint:gosec // G115: a row number is at most maxImportRows (1000); the preview stops a longer file
			skipped = append(skipped, int32(row.RowNumber))

			continue
		}

		err := commit.unit(ctx, row)
		if err != nil {
			return UnitImport{}, err
		}
	}

	importID := uuid.Must(uuid.NewV7())

	unitImport, err := scanImport(
		call.Tx.QueryRow(
			ctx,
			`INSERT INTO assets.unit_imports (id, tenant_id, customer_id, preview_id, source_object_key, created_property_ids,
		created_space_ids, created_unit_ids, skipped_row_numbers, undo_until, state) VALUES ($1,
		current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7, $8, $9, 'imported')
		RETURNING `+importCols,
			importID,
			preview.customer,
			input.PreviewID,
			"inline:"+preview.fileName,
			commit.properties,
			commit.spaces,
			commit.units,
			skipped,
			call.Now.Add(importUndoTime),
		),
	)
	if err != nil {
		return UnitImport{}, err
	}

	call.Audit(
		ops.AuditEntry{
			Action:          "units.importCommit",
			TargetKind:      "unit_import",
			TargetID:        importID.String(),
			NextVersion:     &unitImport.Version,
			PreviousVersion: nil,
			Reason:          "",
			Result:          "",
			Before:          nil,
			After:           nil,
		},
	)

	return unitImport, nil
}

// takenPreview is a stored preview a commit takes: its checked rows, file name, customer and customer organization.
type takenPreview struct {
	rows          []storedRow
	fileName      string
	customer, org uuid.UUID
}

// takePreview deletes the stored preview and returns it while it is open, for the customer named, and the customer is
// active; otherwise the preview has to be made again.
func takePreview(
	ctx context.Context,
	call *ops.Call,
	input *ImportCommitInput,
) (takenPreview, error) {
	var (
		raw     []byte
		expires time.Time
		taken   takenPreview
	)

	err := call.Tx.QueryRow(ctx, `DELETE FROM assets.unit_import_previews WHERE id = $1 RETURNING rows, file_name,
		expires_at,
		customer_id`, input.PreviewID).
		Scan(&raw, &taken.fileName, &expires, &taken.customer)
	if errors.Is(err, pgx.ErrNoRows) ||
		(err == nil && (!call.Now.Before(expires) || taken.customer != input.CustomerID)) {
		return taken, conflict(apperr.KeyPreviewExpired) // validate again
	}

	if err != nil {
		return taken, apperr.Wrap(err, "delete assets.unit_import_previews")
	}

	scanErr := call.Tx.QueryRow(ctx, `SELECT organization_id FROM assets.customers WHERE id = $1 AND status =
		'active'`, taken.customer).
		Scan(&taken.org)
	if scanErr != nil {
		return taken, conflict(apperr.KeyCustomerInactive)
	}

	unmarshalErr := json.Unmarshal(raw, &taken.rows)

	return taken, apperr.Wrap(unmarshalErr, "json.Unmarshal")
}

// importCommit creates the rows of a checked import: the properties and locations its placeholders stand for, once
// each, and the units with their devices.
type importCommit struct {
	models   ImportModels
	call     *ops.Call
	org      uuid.UUID
	fileName string
	zone     *time.Location
	created  map[uuid.UUID]uuid.UUID // placeholder → created property or space id

	properties, spaces, units []uuid.UUID // what the import created
}

// unit creates the row's unit in its property and location, and binds the device its serial names (bound or tampered
// since the preview: validate again).
func (commit *importCommit) unit(ctx context.Context, row storedRow) error {
	propertyID, err := commit.property(ctx, row)
	if err != nil {
		return err
	}

	space, err := commit.locations(ctx, row, propertyID)
	if err != nil {
		return err
	}

	var installed, warranty *time.Time

	if row.InstalledOn != nil {
		t, _ := time.ParseInLocation(time.DateOnly, *row.InstalledOn, commit.zone)
		installed = &t
	}

	if row.WarrantyEnd != nil {
		t, _ := time.ParseInLocation(time.DateOnly, *row.WarrantyEnd, commit.zone)
		warranty = &t
	}

	unitID := uuid.Must(uuid.NewV7())

	_, execErr := commit.call.Tx.Exec(
		ctx,
		`INSERT INTO assets.units (id, tenant_id, customer_org_id, property_id, space_id, display_name, model_id,
			installed_at,
		warranty_ends_at, capability_version) VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7, $8,
		$9)`,
		unitID,
		commit.org,
		propertyID,
		space,
		row.UnitName,
		row.ModelID,
		installed,
		warranty,
		row.CapVersion,
	)
	if execErr != nil {
		return apperr.Wrap(execErr, "insert assets.units")
	}

	commit.units = append(commit.units, unitID)
	commit.call.Emit(
		ops.Event{
			AggregateType: kindUnit,
			AggregateID:   unitID,
			Type:          eventUnitChanged,
			Payload:       nil,
		},
	)

	if row.DeviceID == nil {
		return nil
	}

	bound, err := commit.models.BindImported(
		ctx,
		commit.call,
		*row.DeviceID,
		unitID,
		commit.org,
		"CSV import "+commit.fileName,
	)
	if err != nil {
		return apperr.Wrap(err, "assets.ImportModels.BindImported")
	}

	if !bound {
		return conflict(apperr.KeyPreviewExpired)
	}

	return nil
}

// property is the row's property: the existing one, or the one its placeholder stands for, created once (someone added
// the property since the preview: validate again).
func (commit *importCommit) property(ctx context.Context, row storedRow) (uuid.UUID, error) {
	if !row.NewProperty {
		return row.PropertyID, nil
	}

	if id, ok := commit.created[row.PropertyID]; ok {
		return id, nil
	}

	var taken bool // someone added the property since the preview: validate again

	err := commit.call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM assets.properties WHERE customer_org_id = $1 AND NOT
		archived
		AND lower(name) = lower($2))`,
		commit.org, row.PropertyName).
		Scan(&taken)
	if err != nil {
		return uuid.Nil, apperr.Wrap(err, "select assets.properties")
	}

	if taken {
		return uuid.Nil, conflict(apperr.KeyPreviewExpired)
	}

	propertyID := uuid.Must(uuid.NewV7())

	_, err = commit.call.Tx.Exec(
		ctx,
		`INSERT INTO assets.properties (id, tenant_id, customer_org_id, kind, name) VALUES ($1,
			current_setting('app.tenant_id')::uuid, $2, 'office', $3)`,
		propertyID,
		commit.org,
		row.PropertyName,
	)
	if err != nil {
		return uuid.Nil, apperr.Wrap(err, "insert assets.properties")
	}

	commit.created[row.PropertyID] = propertyID
	commit.properties = append(commit.properties, propertyID)
	commit.call.Emit(
		ops.Event{
			AggregateType: kindProperty,
			AggregateID:   propertyID,
			Type:          eventLocationChanged,
			Payload:       nil,
		},
	)

	return propertyID, nil
}

// locations creates the row's floor and room where they are new and returns the unit's location: the room, else the
// floor, else none (the property root).
func (commit *importCommit) locations(
	ctx context.Context,
	row storedRow,
	property uuid.UUID,
) (*uuid.UUID, error) {
	var parent *uuid.UUID

	for _, level := range []struct {
		placeholder *uuid.UUID
		name        *string
		kind        string
	}{{row.FloorID, row.FloorName, levelFloor}, {row.RoomID, row.RoomName, levelRoom}} {
		if level.placeholder == nil {
			continue
		}

		var name string
		if level.name != nil {
			name = *level.name
		}

		id, err := commit.space(ctx, property, *level.placeholder, parent, level.kind, name)
		if err != nil {
			return nil, err
		}

		parent = &id
	}

	return parent, nil
}

// space is the location a placeholder stands for: one the import created already, an existing one, or a new one under
// parent.
func (commit *importCommit) space(
	ctx context.Context,
	property, placeholder uuid.UUID,
	parent *uuid.UUID,
	kind, name string,
) (uuid.UUID, error) {
	if id, ok := commit.created[placeholder]; ok {
		return id, nil
	}

	var present bool

	err := commit.call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM assets.spaces WHERE id = $1 AND NOT
		archived)`, placeholder).
		Scan(&present)
	if err != nil {
		return uuid.Nil, apperr.Wrap(err, "select assets.spaces")
	}

	if present {
		return placeholder, nil
	}

	spaceID := uuid.Must(uuid.NewV7())

	_, err = commit.call.Tx.Exec(
		ctx,
		`INSERT INTO assets.spaces (id, tenant_id, property_id, parent_space_id, kind, name)
		VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5)`,
		spaceID,
		property,
		parent,
		kind,
		name,
	)
	if err != nil {
		return uuid.Nil, dupSibling(err)
	}

	commit.created[placeholder] = spaceID
	commit.spaces = append(commit.spaces, spaceID)
	commit.call.Emit(
		ops.Event{
			AggregateType: kindSpace,
			AggregateID:   spaceID,
			Type:          eventLocationChanged,
			Payload:       nil,
		},
	)

	return spaceID, nil
}

// ImportUndoInput is units.importUndo input.
type ImportUndoInput struct {
	ImportID uuid.UUID `json:"importId"`
	Reason   string    `json:"reason"`
}

// Validate implements ops.Validator.
func (in *ImportUndoInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.ImportID == uuid.Nil {
		fieldErrs["importId"] = apperr.KeyRequired
	}

	if !strLen(in.Reason, 1, maxText) {
		fieldErrs["reason"] = apperr.KeyLength
	}

	return fieldErrs
}

// unitsImportUndo serves units.importUndo.
//
//	@Summary		units.importUndo (write)
//	@ID				units.importUndo
//	@Description	Authorization: admin:asset.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR111 only within undoUntil (24 h) and while no created unit has telemetry or jobs
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A18
//	@Tags			units
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer			true	"all: required (target unit_import, read none)"
//	@Param			importId			path		string			true	"input field importId"
//	@Param			request				body		ImportUndoInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=UnitImport}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/units/imports/{importId}/undo [post]
func (m *Module) unitsImportUndo(
	ctx context.Context,
	call *ops.Call,
	input *ImportUndoInput,
) (UnitImport, error) {
	unitImport, err := scanImport(
		call.Tx.QueryRow(
			ctx,
			`SELECT `+importCols+` FROM assets.unit_imports WHERE id = $1 FOR UPDATE`,
			input.ImportID,
		),
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return UnitImport{}, notFound()
	}

	if err != nil {
		return UnitImport{}, err
	}

	switch {
	case unitImport.Version != *call.ExpectedVersion:
		return UnitImport{}, conflict(apperr.KeyVersionConflict)
	case unitImport.State != "imported":
		return UnitImport{}, conflict("error.alreadyUndone")
	case !call.Now.Before(unitImport.UndoUntil):
		return UnitImport{}, conflict("error.undoWindowClosed")
	}

	err = m.unitsInUse(ctx, call, unitImport.CreatedUnitIDs)
	if err != nil {
		return UnitImport{}, err
	}
	// the devices the serial column bound go back to unbound; history alone (no telemetry yet) does not block the undo
	unbindUnitsErr := m.Import.UnbindUnits(
		ctx,
		call,
		unitImport.CreatedUnitIDs,
		"CSV import undone: "+input.Reason,
	)
	if unbindUnitsErr != nil {
		return UnitImport{}, apperr.Wrap(unbindUnitsErr, "assets.ImportModels.UnbindUnits")
	}

	for _, created := range []struct {
		table string
		ids   []uuid.UUID
	}{
		{"assets.units", unitImport.CreatedUnitIDs},
		{"assets.spaces", unitImport.CreatedSpaceIDs},
		{"assets.properties", unitImport.CreatedPropertyIDs},
	} {
		_, err := call.Tx.Exec(
			ctx,
			"UPDATE "+created.table+" SET archived = true, version = version + 1, updated_at = platform.app_now() WHERE id = "+
				"ANY($1) AND NOT archived",
			created.ids,
		)
		if err != nil {
			return UnitImport{}, apperr.Wrap(err, "update (unitsImportUndo)")
		}
	}

	unitImport, err = scanImport(
		call.Tx.QueryRow(
			ctx,
			`UPDATE assets.unit_imports SET state = 'undone', version = version + 1, updated_at = platform.app_now() WHERE id =
				$1 RETURNING `+importCols,
			input.ImportID,
		),
	)
	if err != nil {
		return UnitImport{}, err
	}

	for _, u := range unitImport.CreatedUnitIDs {
		call.Emit(
			ops.Event{
				AggregateType: kindUnit,
				AggregateID:   u,
				Type:          eventUnitChanged,
				Payload:       nil,
			},
		)
	}

	call.Audit(
		ops.AuditEntry{
			Action:          "units.importUndo",
			TargetKind:      "unit_import",
			TargetID:        unitImport.ID.String(),
			PreviousVersion: call.ExpectedVersion,
			NextVersion:     &unitImport.Version,
			Reason:          input.Reason, Result: "", Before: nil, After: nil,
		},
	)

	return unitImport, nil
}

// unitsInUse refuses an undo while a unit the import created has telemetry or jobs.
func (m *Module) unitsInUse(ctx context.Context, call *ops.Call, units []uuid.UUID) error {
	for _, unit := range units {
		for _, usage := range m.Activity {
			busy, err := usage.UnitInUse(ctx, call, unit)
			if err != nil {
				return apperr.Wrap(err, "assets.UnitUsage.UnitInUse")
			}

			if busy {
				return conflict("error.importUnitInUse")
			}
		}
	}

	return nil
}
