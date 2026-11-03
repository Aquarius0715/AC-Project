package integration_test

import (
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

const fieldWaves = "waves"

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestFirmwareCampaigns(t *testing.T) {
	srv := server(t)
	model := seed.ID("ventilation-demo").String()

	devs := make([]string, 0, 3)

	for range 3 {
		u := newUnit(t, srv, "FW AC")
		_, res := write(
			srv,
			hq(),
			"devices.register",
			`{"serial":"FW-`+uuid.NewString()[:8]+`","sensorTypes":[],"unitId":"`+u+`"}`,
			0,
		)
		devs = append(devs, `"`+asStr(t, data(res)["id"])+`"`)
	}

	start := clock().Add(48 * time.Hour).Format(time.RFC3339)
	body := func(extra map[string]string) string {
		f := map[string]string{
			fieldModelID:              `"` + model + `"`,
			fieldTargetVersion:        `"v2"`,
			fieldDeviceIDs:            "[" + strings.Join(devs, ",") + "]",
			fieldWaves:                `[{"label":"Pilot","percent":34},{"label":"All","percent":100}]`,
			"window":                  `{"startLocal":"01:00","endLocal":"05:00"}`,
			"autoPauseFailurePercent": "5",
			fieldStartAt:              `"` + start + `"`,
		}
		maps.Copy(f, extra)

		parts := make([]string, 0, len(f))
		for k, v := range f {
			parts = append(parts, `"`+k+`":`+v)
		}

		return "{" + strings.Join(parts, ",") + "}"
	}

	code, resp := write(srv, hq(), "firmwareCampaigns.schedule", body(nil), 0)
	if code != 200 || data(resp)["state"] != statusScheduled ||
		len(asArr(t, data(resp)["results"])) != 3 ||
		!strings.HasPrefix(asStr(t, data(resp)["checksum"]), "sha256:") ||
		data(resp)["createdByMembershipId"] != seed.ID("hq-operator").
			String() { // who scheduled it (Figma 04-9 "Created", IR344)
		t.Fatalf("schedule: %d %v", code, resp)
	}

	camp := asStr(t, data(resp)["id"])
	if asNum(t, asObj(t, data(resp)["progress"])["pending"]) != 3 {
		t.Fatal("progress")
	}
	// a device with an unresolved tamper, and one on a unit of another model (cap-split-std, SPL-100)
	tu := newUnit(t, srv, "FW AC")
	_, tamperedDevice := write(
		srv,
		hq(),
		"devices.register",
		`{"serial":"FW-`+uuid.NewString()[:8]+`","sensorTypes":[],"unitId":"`+tu+`"}`,
		0,
	)
	tampered := asStr(t, data(tamperedDevice)["id"])
	owner(t, `UPDATE devices.devices SET tamper = 'detected' WHERE id = $1`, tampered)

	_, otherUnit := write(
		srv,
		hq(),
		"units.save",
		`{"customerOrgId":"`+seed.ID("org-customer-b").
			String()+
			`","propertyId":"`+seed.ID("property-home-b").
			String()+
			`","spaceId":null,"displayName":"Split `+uuid.NewString()[:8]+`","modelId":"`+seed.ID("cap-split-std").
			String()+
			`","type":"split","installedAt":null,"serviceScope":["indoor"]}`,
		0,
	)

	_, otherDevice := write(
		srv,
		hq(),
		"devices.register",
		`{"serial":"FW-`+uuid.NewString()[:8]+`","sensorTypes":[],"unitId":"`+asStr(
			t,
			data(otherUnit)["id"],
		)+`"}`,
		0,
	)
	for name, testCase := range map[string]struct{ device, key string }{
		"tampered device": {tampered, apperr.KeyTamperUnresolved},
		"other model":     {asStr(t, data(otherDevice)["id"]), "error.otherModel"},
	} {
		if code, res := write(
			srv,
			hq(),
			"firmwareCampaigns.schedule",
			body(map[string]string{fieldDeviceIDs: `["` + testCase.device + `"]`}),
			0,
		); code != 422 ||
			asObj(t, res["fieldErrors"])["deviceIds"] != testCase.key {
			t.Errorf("%s: %d %v", name, code, res)
		}
	}

	bad := map[string]map[string]string{
		"start < 24 h": {
			fieldStartAt: `"` + clock().Add(time.Hour).Format(time.RFC3339) + `"`,
		},
		"waves not ascending": {
			fieldWaves: `[{"label":"A","percent":50},{"label":"B","percent":40}]`,
		},
		"waves end < 100":   {fieldWaves: `[{"label":"A","percent":50}]`},
		"auto pause 60":     {"autoPauseFailurePercent": "60"},
		"bad window":        {"window": `{"startLocal":"25:00","endLocal":"05:00"}`},
		"unsupported fw":    {fieldTargetVersion: `"v9"`},
		"already on target": {fieldTargetVersion: `"v1"`},
		"unknown device":    {fieldDeviceIDs: `["` + uuid.NewString() + `"]`},
		"duplicate device":  {fieldDeviceIDs: "[" + devs[0] + "," + devs[0] + "]"},
		caseUnknownModel:    {fieldModelID: `"` + seed.ID("ventilation-demo-b").String() + `"`},
	}
	for name, e := range bad {
		code, res := write(srv, hq(), "firmwareCampaigns.schedule", body(e), 0)
		if name == caseUnknownModel { // not a VALIDATION: the model is NOT_FOUND
			if code != 404 {
				t.Errorf("%s: %d", name, code)
			}

			continue
		}

		refusal(t, name, code, res, map[string]string{
			"start < 24 h": "startAt=error.atLeast24h", "waves not ascending": "waves[1]=error.ascending",
			"waves end < 100": "waves=error.mustEndAt100",
			"auto pause 60":   "autoPauseFailurePercent=error.range", "bad window": "window=error.invalid",
			"unsupported fw":    "targetVersion=error.unsupportedFirmware",
			"already on target": "deviceIds=error.alreadyOnTarget", "unknown device": "deviceIds=error.unknownDevice",
			"duplicate device": "deviceIds=error.duplicate",
		}[name])
	}
	// list / get
	if code, res := post(
		srv,
		hq(),
		"firmwareCampaigns.list",
		`{"filters":{"modelId":"`+model+`","status":"scheduled"},"limit":100}`,
	); code != 200 ||
		len(items(t, res)) == 0 {
		t.Fatalf("list: %d", code)
	}

	if code, res := post(
		srv,
		hq(),
		"firmwareCampaigns.list",
		`{"filters":{"status":"x"}}`,
	); code != 422 ||
		fields(res) != refusedFilterStatusInvalid {
		t.Errorf("bad status: %d %s", code, fields(res))
	}

	if code, _ := post(
		srv,
		hq(),
		"firmwareCampaigns.get",
		`{"id":"`+uuid.NewString()+`"}`,
	); code != 404 {
		t.Error("unknown campaign")
	}

	if code, _ := post(srv, customerA(), "firmwareCampaigns.list", `{}`); code != 403 {
		t.Error("client")
	}
	// control transitions
	ctl := func(action, extra string, version int) (int, map[string]any) {
		return write(
			srv,
			hq(),
			"firmwareCampaigns.control",
			`{"campaignId":"`+camp+`","action":"`+action+`"`+extra+`}`,
			version,
		)
	}
	if code, _ := ctl("resume", "", 1); code != 409 {
		t.Error("resume a scheduled campaign")
	}

	if code, m := ctl("pause", "", 1); code != 200 || data(m)["state"] != statusPaused {
		t.Fatalf("pause: %d", code)
	}

	if code, m := ctl("resume", "", 2); code != 200 || data(m)["state"] != statusScheduled {
		t.Fatalf("resume before start returns to scheduled: %d %v", code, m)
	}

	if code, _ := ctl("retry_device", `,"deviceId":`+devs[0], 3); code != 409 {
		t.Error("retry only while running/paused")
	}

	owner(t, `UPDATE devices.firmware_campaigns SET state = 'running' WHERE id = $1`, camp)

	if code, _ := ctl("retry_device", `,"deviceId":`+devs[0], 3); code != 409 {
		t.Error("retry only failed/skipped devices")
	}

	owner(
		t,
		`UPDATE devices.firmware_campaign_devices SET result = 'failed', detail = 'error.checksumMismatch' WHERE campaign_id =
			$1 AND device_id = $2`,
		camp,
		strings.Trim(devs[0], `"`),
	)

	if code, res := ctl(
		"retry_device",
		`,"deviceId":`+devs[0],
		3,
	); code != 200 ||
		asNum(t, asObj(t, data(res)["progress"])["failed"]) != 0 {
		t.Fatalf("retry: %d %v", code, res)
	}

	if code, m := ctl("abort", "", 4); code != 422 || fields(m) != refusedReasonLength {
		t.Errorf("abort needs a reason: %d %s", code, fields(m))
	}

	if code, m := ctl(
		"abort",
		`,"reason":"vendor recall"`,
		4,
	); code != 200 || data(m)["state"] != "aborted" ||
		data(m)["reason"] != "vendor recall" {
		t.Fatalf("abort: %d", code)
	}

	if code, _ := ctl("pause", "", 5); code != 409 {
		t.Error("aborted is terminal")
	}

	if code, _ := ctl("pause", "", 1); code != 409 {
		t.Error("stale version")
	}

	if code, m := ctl("explode", "", 5); code != 422 || fields(m) != "action=error.pathMismatch" {
		t.Errorf("bad action: %d %s", code, fields(m))
	}

	if code, _ := write(
		srv,
		hq(),
		"firmwareCampaigns.control",
		`{"campaignId":"`+uuid.NewString()+`","action":"pause"}`,
		1,
	); code != 404 {
		t.Errorf("control of an unknown campaign: %d", code)
	}
}
