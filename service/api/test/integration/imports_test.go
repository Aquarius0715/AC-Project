package integration

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

func jsonStr(s string) string {
	b, _ := json.Marshal(s)

	return string(b)
}

func jsonStr2(v any) string {
	b, _ := json.Marshal(v)

	return string(b)
}

const mapping = `{"property":"Property","floor":"Floor","room":"Room","unit_name":"Unit","model_code":"Model",` +
	`"serial":"Serial","installed_on":"Installed","warranty_end":"Warranty"}`

func TestUnitsImport(t *testing.T) {
	srv := server(t)
	cust := seed.ID("cust-b").String()
	org := seed.ID("org-customer-b").String()
	prop := "Import site " + uuid.NewString()[:6]

	code, resp := write(
		srv,
		&hq,
		"properties.save",
		`{"customerOrgId":"`+org+`","kind":"office","name":"`+prop+`","address":null,"accessInstructions":null}`,
		0,
	)
	if code != 200 {
		t.Fatalf("property: %d %v", code, resp)
	}

	propID := asStr(t, data(resp)["id"])
	if code, res := write(
		srv,
		&hq,
		"spaces.save",
		`{"propertyId":"`+propID+`","parentSpaceId":null,"kind":"floor","name":"1F"}`,
		0,
	); code != 200 {
		t.Fatalf("floor: %d %v", code, res)
	}
	// a device bound to some unit, so its serial is an error row
	serial := "IMP-" + uuid.NewString()[:8]
	dev := uuid.NewString()
	_, um := write(
		srv,
		&hq,
		"units.save",
		`{"customerOrgId":"`+org+`","propertyId":"`+propID+`","spaceId":null,"displayName":"Bound AC",`+
			`"modelId":"`+seed.ID("ventilation-demo").
			String()+
			`","type":"split","installedAt":null,"serviceScope":["indoor"]}`,
		0,
	)
	boundUnit := data(um)["id"]
	owner(
		t,
		`INSERT INTO devices.devices (id, tenant_id, serial, target_unit_id, created_by_membership_id, firmware_version)
			VALUES ($1,$2,$3,$4,$5,'v1')`,
		dev,
		seed.ID("tenant-a"),
		serial,
		boundUnit,
		seed.ID("hq-operator"),
	)
	owner(
		t,
		`INSERT INTO devices.device_bindings (tenant_id, device_id, unit_id, customer_org_id, bound_at, actor_membership_id)
			VALUES ($1,$2,$3,$4,now(),$5)`,
		seed.ID("tenant-a"),
		dev,
		boundUnit,
		org,
		seed.ID("hq-operator"),
	)

	// a free device: its serial binds the imported unit (DD-A18 serial column)
	free := "FREE-" + uuid.NewString()[:8]
	freeDev := uuid.NewString()
	owner(
		t,
		`INSERT INTO devices.devices (id, tenant_id, serial, target_unit_id, created_by_membership_id, firmware_version)
			VALUES ($1,$2,$3,$4,$5,'v1')`,
		freeDev,
		seed.ID("tenant-a"),
		free,
		boundUnit,
		seed.ID("hq-operator"),
	)

	nowhere := "Nowhere " + uuid.NewString()[:6]

	csv := "\ufeffProperty,Floor,Room,Unit,Model,Serial,Installed,Warranty\n" +
		prop + ",1F,,Desk AC 1,SPL-100,,2026-01-10,2027-01-10\n" + // ready (existing floor)
		prop + ",1F,Meeting 3,Meeting AC,spl-200v,,,\n" + // warning: creates room
		prop + ",1F,Meeting 3,Meeting AC 2,SPL-100,,,\n" + // warning: same new room
		prop + ",,,Lobby AC,CS-XX99,,,\n" + // error: unknown model
		prop + ",,,Lobby AC 2,SPL-100," + serial + ",,\n" + // error: serial bound
		prop + ",1F,,Desk AC 1,SPL-100,,,\n" + // error: duplicate name in file
		nowhere + ",,,X,SPL-100,,,\n" + // warning: creates the property
		prop + ",,,Future,SPL-100,,2030-01-01,\n" + // error: future install
		prop + ",,,Bad date,SPL-100,,2026-13-40,\n" + // error: invalid date
		prop + ",,,,SPL-100,,,\n" + // error: missing unit name
		prop + ",,,Serial AC,SPL-100," + strings.ToLower(free) + ",,\n" + // ready: binds the free device
		prop + ",,,Serial AC 2,SPL-100," + free + ",,\n" + // error: the same serial twice in the file
		prop + ",,,Ghost AC,SPL-100,NO-SUCH-" + free + ",,\n" // error: unknown serial
	body := `{"customerId":"` + cust + `","fileName":"units.csv","csvText":` + jsonStr(
		csv,
	) + `,"mapping":` + mapping + `}`

	code, resp = post(srv, &hq, "units.importPreview", body)
	if code != 200 {
		t.Fatalf("preview: %d %v", code, resp)
	}

	p := data(resp)
	if asNum(t, p["readyCount"]) != 2 || asNum(t, p["warningCount"]) != 3 ||
		asNum(t, p["errorCount"]) != 8 {
		t.Fatalf(
			"counts: %v %v %v / rows %v",
			p["readyCount"],
			p["warningCount"],
			p["errorCount"],
			p["rows"],
		)
	}

	previewID := asStr(t, p["previewId"])
	keys := map[string]string{}

	for _, r := range asArr(t, p["rows"]) {
		row := asObj(t, r)
		if k, ok := row["messageKey"].(string); ok {
			keys[asStr(t, row["unitName"])] = k
		}
	}

	for unit, key := range map[string]string{
		"X": "warning.importCreatesProperty", "Meeting AC": "warning.importCreatesSpace", "Lobby AC 2": "error.serialBound",
		"Serial AC 2": apperr.KeyDuplicateSerial, "Ghost AC": "error.unknownSerial",
	} {
		if keys[unit] != key {
			t.Errorf("%s: %q, want %q", unit, keys[unit], key)
		}
	}

	// preview validation
	want := map[string]string{ // the field each case is refused for
		"bad quoting":               "csvText=error.malformedCsv",
		"column not in header":      "mapping.unit_name=error.columnNotFound",
		"header only":               "csvText=error.noRows",
		"missing unit_name mapping": "mapping.unit_name=error.required",
		"too many rows":             "csvText=error.tooManyRows",
		"unknown mapping key":       "mapping.color=error.unknownColumn",
	}

	for name, b := range map[string]string{
		"missing unit_name mapping": `{"customerId":"` + cust + `","fileName":"a.csv","csvText":"a,b\n1,2",` +
			`"mapping":{"property":"a","model_code":"b"}}`,
		"unknown mapping key": `{"customerId":"` + cust + `","fileName":"a.csv","csvText":"a,b\n1,2",` +
			`"mapping":{"property":"a","unit_name":"a","model_code":"b","color":"a"}}`,
		"column not in header": `{"customerId":"` + cust + `","fileName":"a.csv","csvText":"a,b\n1,2",` +
			`"mapping":{"property":"a","unit_name":"zz","model_code":"b"}}`,
		"header only": `{"customerId":"` + cust + `","fileName":"a.csv","csvText":"a,b\n",` +
			`"mapping":{"property":"a","unit_name":"a","model_code":"b"}}`,
		"too many rows": `{"customerId":"` + cust + `","fileName":"a.csv",` +
			`"csvText":` + jsonStr("p,u,m\n"+strings.Repeat("x,y,z\n", 1001)) + `,"mapping":{"property":"p","unit_name":"u",` +
			`"model_code":"m"}}`,
		"bad quoting": `{"customerId":"` + cust + `","fileName":"a.csv",` +
			`"csvText":` + jsonStr("p,u,m\n\"x,y\n") + `,"mapping":{"property":"p","unit_name":"u","model_code":"m"}}`,
	} {
		code, m := post(srv, &hq, "units.importPreview", b)
		refusal(t, name, code, m, want[name])
	}

	if code, _ := post(
		srv,
		&hq,
		"units.importPreview",
		strings.Replace(body, cust, uuid.NewString(), 1),
	); code != 404 {
		t.Error("unknown customer")
	}

	// commit with another customer → CONFLICT (re-validate); then the real commit
	if code, _ := write(
		srv,
		&hq,
		"units.importCommit",
		`{"previewId":"`+previewID+`","customerId":"`+seed.ID("cust-a").String()+`"}`,
		0,
	); code != 409 {
		t.Fatalf("wrong customer: %d", code)
	}

	code, resp = post(srv, &hq, "units.importPreview", body)
	if code != 200 {
		t.Fatalf("preview again: %d %v", code, resp)
	}

	previewID = asStr(t, data(resp)["previewId"])

	code, resp = write(
		srv,
		&hq,
		"units.importCommit",
		`{"previewId":"`+previewID+`","customerId":"`+cust+`"}`,
		0,
	)
	if code != 200 {
		t.Fatalf("commit: %d %v", code, resp)
	}

	imp := data(resp)
	if len(asArr(t, imp["createdUnitIds"])) != 5 || len(asArr(t, imp["createdSpaceIds"])) != 1 ||
		len(asArr(t, imp["createdPropertyIds"])) != 1 ||
		len(asArr(t, imp["skippedRowNumbers"])) != 8 ||
		imp["state"] != "imported" {
		t.Fatalf("import result: %v", imp)
	}

	if code, _ := write(
		srv,
		&hq,
		"units.importCommit",
		`{"previewId":"`+previewID+`","customerId":"`+cust+`"}`,
		0,
	); code != 409 {
		t.Error("a preview is used once")
	}
	// the created units exist under the new room
	_, resp = post(srv, &hq, "units.list", `{"filters":{"propertyId":"`+propID+`"},"limit":100}`)
	if len(
		items(resp),
	) != 5 { // 4 imported here + the bound unit (the fifth went to the new property)
		t.Fatalf("units after import: %d", len(items(resp)))
	}

	var desk, serialUnit string

	for _, u := range items(resp) {
		switch u["displayName"] {
		case "Desk AC 1":
			desk = asStr(t, u["id"])
		case "Serial AC":
			serialUnit = asStr(t, u["id"])
		}
	}
	// CSV dates are Kuala Lumpur calendar dates
	if _, g := post(
		srv,
		&hq,
		"units.get",
		`{"id":"`+desk+`"}`,
	); data(g)["installedAt"] != "2026-01-09T16:00:00Z" {
		t.Errorf("installedAt: %v", data(g)["installedAt"])
	}

	if _, doc := post(
		srv,
		&hq,
		"devices.list",
		`{"filters":{"unitId":"`+serialUnit+`"}}`,
	); len(items(doc)) != 1 ||
		items(doc)[0]["id"] != freeDev {
		t.Fatalf("the serial binds the device: %v", doc)
	}

	if _, pl := post(
		srv,
		&hq,
		"properties.list",
		`{"filters":{"customerId":"`+cust+`"},"limit":100}`,
	); !strings.Contains(
		jsonStr2(pl),
		nowhere,
	) {
		t.Fatal("the new property exists")
	}

	// undo: stale version, success, twice
	importID := asStr(t, imp["id"])
	if code, _ := write(
		srv,
		&hq,
		"units.importUndo",
		`{"importId":"`+importID+`","reason":"wrong file"}`,
		2,
	); code != 409 {
		t.Error("stale version")
	}

	code, resp = write(
		srv,
		&hq,
		"units.importUndo",
		`{"importId":"`+importID+`","reason":"wrong file"}`,
		1,
	)
	if code != 200 || data(resp)["state"] != "undone" {
		t.Fatalf("undo: %d %v", code, resp)
	}

	_, resp = post(srv, &hq, "units.list", `{"filters":{"propertyId":"`+propID+`"}}`)
	if len(items(resp)) != 1 {
		t.Fatal("undo archives created units")
	}

	if _, doc := post(
		srv,
		&hq,
		"devices.list",
		`{"filters":{"unitId":"`+serialUnit+`"}}`,
	); len(
		items(doc),
	) != 0 {
		t.Fatalf("undo releases the device: %v", doc)
	}

	if _, pl := post(
		srv,
		&hq,
		"properties.list",
		`{"filters":{"customerId":"`+cust+`"},"limit":100}`,
	); strings.Contains(
		jsonStr2(pl),
		nowhere,
	) {
		t.Fatal("undo archives the created property")
	}

	if code, _ := write(
		srv,
		&hq,
		"units.importUndo",
		`{"importId":"`+importID+`","reason":"again"}`,
		2,
	); code != 409 {
		t.Error("undo twice")
	}

	if code, _ := write(
		srv,
		&hq,
		"units.importUndo",
		`{"importId":"`+uuid.NewString()+`","reason":"x"}`,
		1,
	); code != 404 {
		t.Error("unknown import")
	}

	// undo blocked by telemetry, and after the 24 h window
	_, resp = post(
		srv,
		&hq,
		"units.importPreview",
		`{"customerId":"`+cust+`","fileName":"b.csv","csvText":`+jsonStr(
			"Property,Unit,Model\n"+prop+",Telemetry AC "+uuid.NewString()[:4]+",SPL-100\n",
		)+`,"mapping":{"property":"Property","unit_name":"Unit","model_code":"Model"}}`,
	)
	_, resp = write(
		srv,
		&hq,
		"units.importCommit",
		`{"previewId":"`+asStr(t, data(resp)["previewId"])+`","customerId":"`+cust+`"}`,
		0,
	)
	imp2 := data(resp)
	unit := asStr(t, asArr(t, imp2["createdUnitIds"])[0])
	owner(
		t,
		`INSERT INTO monitoring.measurements (tenant_id, unit_id, sensor_id, metric, observed_at, sequence, value, unit,
			origin, quality, received_at, event_id)
		VALUES ($1,$2,$3,'temperature',$4,1,25,'°C','measured','valid',$4,$5)`,
		seed.ID("tenant-a"),
		unit,
		uuid.New(),
		clock,
		uuid.New(),
	)

	if code, res := write(
		srv,
		&hq,
		"units.importUndo",
		`{"importId":"`+asStr(t, imp2["id"])+`","reason":"x"}`,
		1,
	); code != 409 ||
		res["messageKey"] != "error.importUnitInUse" {
		t.Fatalf("telemetry blocks undo: %d %v", code, res)
	}

	owner(
		t,
		`UPDATE assets.unit_imports SET undo_until = $2 WHERE id = $1`,
		imp2["id"],
		clock.Add(-time.Minute),
	)

	if code, res := write(
		srv,
		&hq,
		"units.importUndo",
		`{"importId":"`+asStr(t, imp2["id"])+`","reason":"x"}`,
		1,
	); code != 409 ||
		res["messageKey"] != "error.undoWindowClosed" {
		t.Fatalf("undo window: %d %v", code, res)
	}
	// expired preview
	_, resp = post(srv, &hq, "units.importPreview", body)
	owner(
		t,
		`UPDATE assets.unit_import_previews SET expires_at = $2 WHERE id = $1`,
		data(resp)["previewId"],
		clock.Add(-time.Second),
	)

	if code, _ := write(
		srv,
		&hq,
		"units.importCommit",
		`{"previewId":"`+asStr(t, data(resp)["previewId"])+`","customerId":"`+cust+`"}`,
		0,
	); code != 409 {
		t.Error("expired preview")
	}

	if code, _ := post(srv, &customerB, "units.importPreview", body); code != 403 {
		t.Error("client cannot import")
	}
}

// TestUnitsImportRefusals covers the import's other refusals (DD-A18): an inactive customer at preview and at commit,
// a header that is not CSV, a unit name over 120 characters, a device with an unresolved tamper, and a preview that
// went stale because its new property was created meanwhile.
func TestUnitsImportRefusals(t *testing.T) {
	srv := server(t)
	cust := seed.ID("cust-b")
	preview := func(csv string) (int, map[string]any) {
		return post(
			srv,
			&hq,
			"units.importPreview",
			`{"customerId":"`+cust.String()+`","fileName":"units.csv","csvText":`+jsonStr(
				csv,
			)+`,"mapping":`+mapping+`}`,
		)
	}
	header := "Property,Floor,Room,Unit,Model,Serial,Installed,Warranty\n"
	site := "Late site " + uuid.NewString()[:6]

	owner(t, `UPDATE assets.customers SET status = 'inactive' WHERE id = $1`, cust)

	code, resp := preview(header + site + ",,,Desk AC,SPL-100,,,\n")

	owner(t, `UPDATE assets.customers SET status = 'active' WHERE id = $1`, cust)

	if code != 409 || resp["messageKey"] != apperr.KeyCustomerInactive {
		t.Errorf("preview for an inactive customer: %d %v", code, resp)
	}

	if code, m := preview(
		`"Property,Unit`,
	); code != 422 ||
		asObj(t, m["fieldErrors"])["csvText"] != apperr.KeyMalformedCsv {
		t.Errorf("a header that is not CSV: %d %v", code, m)
	}
	// a long unit name and a tampered device are row errors
	tampered := "TMP-" + uuid.NewString()[:8]
	owner(
		t,
		`INSERT INTO devices.devices (tenant_id, serial, target_unit_id, created_by_membership_id, firmware_version, tamper)
			VALUES ($1,$2,$3,$4,'v1','detected')`,
		seed.ID("tenant-a"),
		tampered,
		newUnit(t, srv, "Target AC"),
		seed.ID("hq-operator"),
	)

	code, resp = preview(
		header + site + ",,," + strings.Repeat(
			"u",
			121,
		) + ",SPL-100,,,\n" + site + ",,,Tampered AC,SPL-100," + tampered + ",,\n",
	)
	if code != 200 {
		t.Fatalf("preview: %d %v", code, resp)
	}

	keys := map[string]string{}

	for _, r := range asArr(t, data(resp)["rows"]) {
		row := asObj(t, r)
		if k, ok := row["messageKey"].(string); ok {
			keys[asStr(t, row["unitName"])] = k
		}
	}

	if keys[strings.Repeat("u", 121)] != apperr.KeyLength ||
		keys["Tampered AC"] != apperr.KeyTamperUnresolved {
		t.Errorf("row errors: %v", keys)
	}
	// a preview whose new property exists by now, and a customer gone inactive before the commit
	code, resp = preview(header + site + ",,,Desk AC,SPL-100,,,\n")
	if code != 200 {
		t.Fatalf("preview: %d %v", code, resp)
	}

	stale := asStr(t, data(resp)["previewId"])

	if code, res := write(
		srv,
		&hq,
		"properties.save",
		`{"customerOrgId":"`+seed.ID("org-customer-b").
			String()+
			`","kind":"office","name":"`+site+`","address":null,"accessInstructions":null}`,
		0,
	); code != 200 {
		t.Fatalf("property: %d %v", code, res)
	}

	if code, res := write(
		srv,
		&hq,
		"units.importCommit",
		`{"previewId":"`+stale+`","customerId":"`+cust.String()+`"}`,
		0,
	); code != 409 ||
		res["messageKey"] != apperr.KeyPreviewExpired {
		t.Errorf("commit after the property was created: %d %v", code, res)
	}

	code, resp = preview(header + site + ",,,Desk AC 2,SPL-100,,,\n")
	if code != 200 {
		t.Fatalf("preview: %d %v", code, resp)
	}

	owner(t, `UPDATE assets.customers SET status = 'inactive' WHERE id = $1`, cust)
	code, resp = write(
		srv,
		&hq,
		"units.importCommit",
		`{"previewId":"`+asStr(t, data(resp)["previewId"])+`","customerId":"`+cust.String()+`"}`,
		0,
	)
	owner(t, `UPDATE assets.customers SET status = 'active' WHERE id = $1`, cust)

	if code != 409 || resp["messageKey"] != apperr.KeyCustomerInactive {
		t.Errorf("commit for an inactive customer: %d %v", code, resp)
	}
}

// DD-A18: a commit validates again what may have changed since the preview — two rows of one new property create it
// once, and a serial that was bound in between turns the preview stale (error.previewExpired), so nothing is imported.
func TestUnitsImportRevalidation(t *testing.T) {
	srv := server(t)
	cust, org := seed.ID("cust-b").String(), seed.ID("org-customer-b").String()
	commit := func(csv string) (int, map[string]any, map[string]any) {
		_, pm := post(
			srv,
			&hq,
			"units.importPreview",
			`{"customerId":"`+cust+`","fileName":"units.csv","csvText":`+jsonStr(
				csv,
			)+`,"mapping":`+mapping+`}`,
		)
		code, res := write(
			srv,
			&hq,
			"units.importCommit",
			`{"previewId":"`+asStr(t, data(pm)["previewId"])+`","customerId":"`+cust+`"}`,
			0,
		)

		return code, res, data(pm)
	}
	header := "Property,Floor,Room,Unit,Model,Serial,Installed,Warranty\n"
	twin := "Twin site " + uuid.NewString()[:6]

	code, resp, p := commit(
		header + twin + ",,,Twin AC 1,SPL-100,,,\n" + twin + ",,,Twin AC 2,SPL-100,,,\n",
	)
	if code != 200 || len(asArr(t, data(resp)["createdPropertyIds"])) != 1 ||
		len(asArr(t, data(resp)["createdUnitIds"])) != 2 {
		t.Fatalf("one new property for two rows: %d %v (preview %v)", code, resp, p["rows"])
	}

	// a free device, bound to another unit after the preview
	serial := "LATE-" + uuid.NewString()[:8]
	dev := uuid.NewString()
	other := newUnit(t, srv, "Took the device")
	owner(
		t,
		`INSERT INTO devices.devices (id, tenant_id, serial, target_unit_id, created_by_membership_id, firmware_version)
			VALUES ($1,$2,$3,$4,$5,'v1')`,
		dev,
		seed.ID("tenant-a"),
		serial,
		other,
		seed.ID("hq-operator"),
	)

	_, pm := post(
		srv,
		&hq,
		"units.importPreview",
		`{"customerId":"`+cust+`","fileName":"units.csv","csvText":`+jsonStr(
			header+twin+",,,Late AC,SPL-100,"+serial+",,\n",
		)+`,"mapping":`+mapping+`}`,
	)
	if asNum(t, data(pm)["readyCount"]) != 1 {
		t.Fatalf("preview with a free serial: %v", pm)
	}

	owner(
		t,
		`INSERT INTO devices.device_bindings (tenant_id, device_id, unit_id, customer_org_id, bound_at, actor_membership_id)
			VALUES ($1,$2,$3,$4,$5,$6)`,
		seed.ID("tenant-a"),
		dev,
		other,
		org,
		clock,
		seed.ID("hq-operator"),
	)
	owner(t, `UPDATE devices.devices SET unit_id = $2 WHERE id = $1`, dev, other)

	if code, res := write(
		srv,
		&hq,
		"units.importCommit",
		`{"previewId":"`+asStr(t, data(pm)["previewId"])+`","customerId":"`+cust+`"}`,
		0,
	); code != 409 ||
		res["messageKey"] != apperr.KeyPreviewExpired {
		t.Errorf("serial bound since the preview: %d %v", code, res)
	}
	// nothing imported: units.list has no search filter, so look through the customer's units (IR388: the earlier
	// check sent one, got VALIDATION and passed on an empty list)
	code, resp = post(srv, &hq, "units.list", `{"filters":{"customerId":"`+cust+`"},"limit":100}`)
	if code != 200 {
		t.Fatalf("units.list: %d %v", code, resp)
	}

	for _, u := range items(resp) {
		if u["displayName"] == "Late AC" {
			t.Errorf("imported after all: %v", u)
		}
	}
}
