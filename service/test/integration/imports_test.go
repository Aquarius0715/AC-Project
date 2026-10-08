package integration

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/internal/seed"
)

func jsonStr(s string) string { b, _ := json.Marshal(s); return string(b) }

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

	csv := "\ufeffProperty,Floor,Room,Unit,Model,Serial,Installed,Warranty\n" +
		prop + ",1F,,Desk AC 1,SPL-100,,2026-01-10,2027-01-10\n" + // ready (existing floor)
		prop + ",1F,Meeting 3,Meeting AC,spl-200v,,,\n" + // warning: creates room
		prop + ",1F,Meeting 3,Meeting AC 2,SPL-100,,,\n" + // warning: same new room
		prop + ",,,Lobby AC,CS-XX99,,,\n" + // error: unknown model
		prop + ",,,Lobby AC 2,SPL-100," + serial + ",,\n" + // error: serial bound
		prop + ",1F,,Desk AC 1,SPL-100,,,\n" + // error: duplicate name in file
		"Nowhere,,,X,SPL-100,,,\n" + // error: unknown property
		prop + ",,,Future,SPL-100,,2030-01-01,\n" + // error: future install
		prop + ",,,Bad date,SPL-100,,2026-13-40,\n" + // error: invalid date
		prop + ",,,,SPL-100,,,\n" // error: missing unit name
	body := `{"customerId":"` + cust + `","fileName":"units.csv","csvText":` + jsonStr(csv) + `,"mapping":` + mapping + `}`
	code, m = post(s, &hq, "units.importPreview", body)
	if code != 200 {
		t.Fatalf("preview: %d %v", code, m)
	}
	p := data(m)
	if p["readyCount"].(float64) != 1 || p["warningCount"].(float64) != 2 || p["errorCount"].(float64) != 7 {
		t.Fatalf("counts: %v %v %v / rows %v", p["readyCount"], p["warningCount"], p["errorCount"], p["rows"])
	}
	previewID := p["previewId"].(string)

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
	if len(imp["createdUnitIds"].([]any)) != 3 || len(imp["createdSpaceIds"].([]any)) != 1 || len(imp["skippedRowNumbers"].([]any)) != 7 || imp["state"] != "imported" {
		t.Fatalf("import result: %v", imp)
	}
	if code, _ := write(s, &hq, "units.importCommit", `{"previewId":"`+previewID+`","customerId":"`+cust+`"}`, 0); code != 409 {
		t.Error("a preview is used once")
	}
	// the created units exist under the new room
	_, m = post(s, &hq, "units.list", `{"filters":{"propertyId":"`+propID+`"},"limit":100}`)
	if len(items(m)) != 4 { // 3 imported + the bound unit
		t.Fatalf("units after import: %d", len(items(m)))
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
