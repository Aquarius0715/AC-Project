package integration_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

// A property keeps HQ's map pin and away/home radius (DEC-109, IR823): saved, read back by properties.list, replaced
// by the next save and cleared by null; off-globe points and radii outside 50–2000 m are refused by field.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestPropertyLocation(t *testing.T) {
	srv := server(t)
	org := seed.ID("org-customer-b").String()
	name := "Pinned " + uuid.NewString()[:6]
	body := func(id, location, radius string) string {
		prefix := `{`
		if id != "" {
			prefix += `"id":"` + id + `",`
		}

		return prefix + `"customerOrgId":"` + org + `","kind":"home","name":"` + name + `","address":null,` +
			`"accessInstructions":null,"location":` + location + `,"geofenceRadiusMeters":` + radius + `}`
	}

	code, resp := write(
		srv,
		hq(),
		"properties.save",
		body("", `{"latitude":3.1579,"longitude":101.7116}`, "150"),
		0,
	)
	if code != 200 {
		t.Fatalf("create with a pin: %d %v", code, resp)
	}

	prop := data(resp)
	id := asStr(t, prop["id"])

	pin := asObj(t, prop["location"])
	if pin["latitude"] != 3.1579 || pin["longitude"] != 101.7116 ||
		prop["geofenceRadiusMeters"] != float64(150) {
		t.Fatalf("saved pin: %v radius %v", pin, prop["geofenceRadiusMeters"])
	}

	listed := readPropertyLocation(t, srv, id)
	if listed == nil || listed["latitude"] != 3.1579 {
		t.Fatalf("properties.list pin: %v", listed)
	}

	// the save replaces the pin; a pin without a radius keeps the default (null)
	code, resp = write(
		srv,
		hq(),
		"properties.save",
		body(id, `{"latitude":-6.2,"longitude":106.8166}`, "null"),
		1,
	)
	if code != 200 || asObj(t, data(resp)["location"])["longitude"] != 106.8166 ||
		data(resp)["geofenceRadiusMeters"] != nil {
		t.Fatalf("move the pin: %d %v", code, resp)
	}

	code, resp = write(srv, hq(), "properties.save", body(id, "null", "null"), 2)
	if code != 200 || data(resp)["location"] != nil {
		t.Fatalf("clear the pin: %d %v", code, resp)
	}

	for bad, want := range map[string]string{
		body("", `{"latitude":90.5,"longitude":0}`, "null"):   "location=error.range",
		body("", `{"latitude":0,"longitude":-180.1}`, "null"): "location=error.range",
		body("", `{"latitude":1,"longitude":1}`, "49"):        "geofenceRadiusMeters=error.range",
		body("", `{"latitude":1,"longitude":1}`, "2001"):      "geofenceRadiusMeters=error.range",
	} {
		if code, res := write(
			srv,
			hq(),
			"properties.save",
			bad,
			0,
		); code != 422 ||
			fields(res) != want {
			t.Errorf("refusal %s: %d %s, want %s", bad[len(bad)-60:], code, fields(res), want)
		}
	}
}

// readPropertyLocation answers the location of one property as properties.list reads it.
func readPropertyLocation(t *testing.T, srv *apiserver.Server, id string) map[string]any {
	t.Helper()

	code, resp := post(
		srv,
		hq(),
		"properties.list",
		`{"filters":{"customerId":"`+seed.ID("cust-b").String()+`"}}`,
	)
	if code != 200 {
		t.Fatalf("properties.list: %d %v", code, resp)
	}

	for _, p := range items(t, resp) {
		if p["id"] == id {
			location, _ := p["location"].(map[string]any)

			return location
		}
	}

	t.Fatalf("property %s not listed", id)

	return nil
}
