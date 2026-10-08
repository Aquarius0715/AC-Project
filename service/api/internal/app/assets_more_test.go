package app

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/core/seed"
)

// owner runs SQL as the schema owner (test fixtures that belong to modules not implemented yet).
func owner(t *testing.T, sql string, args ...any) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, "postgres://postgres:local@localhost:5432/ac?sslmode=disable")
	if err != nil {
		t.Skip(err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, sql, args...); err != nil {
		t.Fatal(err)
	}
}

// ownerScan runs a single-row query as the database owner and scans it into dest.
func ownerScan(t *testing.T, sql string, args []any, dest ...any) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, "postgres://postgres:local@localhost:5432/ac?sslmode=disable")
	if err != nil {
		t.Skip(err)
	}
	defer conn.Close(ctx)
	if err := conn.QueryRow(ctx, sql, args...).Scan(dest...); err != nil {
		t.Fatal(err)
	}
}

func newPolicy(t *testing.T, customer, kind string) string {
	id := uuid.NewString()
	var cust any = seed.ID(customer)
	cond, rules := `{"metric":"temperature"}`, any(nil)
	if kind == "default_alert" {
		cust, cond, rules = nil, "", `[]`
	}
	var condArg any = cond
	if cond == "" {
		condArg = nil
	}
	if kind == "default_alert" { // one default policy per tenant: reuse it when it exists
		owner(t, `INSERT INTO monitoring.alert_policies (id, tenant_id, kind, customer_id, name, owner_membership_id, created_by_user_id, timezone, condition, rules)
			SELECT $1,$2,'default_alert',NULL,'Default policy',$3,$4,'Asia/Kuala_Lumpur',NULL,'[]'
			WHERE NOT EXISTS (SELECT 1 FROM monitoring.alert_policies WHERE tenant_id = $2 AND kind = 'default_alert')`,
			id, seed.ID("tenant-a"), seed.ID("hq-operator"), seed.ID("user-hq-operator"))
		return defaultPolicyID(t)
	}
	owner(t, `INSERT INTO monitoring.alert_policies (id, tenant_id, kind, customer_id, name, owner_membership_id, created_by_user_id, timezone, condition, rules)
		VALUES ($1,$2,$3,$4,'test policy',$5,$6,'Asia/Kuala_Lumpur',$7,$8)`, id, seed.ID("tenant-a"), kind, cust, seed.ID("hq-operator"), seed.ID("user-hq-operator"), condArg, rules)
	return id
}

func TestUnitsSetAlertPoliciesAndDelete(t *testing.T) {
	s := server(t)
	pA, pA2, pB := newPolicy(t, "cust-a", "alert"), newPolicy(t, "cust-a", "alert"), newPolicy(t, "cust-b", "alert")
	def := newPolicy(t, "cust-a", "default_alert")
	unit := seed.ID("unit-non-rto").String()
	_, m := post(s, &customerA, "units.get", `{"id":"`+unit+`"}`)
	v := ver(m)
	set := func(a *actor, ids string, version int) (int, map[string]any) {
		return write(s, a, "units.setAlertPolicies", `{"unitId":"`+unit+`","alertPolicyIds":[`+ids+`]}`, version)
	}
	code, m := set(&customerA, `"`+pA+`","`+pA2+`"`, v)
	if code != 200 || len(data(m)["alertPolicyIds"].([]any)) != 2 {
		t.Fatalf("attach: %d %v", code, m)
	}
	v++
	if code, m := set(&customerA, `"`+pA2+`"`, v); code != 200 || len(data(m)["alertPolicyIds"].([]any)) != 1 {
		t.Fatalf("replace: %d", code)
	} else {
		v++
	}
	for name, tc := range map[string]struct {
		ids  string
		code int
	}{
		"other customer's policy": {`"` + pB + `"`, 404},
		"default policy":          {`"` + def + `"`, 422},
		"unknown policy":          {`"` + uuid.NewString() + `"`, 404},
		"duplicate":               {`"` + pA + `","` + pA + `"`, 422},
	} {
		if code, _ := set(&customerA, tc.ids, v); code != tc.code {
			t.Errorf("%s: %d want %d", name, code, tc.code)
		}
	}
	if code, _ := set(&customerB, `"`+pB+`"`, v); code != 404 {
		t.Error("customer-b cannot touch customer-a's unit")
	}
	if code, _ := set(&customerA, ``, v-1); code != 409 {
		t.Error("stale version")
	}
	if code, m := set(&hq, ``, v); code != 200 || len(data(m)["alertPolicyIds"].([]any)) != 0 {
		t.Fatalf("detach all: %d", code)
	}

	// delete: a fresh unit without contracts, jobs or IoT is deleted; a unit with a device binding is CONFLICT
	prop, model := seed.ID("property-home-a").String(), seed.ID("ventilation-demo").String()
	mk := func() string {
		code, m := write(s, &hq, "units.save", `{"customerOrgId":"`+seed.ID("org-customer-a").String()+`","propertyId":"`+prop+`","spaceId":null,"displayName":"Temp `+uuid.NewString()[:6]+`","modelId":"`+model+`","type":"split","installedAt":null,"serviceScope":["indoor"]}`, 0)
		if code != 200 {
			t.Fatalf("unit: %d %v", code, m)
		}
		return data(m)["id"].(string)
	}
	u1 := mk()
	if code, _ := write(s, &hq, "units.delete", `{"id":"`+u1+`","reason":"created by mistake"}`, 2); code != 409 {
		t.Error("stale delete")
	}
	if code, m := write(s, &hq, "units.delete", `{"id":"`+u1+`","reason":"created by mistake"}`, 1); code != 200 || data(m)["deleted"] != true {
		t.Fatalf("delete: %d %v", code, m)
	}
	if code, _ := post(s, &hq, "units.get", `{"id":"`+u1+`"}`); code != 404 {
		t.Error("deleted unit is gone")
	}
	if code, _ := write(s, &hq, "units.delete", `{"id":"`+u1+`","reason":"again"}`, 1); code != 404 {
		t.Error("delete twice")
	}
	u2 := mk()
	dev := uuid.NewString()
	owner(t, `INSERT INTO devices.devices (id, tenant_id, serial, target_unit_id, created_by_membership_id, firmware_version) VALUES ($1,$2,$3,$4,$5,'v1')`,
		dev, seed.ID("tenant-a"), "T-"+dev[:8], u2, seed.ID("hq-operator"))
	owner(t, `INSERT INTO devices.device_bindings (tenant_id, device_id, unit_id, customer_org_id, bound_at, actor_membership_id) VALUES ($1,$2,$3,$4,now(),$5)`,
		seed.ID("tenant-a"), dev, u2, seed.ID("org-customer-a"), seed.ID("hq-operator"))
	if code, m := write(s, &hq, "units.delete", `{"id":"`+u2+`","reason":"x"}`, 1); code != 409 || m["messageKey"] != "error.unitInUse" {
		t.Fatalf("unit with IoT history must not be deleted: %d %v", code, m)
	}
	if code, _ := write(s, &customerA, "units.delete", `{"id":"`+u2+`","reason":"x"}`, 1); code != 403 {
		t.Error("client cannot delete units")
	}
}

func TestOrganizations(t *testing.T) {
	s := server(t)
	code, m := post(s, &hq, "organizations.list", `{"filters":{"kind":"customer","status":"active"}}`)
	if code != 200 || len(items(m)) < 2 {
		t.Fatalf("list: %d %v", code, m)
	}
	for _, f := range []string{`{"filters":{"kind":"x"}}`, `{"filters":{"status":"x"}}`, `{"filters":{"y":1}}`, `{"sort":{"field":"name","direction":"asc"}}`} {
		if code, _ := post(s, &hq, "organizations.list", f); code != 422 {
			t.Errorf("%s: %d", f, code)
		}
	}
	if code, _ := post(s, &customerA, "organizations.list", `{}`); code != 403 {
		t.Error("client cannot list organizations")
	}
	code, m = write(s, &hq, "organizations.save", `{"name":"Chong Family Office","kind":"customer","status":"active"}`, 0)
	if code != 200 || ver(m) != 1 {
		t.Fatalf("create: %d %v", code, m)
	}
	id := data(m)["id"].(string)
	if code, m := write(s, &hq, "organizations.save", `{"id":"`+id+`","name":"Chong Office","kind":"customer","status":"inactive"}`, 1); code != 200 || data(m)["status"] != "inactive" {
		t.Fatalf("update: %d", code)
	}
	if code, _ := write(s, &hq, "organizations.save", `{"id":"`+id+`","name":"x","kind":"contractor","status":"active"}`, 2); code != 422 {
		t.Error("kind is immutable")
	}
	if code, _ := write(s, &hq, "organizations.save", `{"id":"`+id+`","name":"x","kind":"customer","status":"active"}`, 1); code != 409 {
		t.Error("stale version")
	}
	if code, _ := write(s, &hq, "organizations.save", `{"id":"`+uuid.NewString()+`","name":"x","kind":"customer","status":"active"}`, 1); code != 404 {
		t.Error("unknown organization")
	}
	if code, _ := write(s, &hq, "organizations.save", `{"name":"","kind":"bank","status":"maybe"}`, 0); code != 422 {
		t.Error("validation")
	}
}

func defaultPolicyID(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, "postgres://postgres:local@localhost:5432/ac?sslmode=disable")
	if err != nil {
		t.Skip(err)
	}
	defer conn.Close(ctx)
	var id string
	if err := conn.QueryRow(ctx, `SELECT id::text FROM monitoring.alert_policies WHERE tenant_id = $1 AND kind = 'default_alert'`, seed.ID("tenant-a")).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
