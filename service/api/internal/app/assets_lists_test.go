package app

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

func TestAssetLists(t *testing.T) {
	s := server(t)
	// customers: HQ only; inactive hidden by default
	code, m := post(s, &hq, "customers.list", `{}`)
	if code != 200 || len(items(m)) == 0 {
		t.Fatalf("customers.list: %d %v", code, m)
	}
	for _, c := range items(m) {
		if c["status"] != "active" {
			t.Fatal("default list shows active customers only (IR40)")
		}
	}
	if code, _ := post(s, &customerA, "customers.list", `{}`); code != 403 {
		t.Fatalf("client cannot list customers: %d", code)
	}
	for _, f := range []string{`{"filters":{"kind":"rto"}}`, `{"filters":{"status":"inactive"}}`, `{"filters":{"organizationId":"` + seed.ID("org-customer-a").String() + `"}}`} {
		if code, _ := post(s, &hq, "customers.list", f); code != 200 {
			t.Errorf("%s: %d", f, code)
		}
	}
	for _, f := range []string{`{"filters":{"kind":"x"}}`, `{"filters":{"status":"x"}}`, `{"filters":{"x":1}}`, `{"sort":{"field":"name","direction":"asc"}}`} {
		if code, _ := post(s, &hq, "customers.list", f); code != 422 {
			t.Errorf("%s: %d", f, code)
		}
	}
	// properties: client sees own only
	_, m = post(s, &customerA, "properties.list", `{}`)
	for _, p := range items(m) {
		if p["customerOrgId"] != seed.ID("org-customer-a").String() {
			t.Fatal("client sees another customer's property")
		}
	}
	if len(items(m)) == 0 {
		t.Fatal("customer-a has properties")
	}
	_, m = post(s, &hq, "properties.list", `{"filters":{"customerId":"`+seed.ID("cust-a").String()+`","kind":"home"}}`)
	if len(items(m)) == 0 {
		t.Fatal("customerId + kind filter")
	}
	if code, _ := post(s, &hq, "properties.list", `{"filters":{"kind":"castle"}}`); code != 422 {
		t.Fatal("bad kind")
	}
	// spaces
	_, m = post(s, &customerA, "spaces.list", `{"filters":{"propertyId":"`+seed.ID("property-home-a").String()+`","kind":"room"}}`)
	if len(items(m)) == 0 {
		t.Fatal("customer-a rooms in Home A")
	}
	_, m = post(s, &customerB, "spaces.list", `{"filters":{"propertyId":"`+seed.ID("property-home-a").String()+`"}}`)
	if len(items(m)) != 0 {
		t.Fatal("customer-b must not see Home A spaces")
	}
	if code, _ := post(s, &hq, "spaces.list", `{"filters":{"kind":"x"}}`); code != 422 {
		t.Fatal("bad space kind")
	}
	if code, _ := post(s, &hq, "spaces.list", `{"limit":0}`); code != 422 {
		t.Fatal("bad limit")
	}
}

func TestCustomersSaveAndRename(t *testing.T) {
	s := server(t)
	org := uuid.NewString()
	body := `{"name":"Test Co","organizationId":"` + org + `","serviceProfile":"general","status":"active"}`
	code, m := write(s, &hq, "customers.save", body, 0)
	if code != 200 || ver(m) != 1 {
		t.Fatalf("customer create: %d %v", code, m)
	}
	id := data(m)["id"].(string)
	if code, _ := write(s, &hq, "customers.save", body, 0); code != 409 {
		t.Fatalf("one customer per organization: %d", code)
	}
	if code, _ := write(s, &hq, "customers.save", `{"name":"","organizationId":"`+org+`","serviceProfile":"x","status":"y"}`, 0); code != 422 {
		t.Fatal("customer validation")
	}
	upd := `{"id":"` + id + `","name":"Test Co 2","organizationId":"` + org + `","serviceProfile":"energy","status":"inactive"}`
	if code, m := write(s, &hq, "customers.save", upd, 1); code != 200 || data(m)["status"] != "inactive" {
		t.Fatalf("customer update: %d", code)
	}
	// inactive customer cannot get new properties
	if code, _ := write(s, &hq, "properties.save", `{"customerOrgId":"`+org+`","kind":"home","name":"x","address":null,"accessInstructions":null}`, 0); code != 409 {
		t.Fatalf("inactive customer property: %d", code)
	}

	// rename: client owner renames own room, sibling uniqueness, other customer's target, stale version
	pr := seed.ID("property-home-a").String()
	code, m = write(s, &hq, "spaces.save", `{"propertyId":"`+pr+`","parentSpaceId":"`+seed.ID("floor-1").String()+`","kind":"room","name":"Rename me `+uuid.NewString()[:6]+`"}`, 0)
	if code != 200 {
		t.Fatalf("room: %d", code)
	}
	room := data(m)["id"].(string)
	newName := "Den " + uuid.NewString()[:6]
	if code, m := write(s, &customerA, "locations.rename", `{"target":{"kind":"space","id":"`+room+`"},"name":"  `+newName+`  "}`, 1); code != 200 || data(m)["name"] != newName {
		t.Fatalf("client rename: %d %v", code, m)
	}
	if code, _ := write(s, &customerA, "locations.rename", `{"target":{"kind":"space","id":"`+room+`"},"name":"x"}`, 1); code != 409 {
		t.Fatal("stale version")
	}
	if code, _ := write(s, &customerB, "locations.rename", `{"target":{"kind":"space","id":"`+room+`"},"name":"x"}`, 2); code != 404 {
		t.Fatal("other customer's space is NOT_FOUND")
	}
	// a sibling with the same name (case-insensitive) conflicts
	_, m = write(s, &hq, "spaces.save", `{"propertyId":"`+pr+`","parentSpaceId":"`+seed.ID("floor-1").String()+`","kind":"room","name":"Sib `+uuid.NewString()[:6]+`"}`, 0)
	sib := data(m)["id"].(string)
	if code, _ := write(s, &hq, "locations.rename", `{"target":{"kind":"space","id":"`+sib+`"},"name":"`+strings.ToUpper(newName)+`"}`, 1); code != 409 {
		t.Fatalf("duplicate sibling name: %d", code)
	}
	if code, m := write(s, &hq, "locations.rename", `{"target":{"kind":"unit","id":"`+seed.ID("unit-bedroom-2").String()+`"},"name":"Bedroom AC #2"}`, 0); code != 422 {
		t.Fatalf("missing version: %d %v", code, m)
	}
	if code, _ := write(s, &hq, "locations.rename", `{"target":{"kind":"garage","id":"`+room+`"},"name":"x"}`, 1); code != 422 {
		t.Fatal("bad kind")
	}
	if code, _ := write(s, &hq, "locations.rename", `{"target":{"kind":"property","id":"`+uuid.NewString()+`"},"name":"x"}`, 1); code != 404 {
		t.Fatal("unknown property")
	}

	// property and unit renames (HQ) and the result shapes
	code, m = write(s, &hq, "properties.save", `{"customerOrgId":"`+seed.ID("org-customer-b").String()+`","kind":"office","name":"P `+uuid.NewString()[:6]+`","address":null,"accessInstructions":null}`, 0)
	if code != 200 {
		t.Fatalf("property: %d", code)
	}
	prop := data(m)["id"].(string)
	office := "Office " + uuid.NewString()[:6]
	if code, m := write(s, &hq, "locations.rename", `{"target":{"kind":"property","id":"`+prop+`"},"name":"`+office+`"}`, 1); code != 200 || data(m)["name"] != office {
		t.Fatalf("property rename: %d %v", code, m)
	}
	code, m = write(s, &hq, "units.save", `{"customerOrgId":"`+seed.ID("org-customer-b").String()+`","propertyId":"`+prop+`","spaceId":null,"displayName":"U1","modelId":"`+seed.ID("ventilation-demo").String()+`","type":"split","installedAt":null,"serviceScope":["indoor"]}`, 0)
	if code != 200 {
		t.Fatalf("unit: %d %v", code, m)
	}
	unit := data(m)["id"].(string)
	if code, m := write(s, &customerB, "locations.rename", `{"target":{"kind":"unit","id":"`+unit+`"},"name":"Reception AC"}`, 1); code != 200 || data(m)["displayName"] != "Reception AC" { // unique: new property
		t.Fatalf("client unit rename: %d %v", code, m)
	}
	if code, _ := write(s, &hq, "customers.save", upd, 1); code != 409 {
		t.Fatal("customer stale version")
	}
	if code, _ := write(s, &hq, "customers.save", strings.Replace(upd, id, uuid.NewString(), 1), 1); code != 404 {
		t.Fatal("unknown customer")
	}
}
