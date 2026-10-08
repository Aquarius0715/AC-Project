package integration

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/internal/seed"
)

func capBody(id, model, extra string) string {
	idf := ""
	if id != "" {
		idf = `"id":"` + id + `",`
	}
	return `{` + idf + `"manufacturer":"TestAir","model":"` + model + `","control":true,"modeControl":true,"fanControl":true,` +
		`"temperature":{"min":16,"max":30,"step":0.5},"modes":["cool","dry"],"fanLevels":["low","high"],"ventilation":false,"ventilationLevels":[],` +
		`"sensors":[{"metric":"temperature","unit":"°C","staleAfterSeconds":120,"boundaryId":null},{"metric":"power","unit":"kW","staleAfterSeconds":120,"boundaryId":"ac_input_electricity"}],` +
		`"firmwareCandidates":["v1"]` + extra + `}`
}

func TestCapabilities(t *testing.T) {
	s := server(t)
	code, m := post(s, &hq, "capabilities.list", `{"limit":100}`)
	if code != 200 || len(items(m)) < 2 {
		t.Fatalf("list: %d %v", code, m)
	}
	if code, _ := post(s, &customerA, "capabilities.list", `{}`); code != 200 {
		t.Error("clients read the model register (client:self)")
	}
	if code, _ := post(s, &hq, "capabilities.list", `{"filters":{"x":1}}`); code != 422 {
		t.Error("no filters")
	}
	model := "TA-" + uuid.NewString()[:6]
	code, m = write(s, &hq, "capabilities.save", capBody("", model, ""), 0)
	if code != 200 || ver(m) != 1 || len(data(m)["sensors"].([]any)) != 2 {
		t.Fatalf("create: %d %v", code, m)
	}
	id := data(m)["id"].(string)
	if code, _ := write(s, &hq, "capabilities.save", capBody("", strings.ToLower(model), ""), 0); code != 409 {
		t.Error("manufacturer+model unique (case-insensitive)")
	}
	bad := map[string]string{
		"temp range":     strings.Replace(capBody("", "x1", ""), `"min":16,"max":30`, `"min":30,"max":16`, 1),
		"bad mode":       strings.Replace(capBody("", "x2", ""), `["cool","dry"]`, `["heat"]`, 1),
		"dup fan":        strings.Replace(capBody("", "x3", ""), `["low","high"]`, `["low","low"]`, 1),
		"unit mismatch":  strings.Replace(capBody("", "x4", ""), `"unit":"°C"`, `"unit":"%"`, 1),
		"stale too low":  strings.Replace(capBody("", "x5", ""), `"staleAfterSeconds":120,"boundaryId":null`, `"staleAfterSeconds":1,"boundaryId":null`, 1),
		"boundary":       strings.Replace(capBody("", "x6", ""), `"staleAfterSeconds":120,"boundaryId":null`, `"staleAfterSeconds":120,"boundaryId":"ac_input_electricity"`, 1),
		"no control":     strings.Replace(capBody("", "x7", ""), `"control":true`, `"control":false`, 1),
		"vent levels":    strings.Replace(capBody("", "x8", ""), `"ventilationLevels":[]`, `"ventilationLevels":["low"]`, 1),
		"empty name":     capBody("", "", ""),
		"dup firmware":   strings.Replace(capBody("", "x9", ""), `["v1"]`, `["v1","v1"]`, 1),
		"modes required": strings.Replace(capBody("", "x10", ""), `"modes":["cool","dry"]`, `"modes":[]`, 1),
	}
	for name, b := range bad {
		if code, _ := write(s, &hq, "capabilities.save", b, 0); code != 422 {
			t.Errorf("%s: %d", name, code)
		}
	}
	// a unit on this model moves to the new capability version
	_, um := write(s, &hq, "units.save", `{"customerOrgId":"`+seed.ID("org-customer-b").String()+`","propertyId":"`+seed.ID("property-home-b").String()+`","spaceId":null,"displayName":"Cap AC `+uuid.NewString()[:4]+`","modelId":"`+id+`","type":"split","installedAt":null,"serviceScope":["indoor"]}`, 0)
	unit, _ := data(um)["id"].(string)
	code, m = write(s, &hq, "capabilities.save", capBody(id, model, `,"changeReason":"added dry mode"`), 1)
	if code != 200 || ver(m) != 2 {
		t.Fatalf("update: %d %v", code, m)
	}
	if unit != "" {
		_, g := post(s, &hq, "units.get", `{"id":"`+unit+`"}`)
		if int(data(g)["capabilityVersion"].(float64)) != 2 {
			t.Fatalf("unit capability version: %v", data(g)["capabilityVersion"])
		}
	}
	if code, _ := write(s, &hq, "capabilities.save", capBody(id, model, ""), 1); code != 409 {
		t.Error("stale version")
	}
	if code, _ := write(s, &hq, "capabilities.save", capBody(uuid.NewString(), "new-"+model, ""), 1); code != 404 {
		t.Error("unknown model")
	}
	if code, _ := write(s, &customerA, "capabilities.save", capBody("", "c-"+model, ""), 0); code != 403 {
		t.Error("client cannot edit models")
	}
	// only the current version is listed
	_, m = post(s, &hq, "capabilities.list", `{"limit":100}`)
	n := 0
	for _, c := range items(m) {
		if c["id"] == id {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("model listed %d times", n)
	}
}
