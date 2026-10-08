package app

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/core/seed"
)

func TestFilterCare(t *testing.T) {
	s := server(t)
	tenant := seed.ID("tenant-a")
	owner(t, `DELETE FROM maintenance.filter_care_settings WHERE customer_id = $1`, seed.ID("cust-b"))
	u := newUnit(t, s, "Filter AC")
	owner(t, `UPDATE assets.units SET connection = 'online' WHERE id = $1`, u)
	dev, sensor := uuid.NewString(), uuid.NewString()
	owner(t, `INSERT INTO devices.devices (id, tenant_id, serial, target_unit_id, created_by_membership_id, firmware_version) VALUES ($1,$2,$3,$4,$5,'v1')`, dev, tenant, "FC-"+dev[:8], u, seed.ID("hq-operator"))
	owner(t, `INSERT INTO devices.sensors (id, tenant_id, device_id, metric, unit, stale_after_seconds) VALUES ($1,$2,$3,'power','kW',1000000)`, sensor, tenant, dev)
	reading := func(ago time.Duration, v float64, seq int) {
		owner(t, `INSERT INTO monitoring.measurements (tenant_id, unit_id, sensor_id, metric, observed_at, sequence, value, unit, origin, quality, received_at, event_id)
			VALUES ($1,$2,$3,'power',$4,$5,$6,'kW','measured','valid',$4,$7)`, tenant, u, sensor, clock.Add(-ago), seq, v, uuid.NewString())
	}
	reading(210*time.Hour, 1.2, 1) // on for 200 h
	reading(10*time.Hour, 0, 2)    // off
	statusOf := func(a *actor) map[string]any {
		_, m := post(s, a, "filterCare.list", `{"filters":{"unitId":"`+u+`"}}`)
		if len(items(m)) != 1 {
			t.Fatalf("list: %v", m)
		}
		return items(m)[0]
	}
	st := statusOf(&customerB)
	if st["runHoursSinceCleaning"].(float64) != 200 || st["status"] != "due_soon" || st["thresholdHours"].(float64) != 250 || st["lastCleanedAt"] != nil {
		t.Fatalf("due soon: %v", st)
	}
	// settings (owner only)
	for name, b := range map[string]string{
		"threshold 49":  `{"thresholdHours":49,"fallbackDays":30,"recipients":"owners","channels":["inApp"]}`,
		"fallback 181":  `{"thresholdHours":null,"fallbackDays":181,"recipients":"owners","channels":["inApp"]}`,
		"no inApp":      `{"thresholdHours":null,"fallbackDays":30,"recipients":"owners","channels":["email"]}`,
		"whatsapp":      `{"thresholdHours":null,"fallbackDays":30,"recipients":"owners","channels":["inApp","whatsapp"]}`,
		"bad recipient": `{"thresholdHours":null,"fallbackDays":30,"recipients":"everyone","channels":["inApp"]}`,
	} {
		if code, _ := write(s, &customerB, "filterCare.saveSettings", b, 0); code != 422 {
			t.Errorf("settings %s: %d", name, code)
		}
	}
	if code, m := write(s, &customerB, "filterCare.saveSettings", `{"thresholdHours":100,"fallbackDays":14,"recipients":"all_users","channels":["inApp","email"]}`, 0); code != 200 ||
		data(m)["version"].(float64) != 1 {
		t.Fatalf("save settings: %d %v", code, m)
	}
	if st := statusOf(&customerB); st["status"] != "overdue" || st["thresholdHours"].(float64) != 100 {
		t.Fatalf("overdue: %v", st)
	}
	owner(t, `UPDATE identity.memberships SET client_role = 'member' WHERE id = $1`, seed.ID("customer-b"))
	if code, _ := write(s, &customerB, "filterCare.saveSettings", `{"thresholdHours":null,"fallbackDays":30,"recipients":"owners","channels":["inApp"]}`, 0); code != 403 {
		t.Error("member saves settings")
	}
	owner(t, `UPDATE identity.memberships SET client_role = 'owner' WHERE id = $1`, seed.ID("customer-b"))
	// mark cleaned → no run time since → fallback days decide
	if code, _ := write(s, &customerA, "filterCare.markCleaned", `{"unitId":"`+u+`"}`, 0); code != 404 {
		t.Error("other customer marks cleaned")
	}
	code, m := write(s, &customerB, "filterCare.markCleaned", `{"unitId":"`+u+`"}`, 0)
	if code != 200 || data(m)["lastCleanedBy"] != "customer" || data(m)["status"] != "ok" || data(m)["runHoursSinceCleaning"] != nil {
		t.Fatalf("mark cleaned: %d %v", code, m)
	}
	owner(t, `UPDATE maintenance.filter_cleanings SET cleaned_at = $2 WHERE unit_id = $1`, u, clock.Add(-12*24*time.Hour))
	if st := statusOf(&customerB); st["status"] != "overdue" || st["runHoursSinceCleaning"].(float64) != 200 {
		t.Fatalf("run time since an older cleaning: %v", st)
	}
	owner(t, `UPDATE assets.units SET connection = 'offline' WHERE id = $1`, u)
	if st := statusOf(&customerB); st["status"] != "due_soon" || st["runHoursSinceCleaning"] != nil { // offline: 12 of 14 days
		t.Fatalf("fallback days: %v", st)
	}
	owner(t, `UPDATE assets.units SET connection = 'online' WHERE id = $1`, u)
	// a technician cleaning (accepted report, filter normal) is newer
	job := uuid.NewString()
	owner(t, `INSERT INTO maintenance.jobs (id, tenant_id, unit_id, customer_org_id, type, status, origin, requested_slot, due_at, completed_at)
		VALUES ($1,$2,$3,$4,'preventive','completed','client_request',tstzrange($5,$6),$6,$6)`, job, tenant, u, seed.ID("org-customer-b"), clock.Add(-3*time.Hour), clock.Add(-2*time.Hour))
	owner(t, `INSERT INTO maintenance.work_reports (id, version, tenant_id, job_id, author_id, state, items) VALUES ($1, 1, $2, $3, $4, 'accepted', '[{"componentKey":"filter","result":"normal"}]')`,
		uuid.NewString(), tenant, job, seed.ID("user-tech-internal-a"))
	if st := statusOf(&customerB); st["lastCleanedBy"] != "technician" || st["lastCleaningJobId"] != job || st["status"] != "ok" {
		t.Fatalf("technician cleaning: %v", st)
	}
	// offline without a cleaning date → unknown
	u2 := newUnit(t, s, "Filter AC 2")
	_, m = post(s, &customerB, "filterCare.list", `{"filters":{"unitId":"`+u2+`","status":"unknown"}}`)
	if len(items(m)) != 1 || items(m)[0]["runHoursSinceCleaning"] != nil {
		t.Fatalf("unknown: %v", m)
	}
	if _, m := post(s, &customerA, "filterCare.list", `{"filters":{"unitId":"`+u+`"}}`); len(items(m)) != 0 {
		t.Error("customer-a sees customer-b's unit")
	}
	if _, m := post(s, &customerB, "filterCare.list", `{"sort":{"field":"status","direction":"desc"},"limit":100}`); len(items(m)) < 2 {
		t.Error("sorted list")
	}
	for _, b := range []string{`{"filters":{"status":"dirty"}}`, `{"filters":{"x":1}}`, `{"sort":{"field":"runHours","direction":"asc"}}`} {
		if code, _ := post(s, &customerB, "filterCare.list", b); code != 422 {
			t.Errorf("%s: %d", b, code)
		}
	}
	owner(t, `UPDATE assets.units SET archived = true WHERE id = $1`, u2)
	if code, _ := write(s, &customerB, "filterCare.markCleaned", `{"unitId":"`+u2+`"}`, 0); code != 422 {
		t.Error("archived unit")
	}
}
