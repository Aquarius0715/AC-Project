package integration

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func blobJSON(name, mime string, data []byte, size int) string {
	return `{"name":"` + name + `","mime":"` + mime + `","size":` + itoa(
		size,
	) + `,"bytes":"` + base64.StdEncoding.EncodeToString(
		data,
	) + `"}`
}

func TestAttachmentsAndSignOff(t *testing.T) {
	srv := server(t)
	job := assignedNow(t, srv)
	write(srv, &techInt, "jobs.start", `{"jobId":"`+job+`","startConfirmed":true}`, 2)
	_, res := write(srv, &techInt, "jobs.saveDraft", draftBody(job, "normal", nil), 0)
	rep := asStr(t, data(res)["id"])
	img, jpg := tinyPNG(t), tinyJPEG(t)
	flood := pngHeader(50000, 50000) // 2.5 gigapixels declared in a few dozen bytes (IR389)
	add := func(file string, v int) (int, map[string]any) {
		return write(
			srv,
			&techInt,
			"attachments.add",
			`{"jobId":"`+job+`","reportId":"`+rep+`","file":`+file+`}`,
			v,
		)
	}
	want := map[string]string{ // the field each case is refused for
		"empty":         "file=error.invalidFile",
		"gif":           "file=error.invalidFile",
		"no name":       "file=error.invalidFile",
		"not a png":     "file=error.invalidFile",
		"png as jpeg":   "file=error.invalidFile",
		"size mismatch": "file=error.invalidFile",
		"pixel flood":   "file=error.invalidFile",
		// IR368: the name is a label without control characters, paths or bidirectional overrides
		"line break in the name": "file=error.invalidFile",
		"path in the name":       "file=error.invalidFile",
		"override in the name":   "file=error.invalidFile",
	}

	for name, f := range map[string]string{
		"gif":           blobJSON("a.gif", "image/gif", img, len(img)),
		"size mismatch": blobJSON("a.png", "image/png", img, 3),
		"pixel flood":   blobJSON("flood.png", "image/png", flood, len(flood)),
		"empty":         blobJSON("a.png", "image/png", nil, 0),
		"no name":       blobJSON(" ", "image/png", img, len(img)),
		// stored and served only as what it is (IR308)
		"not a png":              blobJSON("a.png", "image/png", []byte("<svg onload=alert(1)>"), 21),
		"png as jpeg":            blobJSON("a.jpg", "image/jpeg", img, len(img)),
		"line break in the name": blobJSON(`a\r\nSet-Cookie: x.png`, "image/png", img, len(img)),
		"path in the name":       blobJSON("../../a.png", "image/png", img, len(img)),
		"override in the name":   blobJSON("photo\u202egnp.exe", "image/png", img, len(img)),
	} {
		code, m := add(f, 1)
		refusal(t, name, code, m, want[name])
	}

	if code, _ := add(blobJSON("a.png", "image/png", img, len(img)), 2); code != 409 {
		t.Error("stale report version")
	}

	if code, _ := write(
		srv,
		&techInt,
		"attachments.add",
		`{"jobId":"`+job+`","reportId":"`+uuid.NewString()+`","file":`+blobJSON(
			"a.png",
			"image/png",
			img,
			len(img),
		)+`}`,
		1,
	); code != 409 {
		t.Error("other report")
	}

	code, res := add(blobJSON(" leak.png ", "image/png", img, len(img)), 1)
	if code != 200 || data(res)["status"] != "ready" || data(res)["name"] != "leak.png" {
		t.Fatalf("add: %d %v", code, res)
	}

	att := asStr(t, data(res)["id"])
	// the draft moved to version 2; link the photo as evidence
	body := strings.Replace(
		draftBody(
			job,
			"normal",
			map[string]string{"reportId": `"` + rep + `"`, "attachmentIds": `["` + att + `"]`},
		),
		`"componentKey":"filter","result":"normal","reason":null,"evidenceIds":[]`,
		`"componentKey":"filter","result":"normal","reason":null,"evidenceIds":["`+att+`"]`,
		1,
	)

	code, res = write(srv, &techInt, "jobs.saveDraft", body, 2)
	if code != 200 || len(asArr(t, data(res)["attachmentRefs"])) != 1 {
		t.Fatalf("draft with attachment: %d %v", code, res)
	}
	// the finding keeps its linked photo (InspectionItem.evidenceIds, IR352)
	for _, it := range asArr(t, data(res)["items"]) {
		if x := asObj(t, it); x["componentKey"] == "filter" {
			if ev, _ := x["evidenceIds"].([]any); len(ev) != 1 || ev[0] != att {
				t.Errorf("filter evidence: %v", x["evidenceIds"])
			}
		}
	}
	// sign-off
	sign := func(extra string, v, rv int) (int, map[string]any) {
		return write(
			srv,
			&techInt,
			"reports.signOff",
			`{"jobId":"`+job+`","reportId":"`+rep+`","reportVersion":`+itoa(
				rv,
			)+`,"signerName":" Aisha "`+extra+`}`,
			v,
		)
	}
	sig := blobJSON("sig.png", "image/png", img, len(img))
	want = map[string]string{ // the field each case is refused for
		"absent without photo": "signature=errors.signature_or_absence",
		"blank absent reason":  "absentReason=error.length",
		"both":                 "signature=errors.signature_or_absence",
		"jpeg signature":       "signature=error.invalidFile",
		"neither":              "signature=errors.signature_or_absence",
	}

	for name, extra := range map[string]string{
		"neither":              `,"signature":null`,
		"both":                 `,"signature":` + sig + `,"absentReason":"out","sitePhoto":` + sig,
		"jpeg signature":       `,"signature":` + blobJSON("sig.jpg", "image/jpeg", img, len(img)),
		"absent without photo": `,"signature":null,"absentReason":"out"`,
		"blank absent reason":  `,"signature":null,"absentReason":" ","sitePhoto":` + sig,
	} {
		code, m := sign(extra, 3, 3)
		refusal(t, name, code, m, want[name])
	}

	if code, _ := sign(`,"signature":`+sig, 3, 2); code != 409 {
		t.Error("signOff of an old version")
	}

	code, res = sign(`,"signature":`+sig, 3, 3)

	so, _ := data(res)["signOff"].(map[string]any)
	if code != 200 || so["signerName"] != "Aisha" || so["signatureAttachmentId"] == nil ||
		asNum(t, so["reportVersion"]) != 3 ||
		asNum(t, data(res)["version"]) != 3 {
		t.Fatalf("signOff: %d %v", code, res)
	}

	if code, resp := sign(
		`,"signature":null,"absentReason":"Customer out","sitePhoto":`+blobJSON(
			"door.jpg",
			"image/jpeg",
			jpg,
			len(jpg),
		),
		3,
		3,
	); code != 200 ||
		asObj(t, data(resp)["signOff"])["sitePhotoAttachmentId"] == nil {
		t.Fatalf("absent sign-off: %d %v", code, resp)
	}
	// getContent
	get := func(a *actor, v int) (int, map[string]any) {
		return post(
			srv,
			a,
			"attachments.getContent",
			`{"jobId":"`+job+`","reportId":"`+rep+`","reportVersion":`+itoa(
				v,
			)+`,"attachmentId":"`+att+`"}`,
		)
	}

	code, res = get(&techInt, 3)
	if code != 200 || data(res)["bytes"] != base64.StdEncoding.EncodeToString(img) ||
		data(res)["mime"] != "image/png" {
		t.Fatalf("getContent: %d %v", code, res)
	}

	if code, _ := get(&customerA, 3); code != 404 {
		t.Error("client before acceptance")
	}

	if code, _ := post(
		srv,
		&hq,
		"attachments.getContent",
		`{"jobId":"`+job+`","reportId":"`+rep+`","reportVersion":3,"attachmentId":"`+uuid.NewString()+`"}`,
	); code != 404 {
		t.Error("unknown attachment")
	}
	// saving the draft clears the sign-off
	if code, resp := write(
		srv,
		&techInt,
		"jobs.saveDraft",
		body,
		3,
	); code != 200 ||
		data(resp)["signOff"] != nil {
		t.Fatal("saveDraft clears the sign-off")
	}
	// limit of 10 per report
	for range 9 {
		owner(
			t,
			`INSERT INTO maintenance.attachments (tenant_id, job_id, report_id, object_key, name, mime, size_bytes, status,
				uploaded_by)
			SELECT tenant_id, job_id, $1, $2, 'x.png', 'image/png', 1, 'ready', author_id FROM maintenance.work_reports WHERE id
			= $1 LIMIT 1`,
			rep,
			"test/"+uuid.NewString(),
		)
	}

	if code, _ := add(blobJSON("11.png", "image/png", img, len(img)), 4); code != 409 {
		t.Error("eleventh attachment")
	}
}
