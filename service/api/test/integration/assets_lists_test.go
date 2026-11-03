package integration

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

func TestAssetLists(t *testing.T) {
	srv := server(t)
	// customers: HQ only; inactive hidden by default
	code, resp := post(srv, &hq, "customers.list", `{}`)
	if code != 200 || len(items(resp)) == 0 {
		t.Fatalf("customers.list: %d %v", code, resp)
	}

	for _, c := range items(resp) {
		if c["status"] != "active" {
			t.Fatal("default list shows active customers only (IR40)")
		}
	}

	if code, _ := post(srv, &customerA, "customers.list", `{}`); code != 403 {
		t.Fatalf("client cannot list customers: %d", code)
	}

	for _, f := range []string{
		`{"filters":{"kind":"rto"}}`, `{"filters":{"status":"inactive"}}`,
		`{"filters":{"organizationId":"` + seed.ID("org-customer-a").String() + `"}}`,
	} {
		if code, _ := post(srv, &hq, "customers.list", f); code != 200 {
			t.Errorf("%s: %d", f, code)
		}
	}

	want := map[string]string{ // the field each case is refused for
		"{\"filters\":{\"kind\":\"x\"}}":                        "filters.kind=error.invalid",
		"{\"filters\":{\"status\":\"x\"}}":                      "filters.status=error.invalid",
		"{\"filters\":{\"x\":1}}":                               "x=error.notAllowed",
		"{\"sort\":{\"field\":\"name\",\"direction\":\"asc\"}}": "sort.field=error.invalid",
	}

	for _, f := range []string{
		`{"filters":{"kind":"x"}}`, `{"filters":{"status":"x"}}`, `{"filters":{"x":1}}`,
		`{"sort":{"field":"name","direction":"asc"}}`,
	} {
		code, m := post(srv, &hq, "customers.list", f)
		refusal(t, f, code, m, want[f])
	}
	// properties: client sees own only
	_, resp = post(srv, &customerA, "properties.list", `{}`)
	for _, p := range items(resp) {
		if p["customerOrgId"] != seed.ID("org-customer-a").String() {
			t.Fatal("client sees another customer's property")
		}
	}

	if len(items(resp)) == 0 {
		t.Fatal("customer-a has properties")
	}

	_, resp = post(
		srv,
		&hq,
		"properties.list",
		`{"filters":{"customerId":"`+seed.ID("cust-a").String()+`","kind":"home"}}`,
	)
	if len(items(resp)) == 0 {
		t.Fatal("customerId + kind filter")
	}

	if code, res := post(
		srv,
		&hq,
		"properties.list",
		`{"filters":{"kind":"castle"}}`,
	); code != 422 ||
		fields(res) != "filters.kind=error.invalid" {
		t.Fatalf("bad kind: %d %s", code, fields(res))
	}
	// spaces
	_, resp = post(
		srv,
		&customerA,
		"spaces.list",
		`{"filters":{"propertyId":"`+seed.ID("property-home-a").String()+`","kind":"room"}}`,
	)
	if len(items(resp)) == 0 {
		t.Fatal("customer-a rooms in Home A")
	}

	_, resp = post(
		srv,
		&customerB,
		"spaces.list",
		`{"filters":{"propertyId":"`+seed.ID("property-home-a").String()+`"}}`,
	)
	if len(items(resp)) != 0 {
		t.Fatal("customer-b must not see Home A spaces")
	}

	if code, res := post(
		srv,
		&hq,
		"spaces.list",
		`{"filters":{"kind":"x"}}`,
	); code != 422 ||
		fields(res) != "filters.kind=error.invalid" {
		t.Fatalf("bad space kind: %d %s", code, fields(res))
	}

	if code, res := post(
		srv,
		&hq,
		"spaces.list",
		`{"limit":0}`,
	); code != 422 ||
		fields(res) != "limit=error.range" {
		t.Fatalf("bad limit: %d %s", code, fields(res))
	}
}

func TestCustomersSaveAndRename(t *testing.T) {
	srv := server(t)
	org := uuid.NewString()
	body := `{"name":"Test Co","organizationId":"` + org + `","serviceProfile":"general","status":"active"}`

	code, resp := write(srv, &hq, "customers.save", body, 0)
	if code != 200 || ver(resp) != 1 {
		t.Fatalf("customer create: %d %v", code, resp)
	}

	id := asStr(t, data(resp)["id"])

	if code, _ := write(srv, &hq, "customers.save", body, 0); code != 409 {
		t.Fatalf("one customer per organization: %d", code)
	}

	if code, res := write(
		srv,
		&hq,
		"customers.save",
		`{"name":"","organizationId":"`+org+`","serviceProfile":"x","status":"y"}`,
		0,
	); code != 422 ||
		fields(res) != "name=error.length,serviceProfile=error.invalid,status=error.invalid" {
		t.Fatalf("customer validation: %d %s", code, fields(res))
	}

	upd := `{"id":"` + id + `","name":"Test Co 2",` +
		`"organizationId":"` + org + `","serviceProfile":"energy",` +
		`"status":"inactive"}`
	if code, res := write(
		srv,
		&hq,
		"customers.save",
		upd,
		1,
	); code != 200 ||
		data(res)["status"] != "inactive" {
		t.Fatalf("customer update: %d", code)
	}
	// inactive customer cannot get new properties
	if code, _ := write(
		srv,
		&hq,
		"properties.save",
		`{"customerOrgId":"`+org+`","kind":"home","name":"x","address":null,"accessInstructions":null}`,
		0,
	); code != 409 {
		t.Fatalf("inactive customer property: %d", code)
	}

	// rename: client owner renames own room, sibling uniqueness, other customer's target, stale version
	pr := seed.ID("property-home-a").String()

	code, resp = write(
		srv,
		&hq,
		"spaces.save",
		`{"propertyId":"`+pr+`","parentSpaceId":"`+seed.ID("floor-1").
			String()+
			`","kind":"room","name":"Rename me `+uuid.NewString()[:6]+`"}`,
		0,
	)
	if code != 200 {
		t.Fatalf("room: %d", code)
	}

	room := asStr(t, data(resp)["id"])

	newName := "Den " + uuid.NewString()[:6]
	if code, res := write(
		srv,
		&customerA,
		"locations.rename",
		`{"target":{"kind":"space","id":"`+room+`"},"name":"  `+newName+`  "}`,
		1,
	); code != 200 ||
		data(res)["name"] != newName {
		t.Fatalf("client rename: %d %v", code, res)
	}

	if code, _ := write(
		srv,
		&customerA,
		"locations.rename",
		`{"target":{"kind":"space","id":"`+room+`"},"name":"x"}`,
		1,
	); code != 409 {
		t.Fatal("stale version")
	}

	if code, _ := write(
		srv,
		&customerB,
		"locations.rename",
		`{"target":{"kind":"space","id":"`+room+`"},"name":"x"}`,
		2,
	); code != 404 {
		t.Fatal("other customer's space is NOT_FOUND")
	}
	// a sibling with the same name (case-insensitive) conflicts
	_, resp = write(
		srv,
		&hq,
		"spaces.save",
		`{"propertyId":"`+pr+`","parentSpaceId":"`+seed.ID("floor-1").
			String()+
			`","kind":"room","name":"Sib `+uuid.NewString()[:6]+`"}`,
		0,
	)

	sib := asStr(t, data(resp)["id"])
	if code, _ := write(
		srv,
		&hq,
		"locations.rename",
		`{"target":{"kind":"space","id":"`+sib+`"},"name":"`+strings.ToUpper(newName)+`"}`,
		1,
	); code != 409 {
		t.Fatalf("duplicate sibling name: %d", code)
	}

	if code, res := write(
		srv,
		&hq,
		"locations.rename",
		`{"target":{"kind":"unit","id":"`+seed.ID("unit-bedroom-2").
			String()+
			`"},"name":"Bedroom AC #2"}`,
		0,
	); code != 422 ||
		fields(res) != "expectedVersion=error.required" {
		t.Fatalf("missing version: %d %v", code, res)
	}

	if code, res := write(
		srv,
		&hq,
		"locations.rename",
		`{"target":{"kind":"garage","id":"`+room+`"},"name":"x"}`,
		1,
	); code != 422 ||
		fields(res) != "target.kind=error.invalid" {
		t.Fatalf("bad kind: %d %s", code, fields(res))
	}

	if code, _ := write(
		srv,
		&hq,
		"locations.rename",
		`{"target":{"kind":"property","id":"`+uuid.NewString()+`"},"name":"x"}`,
		1,
	); code != 404 {
		t.Fatal("unknown property")
	}

	// property and unit renames (HQ) and the result shapes
	code, resp = write(
		srv,
		&hq,
		"properties.save",
		`{"customerOrgId":"`+seed.ID("org-customer-b").
			String()+
			`","kind":"office","name":"P `+uuid.NewString()[:6]+`","address":null,"accessInstructions":null}`,
		0,
	)
	if code != 200 {
		t.Fatalf("property: %d", code)
	}

	prop := asStr(t, data(resp)["id"])

	office := "Office " + uuid.NewString()[:6]
	if code, res := write(
		srv,
		&hq,
		"locations.rename",
		`{"target":{"kind":"property","id":"`+prop+`"},"name":"`+office+`"}`,
		1,
	); code != 200 ||
		data(res)["name"] != office {
		t.Fatalf("property rename: %d %v", code, res)
	}

	code, resp = write(
		srv,
		&hq,
		"units.save",
		`{"customerOrgId":"`+seed.ID("org-customer-b").
			String()+
			`","propertyId":"`+prop+`","spaceId":null,"displayName":"U1","modelId":"`+seed.ID("ventilation-demo").
			String()+
			`","type":"split","installedAt":null,"serviceScope":["indoor"]}`,
		0,
	)
	if code != 200 {
		t.Fatalf("unit: %d %v", code, resp)
	}

	unit := asStr(t, data(resp)["id"])
	if code, res := write(
		srv,
		&customerB,
		"locations.rename",
		`{"target":{"kind":"unit","id":"`+unit+`"},"name":"Reception AC"}`,
		1,
	); code != 200 ||
		data(res)["displayName"] != "Reception AC" { // unique: new property
		t.Fatalf("client unit rename: %d %v", code, res)
	}

	if code, _ := write(srv, &hq, "customers.save", upd, 1); code != 409 {
		t.Fatal("customer stale version")
	}

	if code, _ := write(
		srv,
		&hq,
		"customers.save",
		strings.Replace(upd, id, uuid.NewString(), 1),
		1,
	); code != 404 {
		t.Fatal("unknown customer")
	}
}
