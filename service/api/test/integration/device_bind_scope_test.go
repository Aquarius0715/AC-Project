package integration_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// A device that is not bound yet goes to the unit HQ planned for it (IR467: bind once installed), or anywhere by HQ or
// the member who registered it: a technician cannot take HQ's device planned for another unit to a unit of theirs
// (NOT_FOUND, as devices.get answers them, IR366, IR709); the device they registered themselves they bind.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestBindOnlyAReadableUnboundDevice(t *testing.T) {
	srv := server(t)
	owner(
		t,
		`DELETE FROM maintenance.assignments WHERE technician_membership_id = $1 AND job_id IN (SELECT id FROM
			maintenance.jobs WHERE customer_org_id = $2)`,
		seed.ID("tech-internal-a"),
		seed.ID("org-customer-b"),
	)

	planned := newUnit(t, srv, "Planned AC")
	mine := newUnit(t, srv, "Mine AC")
	job := assign(
		t,
		mine,
		"tech-internal-a",
		clock().Add(-time.Hour),
		clock().Add(time.Hour),
		"active",
	)

	register := func(who *actor, unit, jobID string) string {
		t.Helper()

		withJob := ""
		if jobID != "" {
			withJob = `,"jobId":"` + jobID + `"`
		}

		code, res := write(
			srv,
			who,
			"devices.register",
			`{"serial":"BS-`+uuid.NewString()[:8]+`","sensorTypes":[],"unitId":"`+unit+`"`+withJob+`}`,
			0,
		)
		if code != 200 {
			t.Fatalf("register: %d %v", code, res)
		}

		return asStr(t, data(res)["id"])
	}

	bind := func(device string) int {
		code, _ := write(
			srv,
			techInt(),
			"devices.bind",
			`{"deviceId":"`+device+`","unitId":"`+mine+`","jobId":"`+job+`","reason":"installed"}`,
			1,
		)

		return code
	}

	hqDevice := register(hq(), planned, "")
	if code, _ := post(srv, techInt(), "devices.get", `{"id":"`+hqDevice+`"}`); code != 404 {
		t.Fatalf("the technician reads HQ's unbound device: %d", code)
	}

	// nor do the writes on a bound device tell a technician that the unbound one exists (CONFLICT notBound)
	if code, _ := write(
		srv,
		techInt(),
		"devices.check",
		`{"id":"`+hqDevice+`","jobId":"`+job+`"}`,
		1,
	); code != 404 {
		t.Errorf("devices.check of HQ's unbound device: %d", code)
	}

	if code := bind(hqDevice); code != 404 {
		t.Errorf("the technician binds HQ's device planned for another unit: %d", code)
	}

	if code := bind(register(techInt(), mine, job)); code != 200 {
		t.Errorf("the technician binds the device they registered: %d", code)
	}
}
