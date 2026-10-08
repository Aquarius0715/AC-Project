package app

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/core/seed"
)

func TestFirmwareCampaigns(t *testing.T) {
	s := server(t)
	model := seed.ID("ventilation-demo").String()
	var devs []string
	for i := 0; i < 3; i++ {
		u := newUnit(t, s, "FW AC")
		_, m := write(s, &hq, "devices.register", `{"serial":"FW-`+uuid.NewString()[:8]+`","sensorTypes":[],"unitId":"`+u+`"}`, 0)
		devs = append(devs, `"`+data(m)["id"].(string)+`"`)
	}
	start := clock.Add(48 * time.Hour).Format(time.RFC3339)
	body := func(extra map[string]string) string {
		f := map[string]string{"modelId": `"` + model + `"`, "targetVersion": `"v2"`, "deviceIds": "[" + strings.Join(devs, ",") + "]",
			"waves": `[{"label":"Pilot","percent":34},{"label":"All","percent":100}]`, "window": `{"startLocal":"01:00","endLocal":"05:00"}`,
			"autoPauseFailurePercent": "5", "startAt": `"` + start + `"`}
		for k, v := range extra {
			f[k] = v
		}
		parts := []string{}
		for k, v := range f {
			parts = append(parts, `"`+k+`":`+v)
		}
		return "{" + strings.Join(parts, ",") + "}"
	}
	code, m := write(s, &hq, "firmwareCampaigns.schedule", body(nil), 0)
	if code != 200 || data(m)["state"] != "scheduled" || len(data(m)["results"].([]any)) != 3 || !strings.HasPrefix(data(m)["checksum"].(string), "sha256:") {
		t.Fatalf("schedule: %d %v", code, m)
	}
	camp := data(m)["id"].(string)
	if data(m)["progress"].(map[string]any)["pending"].(float64) != 3 {
		t.Fatal("progress")
	}
	bad := map[string]map[string]string{
		"start < 24 h":        {"startAt": `"` + clock.Add(time.Hour).Format(time.RFC3339) + `"`},
		"waves not ascending": {"waves": `[{"label":"A","percent":50},{"label":"B","percent":40}]`},
		"waves end < 100":     {"waves": `[{"label":"A","percent":50}]`},
		"auto pause 60":       {"autoPauseFailurePercent": "60"},
		"bad window":          {"window": `{"startLocal":"25:00","endLocal":"05:00"}`},
		"unsupported fw":      {"targetVersion": `"v9"`},
		"already on target":   {"targetVersion": `"v1"`},
		"unknown device":      {"deviceIds": `["` + uuid.NewString() + `"]`},
		"duplicate device":    {"deviceIds": "[" + devs[0] + "," + devs[0] + "]"},
		"other model":         {"modelId": `"` + seed.ID("ventilation-demo-b").String() + `"`},
	}
	for name, e := range bad {
		code, _ := write(s, &hq, "firmwareCampaigns.schedule", body(e), 0)
		if code != 422 && !(name == "other model" && code == 404) {
			t.Errorf("%s: %d", name, code)
		}
	}
	// list / get
	if code, m := post(s, &hq, "firmwareCampaigns.list", `{"filters":{"modelId":"`+model+`","status":"scheduled"},"limit":100}`); code != 200 || len(items(m)) == 0 {
		t.Fatalf("list: %d", code)
	}
	if code, _ := post(s, &hq, "firmwareCampaigns.list", `{"filters":{"status":"x"}}`); code != 422 {
		t.Error("bad status")
	}
	if code, _ := post(s, &hq, "firmwareCampaigns.get", `{"id":"`+uuid.NewString()+`"}`); code != 404 {
		t.Error("unknown campaign")
	}
	if code, _ := post(s, &customerA, "firmwareCampaigns.list", `{}`); code != 403 {
		t.Error("client")
	}
	// control transitions
	ctl := func(action, extra string, v int) (int, map[string]any) {
		return write(s, &hq, "firmwareCampaigns.control", `{"campaignId":"`+camp+`","action":"`+action+`"`+extra+`}`, v)
	}
	if code, _ := ctl("resume", "", 1); code != 409 {
		t.Error("resume a scheduled campaign")
	}
	if code, m := ctl("pause", "", 1); code != 200 || data(m)["state"] != "paused" {
		t.Fatalf("pause: %d", code)
	}
	if code, m := ctl("resume", "", 2); code != 200 || data(m)["state"] != "scheduled" {
		t.Fatalf("resume before start returns to scheduled: %d %v", code, m)
	}
	if code, _ := ctl("retry_device", `,"deviceId":`+devs[0], 3); code != 409 {
		t.Error("retry only while running/paused")
	}
	owner(t, `UPDATE devices.firmware_campaigns SET state = 'running' WHERE id = $1`, camp)
	if code, _ := ctl("retry_device", `,"deviceId":`+devs[0], 3); code != 409 {
		t.Error("retry only failed/skipped devices")
	}
	owner(t, `UPDATE devices.firmware_campaign_devices SET result = 'failed', detail = 'error.checksumMismatch' WHERE campaign_id = $1 AND device_id = $2`, camp, strings.Trim(devs[0], `"`))
	if code, m := ctl("retry_device", `,"deviceId":`+devs[0], 3); code != 200 || data(m)["progress"].(map[string]any)["failed"].(float64) != 0 {
		t.Fatalf("retry: %d %v", code, m)
	}
	if code, _ := ctl("abort", "", 4); code != 422 {
		t.Error("abort needs a reason")
	}
	if code, m := ctl("abort", `,"reason":"vendor recall"`, 4); code != 200 || data(m)["state"] != "aborted" || data(m)["reason"] != "vendor recall" {
		t.Fatalf("abort: %d", code)
	}
	if code, _ := ctl("pause", "", 5); code != 409 {
		t.Error("aborted is terminal")
	}
	if code, _ := ctl("pause", "", 1); code != 409 {
		t.Error("stale version")
	}
	if code, _ := ctl("explode", "", 5); code != 422 {
		t.Error("bad action")
	}
}
