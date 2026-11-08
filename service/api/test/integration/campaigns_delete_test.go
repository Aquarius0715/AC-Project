package integration_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// A firmware campaign that has ended (aborted or completed) is deleted with its device rows; one that has not is
// refused with the reason (DEC-103). HQ only.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestFirmwareCampaignsDelete(t *testing.T) {
	srv := server(t)
	unit := newUnit(t, srv, "FW Del AC")

	_, res := write(
		srv,
		hq(),
		"devices.register",
		`{"serial":"FWD-`+uuid.NewString()[:8]+`","sensorTypes":[],"unitId":"`+unit+`"}`,
		0,
	)
	dev := asStr(t, data(res)["id"])
	start := clock().Add(48 * time.Hour).Format(time.RFC3339)

	code, resp := write(
		srv,
		hq(),
		"firmwareCampaigns.schedule",
		`{"modelId":"`+seed.ID("ventilation-demo").
			String()+
			`","targetVersion":"v2","deviceIds":["`+dev+`"],`+
			`"waves":[{"label":"All","percent":100}],"window":{"startLocal":"01:00","endLocal":"05:00"},`+
			`"autoPauseFailurePercent":5,"startAt":"`+start+`"}`,
		0,
	)
	if code != 200 {
		t.Fatalf("schedule: %d %v", code, resp)
	}

	camp := asStr(t, data(resp)["id"])
	version := ver(t, resp)
	remove := func(a *actor, v int) (int, map[string]any) {
		return write(
			srv,
			a,
			"firmwareCampaigns.delete",
			`{"id":"`+camp+`","reason":"scheduled by mistake"}`,
			v,
		)
	}

	if code, r := remove(hq(), version); code != 409 || r["messageKey"] != "error.campaignActive" {
		t.Errorf("delete of a scheduled campaign: %d %v", code, r["messageKey"])
	}

	code, resp = write(
		srv,
		hq(),
		"firmwareCampaigns.control",
		`{"campaignId":"`+camp+`","action":"abort","reason":"wrong target"}`,
		version,
	)
	if code != 200 {
		t.Fatalf("abort: %d %v", code, resp)
	}

	version = ver(t, resp)

	for name, who := range map[string]*actor{"client": customerB(), "technician": techInt()} {
		if code, _ := remove(who, version); code == 200 {
			t.Errorf("%s deletes a campaign", name)
		}
	}

	if code, res := write(
		srv,
		hq(),
		"firmwareCampaigns.delete",
		`{"id":"`+camp+`","reason":""}`,
		version,
	); code != 422 || fields(res) != refusedReasonLength {
		t.Errorf("delete needs a reason: %d %s", code, fields(res))
	}

	if code, _ := remove(hq(), version+5); code != 409 {
		t.Error("delete at a stale version")
	}

	code, resp = remove(hq(), version)
	if code != 200 || data(resp)["deleted"] != true || data(resp)["id"] != camp {
		t.Fatalf("delete: %d %v", code, resp)
	}

	if code, _ := post(srv, hq(), "firmwareCampaigns.get", `{"id":"`+camp+`"}`); code != 404 {
		t.Error("a deleted campaign is gone")
	}

	if code, _ := remove(hq(), version); code != 404 {
		t.Error("a campaign is deleted once")
	}

	var left int

	ownerScan(
		t,
		`SELECT count(*) FROM devices.firmware_campaign_devices WHERE campaign_id = $1`,
		[]any{camp},
		&left,
	)

	if left != 0 {
		t.Errorf("the campaign's device rows stay: %d", left)
	}

	// the device itself stays registered
	if code, _ := post(srv, hq(), "devices.get", `{"id":"`+dev+`"}`); code != 200 {
		t.Error("the device is gone with the campaign")
	}
}
