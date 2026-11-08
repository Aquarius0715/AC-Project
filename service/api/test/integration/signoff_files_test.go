package integration_test

import "testing"

// A sign-off binds one signature (or absence reason with a site photo) to the draft. The file of a sign-off that is
// replaced or cleared (a new sign-off, a photo added, the draft saved) is no part of the report any more: it must not
// stay among the report's attachments, where the technician's app shows it as a photo and where it counts toward the 10
// of attachments.add (DD-T09).
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestSignOffFilesFollowTheSignOff(t *testing.T) {
	srv := server(t)
	job := assignedNow(t, srv)
	write(srv, techInt(), "jobs.start", `{"jobId":"`+job+`","startConfirmed":true}`, 2)
	_, res := write(srv, techInt(), "jobs.saveDraft", draftBody(job, "normal", nil), 0)
	rep := asStr(t, data(res)["id"])
	img := tinyPNG(t)
	sig := blobJSON("sig.png", "image/png", img, len(img))

	files := func() int {
		var n int

		ownerScan(
			t,
			`SELECT count(*) FROM maintenance.attachments WHERE report_id = $1`,
			[]any{rep},
			&n,
		)

		return n
	}
	sign := func(version int) {
		t.Helper()

		code, res := write(
			srv,
			techInt(),
			"reports.signOff",
			`{"jobId":"`+job+`","reportId":"`+rep+`","reportVersion":`+itoa(
				version,
			)+`,"signerName":"Aisha","signature":`+sig+`}`,
			version,
		)
		if code != 200 {
			t.Fatalf("signOff: %d %v", code, res)
		}
	}

	sign(1)

	if n := files(); n != 1 {
		t.Fatalf("one sign-off: %d files", n)
	}

	sign(1) // signed again on the same version: the first signature is replaced

	if n := files(); n != 1 {
		t.Errorf("signed twice: %d files, want only the current signature", n)
	}

	code, res := write(
		srv,
		techInt(),
		"attachments.add",
		`{"jobId":"`+job+`","reportId":"`+rep+`","file":`+blobJSON(
			"a.png",
			"image/png",
			img,
			len(img),
		)+`}`,
		1,
	)
	if code != 200 {
		t.Fatalf("add: %d %v", code, res)
	}

	if n := files(); n != 1 {
		t.Errorf("a photo added clears the sign-off: %d files, want the photo only", n)
	}

	// saving the draft clears the sign-off too: its file goes, the photo stays
	sign(2)

	if n := files(); n != 2 {
		t.Fatalf("a photo and a signature: %d files", n)
	}

	if code, res := write(
		srv,
		techInt(),
		"jobs.saveDraft",
		draftBody(job, "normal", map[string]string{fieldReportID: `"` + rep + `"`}),
		2,
	); code != 200 {
		t.Fatalf("saveDraft: %d %v", code, res)
	}

	if n := files(); n != 1 {
		t.Errorf("saveDraft clears the sign-off: %d files, want the photo only", n)
	}
}
