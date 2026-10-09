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
	BindImported(ctx context.Context, c *ops.Call, device, unit, org uuid.UUID, reason string) (bool, error)
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

var importColumns = map[string]bool{"property": true, "floor": true, "room": true, "unit_name": true, "model_code": true,
	"serial": true, "installed_on": true, "warranty_end": true}

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
	fe := map[string]string{}
	if in.CustomerID == uuid.Nil {
		fe["customerId"] = "error.required"
	}
	if !strLen(in.FileName, 1, 200) {
		fe["fileName"] = "error.length"
	}
	if strings.TrimSpace(in.CSVText) == "" {
		fe["csvText"] = "error.required"
	}
	for k := range in.Mapping {
		if !importColumns[k] {
			fe["mapping."+k] = "error.unknownColumn"
		}
	}
	for _, k := range []string{"property", "unit_name", "model_code"} {
		if strings.TrimSpace(in.Mapping[k]) == "" {
			fe["mapping."+k] = "error.required"
		}
	}
	return fe
}

func ptr(s string) *string { return &s }

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

func (m *Module) unitsImportPreview(ctx context.Context, c *ops.Call, in *ImportPreviewInput) (ImportPreview, error) {
	var org uuid.UUID
	var status string
	err := c.Tx.QueryRow(ctx, `SELECT organization_id, status FROM assets.customers WHERE id = $1`, in.CustomerID).Scan(&org, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ImportPreview{}, notFound()
	}
	if err != nil {
		return ImportPreview{}, err
	}
	if status != "active" {
		return ImportPreview{}, conflict("error.customerInactive")
	}
	r := csv.NewReader(strings.NewReader(strings.TrimPrefix(in.CSVText, "\ufeff")))
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil {
		return ImportPreview{}, apperr.Fields(map[string]string{"csvText": "error.malformedCsv"})
	}
	idx := map[string]int{}
	for key, col := range in.Mapping {
		found := false
		for i, h := range header {
			if strings.EqualFold(strings.TrimSpace(h), strings.TrimSpace(col)) {
				idx[key], found = i, true
			}
		}
		if !found && strings.TrimSpace(col) != "" {
			return ImportPreview{}, apperr.Fields(map[string]string{"mapping." + key: "error.columnNotFound"})
		}
	}
	var stored []storedRow
	planned := map[locKey]bool{} // spaces this file would create
	unitNames := map[string]bool{}
	serials := map[string]bool{}
	for n := 1; ; n++ {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return ImportPreview{}, apperr.Fields(map[string]string{"csvText": "error.malformedCsv"})
		}
		if n > maxImportRows {
			return ImportPreview{}, apperr.Fields(map[string]string{"csvText": "error.tooManyRows"})
		}
		row := storedRow{ImportRow: ImportRow{RowNumber: n, Result: "ready"}}
		get := func(k string) *string { return cell(rec, idx, k) }
		fail := func(key string) { row.Result, row.MessageKey = "error", ptr(key) }
		warn := func(key string) {
			if row.Result == "ready" {
				row.Result, row.MessageKey = "warning", ptr(key)
			}
		}
		if p := get("property"); p != nil {
			row.PropertyName = *p
		}
		if u := get("unit_name"); u != nil {
			row.UnitName = *u
		}
		if mc := get("model_code"); mc != nil {
			row.ModelCode = *mc
		}
		row.FloorName, row.RoomName, row.Serial, row.InstalledOn, row.WarrantyEnd = get("floor"), get("room"), get("serial"), get("installed_on"), get("warranty_end")
		switch {
		case row.PropertyName == "" || row.UnitName == "" || row.ModelCode == "":
			fail("error.importMissingField")
		case !strLen(row.UnitName, 1, 120):
			fail("error.length")
		}
		if row.Result != "error" {
			err := c.Tx.QueryRow(ctx, `SELECT id FROM assets.properties WHERE customer_org_id = $1 AND NOT archived AND lower(name) = lower($2) ORDER BY id LIMIT 1`,
				org, row.PropertyName).Scan(&row.PropertyID)
			if errors.Is(err, pgx.ErrNoRows) { // created at import (Figma 02 import step 1): kind office, no address yet
				warn("warning.importCreatesProperty")
				row.NewProperty = true
				row.PropertyID = uuid.NewSHA1(org, []byte("property/"+strings.ToLower(row.PropertyName))) // stable placeholder
			} else if err != nil {
				return ImportPreview{}, err
			}
		}
		if row.Result != "error" {
			id, v, ok, err := m.Import.ByCode(ctx, c, row.ModelCode)
			if err != nil {
				return ImportPreview{}, err
			}
			if !ok {
				fail("error.unknownModel")
			}
			row.ModelID, row.CapVersion = id, v
		}
		for _, d := range []*string{row.InstalledOn, row.WarrantyEnd} {
			if row.Result != "error" && d != nil {
				if _, err := time.ParseInLocation("2006-01-02", *d, kualaLumpur); err != nil {
					fail("error.importInvalidDate")
				}
			}
		}
		if row.Result != "error" && row.InstalledOn != nil {
			if t, _ := time.ParseInLocation("2006-01-02", *row.InstalledOn, kualaLumpur); t.After(c.Now) {
				fail("error.future")
			}
		}
		if row.Result != "error" && row.Serial != nil { // the device must exist, be free and untampered; one row per serial
			id, state, err := m.Import.SerialState(ctx, c, *row.Serial)
			if err != nil {
				return ImportPreview{}, err
			}
			switch key := strings.ToUpper(*row.Serial); {
			case serials[key]:
				fail("error.duplicateSerial")
			case state == "unknown":
				fail("error.unknownSerial")
			case state == "bound":
				fail("error.serialBound")
			case state == "tamper":
				fail("error.tamperUnresolved")
			default:
				row.DeviceID = &id
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
		}{{row.FloorName, "floor", &row.FloorID}, {row.RoomName, "room", &row.RoomID}} {
			if row.Result == "error" || lvl.name == nil {
				continue
			}
			var id uuid.UUID
			// an existing location of any kind with this name under the parent is reused: names are unique per parent
			err := c.Tx.QueryRow(ctx, `SELECT id FROM assets.spaces WHERE property_id = $1 AND parent_space_id IS NOT DISTINCT FROM $2
				AND NOT archived AND lower(name) = lower($3) ORDER BY id LIMIT 1`, row.PropertyID, parentSpace, *lvl.name).Scan(&id)
			if errors.Is(err, pgx.ErrNoRows) {
				warn("warning.importCreatesSpace")
				k := locKey{parent, lvl.kind, strings.ToLower(*lvl.name)}
				planned[k] = true
				id = uuid.NewSHA1(row.PropertyID, []byte(lvl.kind+"/"+parent.String()+"/"+strings.ToLower(*lvl.name))) // stable placeholder
			} else if err != nil {
				return ImportPreview{}, err
			}
			*lvl.dst = &id
			parent, parentSpace = id, &id
		}
		if row.Result != "error" {
			key := parent.String() + "/" + strings.ToLower(row.UnitName)
			var exists bool
			if err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM assets.units WHERE property_id = $1 AND space_id IS NOT DISTINCT FROM $2
				AND NOT archived AND lower(display_name) = lower($3))`, row.PropertyID, parentSpace, row.UnitName).Scan(&exists); err != nil {
				return ImportPreview{}, err
			}
			if exists || unitNames[key] {
				fail("error.duplicateSiblingName")
			}
			unitNames[key] = true
		}
		stored = append(stored, row)
	}
	if len(stored) == 0 {
		return ImportPreview{}, apperr.Fields(map[string]string{"csvText": "error.noRows"})
	}
	p := ImportPreview{PreviewID: uuid.Must(uuid.NewV7()), CustomerID: in.CustomerID, FileName: in.FileName, ExpiresAt: c.Now.Add(previewTTL)}
	for _, s := range stored {
		p.Rows = append(p.Rows, s.ImportRow)
		switch s.Result {
		case "ready":
			p.ReadyCount++
		case "warning":
			p.WarningCount++
		default:
			p.ErrorCount++
		}
	}
	rows, _ := json.Marshal(stored)
	// the preview is not business data: stored outside the operation's read-only transaction
	if err := m.Store.Run(ctx, false, c.Principal, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO assets.unit_import_previews (id, tenant_id, customer_id, membership_id, file_name, rows, expires_at)
			VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6)`, p.PreviewID, in.CustomerID, c.Principal.MembershipID, in.FileName, rows, p.ExpiresAt)
		return err
	}); err != nil {
		return ImportPreview{}, err
	}
	return p, nil
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

const importCols = `id, tenant_id, version, created_at, updated_at, customer_id, preview_id, created_property_ids, created_space_ids, created_unit_ids, skipped_row_numbers, undo_until, state`

func scanImport(r pgx.Row) (UnitImport, error) {
	var x UnitImport
	err := r.Scan(&x.ID, &x.TenantID, &x.Version, &x.CreatedAt, &x.UpdatedAt, &x.CustomerID, &x.PreviewID, &x.CreatedPropertyIDs, &x.CreatedSpaceIDs,
		&x.CreatedUnitIDs, &x.SkippedRowNumbers, &x.UndoUntil, &x.State)
	return x, err
}

// ImportCommitInput is units.importCommit input.
type ImportCommitInput struct {
	PreviewID  uuid.UUID `json:"previewId"`
	CustomerID uuid.UUID `json:"customerId"`
}

// Validate implements ops.Validator.
func (in *ImportCommitInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.PreviewID == uuid.Nil {
		fe["previewId"] = "error.required"
	}
	if in.CustomerID == uuid.Nil {
		fe["customerId"] = "error.required"
	}
	return fe
}

func (m *Module) unitsImportCommit(ctx context.Context, c *ops.Call, in *ImportCommitInput) (UnitImport, error) {
	var raw []byte
	var fileName string
	var expires time.Time
	var customer uuid.UUID
	err := c.Tx.QueryRow(ctx, `DELETE FROM assets.unit_import_previews WHERE id = $1 RETURNING rows, file_name, expires_at, customer_id`, in.PreviewID).
		Scan(&raw, &fileName, &expires, &customer)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (!c.Now.Before(expires) || customer != in.CustomerID)) {
		return UnitImport{}, conflict("error.previewExpired") // validate again
	}
	if err != nil {
		return UnitImport{}, err
	}
	var org uuid.UUID
	if err := c.Tx.QueryRow(ctx, `SELECT organization_id FROM assets.customers WHERE id = $1 AND status = 'active'`, customer).Scan(&org); err != nil {
		return UnitImport{}, conflict("error.customerInactive")
	}
	var rows []storedRow
	if err := json.Unmarshal(raw, &rows); err != nil {
		return UnitImport{}, err
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
		if err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM assets.properties WHERE customer_org_id = $1 AND NOT archived AND lower(name) = lower($2))`,
			org, row.PropertyName).Scan(&taken); err != nil {
			return uuid.Nil, err
		}
		if taken {
			return uuid.Nil, conflict("error.previewExpired")
		}
		id := uuid.Must(uuid.NewV7())
		if _, err := c.Tx.Exec(ctx, `INSERT INTO assets.properties (id, tenant_id, customer_org_id, kind, name) VALUES ($1, current_setting('app.tenant_id')::uuid, $2, 'office', $3)`,
			id, org, row.PropertyName); err != nil {
			return uuid.Nil, err
		}
		created[row.PropertyID] = id
		properties = append(properties, id)
		c.Emit(ops.Event{AggregateType: "property", AggregateID: id, Type: "LocationChanged"})
		return id, nil
	}
	resolve := func(propertyID uuid.UUID, placeholder *uuid.UUID, parent *uuid.UUID, kind, name string) (*uuid.UUID, error) {
		if placeholder == nil {
			return nil, nil
		}
		if real, ok := created[*placeholder]; ok {
			return &real, nil
		}
		var ok bool
		if err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM assets.spaces WHERE id = $1 AND NOT archived)`, *placeholder).Scan(&ok); err != nil {
			return nil, err
		}
		if ok {
			return placeholder, nil
		}
		id := uuid.Must(uuid.NewV7())
		if _, err := c.Tx.Exec(ctx, `INSERT INTO assets.spaces (id, tenant_id, property_id, parent_space_id, kind, name)
			VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5)`, id, propertyID, parent, kind, name); err != nil {
			return nil, dupSibling(err)
		}
		created[*placeholder] = id
		spaces = append(spaces, id)
		c.Emit(ops.Event{AggregateType: "space", AggregateID: id, Type: "LocationChanged"})
		return &id, nil
	}
	for _, row := range rows {
		if row.Result == "error" {
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
		floor, err := resolve(propertyID, row.FloorID, nil, "floor", floorName)
		if err != nil {
			return UnitImport{}, err
		}
		room, err := resolve(propertyID, row.RoomID, floor, "room", roomName)
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
		id := uuid.Must(uuid.NewV7())
		if _, err := c.Tx.Exec(ctx, `INSERT INTO assets.units (id, tenant_id, customer_org_id, property_id, space_id, display_name, model_id, installed_at,
			warranty_ends_at, capability_version) VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7, $8, $9)`,
			id, org, propertyID, space, row.UnitName, row.ModelID, installed, warranty, row.CapVersion); err != nil {
			return UnitImport{}, err
		}
		units = append(units, id)
		c.Emit(ops.Event{AggregateType: "unit", AggregateID: id, Type: "UnitChanged"})
		if row.DeviceID != nil { // the serial column binds the device (bound or tampered since the preview: validate again)
			ok, err := m.Import.BindImported(ctx, c, *row.DeviceID, id, org, "CSV import "+fileName)
			if err != nil {
				return UnitImport{}, err
			}
			if !ok {
				return UnitImport{}, conflict("error.previewExpired")
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
	id := uuid.Must(uuid.NewV7())
	x, err := scanImport(c.Tx.QueryRow(ctx, `INSERT INTO assets.unit_imports (id, tenant_id, customer_id, preview_id, source_object_key, created_property_ids,
		created_space_ids, created_unit_ids, skipped_row_numbers, undo_until, state) VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7, $8, $9, 'imported')
		RETURNING `+importCols, id, customer, in.PreviewID, "inline:"+fileName, properties, spaces, units, skipped, c.Now.Add(importUndoTime)))
	if err != nil {
		return UnitImport{}, err
	}
	c.Audit(ops.AuditEntry{Action: "units.importCommit", TargetKind: "unit_import", TargetID: id.String(), NextVersion: &x.Version})
	return x, nil
}

// ImportUndoInput is units.importUndo input.
type ImportUndoInput struct {
	ImportID uuid.UUID `json:"importId"`
	Reason   string    `json:"reason"`
}

// Validate implements ops.Validator.
func (in *ImportUndoInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.ImportID == uuid.Nil {
		fe["importId"] = "error.required"
	}
	if !strLen(in.Reason, 1, 1000) {
		fe["reason"] = "error.length"
	}
	return fe
}

func (m *Module) unitsImportUndo(ctx context.Context, c *ops.Call, in *ImportUndoInput) (UnitImport, error) {
	x, err := scanImport(c.Tx.QueryRow(ctx, `SELECT `+importCols+` FROM assets.unit_imports WHERE id = $1 FOR UPDATE`, in.ImportID))
	if errors.Is(err, pgx.ErrNoRows) {
		return UnitImport{}, notFound()
	}
	if err != nil {
		return UnitImport{}, err
	}
	switch {
	case x.Version != *c.ExpectedVersion:
		return UnitImport{}, conflict("error.versionConflict")
	case x.State != "imported":
		return UnitImport{}, conflict("error.alreadyUndone")
	case !c.Now.Before(x.UndoUntil):
		return UnitImport{}, conflict("error.undoWindowClosed")
	}
	for _, u := range x.CreatedUnitIDs {
		for _, a := range m.Activity {
			busy, err := a.UnitInUse(ctx, c, u)
			if err != nil {
				return UnitImport{}, err
			}
			if busy {
				return UnitImport{}, conflict("error.importUnitInUse")
			}
		}
	}
	// the devices the serial column bound go back to unbound; history alone (no telemetry yet) does not block the undo
	if err := m.Import.UnbindUnits(ctx, c, x.CreatedUnitIDs, "CSV import undone: "+in.Reason); err != nil {
		return UnitImport{}, err
	}
	for _, q := range []struct {
		table string
		ids   []uuid.UUID
	}{{"assets.units", x.CreatedUnitIDs}, {"assets.spaces", x.CreatedSpaceIDs}, {"assets.properties", x.CreatedPropertyIDs}} {
		if _, err := c.Tx.Exec(ctx, "UPDATE "+q.table+" SET archived = true, version = version + 1, updated_at = platform.app_now() WHERE id = ANY($1) AND NOT archived", q.ids); err != nil {
			return UnitImport{}, err
		}
	}
	x, err = scanImport(c.Tx.QueryRow(ctx, `UPDATE assets.unit_imports SET state = 'undone', version = version + 1, updated_at = platform.app_now() WHERE id = $1 RETURNING `+importCols, in.ImportID))
	if err != nil {
		return UnitImport{}, err
	}
	for _, u := range x.CreatedUnitIDs {
		c.Emit(ops.Event{AggregateType: "unit", AggregateID: u, Type: "UnitChanged"})
	}
	c.Audit(ops.AuditEntry{Action: "units.importUndo", TargetKind: "unit_import", TargetID: x.ID.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &x.Version, Reason: in.Reason})
	return x, nil
}
