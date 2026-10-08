package app

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func blobJSON(name, mime string, data []byte, size int) string {
	return `{"name":"` + name + `","mime":"` + mime + `","size":` + itoa(size) + `,"bytes":"` + base64.StdEncoding.EncodeToString(data) + `"}`
}

func TestAttachmentsAndSignOff(t *testing.T) {
	s := server(t)
	job := assignedNow(t, s)
	write(s, &techInt, "jobs.start", `{"jobId":"`+job+`","startConfirmed":true}`, 2)
	_, m := write(s, &techInt, "jobs.saveDraft", draftBody(job, "normal", nil), 0)
	rep := data(m)["id"].(string)
	img := []byte("\x89PNG photo bytes")
	add := func(file string, v int) (int, map[string]any) {
		return write(s, &techInt, "attachments.add", `{"jobId":"`+job+`","reportId":"`+rep+`","file":`+file+`}`, v)
	}
	for name, f := range map[string]string{
		"gif":           blobJSON("a.gif", "image/gif", img, len(img)),
		"size mismatch": blobJSON("a.png", "image/png", img, 3),
		"empty":         blobJSON("a.png", "image/png", nil, 0),
		"no name":       blobJSON(" ", "image/png", img, len(img)),
	} {
		if code, _ := add(f, 1); code != 422 {
			t.Errorf("add %s: %d", name, code)
		}
	}
	if code, _ := add(blobJSON("a.png", "image/png", img, len(img)), 2); code != 409 {
		t.Error("stale report version")
	}
	if code, _ := write(s, &techInt, "attachments.add", `{"jobId":"`+job+`","reportId":"`+uuid.NewString()+`","file":`+blobJSON("a.png", "image/png", img, len(img))+`}`, 1); code != 409 {
		t.Error("other report")
	}
	code, m := add(blobJSON(" leak.png ", "image/png", img, len(img)), 1)
	if code != 200 || data(m)["status"] != "ready" || data(m)["name"] != "leak.png" {
		t.Fatalf("add: %d %v", code, m)
	}
	att := data(m)["id"].(string)
	// the draft moved to version 2; link the photo as evidence
	body := strings.Replace(draftBody(job, "normal", map[string]string{"reportId": `"` + rep + `"`, "attachmentIds": `["` + att + `"]`}),
		`"componentKey":"filter","result":"normal","reason":null,"evidenceIds":[]`, `"componentKey":"filter","result":"normal","reason":null,"evidenceIds":["`+att+`"]`, 1)
	if code, m := write(s, &techInt, "jobs.saveDraft", body, 2); code != 200 || len(data(m)["attachmentRefs"].([]any)) != 1 {
		t.Fatalf("draft with attachment: %d %v", code, m)
	}
	// sign-off
	sign := func(extra string, v, rv int) (int, map[string]any) {
		return write(s, &techInt, "reports.signOff", `{"jobId":"`+job+`","reportId":"`+rep+`","reportVersion":`+itoa(rv)+`,"signerName":" Aisha "`+extra+`}`, v)
	}
	sig := blobJSON("sig.png", "image/png", img, len(img))
	for name, extra := range map[string]string{
		"neither":              `,"signature":null`,
		"both":                 `,"signature":` + sig + `,"absentReason":"out","sitePhoto":` + sig,
		"jpeg signature":       `,"signature":` + blobJSON("sig.jpg", "image/jpeg", img, len(img)),
		"absent without photo": `,"signature":null,"absentReason":"out"`,
		"blank absent reason":  `,"signature":null,"absentReason":" ","sitePhoto":` + sig,
	} {
		if code, _ := sign(extra, 3, 3); code != 422 {
			t.Errorf("signOff %s: %d", name, code)
		}
	}
	if code, _ := sign(`,"signature":`+sig, 3, 2); code != 409 {
		t.Error("signOff of an old version")
	}
	code, m = sign(`,"signature":`+sig, 3, 3)
	so, _ := data(m)["signOff"].(map[string]any)
	if code != 200 || so["signerName"] != "Aisha" || so["signatureAttachmentId"] == nil || so["reportVersion"].(float64) != 3 || data(m)["version"].(float64) != 3 {
		t.Fatalf("signOff: %d %v", code, m)
	}
	if code, m := sign(`,"signature":null,"absentReason":"Customer out","sitePhoto":`+blobJSON("door.jpg", "image/jpeg", img, len(img)), 3, 3); code != 200 ||
		data(m)["signOff"].(map[string]any)["sitePhotoAttachmentId"] == nil {
		t.Fatalf("absent sign-off: %d %v", code, m)
	}
	// getContent
	get := func(a *actor, v int) (int, map[string]any) {
		return post(s, a, "attachments.getContent", `{"jobId":"`+job+`","reportId":"`+rep+`","reportVersion":`+itoa(v)+`,"attachmentId":"`+att+`"}`)
	}
	code, m = get(&techInt, 3)
	if code != 200 || data(m)["bytes"] != base64.StdEncoding.EncodeToString(img) || data(m)["mime"] != "image/png" {
		t.Fatalf("getContent: %d %v", code, m)
	}
	if code, _ := get(&customerA, 3); code != 404 {
		t.Error("client before acceptance")
	}
	if code, _ := post(s, &hq, "attachments.getContent", `{"jobId":"`+job+`","reportId":"`+rep+`","reportVersion":3,"attachmentId":"`+uuid.NewString()+`"}`); code != 404 {
		t.Error("unknown attachment")
	}
	// saving the draft clears the sign-off
	if _, m := write(s, &techInt, "jobs.saveDraft", body, 3); data(m)["signOff"] != nil {
		t.Fatal("saveDraft clears the sign-off")
	}
	// limit of 10 per report
	for i := 0; i < 9; i++ {
		owner(t, `INSERT INTO maintenance.attachments (tenant_id, job_id, report_id, object_key, name, mime, size_bytes, status, uploaded_by)
			SELECT tenant_id, job_id, $1, $2, 'x.png', 'image/png', 1, 'ready', author_id FROM maintenance.work_reports WHERE id = $1 LIMIT 1`, rep, "test/"+uuid.NewString())
	}
	if code, _ := add(blobJSON("11.png", "image/png", img, len(img)), 4); code != 409 {
		t.Error("eleventh attachment")
	}
}
