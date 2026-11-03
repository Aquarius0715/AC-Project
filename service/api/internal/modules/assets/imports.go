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

// kualaLumpur reads the CSV dates (YYYY-MM-DD) as calendar dates of the tenant time zone, like the unit form.
var kualaLumpur = func() *time.Location {
	l, err := time.LoadLocation("Asia/Kuala_Lumpur")
	if err != nil {
		return time.FixedZone("MYT", 8*3600)
	}

	return l
}()

const (
	maxImportRows  = 1000
	previewTTL     = 30 * time.Minute
	importUndoTime = 24 * time.Hour
)

var importColumns = map[string]bool{
	kindProperty: true, levelFloor: true, levelRoom: true, "unit_name": true, "model_code": true,
	"serial": true, "installed_on": true, "warranty_end": true,
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

	if !strLen(in.FileName, 1, 200) {
		fieldErrs["fileName"] = apperr.KeyLength
	}

	if strings.TrimSpace(in.CSVText) == "" {
		fieldErrs[keyCSVText] = apperr.KeyRequired
	}

	for k := range in.Mapping {
		if !importColumns[k] {
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

type locKey struct {
	parent uuid.UUID
	kind   string
	name   string
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

	idx := map[string]int{}

	for key, col := range input.Mapping {
		found := false

		for i, h := range header {
			if strings.EqualFold(strings.TrimSpace(h), strings.TrimSpace(col)) {
				idx[key], found = i, true
			}
		}

		if !found && strings.TrimSpace(col) != "" {
			return ImportPreview{}, apperr.Fields(
				map[string]string{"mapping." + key: "error.columnNotFound"},
			)
		}
	}

	var stored []storedRow

	planned := map[locKey]bool{} // spaces this file would create
	unitNames := map[string]bool{}
	serials := map[string]bool{}

	for rowNumber := 1; ; rowNumber++ {
		rec, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return ImportPreview{}, apperr.Fields(
				map[string]string{keyCSVText: apperr.KeyMalformedCsv},
			)
		}

		if rowNumber > maxImportRows {
			return ImportPreview{}, apperr.Fields(
				map[string]string{keyCSVText: "error.tooManyRows"},
			)
		}

		row := storedRow{
			ImportRow: ImportRow{
				RowNumber:    rowNumber,
				Result:       rowReady,
				PropertyName: "",
				FloorName:    nil,
				RoomName:     nil,
				UnitName:     "",
				ModelCode:    "",
				Serial:       nil,
				InstalledOn:  nil,
				WarrantyEnd:  nil,
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
		get := func(k string) *string { return cell(rec, idx, k) }
		fail := func(key string) { row.Result, row.MessageKey = rowError, new(key) }
		warn := func(key string) {
			if row.Result == rowReady {
				row.Result, row.MessageKey = rowWarning, new(key)
			}
		}

		if p := get(kindProperty); p != nil {
			row.PropertyName = *p
		}

		if u := get("unit_name"); u != nil {
			row.UnitName = *u
		}

		if mc := get("model_code"); mc != nil {
			row.ModelCode = *mc
		}

		row.FloorName, row.RoomName, row.Serial, row.InstalledOn, row.WarrantyEnd = get(
			levelFloor,
		), get(
			levelRoom,
		), get(
			"serial",
		), get(
			"installed_on",
		), get(
			"warranty_end",
		)
		switch {
		case row.PropertyName == "" || row.UnitName == "" || row.ModelCode == "":
			fail("error.importMissingField")
		case !strLen(row.UnitName, 1, 120):
			fail(apperr.KeyLength)
		}

		if row.Result != rowError {
			err := call.Tx.QueryRow(ctx, `SELECT id FROM assets.properties WHERE customer_org_id = $1 AND NOT archived AND
				lower(name) = lower($2) ORDER BY id LIMIT 1`,
				org, row.PropertyName).
				Scan(&row.PropertyID)
			if errors.Is(
				err,
				pgx.ErrNoRows,
			) { // created at import (Figma 02 import step 1): kind office, no address yet
				warn("warning.importCreatesProperty")

				row.NewProperty = true
				row.PropertyID = uuid.NewSHA1(
					org,
					[]byte("property/"+strings.ToLower(row.PropertyName)),
				) // stable placeholder
			} else if err != nil {
				return ImportPreview{}, apperr.Wrap(err, "select assets.properties")
			}
		}

		if row.Result != rowError {
			modelID, capVersion, ok, err := m.Import.ByCode(ctx, call, row.ModelCode)
			if err != nil {
				return ImportPreview{}, apperr.Wrap(err, "assets.ImportModels.ByCode")
			}

			if !ok {
				fail("error.unknownModel")
			}

			row.ModelID, row.CapVersion = modelID, capVersion
		}

		for _, d := range []*string{row.InstalledOn, row.WarrantyEnd} {
			if row.Result != rowError && d != nil {
				_, err := time.ParseInLocation("2006-01-02", *d, kualaLumpur)
				if err != nil {
					fail("error.importInvalidDate")
				}
			}
		}

		if row.Result != rowError && row.InstalledOn != nil {
			if t, _ := time.ParseInLocation(
				"2006-01-02",
				*row.InstalledOn,
				kualaLumpur,
			); t.After(
				call.Now,
			) {
				fail(apperr.KeyFuture)
			}
		}

		if row.Result != rowError &&
			row.Serial != nil { // the device must exist, be free and untampered; one row per serial
			deviceID, state, err := m.Import.SerialState(ctx, call, *row.Serial)
			if err != nil {
				return ImportPreview{}, apperr.Wrap(err, "assets.ImportModels.SerialState")
			}

			switch key := strings.ToUpper(*row.Serial); {
			case serials[key]:
				fail(apperr.KeyDuplicateSerial)
			case state == stateUnknown:
				fail("error.unknownSerial")
			case state == "bound":
				fail("error.serialBound")
			case state == "tamper":
				fail(apperr.KeyTamperUnresolved)
			default:
				row.DeviceID = &deviceID
				serials[key] = true
			}
		}
		// locations: floor under the property, room under the floor (or the property root)
		parent := row.PropertyID

		var parentSpace *uuid.UUID

		for _, lvl := range []struct {
			name *string
			kind string
			dst  **uuid.UUID
		}{{row.FloorName, levelFloor, &row.FloorID}, {row.RoomName, levelRoom, &row.RoomID}} {
			if row.Result == rowError || lvl.name == nil {
				continue
			}

			var spaceID uuid.UUID
			// an existing location of any kind with this name under the parent is reused: names are unique per parent
			err := call.Tx.QueryRow(ctx, `SELECT id FROM assets.spaces WHERE property_id = $1 AND parent_space_id IS NOT DISTINCT
				FROM $2
				AND NOT archived AND lower(name) = lower($3) ORDER BY id LIMIT 1`, row.PropertyID, parentSpace, *lvl.name).
				Scan(&spaceID)
			if errors.Is(err, pgx.ErrNoRows) {
				warn("warning.importCreatesSpace")

				k := locKey{parent, lvl.kind, strings.ToLower(*lvl.name)}
				planned[k] = true
				spaceID = uuid.NewSHA1(
					row.PropertyID,
					[]byte(lvl.kind+"/"+parent.String()+"/"+strings.ToLower(*lvl.name)),
				) // stable placeholder
			} else if err != nil {
				return ImportPreview{}, apperr.Wrap(err, "select assets.spaces")
			}

			*lvl.dst = &spaceID
			parent, parentSpace = spaceID, &spaceID
		}

		if row.Result != rowError {
			key := parent.String() + "/" + strings.ToLower(row.UnitName)

			var exists bool

			err := call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM assets.units WHERE property_id = $1 AND space_id IS NOT
				DISTINCT FROM $2
				AND NOT archived AND lower(display_name) = lower($3))`, row.PropertyID, parentSpace, row.UnitName).
				Scan(&exists)
			if err != nil {
				return ImportPreview{}, apperr.Wrap(err, "select assets.units")
			}

			if exists || unitNames[key] {
				fail(apperr.KeyDuplicateSiblingName)
			}

			unitNames[key] = true
		}

		stored = append(stored, row)
	}

	if len(stored) == 0 {
		return ImportPreview{}, apperr.Fields(map[string]string{keyCSVText: "error.noRows"})
	}

	importPreview := ImportPreview{
		PreviewID:  uuid.Must(uuid.NewV7()),
		CustomerID: input.CustomerID,
		FileName:   input.FileName,
		ExpiresAt: call.Now.Add(
			previewTTL,
		),
		Rows:         nil,
		ReadyCount:   0,
		WarningCount: 0,
		ErrorCount:   0,
	}
	for _, s := range stored {
		importPreview.Rows = append(importPreview.Rows, s.ImportRow)
		switch s.Result {
		case rowReady:
			importPreview.ReadyCount++
		case rowWarning:
			importPreview.WarningCount++
		default:
			importPreview.ErrorCount++
		}
	}

	rows, _ := json.Marshal(stored)
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
	var (
		raw      []byte
		fileName string
		expires  time.Time
		customer uuid.UUID
	)

	err := call.Tx.QueryRow(ctx, `DELETE FROM assets.unit_import_previews WHERE id = $1 RETURNING rows, file_name,
		expires_at,
		customer_id`, input.PreviewID).
		Scan(&raw, &fileName, &expires, &customer)
	if errors.Is(err, pgx.ErrNoRows) ||
		(err == nil && (!call.Now.Before(expires) || customer != input.CustomerID)) {
		return UnitImport{}, conflict(apperr.KeyPreviewExpired) // validate again
	}

	if err != nil {
		return UnitImport{}, apperr.Wrap(err, "delete assets.unit_import_previews")
	}

	var org uuid.UUID

	scanErr := call.Tx.QueryRow(ctx, `SELECT organization_id FROM assets.customers WHERE id = $1 AND status =
		'active'`, customer).
		Scan(&org)
	if scanErr != nil {
		return UnitImport{}, conflict(apperr.KeyCustomerInactive)
	}

	var rows []storedRow

	unmarshalErr := json.Unmarshal(raw, &rows)
	if unmarshalErr != nil {
		return UnitImport{}, apperr.Wrap(unmarshalErr, "json.Unmarshal")
	}

	created := map[uuid.UUID]uuid.UUID{} // placeholder → created property or space id

	var properties, spaces, units []uuid.UUID

	skipped := []int32{}
	property := func(row storedRow) (uuid.UUID, error) {
		if !row.NewProperty {
			return row.PropertyID, nil
		}

		if real, ok := created[row.PropertyID]; ok {
			return real, nil
		}

		var taken bool // someone added the property since the preview: validate again

		err := call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM assets.properties WHERE customer_org_id = $1 AND NOT
			archived
			AND lower(name) = lower($2))`,
			org, row.PropertyName).
			Scan(&taken)
		if err != nil {
			return uuid.Nil, apperr.Wrap(err, "select assets.properties")
		}

		if taken {
			return uuid.Nil, conflict(apperr.KeyPreviewExpired)
		}

		propertyID := uuid.Must(uuid.NewV7())

		_, err = call.Tx.Exec(
			ctx,
			`INSERT INTO assets.properties (id, tenant_id, customer_org_id, kind, name) VALUES ($1,
				current_setting('app.tenant_id')::uuid, $2, 'office', $3)`,
			propertyID,
			org,
			row.PropertyName,
		)
		if err != nil {
			return uuid.Nil, apperr.Wrap(err, "insert assets.properties")
		}

		created[row.PropertyID] = propertyID
		properties = append(properties, propertyID)
		call.Emit(
			ops.Event{
				AggregateType: kindProperty,
				AggregateID:   propertyID,
				Type:          eventLocationChanged,
				Payload:       nil,
			},
		)

		return propertyID, nil
	}
	resolve := func(propertyID uuid.UUID, placeholder *uuid.UUID, parent *uuid.UUID, kind, name string) (*uuid.UUID,
		error,
	) {
		if placeholder == nil {
			return nil, nil
		}

		if real, ok := created[*placeholder]; ok {
			return &real, nil
		}

		var present bool

		err := call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM assets.spaces WHERE id = $1 AND NOT
			archived)`, *placeholder).
			Scan(&present)
		if err != nil {
			return nil, apperr.Wrap(err, "select assets.spaces")
		}

		if present {
			return placeholder, nil
		}

		spaceID := uuid.Must(uuid.NewV7())

		_, err = call.Tx.Exec(
			ctx,
			`INSERT INTO assets.spaces (id, tenant_id, property_id, parent_space_id, kind, name)
			VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5)`,
			spaceID,
			propertyID,
			parent,
			kind,
			name,
		)
		if err != nil {
			return nil, dupSibling(err)
		}

		created[*placeholder] = spaceID
		spaces = append(spaces, spaceID)
		call.Emit(
			ops.Event{
				AggregateType: kindSpace,
				AggregateID:   spaceID,
				Type:          eventLocationChanged,
				Payload:       nil,
			},
		)

		return &spaceID, nil
	}

	for _, row := range rows {
		if row.Result == rowError {
			skipped = append(skipped, int32(row.RowNumber))

			continue
		}

		var floorName, roomName string
		if row.FloorName != nil {
			floorName = *row.FloorName
		}

		if row.RoomName != nil {
			roomName = *row.RoomName
		}

		propertyID, err := property(row)
		if err != nil {
			return UnitImport{}, err
		}

		floor, err := resolve(propertyID, row.FloorID, nil, levelFloor, floorName)
		if err != nil {
			return UnitImport{}, err
		}

		room, err := resolve(propertyID, row.RoomID, floor, levelRoom, roomName)
		if err != nil {
			return UnitImport{}, err
		}

		space := floor
		if room != nil {
			space = room
		}

		var installed, warranty *time.Time

		if row.InstalledOn != nil {
			t, _ := time.ParseInLocation("2006-01-02", *row.InstalledOn, kualaLumpur)
			installed = &t
		}

		if row.WarrantyEnd != nil {
			t, _ := time.ParseInLocation("2006-01-02", *row.WarrantyEnd, kualaLumpur)
			warranty = &t
		}

		unitID := uuid.Must(uuid.NewV7())

		_, execErr := call.Tx.Exec(
			ctx,
			`INSERT INTO assets.units (id, tenant_id, customer_org_id, property_id, space_id, display_name, model_id,
				installed_at,
			warranty_ends_at, capability_version) VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7, $8,
			$9)`,
			unitID,
			org,
			propertyID,
			space,
			row.UnitName,
			row.ModelID,
			installed,
			warranty,
			row.CapVersion,
		)
		if execErr != nil {
			return UnitImport{}, apperr.Wrap(execErr, "insert assets.units")
		}

		units = append(units, unitID)
		call.Emit(
			ops.Event{
				AggregateType: kindUnit,
				AggregateID:   unitID,
				Type:          eventUnitChanged,
				Payload:       nil,
			},
		)

		if row.DeviceID != nil { // the serial column binds the device (bound or tampered since the preview: validate again)
			bound, err := m.Import.BindImported(
				ctx,
				call,
				*row.DeviceID,
				unitID,
				org,
				"CSV import "+fileName,
			)
			if err != nil {
				return UnitImport{}, apperr.Wrap(err, "assets.ImportModels.BindImported")
			}

			if !bound {
				return UnitImport{}, conflict(apperr.KeyPreviewExpired)
			}
		}
	}

	if properties == nil {
		properties = []uuid.UUID{}
	}

	if spaces == nil {
		spaces = []uuid.UUID{}
	}

	if units == nil {
		units = []uuid.UUID{}
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
			customer,
			input.PreviewID,
			"inline:"+fileName,
			properties,
			spaces,
			units,
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

	if !strLen(in.Reason, 1, 1000) {
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

	for _, u := range unitImport.CreatedUnitIDs {
		for _, a := range m.Activity {
			busy, err := a.UnitInUse(ctx, call, u)
			if err != nil {
				return UnitImport{}, apperr.Wrap(err, "assets.UnitUsage.UnitInUse")
			}

			if busy {
				return UnitImport{}, conflict("error.importUnitInUse")
			}
		}
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
