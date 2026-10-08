package app

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/core/seed"
)

// assignedNow creates an internal job assigned to tech-internal-a whose work window contains the fixture clock.
func assignedNow(t *testing.T, s *Server) string {
	t.Helper()
	owner(t, `UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = $1 AND status = 'active' AND scheduled && tstzrange($2, $3)`,
		seed.ID("tech-internal-a"), clock.Add(-2*time.Hour), clock.Add(30*time.Hour))
	unit := seed.ID("unit-non-rto").String()
	_, m := write(s, &hq, "jobs.create", jobBody(unit, map[string]string{"alternativeSlots": "[]"}), 0)
	job := data(m)["id"].(string)
	ts := func(h int) string { return clock.Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	if code, m := write(s, &hq, "jobs.assign", `{"jobId":"`+job+`","technicianMembershipId":"`+seed.ID("tech-internal-a").String()+`","startAt":"`+ts(25)+`","endAt":"`+ts(27)+`"}`, 1); code != 200 {
		t.Fatalf("assign: %d %v", code, m)
	}
	owner(t, `UPDATE maintenance.assignments SET valid_from = $2, valid_until = $3, scheduled = tstzrange($2, $3), created_at = $2 WHERE job_id = $1 AND status = 'active'`,
		job, clock.Add(-time.Hour), clock.Add(time.Hour))
	return job
}

func TestOnSiteOperations(t *testing.T) {
	s := server(t)
	job := assignedNow(t, s)
	unit := seed.ID("unit-non-rto").String()

	// acknowledgement
	if code, _ := write(s, &techInt, "jobs.acknowledgeAssignment", `{"jobId":"`+job+`","decision":"cant_make"}`, 2); code != 422 {
		t.Error("cant_make needs a reason")
	}
	if code, _ := write(s, &techInt, "jobs.acknowledgeAssignment", `{"jobId":"`+job+`","decision":"accept","reason":"x"}`, 2); code != 422 {
		t.Error("accept takes no reason")
	}
	if code, _ := write(s, &techInt, "jobs.acknowledgeAssignment", `{"jobId":"`+job+`","decision":"maybe"}`, 2); code != 422 {
		t.Error("bad decision")
	}
	if code, _ := write(s, &techA, "jobs.acknowledgeAssignment", `{"jobId":"`+job+`","decision":"accept"}`, 2); code != 404 {
		t.Error("another technician")
	}
	if code, m := write(s, &techInt, "jobs.acknowledgeAssignment", `{"jobId":"`+job+`","decision":"accept"}`, 2); code != 200 || data(m)["acknowledgement"] != "accepted" || data(m)["jobId"] != job {
		t.Fatalf("ack: %d %v", code, m)
	}
	if code, _ := write(s, &techInt, "jobs.acknowledgeAssignment", `{"jobId":"`+job+`","decision":"accept"}`, 3); code != 409 {
		t.Error("ack twice")
	}

	// pause before start, start, check-in, pause/resume
	if code, _ := write(s, &techInt, "jobs.pauseWork", `{"jobId":"`+job+`","paused":true}`, 3); code != 409 {
		t.Error("pause before start")
	}
	if code, _ := write(s, &techInt, "jobs.start", `{"jobId":"`+job+`","startConfirmed":false}`, 3); code != 422 {
		t.Error("start not confirmed")
	}
	if code, _ := write(s, &techInt, "jobs.start", `{"jobId":"`+job+`","startConfirmed":true}`, 2); code != 409 {
		t.Error("start stale version")
	}
	if code, m := write(s, &techInt, "jobs.start", `{"jobId":"`+job+`","startConfirmed":true}`, 3); code != 200 || data(m)["status"] != "in_progress" || data(m)["startedAt"] == nil {
		t.Fatalf("start: %d %v", code, m)
	}
	if code, _ := write(s, &techInt, "jobs.start", `{"jobId":"`+job+`","startConfirmed":true}`, 4); code != 409 {
		t.Error("start twice")
	}
	for name, b := range map[string]string{
		"too far":       `{"jobId":"` + job + `","method":"location_qr","distanceMeters":201,"qrUnitId":"` + unit + `"}`,
		"no qr":         `{"jobId":"` + job + `","method":"location_qr","distanceMeters":10,"qrUnitId":null}`,
		"manual reason": `{"jobId":"` + job + `","method":"manual","distanceMeters":null,"qrUnitId":null,"reason":"  "}`,
		"bad method":    `{"jobId":"` + job + `","method":"gps","distanceMeters":null,"qrUnitId":null}`,
		"qr mismatch":   `{"jobId":"` + job + `","method":"location_qr","distanceMeters":10,"qrUnitId":"` + uuid.NewString() + `"}`,
	} {
		if code, _ := write(s, &techInt, "jobs.checkIn", b, 4); code != 422 {
			t.Errorf("checkIn %s: %d", name, code)
		}
	}
	code, m := write(s, &techInt, "jobs.checkIn", `{"jobId":"`+job+`","method":"location_qr","distanceMeters":200,"qrUnitId":"`+unit+`"}`, 4)
	tos, _ := data(m)["timeOnSite"].(map[string]any)
	if code != 200 || tos["arrivedAt"] == nil || tos["checkInMethod"] != "location_qr" || tos["distanceMeters"].(float64) != 200 {
		t.Fatalf("checkIn: %d %v", code, m)
	}
	if code, _ := write(s, &techInt, "jobs.checkIn", `{"jobId":"`+job+`","method":"manual","distanceMeters":null,"qrUnitId":null,"reason":"again"}`, 5); code != 409 {
		t.Error("check in twice")
	}
	if code, _ := write(s, &techInt, "jobs.pauseWork", `{"jobId":"`+job+`","paused":false}`, 5); code != 409 {
		t.Error("resume without pause")
	}
	if code, m := write(s, &techInt, "jobs.pauseWork", `{"jobId":"`+job+`","paused":true}`, 5); code != 200 || len(data(m)["timeOnSite"].(map[string]any)["pauses"].([]any)) != 1 {
		t.Fatalf("pause: %d %v", code, m)
	}
	if code, _ := write(s, &techInt, "jobs.pauseWork", `{"jobId":"`+job+`","paused":true}`, 6); code != 409 {
		t.Error("pause twice")
	}
	if code, m := write(s, &techInt, "jobs.pauseWork", `{"jobId":"`+job+`","paused":false}`, 6); code != 200 ||
		data(m)["timeOnSite"].(map[string]any)["pauses"].([]any)[0].(map[string]any)["to"] == nil {
		t.Fatalf("resume: %d", code)
	}
	if code, _ := write(s, &techInt, "jobs.pauseWork", `{"jobId":"`+job+`"}`, 7); code != 422 {
		t.Error("paused required")
	}

	// manual check-in starts an assigned job; outside the work window is FORBIDDEN
	j2 := assignedNow(t, s)
	if code, m := write(s, &techInt, "jobs.checkIn", `{"jobId":"`+j2+`","method":"manual","distanceMeters":null,"qrUnitId":null,"reason":"QR label damaged"}`, 2); code != 200 ||
		data(m)["status"] != "in_progress" || data(m)["timeOnSite"].(map[string]any)["checkInReason"] != "QR label damaged" {
		t.Fatalf("manual checkIn: %d %v", code, m)
	}
	j3 := assignedNow(t, s)
	owner(t, `UPDATE maintenance.assignments SET valid_from = $2, valid_until = $3, scheduled = tstzrange($2, $3) WHERE job_id = $1 AND status = 'active'`,
		j3, clock.Add(time.Hour), clock.Add(2*time.Hour))
	if code, m := write(s, &techInt, "jobs.start", `{"jobId":"`+j3+`","startConfirmed":true}`, 2); code != 403 || m["messageKey"] != "errors.assignment_not_started" {
		t.Errorf("before the work window: %d %v", code, m)
	}
	if code, _ := write(s, &techInt, "jobs.start", `{"jobId":"`+uuid.NewString()+`","startConfirmed":true}`, 1); code != 404 {
		t.Error("unknown job")
	}
	if code, _ := write(s, &hq, "jobs.start", `{"jobId":"`+j3+`","startConfirmed":true}`, 2); code != 403 {
		t.Error("HQ cannot start")
	}
	alt := `"alternativeSlot":{"startAt":"` + clock.Add(48*time.Hour).Format(time.RFC3339) + `","endAt":"` + clock.Add(50*time.Hour).Format(time.RFC3339) + `"}`
	// acknowledgement after the viewing window ended
	owner(t, `UPDATE maintenance.assignments SET valid_from = $2, valid_until = $3, scheduled = tstzrange($2, $3) WHERE job_id = $1 AND status = 'active'`,
		j3, clock.Add(-3*time.Hour), clock.Add(-2*time.Hour))
	if code, _ := write(s, &techInt, "jobs.acknowledgeAssignment", `{"jobId":"`+j3+`","decision":"cant_make","reason":"sick",`+alt+`}`, 2); code != 403 {
		t.Error("ack after the window")
	}
	owner(t, `UPDATE maintenance.assignments SET valid_from = $2, valid_until = $3, scheduled = tstzrange($2, $3) WHERE job_id = $1 AND status = 'active'`,
		j3, clock.Add(time.Hour), clock.Add(2*time.Hour))
	if code, m := write(s, &techInt, "jobs.acknowledgeAssignment", `{"jobId":"`+j3+`","decision":"cant_make","reason":"sick",`+alt+`}`, 2); code != 200 ||
		data(m)["alternativeSlot"] == nil || data(m)["acknowledgement"] != "cant_make" {
		t.Fatalf("cant_make: %d %v", code, m)
	}
}
