package integration

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

func jsonStr(s string) string { b, _ := json.Marshal(s); return string(b) }
func jsonStr2(v any) string   { b, _ := json.Marshal(v); return string(b) }

const mapping = `{"property":"Property","floor":"Floor","room":"Room","unit_name":"Unit","model_code":"Model","serial":"Serial","installed_on":"Installed","warranty_end":"Warranty"}`

func TestUnitsImport(t *testing.T) {
	s := server(t)
	cust := seed.ID("cust-b").String()
	org := seed.ID("org-customer-b").String()
	prop := "Import site " + uuid.NewString()[:6]
	code, m := write(s, &hq, "properties.save", `{"customerOrgId":"`+org+`","kind":"office","name":"`+prop+`","address":null,"accessInstructions":null}`, 0)
	if code != 200 {
		t.Fatalf("property: %d %v", code, m)
	}
	propID := data(m)["id"].(string)
	_, m = write(s, &hq, "spaces.save", `{"propertyId":"`+propID+`","parentSpaceId":null,"kind":"floor","name":"1F"}`, 0)
	// a device bound to some unit, so its serial is an error row
	serial := "IMP-" + uuid.NewString()[:8]
	dev := uuid.NewString()
	_, um := write(s, &hq, "units.save", `{"customerOrgId":"`+org+`","propertyId":"`+propID+`","spaceId":null,"displayName":"Bound AC","modelId":"`+seed.ID("ventilation-demo").String()+`","type":"split","installedAt":null,"serviceScope":["indoor"]}`, 0)
	boundUnit := data(um)["id"]
	owner(t, `INSERT INTO devices.devices (id, tenant_id, serial, target_unit_id, created_by_membership_id, firmware_version) VALUES ($1,$2,$3,$4,$5,'v1')`,
		dev, seed.ID("tenant-a"), serial, boundUnit, seed.ID("hq-operator"))
	owner(t, `INSERT INTO devices.device_bindings (tenant_id, device_id, unit_id, customer_org_id, bound_at, actor_membership_id) VALUES ($1,$2,$3,$4,now(),$5)`,
		seed.ID("tenant-a"), dev, boundUnit, org, seed.ID("hq-operator"))

	// a free device: its serial binds the imported unit (DD-A18 serial column)
	free := "FREE-" + uuid.NewString()[:8]
	freeDev := uuid.NewString()
	owner(t, `INSERT INTO devices.devices (id, tenant_id, serial, target_unit_id, created_by_membership_id, firmware_version) VALUES ($1,$2,$3,$4,$5,'v1')`,
		freeDev, seed.ID("tenant-a"), free, boundUnit, seed.ID("hq-operator"))
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
	body := `{"customerId":"` + cust + `","fileName":"units.csv","csvText":` + jsonStr(csv) + `,"mapping":` + mapping + `}`
	code, m = post(s, &hq, "units.importPreview", body)
	if code != 200 {
		t.Fatalf("preview: %d %v", code, m)
	}
	p := data(m)
	if p["readyCount"].(float64) != 2 || p["warningCount"].(float64) != 3 || p["errorCount"].(float64) != 8 {
		t.Fatalf("counts: %v %v %v / rows %v", p["readyCount"], p["warningCount"], p["errorCount"], p["rows"])
	}
	previewID := p["previewId"].(string)
	keys := map[string]string{}
	for _, r := range p["rows"].([]any) {
		row := r.(map[string]any)
		if k, ok := row["messageKey"].(string); ok {
			keys[row["unitName"].(string)] = k
		}
	}
	for unit, key := range map[string]string{"X": "warning.importCreatesProperty", "Meeting AC": "warning.importCreatesSpace", "Lobby AC 2": "error.serialBound",
		"Serial AC 2": "error.duplicateSerial", "Ghost AC": "error.unknownSerial"} {
		if keys[unit] != key {
			t.Errorf("%s: %q, want %q", unit, keys[unit], key)
		}
	}

	// preview validation
	for name, b := range map[string]string{
		"missing unit_name mapping": `{"customerId":"` + cust + `","fileName":"a.csv","csvText":"a,b\n1,2","mapping":{"property":"a","model_code":"b"}}`,
		"unknown mapping key":       `{"customerId":"` + cust + `","fileName":"a.csv","csvText":"a,b\n1,2","mapping":{"property":"a","unit_name":"a","model_code":"b","color":"a"}}`,
		"column not in header":      `{"customerId":"` + cust + `","fileName":"a.csv","csvText":"a,b\n1,2","mapping":{"property":"a","unit_name":"zz","model_code":"b"}}`,
		"header only":               `{"customerId":"` + cust + `","fileName":"a.csv","csvText":"a,b\n","mapping":{"property":"a","unit_name":"a","model_code":"b"}}`,
		"too many rows":             `{"customerId":"` + cust + `","fileName":"a.csv","csvText":` + jsonStr("p,u,m\n"+strings.Repeat("x,y,z\n", 1001)) + `,"mapping":{"property":"p","unit_name":"u","model_code":"m"}}`,
		"bad quoting":               `{"customerId":"` + cust + `","fileName":"a.csv","csvText":` + jsonStr("p,u,m\n\"x,y\n") + `,"mapping":{"property":"p","unit_name":"u","model_code":"m"}}`,
	} {
		if code, _ := post(s, &hq, "units.importPreview", b); code != 422 {
			t.Errorf("%s: %d", name, code)
		}
	}
	if code, _ := post(s, &hq, "units.importPreview", strings.Replace(body, cust, uuid.NewString(), 1)); code != 404 {
		t.Error("unknown customer")
	}

	// commit with another customer → CONFLICT (re-validate); then the real commit
	if code, _ := write(s, &hq, "units.importCommit", `{"previewId":"`+previewID+`","customerId":"`+seed.ID("cust-a").String()+`"}`, 0); code != 409 {
		t.Fatalf("wrong customer: %d", code)
	}
	code, m = post(s, &hq, "units.importPreview", body)
	previewID = data(m)["previewId"].(string)
	code, m = write(s, &hq, "units.importCommit", `{"previewId":"`+previewID+`","customerId":"`+cust+`"}`, 0)
	if code != 200 {
		t.Fatalf("commit: %d %v", code, m)
	}
	imp := data(m)
	if len(imp["createdUnitIds"].([]any)) != 5 || len(imp["createdSpaceIds"].([]any)) != 1 || len(imp["createdPropertyIds"].([]any)) != 1 ||
		len(imp["skippedRowNumbers"].([]any)) != 8 || imp["state"] != "imported" {
		t.Fatalf("import result: %v", imp)
	}
	if code, _ := write(s, &hq, "units.importCommit", `{"previewId":"`+previewID+`","customerId":"`+cust+`"}`, 0); code != 409 {
		t.Error("a preview is used once")
	}
	// the created units exist under the new room
	_, m = post(s, &hq, "units.list", `{"filters":{"propertyId":"`+propID+`"},"limit":100}`)
	if len(items(m)) != 5 { // 4 imported here + the bound unit (the fifth went to the new property)
		t.Fatalf("units after import: %d", len(items(m)))
	}
	var desk, serialUnit string
	for _, u := range items(m) {
		switch u["displayName"] {
		case "Desk AC 1":
			desk = u["id"].(string)
		case "Serial AC":
			serialUnit = u["id"].(string)
		}
	}
	// CSV dates are Kuala Lumpur calendar dates
	if _, g := post(s, &hq, "units.get", `{"id":"`+desk+`"}`); data(g)["installedAt"] != "2026-01-09T16:00:00Z" {
		t.Errorf("installedAt: %v", data(g)["installedAt"])
	}
	if _, d := post(s, &hq, "devices.list", `{"filters":{"unitId":"`+serialUnit+`"}}`); len(items(d)) != 1 || items(d)[0]["id"] != freeDev {
		t.Fatalf("the serial binds the device: %v", d)
	}
	if _, pl := post(s, &hq, "properties.list", `{"filters":{"customerId":"`+cust+`"},"limit":100}`); !strings.Contains(jsonStr2(pl), nowhere) {
		t.Fatal("the new property exists")
	}

	// undo: stale version, success, twice
	importID := imp["id"].(string)
	if code, _ := write(s, &hq, "units.importUndo", `{"importId":"`+importID+`","reason":"wrong file"}`, 2); code != 409 {
		t.Error("stale version")
	}
	code, m = write(s, &hq, "units.importUndo", `{"importId":"`+importID+`","reason":"wrong file"}`, 1)
	if code != 200 || data(m)["state"] != "undone" {
		t.Fatalf("undo: %d %v", code, m)
	}
	_, m = post(s, &hq, "units.list", `{"filters":{"propertyId":"`+propID+`"}}`)
	if len(items(m)) != 1 {
		t.Fatal("undo archives created units")
	}
	if _, d := post(s, &hq, "devices.list", `{"filters":{"unitId":"`+serialUnit+`"}}`); len(items(d)) != 0 {
		t.Fatalf("undo releases the device: %v", d)
	}
	if _, pl := post(s, &hq, "properties.list", `{"filters":{"customerId":"`+cust+`"},"limit":100}`); strings.Contains(jsonStr2(pl), nowhere) {
		t.Fatal("undo archives the created property")
	}
	if code, _ := write(s, &hq, "units.importUndo", `{"importId":"`+importID+`","reason":"again"}`, 2); code != 409 {
		t.Error("undo twice")
	}
	if code, _ := write(s, &hq, "units.importUndo", `{"importId":"`+uuid.NewString()+`","reason":"x"}`, 1); code != 404 {
		t.Error("unknown import")
	}

	// undo blocked by telemetry, and after the 24 h window
	_, m = post(s, &hq, "units.importPreview", `{"customerId":"`+cust+`","fileName":"b.csv","csvText":`+jsonStr("Property,Unit,Model\n"+prop+",Telemetry AC "+uuid.NewString()[:4]+",SPL-100\n")+`,"mapping":{"property":"Property","unit_name":"Unit","model_code":"Model"}}`)
	_, m = write(s, &hq, "units.importCommit", `{"previewId":"`+data(m)["previewId"].(string)+`","customerId":"`+cust+`"}`, 0)
	imp2 := data(m)
	unit := imp2["createdUnitIds"].([]any)[0].(string)
	owner(t, `INSERT INTO monitoring.measurements (tenant_id, unit_id, sensor_id, metric, observed_at, sequence, value, unit, origin, quality, received_at, event_id)
		VALUES ($1,$2,$3,'temperature',$4,1,25,'°C','measured','valid',$4,$5)`, seed.ID("tenant-a"), unit, uuid.New(), clock, uuid.New())
	if code, m := write(s, &hq, "units.importUndo", `{"importId":"`+imp2["id"].(string)+`","reason":"x"}`, 1); code != 409 || m["messageKey"] != "error.importUnitInUse" {
		t.Fatalf("telemetry blocks undo: %d %v", code, m)
	}
	owner(t, `UPDATE assets.unit_imports SET undo_until = $2 WHERE id = $1`, imp2["id"], clock.Add(-time.Minute))
	if code, m := write(s, &hq, "units.importUndo", `{"importId":"`+imp2["id"].(string)+`","reason":"x"}`, 1); code != 409 || m["messageKey"] != "error.undoWindowClosed" {
		t.Fatalf("undo window: %d %v", code, m)
	}
	// expired preview
	_, m = post(s, &hq, "units.importPreview", body)
	owner(t, `UPDATE assets.unit_import_previews SET expires_at = $2 WHERE id = $1`, data(m)["previewId"], clock.Add(-time.Second))
	if code, _ := write(s, &hq, "units.importCommit", `{"previewId":"`+data(m)["previewId"].(string)+`","customerId":"`+cust+`"}`, 0); code != 409 {
		t.Error("expired preview")
	}
	if code, _ := post(s, &customerB, "units.importPreview", body); code != 403 {
		t.Error("client cannot import")
	}
}

// TestUnitsImportRefusals covers the import's other refusals (DD-A18): an inactive customer at preview and at commit,
// a header that is not CSV, a unit name over 120 characters, a device with an unresolved tamper, and a preview that
// went stale because its new property was created meanwhile.
func TestUnitsImportRefusals(t *testing.T) {
	s := server(t)
	cust := seed.ID("cust-b")
	preview := func(csv string) (int, map[string]any) {
		return post(s, &hq, "units.importPreview", `{"customerId":"`+cust.String()+`","fileName":"units.csv","csvText":`+jsonStr(csv)+`,"mapping":`+mapping+`}`)
	}
	header := "Property,Floor,Room,Unit,Model,Serial,Installed,Warranty\n"
	site := "Late site " + uuid.NewString()[:6]
	owner(t, `UPDATE assets.customers SET status = 'inactive' WHERE id = $1`, cust)
	code, m := preview(header + site + ",,,Desk AC,SPL-100,,,\n")
	owner(t, `UPDATE assets.customers SET status = 'active' WHERE id = $1`, cust)
	if code != 409 || m["messageKey"] != "error.customerInactive" {
		t.Errorf("preview for an inactive customer: %d %v", code, m)
	}
	if code, m := preview(`"Property,Unit`); code != 422 || m["fieldErrors"].(map[string]any)["csvText"] != "error.malformedCsv" {
		t.Errorf("a header that is not CSV: %d %v", code, m)
	}
	// a long unit name and a tampered device are row errors
	tampered := "TMP-" + uuid.NewString()[:8]
	owner(t, `INSERT INTO devices.devices (tenant_id, serial, target_unit_id, created_by_membership_id, firmware_version, tamper) VALUES ($1,$2,$3,$4,'v1','detected')`,
		seed.ID("tenant-a"), tampered, newUnit(t, s, "Target AC"), seed.ID("hq-operator"))
	code, m = preview(header + site + ",,," + strings.Repeat("u", 121) + ",SPL-100,,,\n" + site + ",,,Tampered AC,SPL-100," + tampered + ",,\n")
	if code != 200 {
		t.Fatalf("preview: %d %v", code, m)
	}
	keys := map[string]string{}
	for _, r := range data(m)["rows"].([]any) {
		row := r.(map[string]any)
		if k, ok := row["messageKey"].(string); ok {
			keys[row["unitName"].(string)] = k
		}
	}
	if keys[strings.Repeat("u", 121)] != "error.length" || keys["Tampered AC"] != "error.tamperUnresolved" {
		t.Errorf("row errors: %v", keys)
	}
	// a preview whose new property exists by now, and a customer gone inactive before the commit
	code, m = preview(header + site + ",,,Desk AC,SPL-100,,,\n")
	if code != 200 {
		t.Fatalf("preview: %d %v", code, m)
	}
	stale := data(m)["previewId"].(string)
	if code, m := write(s, &hq, "properties.save", `{"customerOrgId":"`+seed.ID("org-customer-b").String()+`","kind":"office","name":"`+site+`","address":null,"accessInstructions":null}`, 0); code != 200 {
		t.Fatalf("property: %d %v", code, m)
	}
	if code, m := write(s, &hq, "units.importCommit", `{"previewId":"`+stale+`","customerId":"`+cust.String()+`"}`, 0); code != 409 || m["messageKey"] != "error.previewExpired" {
		t.Errorf("commit after the property was created: %d %v", code, m)
	}
	code, m = preview(header + site + ",,,Desk AC 2,SPL-100,,,\n")
	if code != 200 {
		t.Fatalf("preview: %d %v", code, m)
	}
	owner(t, `UPDATE assets.customers SET status = 'inactive' WHERE id = $1`, cust)
	code, m = write(s, &hq, "units.importCommit", `{"previewId":"`+data(m)["previewId"].(string)+`","customerId":"`+cust.String()+`"}`, 0)
	owner(t, `UPDATE assets.customers SET status = 'active' WHERE id = $1`, cust)
	if code != 409 || m["messageKey"] != "error.customerInactive" {
		t.Errorf("commit for an inactive customer: %d %v", code, m)
	}
}

// DD-A18: a commit validates again what may have changed since the preview — two rows of one new property create it
// once, and a serial that was bound in between turns the preview stale (error.previewExpired), so nothing is imported.
func TestUnitsImportRevalidation(t *testing.T) {
	s := server(t)
	cust, org := seed.ID("cust-b").String(), seed.ID("org-customer-b").String()
	commit := func(csv string) (int, map[string]any, map[string]any) {
		_, pm := post(s, &hq, "units.importPreview", `{"customerId":"`+cust+`","fileName":"units.csv","csvText":`+jsonStr(csv)+`,"mapping":`+mapping+`}`)
		code, m := write(s, &hq, "units.importCommit", `{"previewId":"`+data(pm)["previewId"].(string)+`","customerId":"`+cust+`"}`, 0)
		return code, m, data(pm)
	}
	header := "Property,Floor,Room,Unit,Model,Serial,Installed,Warranty\n"
	twin := "Twin site " + uuid.NewString()[:6]
	code, m, p := commit(header + twin + ",,,Twin AC 1,SPL-100,,,\n" + twin + ",,,Twin AC 2,SPL-100,,,\n")
	if code != 200 || len(data(m)["createdPropertyIds"].([]any)) != 1 || len(data(m)["createdUnitIds"].([]any)) != 2 {
		t.Fatalf("one new property for two rows: %d %v (preview %v)", code, m, p["rows"])
	}

	// a free device, bound to another unit after the preview
	serial := "LATE-" + uuid.NewString()[:8]
	dev := uuid.NewString()
	other := newUnit(t, s, "Took the device")
	owner(t, `INSERT INTO devices.devices (id, tenant_id, serial, target_unit_id, created_by_membership_id, firmware_version) VALUES ($1,$2,$3,$4,$5,'v1')`,
		dev, seed.ID("tenant-a"), serial, other, seed.ID("hq-operator"))
	_, pm := post(s, &hq, "units.importPreview", `{"customerId":"`+cust+`","fileName":"units.csv","csvText":`+jsonStr(header+twin+",,,Late AC,SPL-100,"+serial+",,\n")+`,"mapping":`+mapping+`}`)
	if data(pm)["readyCount"].(float64) != 1 {
		t.Fatalf("preview with a free serial: %v", pm)
	}
	owner(t, `INSERT INTO devices.device_bindings (tenant_id, device_id, unit_id, customer_org_id, bound_at, actor_membership_id) VALUES ($1,$2,$3,$4,$5,$6)`,
		seed.ID("tenant-a"), dev, other, org, clock, seed.ID("hq-operator"))
	owner(t, `UPDATE devices.devices SET unit_id = $2 WHERE id = $1`, dev, other)
	if code, m := write(s, &hq, "units.importCommit", `{"previewId":"`+data(pm)["previewId"].(string)+`","customerId":"`+cust+`"}`, 0); code != 409 || m["messageKey"] != "error.previewExpired" {
		t.Errorf("serial bound since the preview: %d %v", code, m)
	}
	_, m = post(s, &hq, "units.list", `{"filters":{"customerId":"`+cust+`","search":"Late AC"},"limit":100}`)
	if len(items(m)) != 0 {
		t.Errorf("nothing imported: %v", items(m))
	}
}
