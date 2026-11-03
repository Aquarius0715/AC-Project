package integration_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

// assignedNow creates an internal job assigned to tech-internal-a whose work window contains the fixture clock.
func assignedNow(t *testing.T, srv *apiserver.Server) string {
	t.Helper()
	owner(
		t,
		`UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = $1 AND status = 'active' AND
			scheduled && tstzrange($2, $3)`,
		seed.ID("tech-internal-a"),
		clock().Add(-2*time.Hour),
		clock().Add(30*time.Hour),
	)
	unit := seed.ID("unit-non-rto").String()
	_, resp := write(
		srv,
		hq(),
		"jobs.create",
		jobBody(unit, map[string]string{"alternativeSlots": "[]"}),
		0,
	)
	job := asStr(t, data(resp)["id"])

	stamp := func(h int) string { return clock().Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	if code, res := write(
		srv,
		hq(),
		"jobs.assign",
		`{"jobId":"`+job+`","technicianMembershipId":"`+seed.ID("tech-internal-a").
			String()+
			`","startAt":"`+stamp(
			25,
		)+`","endAt":"`+stamp(
			27,
		)+`"}`,
		1,
	); code != 200 {
		t.Fatalf("assign: %d %v", code, res)
	}

	owner(
		t,
		`UPDATE maintenance.assignments SET valid_from = $2, valid_until = $3, scheduled = tstzrange($2, $3), created_at = $2
			WHERE job_id = $1 AND status = 'active'`,
		job,
		clock().Add(-time.Hour),
		clock().Add(time.Hour),
	)

	return job
}

func TestOnSiteOperations(t *testing.T) {
	srv := server(t)
	job := assignedNow(t, srv)
	unit := seed.ID("unit-non-rto").String()

	// acknowledgement
	if code, res := write(
		srv,
		techInt(),
		"jobs.acknowledgeAssignment",
		`{"jobId":"`+job+`","decision":"cant_make"}`,
		2,
	); code != 422 ||
		fields(res) != "reason=error.required" {
		t.Errorf("cant_make needs a reason: %d %s", code, fields(res))
	}

	if code, res := write(
		srv,
		techInt(),
		"jobs.acknowledgeAssignment",
		`{"jobId":"`+job+`","decision":"accept","reason":"x"}`,
		2,
	); code != 422 ||
		fields(res) != "decision=error.fieldsNotAllowed" {
		t.Errorf("accept takes no reason: %d %s", code, fields(res))
	}

	if code, res := write(
		srv,
		techInt(),
		"jobs.acknowledgeAssignment",
		`{"jobId":"`+job+`","decision":"maybe"}`,
		2,
	); code != 422 ||
		fields(res) != "decision=error.invalid" {
		t.Errorf("bad decision: %d %s", code, fields(res))
	}

	if code, _ := write(
		srv,
		techA(),
		"jobs.acknowledgeAssignment",
		`{"jobId":"`+job+`","decision":"accept"}`,
		2,
	); code != 404 {
		t.Error("another technician")
	}
	// IR237: the client sees the technician's name only once the technician accepted
	clientName := func() any {
		_, m := post(srv, customerA(), "jobs.get", `{"jobId":"`+job+`"}`)

		return asObj(t, data(m)["assignment"])["technicianName"]
	}
	if n := clientName(); n != nil {
		t.Errorf("technician name before acceptance: %v", n)
	}

	if code, res := write(
		srv,
		techInt(),
		"jobs.acknowledgeAssignment",
		`{"jobId":"`+job+`","decision":"accept"}`,
		2,
	); code != 200 || data(res)["acknowledgement"] != "accepted" ||
		data(res)["jobId"] != job {
		t.Fatalf("ack: %d %v", code, res)
	}

	if n, _ := clientName().(string); n == "" {
		t.Error("the client sees the technician's name after acceptance")
	}

	if _, m := post(
		srv,
		hq(),
		"jobs.get",
		`{"jobId":"`+job+`"}`,
	); asObj(t, data(m)["assignment"])["technicianName"] != nil {
		t.Error("only the client projection carries the name")
	}

	if code, _ := write(
		srv,
		techInt(),
		"jobs.acknowledgeAssignment",
		`{"jobId":"`+job+`","decision":"accept"}`,
		3,
	); code != 409 {
		t.Error("ack twice")
	}

	// a pause before the start, the start, check-in, pause/resume
	if code, _ := write(
		srv,
		techInt(),
		"jobs.pauseWork",
		`{"jobId":"`+job+`","paused":true}`,
		3,
	); code != 409 {
		t.Error("pause before start")
	}

	if code, res := write(
		srv,
		techInt(),
		"jobs.start",
		`{"jobId":"`+job+`","startConfirmed":false}`,
		3,
	); code != 422 ||
		fields(res) != "startConfirmed=error.required" {
		t.Errorf("start not confirmed: %d %s", code, fields(res))
	}

	if code, _ := write(
		srv,
		techInt(),
		"jobs.start",
		`{"jobId":"`+job+`","startConfirmed":true}`,
		2,
	); code != 409 {
		t.Error("start stale version")
	}

	if code, res := write(
		srv,
		techInt(),
		"jobs.start",
		`{"jobId":"`+job+`","startConfirmed":true}`,
		3,
	); code != 200 || data(res)["status"] != "in_progress" ||
		data(res)["startedAt"] == nil {
		t.Fatalf("start: %d %v", code, res)
	}

	if code, _ := write(
		srv,
		techInt(),
		"jobs.start",
		`{"jobId":"`+job+`","startConfirmed":true}`,
		4,
	); code != 409 {
		t.Error("start twice")
	}

	want := map[string]string{ // the field each case is refused for
		"bad method":    "method=error.invalid",
		"manual reason": "reason=error.length",
		"no qr":         "qrUnitId=error.required",
		"qr mismatch":   "qrUnitId=errors.qr_unit_mismatch",
		"too far":       "distanceMeters=errors.too_far",
	}

	for name, b := range map[string]string{
		"too far":       `{"jobId":"` + job + `","method":"location_qr","distanceMeters":201,"qrUnitId":"` + unit + `"}`,
		"no qr":         `{"jobId":"` + job + `","method":"location_qr","distanceMeters":10,"qrUnitId":null}`,
		"manual reason": `{"jobId":"` + job + `","method":"manual","distanceMeters":null,"qrUnitId":null,"reason":"  "}`,
		"bad method":    `{"jobId":"` + job + `","method":"gps","distanceMeters":null,"qrUnitId":null}`,
		"qr mismatch": `{"jobId":"` + job + `","method":"location_qr","distanceMeters":10,` +
			`"qrUnitId":"` + uuid.NewString() + `"}`,
	} {
		code, m := write(srv, techInt(), "jobs.checkIn", b, 4)
		refusal(t, name, code, m, want[name])
	}

	code, resp := write(
		srv,
		techInt(),
		"jobs.checkIn",
		`{"jobId":"`+job+`","method":"location_qr","distanceMeters":200,"qrUnitId":"`+unit+`"}`,
		4,
	)

	tos, _ := data(resp)["timeOnSite"].(map[string]any)
	if code != 200 || tos["arrivedAt"] == nil || tos["checkInMethod"] != "location_qr" ||
		asNum(t, tos["distanceMeters"]) != 200 {
		t.Fatalf("checkIn: %d %v", code, resp)
	}

	if code, _ := write(
		srv,
		techInt(),
		"jobs.checkIn",
		`{"jobId":"`+job+`","method":"manual","distanceMeters":null,"qrUnitId":null,"reason":"again"}`,
		5,
	); code != 409 {
		t.Error("check in twice")
	}

	if code, _ := write(
		srv,
		techInt(),
		"jobs.pauseWork",
		`{"jobId":"`+job+`","paused":false}`,
		5,
	); code != 409 {
		t.Error("resume without pause")
	}

	if code, res := write(
		srv,
		techInt(),
		"jobs.pauseWork",
		`{"jobId":"`+job+`","paused":true}`,
		5,
	); code != 200 ||
		len(asArr(t, asObj(t, data(res)["timeOnSite"])["pauses"])) != 1 {
		t.Fatalf("pause: %d %v", code, res)
	}

	if code, _ := write(
		srv,
		techInt(),
		"jobs.pauseWork",
		`{"jobId":"`+job+`","paused":true}`,
		6,
	); code != 409 {
		t.Error("pause twice")
	}

	if code, res := write(
		srv,
		techInt(),
		"jobs.pauseWork",
		`{"jobId":"`+job+`","paused":false}`,
		6,
	); code != 200 ||
		asObj(t, asArr(t, asObj(t, data(res)["timeOnSite"])["pauses"])[0])["to"] == nil {
		t.Fatalf("resume: %d", code)
	}

	if code, res := write(
		srv,
		techInt(),
		"jobs.pauseWork",
		`{"jobId":"`+job+`"}`,
		7,
	); code != 422 ||
		fields(res) != "paused=error.required" {
		t.Errorf("paused required: %d %s", code, fields(res))
	}

	// manual check-in starts an assigned job; outside the work window is FORBIDDEN
	j2 := assignedNow(t, srv)
	if code, res := write(
		srv,
		techInt(),
		"jobs.checkIn",
		`{"jobId":"`+j2+`","method":"manual","distanceMeters":null,"qrUnitId":null,"reason":"QR label damaged"}`,
		2,
	); code != 200 ||
		data(res)["status"] != "in_progress" ||
		asObj(t, data(res)["timeOnSite"])["checkInReason"] != "QR label damaged" {
		t.Fatalf("manual checkIn: %d %v", code, res)
	}

	j3 := assignedNow(t, srv)
	owner(
		t,
		`UPDATE maintenance.assignments SET valid_from = $2, valid_until = $3, scheduled = tstzrange($2, $3) WHERE job_id = $1
			AND status = 'active'`,
		j3,
		clock().Add(time.Hour),
		clock().Add(2*time.Hour),
	)

	if code, res := write(
		srv,
		techInt(),
		"jobs.start",
		`{"jobId":"`+j3+`","startConfirmed":true}`,
		2,
	); code != 403 ||
		res["messageKey"] != "errors.assignment_not_started" {
		t.Errorf("before the work window: %d %v", code, res)
	}

	if code, _ := write(
		srv,
		techInt(),
		"jobs.start",
		`{"jobId":"`+uuid.NewString()+`","startConfirmed":true}`,
		1,
	); code != 404 {
		t.Error("unknown job")
	}

	if code, _ := write(
		srv,
		hq(),
		"jobs.start",
		`{"jobId":"`+j3+`","startConfirmed":true}`,
		2,
	); code != 403 {
		t.Error("HQ cannot start")
	}

	alt := `"alternativeSlot":{"startAt":"` + clock().Add(48*time.Hour).
		Format(time.RFC3339) +
		`","endAt":"` + clock().Add(50*time.Hour).
		Format(time.RFC3339) +
		`"}`
	// acknowledgement after the viewing window ended
	owner(
		t,
		`UPDATE maintenance.assignments SET valid_from = $2, valid_until = $3, scheduled = tstzrange($2, $3) WHERE job_id = $1
			AND status = 'active'`,
		j3,
		clock().Add(-3*time.Hour),
		clock().Add(-2*time.Hour),
	)

	if code, _ := write(
		srv,
		techInt(),
		"jobs.acknowledgeAssignment",
		`{"jobId":"`+j3+`","decision":"cant_make","reason":"sick",`+alt+`}`,
		2,
	); code != 403 {
		t.Error("ack after the window")
	}

	owner(
		t,
		`UPDATE maintenance.assignments SET valid_from = $2, valid_until = $3, scheduled = tstzrange($2, $3) WHERE job_id = $1
			AND status = 'active'`,
		j3,
		clock().Add(time.Hour),
		clock().Add(2*time.Hour),
	)

	if code, res := write(
		srv,
		techInt(),
		"jobs.acknowledgeAssignment",
		`{"jobId":"`+j3+`","decision":"cant_make","reason":"sick",`+alt+`}`,
		2,
	); code != 200 ||
		data(res)["alternativeSlot"] == nil ||
		data(res)["acknowledgement"] != "cant_make" {
		t.Fatalf("cant_make: %d %v", code, res)
	}
}
