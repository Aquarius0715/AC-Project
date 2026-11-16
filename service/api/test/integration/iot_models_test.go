package integration_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

const refusedFirmwareCandidatesInvalid = "firmwareCandidates=" + apperr.KeyInvalid

// iotBody is an iotModels.save body: two sensors and the firmware candidate v1.
func iotBody(id, model, extra string) string {
	idf := ""
	if id != "" {
		idf = `"id":"` + id + `",`
	}

	return `{` + idf + `"manufacturer":"TestNode","model":"` + model + `",` +
		`"sensors":[{"metric":"temperature","unit":"°C","staleAfterSeconds":120,"boundaryId":null},{"metric":"power",` +
		`"unit":"kW","staleAfterSeconds":120,"boundaryId":"ac_input_electricity"}],` +
		`"firmwareCandidates":["v1"]` + extra + `}`
}

// DEC-105: the IoT model register: create, refusals, a new version that moves its units and devices, the version
// history, the delete refusals.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestIoTModels(t *testing.T) {
	srv := server(t)

	if code, _ := post(srv, customerA(), "iotModels.list", `{}`); code != 200 {
		t.Error("clients read the IoT model register")
	}

	if code, res := post(srv, hq(), "iotModels.list", `{"filters":{"x":1}}`); code != 422 ||
		fields(res) != refusedUnknownFieldX {
		t.Errorf("no filters: %d %s", code, fields(res))
	}

	model := "IM-" + uuid.NewString()[:6]

	code, resp := write(srv, hq(), "iotModels.save", iotBody("", model, ""), 0)
	if code != 200 || ver(t, resp) != 1 || len(asArr(t, data(resp)["sensors"])) != 2 {
		t.Fatalf("create: %d %v", code, resp)
	}

	modelID := asStr(t, data(resp)["id"])

	if code, _ := write(
		srv,
		hq(),
		"iotModels.save",
		iotBody("", strings.ToLower(model), ""),
		0,
	); code != 409 {
		t.Error("manufacturer+model unique (case-insensitive)")
	}

	bad := map[string]string{
		caseUnitMismatch: strings.Replace(iotBody("", "x4", ""), `"unit":"°C"`, `"unit":"%"`, 1),
		"stale too low": strings.Replace(
			iotBody(
				"",
				"x5",
				"",
			),
			`"staleAfterSeconds":120,"boundaryId":null`,
			`"staleAfterSeconds":1,"boundaryId":null`,
			1,
		),
		fieldBoundary: strings.Replace(
			iotBody("", "x6", ""),
			`"staleAfterSeconds":120,"boundaryId":null`,
			`"staleAfterSeconds":120,"boundaryId":"ac_input_electricity"`,
			1,
		),
		caseEmptyName:  iotBody("", "", ""),
		"dup firmware": strings.Replace(iotBody("", "x9", ""), `["v1"]`, `["v1","v1"]`, 1),
		"firmware too long": strings.Replace(
			iotBody("", "x11", ""), `["v1"]`, `["`+strings.Repeat("v", 33)+`"]`, 1),
		"firmware blank":    strings.Replace(iotBody("", "x12", ""), `["v1"]`, `[" "]`, 1),
		"firmware dup trim": strings.Replace(iotBody("", "x13", ""), `["v1"]`, `["v1"," v1 "]`, 1),
		"no manufacturer":   strings.Replace(iotBody("", "x14", ""), `"TestNode"`, `""`, 1),
		"blank reason":      iotBody("", "x15", `,"changeReason":"  "`),
	}
	want := map[string]string{
		fieldBoundary:       "sensors[0].boundaryId=error.invalid",
		"dup firmware":      refusedFirmwareCandidatesInvalid,
		"firmware too long": refusedFirmwareCandidatesInvalid,
		"firmware blank":    refusedFirmwareCandidatesInvalid,
		"firmware dup trim": refusedFirmwareCandidatesInvalid,
		caseEmptyName:       "model=error.length",
		"stale too low":     "sensors[0].staleAfterSeconds=error.range",
		caseUnitMismatch:    "sensors[0].unit=error.unitMismatch",
		"no manufacturer":   "manufacturer=error.length",
		"blank reason":      "changeReason=error.length",
	}

	for name, b := range bad {
		code, m := write(srv, hq(), "iotModels.save", b, 0)
		refusal(t, name, code, m, want[name])
	}
	// a unit with the IoT model and its device move to the new version
	_, savedUnit := write(
		srv,
		hq(),
		"units.save",
		`{"customerOrgId":"`+seed.ID("org-customer-b").
			String()+
			`","propertyId":"`+seed.ID("property-home-b").
			String()+
			`","spaceId":null,"displayName":"IoT AC `+uuid.NewString()[:4]+`","modelId":"`+seed.ID("ventilation-demo").
			String()+
			`","iotModelId":"`+modelID+`","type":"split","installedAt":null,"serviceScope":["indoor"]}`,
		0,
	)
	unit := asStr(t, data(savedUnit)["id"])

	code, reg := write(
		srv,
		hq(),
		"devices.register",
		`{"serial":"IM-`+uuid.NewString()[:8]+`","sensorTypes":["temperature"],"unitId":"`+unit+`"}`,
		0,
	)
	if code != 200 || data(reg)["iotModelId"] != modelID ||
		asNum(t, data(reg)["iotModelVersion"]) != 1 {
		t.Fatalf("register on the unit's IoT model: %d %v", code, reg)
	}

	if code, res := write(
		srv,
		hq(),
		"devices.register",
		`{"serial":"IM-`+uuid.NewString()[:8]+`","sensorTypes":["humidity"],"unitId":"`+unit+`"}`,
		0,
	); code != 422 || fields(res) != "sensorTypes=error.notInCapability" {
		t.Errorf("a sensor the IoT model lacks: %d %v", code, res)
	}

	if code, res := write(
		srv,
		hq(),
		"iotModels.save",
		iotBody(modelID, model, ""),
		1,
	); code != 422 || asObj(t, res["fieldErrors"])["changeReason"] != apperr.KeyRequired {
		t.Errorf("update without a change reason: %d %v", code, res)
	}

	code, resp = write(
		srv,
		hq(),
		"iotModels.save",
		strings.Replace(
			iotBody(modelID, model, `,"changeReason":"firmware v2"`),
			`["v1"]`,
			`["v1","v2"]`,
			1,
		),
		1,
	)
	if code != 200 || ver(t, resp) != 2 {
		t.Fatalf("update: %d %v", code, resp)
	}

	_, unitDetail := post(srv, hq(), "units.get", `{"id":"`+unit+`"}`)
	if int(asNum(t, data(unitDetail)["iotModelVersion"])) != 2 {
		t.Fatalf("unit IoT model version: %v", data(unitDetail)["iotModelVersion"])
	}

	if asObj(t, data(unitDetail)["iotModel"])["id"] != modelID {
		t.Errorf("the unit detail names its IoT model: %v", data(unitDetail)["iotModel"])
	}

	dev := asStr(t, data(reg)["id"])
	_, d := post(srv, hq(), "devices.get", `{"id":"`+dev+`"}`)

	if int(asNum(t, data(d)["iotModelVersion"])) != 2 {
		t.Fatalf("device IoT model version: %v", data(d)["iotModelVersion"])
	}

	day := `"from":"` + clock().Add(-time.Hour).
		Format(time.RFC3339) +
		`","to":"` + clock().Add(time.Hour).
		Format(time.RFC3339) +
		`"`
	_, audit := post(
		srv,
		hq(),
		"audit.list",
		`{"filters":{`+day+`,"targetKind":"iotModel","targetId":"`+modelID+`"},"limit":10}`,
	)

	var saved map[string]any

	for _, a := range items(t, audit) {
		if a["nextVersion"] == float64(2) {
			saved = a
		}
	}

	if saved == nil || saved["reason"] != "firmware v2" ||
		len(asObj(t, saved["maskedAfter"])) != 1 {
		t.Fatalf("IoT model audit: %v", audit)
	}

	if code, _ := write(
		srv,
		hq(),
		"iotModels.save",
		iotBody(modelID, model, `,"changeReason":"again"`),
		1,
	); code != 409 {
		t.Error("stale version")
	}

	if code, _ := write(
		srv,
		customerA(),
		"iotModels.save",
		iotBody("", "c-"+model, ""),
		0,
	); code != 403 {
		t.Error("client cannot edit IoT models")
	}

	// in use by a unit and a device: not deletable
	if code, res := write(
		srv,
		hq(),
		"iotModels.delete",
		`{"id":"`+modelID+`","reason":"cleanup"}`,
		2,
	); code != 409 || res["messageKey"] != "error.modelInUse" {
		t.Errorf("an IoT model in use is refused: %d %v", code, res)
	}

	unused := asStr(t, data(createIoT(t, srv))["id"])
	if code, res := write(
		srv,
		hq(),
		"iotModels.delete",
		`{"id":"`+unused+`","reason":"cleanup"}`,
		1,
	); code != 200 {
		t.Errorf("an unused IoT model is deleted: %d %v", code, res)
	}
}

// DEC-105: an update or delete of an unknown IoT model is NOT_FOUND, a delete on a stale version is a version
// conflict, and a model no unit or device uses any more is still kept while a firmware campaign names it.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestIoTModelWriteRefusals(t *testing.T) {
	srv := server(t)
	unknown := uuid.NewString()

	code, res := write(
		srv,
		hq(),
		"iotModels.save",
		iotBody(unknown, "IM-"+uuid.NewString()[:8], `,"changeReason":"rename"`),
		1,
	)
	if code != 404 || res["messageKey"] != apperr.KeyNotFound {
		t.Errorf("update of an unknown IoT model: %d %v", code, res)
	}

	code, res = write(srv, hq(), "iotModels.delete", `{"id":"`+unknown+`","reason":"cleanup"}`, 1)
	if code != 404 || res["messageKey"] != apperr.KeyNotFound {
		t.Errorf("delete of an unknown IoT model: %d %v", code, res)
	}

	stale := asStr(t, data(createIoT(t, srv))["id"])

	code, res = write(srv, hq(), "iotModels.delete", `{"id":"`+stale+`","reason":"cleanup"}`, 2)
	if code != 409 || res["messageKey"] != apperr.KeyVersionConflict {
		t.Errorf("delete on a stale version: %d %v", code, res)
	}

	_, created := write(
		srv,
		hq(),
		"iotModels.save",
		strings.Replace(iotBody("", "IM-"+uuid.NewString()[:8], ""), `["v1"]`, `["v1","v2"]`, 1),
		0,
	)
	modelID := asStr(t, data(created)["id"])
	_, savedUnit := write(
		srv,
		hq(),
		"units.save",
		`{"customerOrgId":"`+seed.ID("org-customer-b").String()+`","propertyId":"`+
			seed.ID("property-home-b").
				String()+
			`","spaceId":null,"displayName":"IoT AC `+uuid.NewString()[:4]+
			`","modelId":"`+seed.ID("ventilation-demo").String()+`","iotModelId":"`+modelID+
			`","type":"split","installedAt":null,"serviceScope":["indoor"]}`,
		0,
	)
	unit := asStr(t, data(savedUnit)["id"])
	_, reg := write(
		srv,
		hq(),
		"devices.register",
		`{"serial":"IMC-`+uuid.NewString()[:8]+`","sensorTypes":["temperature"],"unitId":"`+unit+`"}`,
		0,
	)
	dev := asStr(t, data(reg)["id"])

	code, res = write(
		srv,
		hq(),
		"firmwareCampaigns.schedule",
		`{"modelId":"`+modelID+`","targetVersion":"v2","deviceIds":["`+dev+`"],`+
			`"waves":[{"label":"All","percent":100}],"window":{"startLocal":"01:00","endLocal":"05:00"},`+
			`"autoPauseFailurePercent":5,"startAt":"`+clock().Add(48*time.Hour).Format(time.RFC3339)+`"}`,
		0,
	)
	if code != 200 {
		t.Fatalf("schedule: %d %v", code, res)
	}
	// the unit and the device leave the model; only the campaign still names it
	owner(
		t,
		`UPDATE assets.units SET iot_model_id = NULL, iot_model_version = NULL WHERE id = $1`,
		unit,
	)
	owner(
		t,
		`UPDATE devices.devices SET iot_model_id = NULL, iot_model_version = NULL WHERE id = $1`,
		dev,
	)

	code, res = write(srv, hq(), "iotModels.delete", `{"id":"`+modelID+`","reason":"cleanup"}`, 1)
	if code != 409 || res["messageKey"] != "error.modelInCampaign" {
		t.Errorf("an IoT model a campaign targets is refused: %d %v", code, res)
	}
}

// createIoT saves a new IoT model with a name of its own.
func createIoT(t *testing.T, srv *apiserver.Server) map[string]any {
	t.Helper()

	code, res := write(srv, hq(), "iotModels.save", iotBody("", "IM-"+uuid.NewString()[:8], ""), 0)
	if code != 200 {
		t.Fatalf("create IoT model: %d %v", code, res)
	}

	return res
}
