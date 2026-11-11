package integration_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestNotificationsInbox(t *testing.T) {
	srv := server(t)
	// earlier runs leave many notifications: mark customer-b's existing ones read so unreadOnly shows this run's
	owner(
		t,
		`UPDATE notify.notifications SET read_at = $2 WHERE recipient_membership_id = $1 AND read_at IS NULL`,
		seed.ID("customer-b"),
		clock(),
	)
	unit := boundUnit(t, srv, "online")
	key, inv := overdueContract(t, srv, unit)

	code, res := write(
		srv,
		restrMgr(),
		"restrictions.schedule",
		scheduleBody(key, inv, []string{unit}, nil),
		0,
	)
	if code != 200 {
		t.Fatalf("schedule: %d %v", code, res)
	}

	rid := asStr(t, data(res)["id"])
	// a notification whose target customer-b cannot read is hidden (IR58)
	hidden := uuid.NewString()
	owner(
		t,
		`INSERT INTO notify.notifications (id, tenant_id, recipient_membership_id, scope_version_at_creation, type,
			template_key, target, params, severity, occurred_at)
		VALUES ($1,$2,$3,1,'fault','alert',$4,'{}','critical',$5)`,
		hidden,
		seed.ID("tenant-a"),
		seed.ID("customer-b"),
		`{"kind":"unit","id":"`+seed.ID("unit-online-rto").String()+`"}`,
		clock(),
	)

	_, res = post(
		srv,
		customerB(),
		"notifications.list",
		`{"filters":{"type":"restriction","unreadOnly":true},"limit":100}`,
	)

	var notice map[string]any

	for _, it := range items(t, res) {
		if asObj(t, it["target"])["id"] == rid {
			notice = it
		}
	}

	if notice == nil || notice["deliveryState"] != "simulated" ||
		asObj(t, notice["params"])["targetName"] == "" ||
		notice["severity"] != severityWarning {
		t.Fatalf("restriction notice: %v", res)
	}

	_, res = post(
		srv,
		customerB(),
		"notifications.list",
		`{"filters":{"severity":"critical"},"limit":100}`,
	)
	for _, it := range items(t, res) {
		if it["id"] == hidden {
			t.Fatal("unreadable target listed")
		}
	}

	want := map[string]string{ // the field each case is refused for
		bodyUnknownKind:                         "kind=error.notAllowed",
		"{\"filters\":{\"severity\":\"high\"}}": refusedFiltersInvalid,
		"{\"filters\":{\"type\":\"spam\"}}":     refusedFiltersInvalid,
	}

	for _, b := range []string{
		`{"filters":{"severity":"high"}}`, bodyUnknownKind,
		`{"filters":{"type":"spam"}}`,
	} {
		code, m := post(srv, customerB(), "notifications.list", b)
		refusal(t, b, code, m, want[b])
	}

	if code, resp := post(
		srv,
		customerB(),
		"notifications.list",
		`{"filters":{"from":"2026-09-14T00:00:00Z","to":"2026-09-01T00:00:00Z"}}`,
	); code != 422 ||
		asObj(t, resp["fieldErrors"])["filters.to"] != apperr.KeyRange {
		t.Errorf("a reversed period: %d %v", code, resp)
	}

	if _, resp := post(
		srv,
		customerB(),
		"notifications.list",
		`{"sort":{"field":"severity","direction":"desc"},"limit":5}`,
	); len(
		items(t, resp),
	) == 0 {
		t.Error("severity sort")
	}

	notificationID := asStr(t, notice["id"])
	if code, _ := write(
		srv,
		customerB(),
		"notifications.markRead",
		`{"id":"`+notificationID+`"}`,
		2,
	); code != 409 {
		t.Error("stale notification version")
	}

	if code, _ := write(
		srv,
		customerA(),
		"notifications.markRead",
		`{"id":"`+notificationID+`"}`,
		1,
	); code != 404 {
		t.Error("mark another member's notification")
	}

	if code, _ := write(
		srv,
		customerB(),
		"notifications.markRead",
		`{"id":"`+hidden+`"}`,
		1,
	); code != 404 {
		t.Error("mark an unreadable target")
	}

	code, res = write(srv, customerB(), "notifications.markRead", `{"id":"`+notificationID+`"}`, 1)
	if code != 200 || data(res)["readAt"] == nil || ver(t, res) != 2 {
		t.Fatalf("markRead: %d %v", code, res)
	}

	if code, resp := write(
		srv,
		customerB(),
		"notifications.markRead",
		`{"id":"`+notificationID+`"}`,
		2,
	); code != 200 ||
		ver(t, resp) != 2 {
		t.Error("markRead again keeps the record")
	}
	// HQ sees the restriction notice target; technicians see nothing for it
	if code, resp := post(
		srv,
		techB(),
		"notifications.list",
		`{"filters":{"type":"restriction"}}`,
	); code != 200 ||
		len(items(t, resp)) != 0 {
		t.Error("technician restriction notices")
	}
}

// TestMarkAllRead: "Mark all as read" is one write for the whole list (IR379), so it spends one of the member's
// writes a minute (IR375). The caller's unread notifications with a readable target are marked; every other ID
// (already read, another member's, an unreadable target, unknown) is skipped alike and stays as it is. The list holds
// 1–100 distinct IDs and takes no expected version.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestMarkAllRead(t *testing.T) {
	srv := server(t)
	note := func(member, unit string) string {
		notificationID := uuid.NewString()
		owner(
			t,
			`INSERT INTO notify.notifications (id, tenant_id, recipient_membership_id, scope_version_at_creation, type,
				template_key, target, params, severity, occurred_at)
			VALUES ($1,$2,$3,1,'fault','alert',$4,'{}','warning',$5)`,
			notificationID,
			seed.ID("tenant-a"),
			seed.ID(member),
			`{"kind":"unit","id":"`+seed.ID(unit).String()+`"}`,
			clock(),
		)

		return notificationID
	}
	mine, mine2, read := note(
		"customer-b",
		"unit-other-customer",
	), note(
		"customer-b",
		"unit-other-customer",
	), note(
		"customer-b",
		"unit-other-customer",
	)
	owner(
		t,
		`UPDATE notify.notifications SET read_at = $2, version = 2 WHERE id = $1`,
		read,
		clock(),
	)

	hidden := note(
		"customer-b",
		"unit-online-rto",
	) // customer A's unit: customer-b cannot read the target (IR58)
	theirs := note("customer-a", "unit-online-rto")
	unknown := uuid.NewString()
	list := func(ids ...string) string { return `{"ids":["` + strings.Join(ids, `","`) + `"]}` }

	many := make([]string, 101)
	for i := range many {
		many[i] = uuid.NewString()
	}

	for name, testCase := range map[string]struct{ body, want string }{
		"no list":     {`{}`, "ids=error.required"},
		"empty list":  {`{"ids":[]}`, "ids=error.required"},
		"101 IDs":     {list(many...), "ids=error.count"},
		"a duplicate": {list(mine, mine), "ids=error.invalid"},
		"the nil ID":  {list(uuid.Nil.String()), "ids=error.invalid"},
	} {
		code, m := write(srv, customerB(), "notifications.markAllRead", testCase.body, 0)
		refusal(t, name, code, m, testCase.want)
	}

	code, res := write(srv, customerB(), "notifications.markAllRead", list(mine), 1)
	refusal(t, "an expected version", code, res, "expectedVersion=error.notAllowed")

	code, res = write(
		srv,
		customerB(),
		"notifications.markAllRead",
		list(mine, read, theirs, hidden, unknown, mine2),
		0,
	)
	if code != 200 {
		t.Fatalf("markAllRead: %d %v", code, res)
	}

	got := func(k string) string {
		listed := asArr(t, data(res)[k])

		out := make([]string, 0, len(listed))
		for _, v := range listed {
			out = append(out, asStr(t, v))
		}

		return strings.Join(out, ",")
	}
	if got("markedIds") != mine+","+mine2 ||
		got("skippedIds") != strings.Join([]string{read, theirs, hidden, unknown}, ",") {
		t.Fatalf("marked %s, skipped %s", got("markedIds"), got("skippedIds"))
	}

	state := func(notificationID string) (bool, int) {
		var (
			readAt  *time.Time
			version int
		)
		ownerScan(
			t,
			`SELECT read_at, version FROM notify.notifications WHERE id = $1`,
			[]any{notificationID},
			&readAt,
			&version,
		)

		return readAt != nil, version
	}
	for id, want := range map[string]struct {
		read    bool
		version int
	}{mine: {true, 2}, mine2: {true, 2}, read: {true, 2}, theirs: {false, 1}, hidden: {false, 1}} {
		if r, v := state(id); r != want.read || v != want.version {
			t.Errorf("%s: read %v version %d, want %v %d", id, r, v, want.read, want.version)
		}
	}
	// a second click finds nothing unread: everything is skipped and nothing changes
	code, res = write(srv, customerB(), "notifications.markAllRead", list(mine, mine2), 0)
	if code != 200 || got("markedIds") != "" || got("skippedIds") != mine+","+mine2 {
		t.Fatalf("again: %d %v", code, res)
	}

	if _, v := state(mine); v != 2 {
		t.Errorf("marked again: version %d", v)
	}
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestNotificationRecipientsPreview(t *testing.T) {
	srv := server(t)
	_, inv := newInvoice(t, srv)

	body := func(channel string) string {
		return `{"target":{"kind":"invoice",` +
			`"id":"` + inv + `"},"templateKey":"payment_reminder",` +
			`"channel":"` + channel + `","query":{}}`
	}
	if code, _ := post(srv, hq(), "notifications.recipients", body("inApp")); code != 409 {
		t.Error("reminder recipients before due")
	}

	owner(t, `UPDATE billing.invoices SET due_at = $2 WHERE id = $1`, inv, clock().Add(-time.Hour))

	code, resp := post(srv, hq(), "notifications.recipients", body("inApp"))
	if code != 200 || len(items(t, resp)) == 0 {
		t.Fatalf("recipients: %d %v", code, resp)
	}

	for _, r := range items(t, resp) {
		if r["role"] != roleClient {
			t.Errorf("payment_reminder recipient %v", r)
		}
	}

	recipient := asStr(t, items(t, resp)[0]["id"])

	if code, _ := post(srv, customerB(), "notifications.recipients", body("inApp")); code != 403 {
		t.Error("client lists reminder recipients")
	}

	if code, _ := post(
		srv,
		customerA(),
		"notifications.recipients",
		`{"target":{"kind":"invoice","id":"`+inv+`"},"templateKey":"payment","channel":"inApp","query":{}}`,
	); code != 404 {
		t.Error("other customer's invoice")
	}

	for payload, want := range map[string]string{
		`{"target":{"kind":"planet",` +
			`"id":"` + inv + `"},"templateKey":"payment","channel":"inApp",` +
			`"query":{}}`: "target=error.invalid",
		`{"target":{"kind":"invoice",` +
			`"id":"` + inv + `"},"templateKey":"spam","channel":"sms",` +
			`"query":{}}`: "templateKey=error.invalid", // and the channel
		`{"target":{"kind":"invoice",` +
			`"id":"` + inv + `"},"templateKey":"payment","channel":"inApp","role":"guest",` +
			`"query":{}}`: "role=error.invalid",
		`{"target":{"kind":"invoice",` +
			`"id":"` + inv + `"},"templateKey":"payment","channel":"inApp",` +
			`"query":{"filters":{"x":1}}}`: refusedUnknownFieldX,
	} {
		code, m := post(srv, hq(), "notifications.recipients", payload)
		refusal(t, payload, code, m, want)
	}

	if _, res := post(
		srv,
		hq(),
		"notifications.recipients",
		`{"target":{"kind":"invoice",`+
			`"id":"`+inv+`"},"templateKey":"payment","channel":"email","role":"admin",`+
			`"query":{"limit":1}}`,
	); len(
		items(t, res),
	) != 1 {
		t.Errorf("admin recipients for payment: %v", res)
	}

	preview := func(recipient string) (int, map[string]any) {
		return post(
			srv,
			hq(),
			"notifications.preview",
			`{"target":{"kind":"invoice",`+
				`"id":"`+inv+`"},"templateKey":"payment_reminder","channel":"email",`+
				`"recipientMembershipId":"`+recipient+`","message":"Please pay"}`,
		)
	}

	code, resp = preview(recipient)
	if code != 200 || data(resp)["deliveryState"] != deliveryPreview ||
		asObj(t, data(resp)["params"])["message"] != "Please pay" ||
		data(resp)["type"] != "payment_reminder" {
		t.Fatalf("preview: %d %v", code, resp)
	}

	var saved int
	ownerScan(
		t,
		`SELECT count(*) FROM notify.notifications WHERE id = $1`,
		[]any{data(resp)["id"]},
		&saved,
	)

	if saved != 0 {
		t.Error("preview saved a notification")
	}

	if code, m := preview(
		seed.ID("customer-a").String(),
	); code != 422 ||
		fields(m) != "recipientMembershipId=errors.recipient_ineligible" {
		t.Errorf("ineligible preview recipient: %d %s", code, fields(m))
	}
	// a payment reminder is about an invoice only
	if code, res := post(
		srv,
		hq(),
		"notifications.preview",
		`{"target":{"kind":"job","id":"`+seed.ID("job-contractor-a").
			String()+
			`"},"templateKey":"payment_reminder","channel":"email",`+
			`"recipientMembershipId":"`+recipient+`","message":"Please `+
			`pay"}`,
	); code != 422 ||
		asObj(t, res["fieldErrors"])["target"] != apperr.KeyInvalid {
		t.Errorf("a payment reminder about a job: %d %v", code, res)
	}

	if code, res := post(
		srv,
		hq(),
		"notifications.preview",
		`{"target":{"kind":"invoice",`+
			`"id":"`+inv+`"},"templateKey":"payment_reminder","channel":"email",`+
			`"recipientMembershipId":"`+recipient+`","message":" "}`,
	); code != 422 ||
		fields(res) != refusedMessageLength {
		t.Errorf("blank message: %d %s", code, fields(res))
	}
	// the message may be as long as a job note (2000, DD-P07); the reason stays at 1000 (IR228)
	long := strings.Repeat("m", 2000)
	if code, res := post(
		srv,
		hq(),
		"notifications.preview",
		`{"target":{"kind":"invoice",`+
			`"id":"`+inv+`"},"templateKey":"payment_reminder","channel":"email",`+
			`"recipientMembershipId":"`+recipient+`","message":"`+long+`"}`,
	); code != 200 {
		t.Errorf("2000-character message: %d %v", code, res)
	}

	for body, field := range map[string]string{
		`"message":"` + long + `m"`:                    "message",
		`"reason":"` + strings.Repeat("r", 1001) + `"`: fieldReason,
	} {
		if code, res := post(
			srv,
			hq(),
			"notifications.preview",
			`{"target":{"kind":"invoice",`+
				`"id":"`+inv+`"},"templateKey":"payment_reminder","channel":"email",`+
				`"recipientMembershipId":"`+recipient+`",`+body+`}`,
		); code != 422 ||
			asObj(t, res["fieldErrors"])[field] != apperr.KeyLength {
			t.Errorf("%s too long: %d %v", field, code, res)
		}
	}
	// unit target: the client of the unit's organization reads it, another client does not
	unit := seed.ID("unit-online-rto").String()
	if code, res := post(
		srv,
		customerA(),
		"notifications.recipients",
		`{"target":{"kind":"unit","id":"`+unit+`"},"templateKey":"alert","channel":"inApp","query":{}}`,
	); code != 200 ||
		len(items(t, res)) == 0 {
		t.Errorf("unit recipients: %d %v", code, res)
	}
	// an alert message is a fault notification about an open alert, of warning severity unless the policy's is given
	// (IR348: as the notifier sends it)
	alertPreview := func(extra string) (int, map[string]any) {
		return post(
			srv,
			hq(),
			"notifications.preview",
			`{"target":{"kind":"unit",`+
				`"id":"`+unit+`"},"templateKey":"alert","channel":"inApp",`+
				`"recipientMembershipId":"`+seed.ID("customer-a").
				String()+
				`"`+extra+`}`,
		)
	}
	if code, m := alertPreview(
		`,"message":"Please check the unit"`,
	); code != 200 || data(m)["type"] != "fault" || data(m)["severity"] != severityWarning ||
		asObj(t, data(m)["params"])["status"] != statusOpen {
		t.Errorf("alert preview: %d %v", code, m)
	}

	if code, m := alertPreview(
		`,"severity":"critical"`,
	); code != 200 || data(m)["severity"] != "critical" ||
		data(m)["deliveryState"] != deliveryPreview {
		t.Errorf("critical alert preview: %d %v", code, m)
	}

	if code, m := alertPreview(
		`,"severity":"loud"`,
	); code != 422 ||
		fields(m) != "severity=error.invalid" {
		t.Errorf("unknown severity: %d %s", code, fields(m))
	}

	if code, res := post(
		srv,
		hq(),
		"notifications.preview",
		`{"target":{"kind":"invoice",`+
			`"id":"`+inv+`"},"templateKey":"payment_reminder","channel":"email",`+
			`"recipientMembershipId":"`+recipient+`","message":"Please pay","severity":"critical"}`,
	); code != 422 ||
		fields(res) != "severity=error.notAllowed" {
		t.Errorf("severity of a payment reminder: %d %s", code, fields(res))
	}

	if code, _ := post(
		srv,
		customerB(),
		"notifications.recipients",
		`{"target":{"kind":"unit","id":"`+unit+`"},"templateKey":"alert","channel":"inApp","query":{}}`,
	); code != 404 {
		t.Error("unit of another customer")
	}
	// IR345: an HQ member who reads units without alert.read is a recipient of other unit notices, never of an alert —
	// policies.save would refuse them (IR120 item 3)
	assetOnly := uuid.NewString()
	owner(
		t,
		`INSERT INTO identity.memberships (id, tenant_id, user_id, organization_id, role, valid_from) VALUES ($1,$2,$3,$4,
			'admin',$5)`,
		assetOnly,
		seed.ID("tenant-a"),
		newUser(t),
		seed.ID("org-operator-a"),
		clock().Add(-time.Hour),
	)
	owner(
		t,
		`INSERT INTO identity.membership_permissions (tenant_id, membership_id, permission) VALUES ($1,$2,'asset.read')`,
		seed.ID("tenant-a"),
		assetOnly,
	)
	t.Cleanup(func() {
		owner(
			t,
			`UPDATE identity.memberships SET valid_until = valid_from + interval '1 second' WHERE id = $1`,
			assetOnly,
		)
	})

	listed := func(template string) (bool, bool) {
		t.Helper()

		var hqOperator, asset bool

		code, res := post(
			srv,
			hq(),
			"notifications.recipients",
			`{"target":{"kind":"unit",`+
				`"id":"`+unit+`"},"templateKey":"`+template+`","channel":"inApp","role":"admin",`+
				`"query":{"limit":100}}`,
		)
		if code != 200 {
			t.Fatalf("%s recipients: %d %v", template, code, res)
		}

		for _, r := range items(t, res) {
			hqOperator = hqOperator || r["id"] == seed.ID("hq-operator").String()
			asset = asset || r["id"] == assetOnly
		}

		return hqOperator, asset
	}
	if op, asset := listed("alert"); !op || asset {
		t.Errorf("alert recipients: hq-operator %v, asset.read only %v", op, asset)
	}

	if _, asset := listed("quality"); !asset {
		t.Error("a unit notice other than an alert reaches asset.read")
	}

	if code, res := post(
		srv,
		hq(),
		"notifications.preview",
		`{"target":{"kind":"unit",`+
			`"id":"`+unit+`"},"templateKey":"alert","channel":"inApp",`+
			`"recipientMembershipId":"`+assetOnly+`"}`,
	); code != 422 ||
		fields(res) != "recipientMembershipId=errors.recipient_ineligible" {
		t.Errorf("alert preview for asset.read only: %d %s", code, fields(res))
	}
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestPreferencesAndConsents(t *testing.T) {
	srv := server(t)
	if code, res := post(
		srv,
		techB(),
		"preferences.get",
		`{}`,
	); code != 200 || data(res)["currency"] != "MYR" ||
		data(res)["timezone"] == "" {
		t.Fatalf("preferences default: %d %v", code, res)
	}

	want := map[string]string{ // the field each case is refused for
		"{\"locale\":\"en\",\"timezone\":\"Local\"}":             refusedTimezoneInvalid,
		"{\"locale\":\"en\",\"timezone\":\"Mars/Base\"}":         refusedTimezoneInvalid,
		"{\"locale\":\"fr\",\"timezone\":\"Asia/Kuala_Lumpur\"}": "locale=error.invalid",
	}

	for _, b := range []string{`{"locale":"fr",` +
		`"timezone":"Asia/Kuala_Lumpur"}`, `{"locale":"en",` +
		`"timezone":"Mars/Base"}`, `{"locale":"en","timezone":"Local"}`} {
		code, m := write(srv, customerB(), "preferences.update", b, 0)
		refusal(t, b, code, m, want[b])
	}

	if code, res := write(
		srv,
		hq(),
		"preferences.update",
		`{"locale":"en","timezone":"UTC","monthlyReportEmail":true}`,
		0,
	); code != 422 ||
		fields(res) != "monthlyReportEmail=error.notAllowed" {
		t.Errorf("monthlyReportEmail for HQ: %d %s", code, fields(res))
	}

	if code, res := write(
		srv,
		customerB(),
		"preferences.update",
		`{"locale":"ms","timezone":"Asia/Singapore","monthlyReportEmail":true}`,
		0,
	); code != 200 ||
		data(res)["locale"] != "ms" {
		t.Fatalf("update: %d %v", code, res)
	}

	// Japanese is a display language too (IR639)
	if code, res := write(
		srv,
		customerB(),
		"preferences.update",
		`{"locale":"ja","timezone":"Asia/Tokyo"}`,
		0,
	); code != 200 ||
		data(res)["locale"] != "ja" {
		t.Fatalf("update to Japanese: %d %v", code, res)
	}

	if _, m := post(srv, customerB(), "preferences.get", `{}`); data(m)["locale"] != "ja" {
		t.Error("the saved Japanese preference")
	}

	if code, res := write(
		srv,
		customerB(),
		"preferences.update",
		`{"locale":"en","timezone":"Asia/Kuala_Lumpur"}`,
		0,
	); code != 200 ||
		data(res)["monthlyReportEmail"] != true {
		t.Fatalf("keep monthly flag: %d %v", code, res)
	}

	if _, m := post(srv, customerB(), "preferences.get", `{}`); data(m)["locale"] != "en" {
		t.Error("saved preferences")
	}

	owner(
		t,
		`UPDATE identity.consents SET granted = false, granted_at = NULL, revoked_at = NULL, version = 1 WHERE membership_id =
			$1`,
		seed.ID("customer-b"),
	)

	if code, res := post(
		srv,
		customerB(),
		"consents.get",
		`{"purpose":"location_automation"}`,
	); code != 200 || data(res)["granted"] != false ||
		ver(t, res) != 1 {
		t.Fatalf("consent get: %d %v", code, res)
	}

	if code, res := post(
		srv,
		customerB(),
		"consents.get",
		`{"purpose":"marketing"}`,
	); code != 422 ||
		fields(res) != "purpose=error.invalid" {
		t.Errorf("bad purpose: %d %s", code, fields(res))
	}

	if code, _ := post(
		srv,
		hq(),
		"consents.get",
		`{"purpose":"location_automation"}`,
	); code != 403 {
		t.Error("HQ consent")
	}

	if code, _ := write(
		srv,
		customerB(),
		"consents.update",
		`{"purpose":"location_automation","granted":true}`,
		3,
	); code != 409 {
		t.Error("stale consent version")
	}

	code, resp := write(
		srv,
		customerB(),
		"consents.update",
		`{"purpose":"location_automation","granted":true}`,
		1,
	)
	if code != 200 || data(resp)["grantedAt"] == nil || ver(t, resp) != 2 {
		t.Fatalf("grant: %d %v", code, resp)
	}

	if code, res := write(
		srv,
		customerB(),
		"consents.update",
		`{"purpose":"location_automation","granted":true}`,
		2,
	); code != 200 ||
		ver(t, res) != 2 {
		t.Error("same value unchanged")
	}

	code, resp = write(
		srv,
		customerB(),
		"consents.update",
		`{"purpose":"location_automation","granted":false}`,
		2,
	)
	if code != 200 || data(resp)["revokedAt"] == nil || data(resp)["grantedAt"] == nil {
		t.Fatalf("revoke: %d %v", code, resp)
	}
}

// TestTechnicianJobNotificationScope: a technician's notification about a job is listed and can be marked read only
// while the assignment lets the technician open the job — active and its scheduled window not over (IR49, IR58, IR253).
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestTechnicianJobNotificationScope(t *testing.T) {
	srv := server(t)
	tenant, tech, job := seed.ID(
		"tenant-a",
	), seed.ID(
		"tech-internal-a",
	), seed.ID(
		"job-contractor-a",
	)
	nid, aid := uuid.NewString(), uuid.NewString()
	owner(
		t,
		`INSERT INTO notify.notifications (id, tenant_id, recipient_membership_id, scope_version_at_creation, type,
			template_key, target, params, severity, occurred_at)
		VALUES ($1,$2,$3,1,'schedule_change','schedule_change',$4,'{}','normal',$5)`,
		nid,
		tenant,
		tech,
		`{"kind":"job","id":"`+job.String()+`"}`,
		clock(),
	)
	owner(
		t,
		`INSERT INTO notify.ref_assignments (id, tenant_id, job_id, technician_membership_id, status, scheduled) VALUES ($1,
			$2,$3,$4,'active',tstzrange($5,$6))`,
		aid,
		tenant,
		job,
		tech,
		clock().Add(-time.Hour),
		clock().Add(time.Hour),
	)
	t.Cleanup(func() {
		owner(t, `DELETE FROM notify.notifications WHERE id = $1`, nid)
		owner(t, `DELETE FROM notify.ref_assignments WHERE id = $1`, aid)
		owner(t, `DELETE FROM notify.ref_job_histories WHERE job_id = $1`, job)
	})

	listed := func() bool {
		_, res := post(
			srv,
			techInt(),
			"notifications.list",
			`{"filters":{"type":"schedule_change"},"limit":100}`,
		)
		for _, it := range items(t, res) {
			if it["id"] == nid {
				return true
			}
		}

		return false
	}
	if !listed() {
		t.Fatal("active assignment inside its window: notification not listed")
	}

	owner(t, `UPDATE notify.ref_assignments SET status = 'revoked' WHERE id = $1`, aid)

	if listed() {
		t.Error("revoked assignment: notification still listed")
	}
	// the job's history snapshot of the technician — frozen when the assignment ended — brings the notice back, so a
	// cancellation or reassignment is told; another technician's snapshot does not (IR601)
	hid, other := uuid.NewString(), uuid.NewString()
	owner(
		t,
		`INSERT INTO notify.ref_job_histories (id, tenant_id, job_id, owner_kind, owner_id) VALUES ($1,$2,$3,'membership',$4),
			($5,$2,$3,'organization',$6)`,
		other,
		tenant,
		job,
		seed.ID("tech-external-a"),
		hid,
		seed.ID("org-contractor-a"),
	)

	if listed() {
		t.Error("another member's or the company's history: notification listed")
	}

	owner(
		t,
		`UPDATE notify.ref_job_histories SET owner_kind = 'membership', owner_id = $2 WHERE id = $1`,
		hid,
		tech,
	)

	if !listed() {
		t.Error("the technician's own history: notification not listed")
	}

	owner(t, `DELETE FROM notify.ref_job_histories WHERE id IN ($1, $2)`, hid, other)

	if listed() {
		t.Error("history gone: notification still listed")
	}

	owner(
		t,
		`UPDATE notify.ref_assignments SET status = 'active', scheduled = tstzrange($2,$3) WHERE id = $1`,
		aid,
		clock().Add(-3*time.Hour),
		clock().Add(-time.Hour),
	)

	if listed() {
		t.Error("assignment window over: notification still listed")
	}

	if code, _ := write(
		srv,
		techInt(),
		"notifications.markRead",
		`{"id":"`+nid+`"}`,
		1,
	); code != 404 {
		t.Errorf("mark read with the job out of reach: %d", code)
	}

	owner(
		t,
		`UPDATE notify.ref_assignments SET scheduled = tstzrange($2,$3) WHERE id = $1`,
		aid,
		clock().Add(-time.Hour),
		clock().Add(time.Hour),
	)

	if code, _ := write(
		srv,
		techInt(),
		"notifications.markRead",
		`{"id":"`+nid+`"}`,
		1,
	); code != 200 {
		t.Errorf("mark read inside the window: %d", code)
	}
}

// IR58: a contractor reads the notifications of jobs offered to its company, and nothing else — not another job, not
// a unit.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestContractorInbox(t *testing.T) {
	srv := server(t)
	note := func(kind, targetID string) string {
		notificationID := uuid.NewString()
		owner(
			t,
			`INSERT INTO notify.notifications (id, tenant_id, recipient_membership_id, scope_version_at_creation, type,
				template_key, target, params, severity, occurred_at)
			VALUES ($1, $2, $3, 1, 'job_update', 'job_update', $4, '{}', 'normal', $5)`,
			notificationID,
			seed.ID("tenant-a"),
			seed.ID("contractor-a"),
			`{"kind":"`+kind+`","id":"`+targetID+`"}`,
			clock(),
		)

		return notificationID
	}
	offered := note("job", seed.ID("job-contractor-a").String())
	other := note("job", uuid.NewString())
	unit := note("unit", seed.ID("unit-online-rto").String())

	code, resp := post(srv, contrA(), "notifications.list", `{"limit":100}`)
	if code != 200 {
		t.Fatalf("contractor inbox: %d %v", code, resp)
	}

	seen := map[string]bool{}
	for _, it := range items(t, resp) {
		seen[asStr(t, it["id"])] = true
	}

	if !seen[offered] || seen[other] || seen[unit] {
		t.Errorf(
			"contractor inbox: offered %v, another job %v, a unit %v",
			seen[offered],
			seen[other],
			seen[unit],
		)
	}

	for _, it := range items(t, resp) { // the accepted job opens for the company: its notice may link it (IR602)
		if it["id"] == offered && it["targetOpen"] != true {
			t.Errorf("the accepted job's notice: targetOpen %v", it["targetOpen"])
		}
	}

	if code, res := post(
		srv,
		contrB(),
		"notifications.list",
		`{"limit":100}`,
	); code != 200 ||
		len(items(t, res)) > 0 {
		for _, it := range items(t, res) {
			if it["id"] == offered {
				t.Error("another contractor reads the notification")
			}
		}
	}
}

// IR602: HQ withdraws an offer by cancelling the job before the company accepted it. The company keeps the notice — the
// offer was its own — but the job page no longer opens for it (jobs.get NOT_FOUND), so the notice says targetOpen
// false and the apps show it without a link.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestWithdrawnOfferNoticeHasNoLink(t *testing.T) {
	srv := server(t)
	stamp := func(h int) string { return clock().Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	_, resp := write(
		srv,
		customerA(),
		"jobs.create",
		jobBody(seed.ID("unit-non-rto").String(), nil),
		0,
	)

	job := asStr(t, data(resp)["id"])
	if code, res := write(
		srv,
		hq(),
		"jobs.offer",
		`{"jobId":"`+job+`","contractorOrgId":"`+seed.ID("org-contractor-a").
			String()+
			`","offerExpiresAt":"`+stamp(
			12,
		)+`","accessValidFrom":"`+stamp(
			0,
		)+`","accessValidUntil":"`+stamp(
			100,
		)+`","termsVersion":"t"}`,
		1,
	); code != 200 {
		t.Fatalf("offer: %d %v", code, res)
	}

	notice := func() map[string]any {
		t.Helper()
		drainEvents(t)

		code, res := post(srv, contrA(), "notifications.list", `{"limit":100}`)
		if code != 200 {
			t.Fatalf("contractor inbox: %d %v", code, res)
		}

		var last map[string]any
		for _, it := range items(t, res) { // newest first: the first notice of the job is its latest
			if tg, _ := it["target"].(map[string]any); tg["kind"] == "job" && tg["id"] == job &&
				last == nil {
				last = it
			}
		}

		return last
	}
	if n := notice(); n == nil || n["targetOpen"] != true {
		t.Fatalf("the open offer's notice links the offer: %v", n)
	}
	// the notice names the job, never the customer's unit (IR775)
	if params, _ := notice()["params"].(map[string]any); params["targetName"] != "Job "+job[:8] {
		t.Errorf("the contractor's notice names %v, want Job %s", params["targetName"], job[:8])
	}

	if code, res := write(
		srv,
		hq(),
		"jobs.cancel",
		`{"jobId":"`+job+`","cancelReason":"the customer withdrew"}`,
		2,
	); code != 200 {
		t.Fatalf("cancel: %d %v", code, res)
	}

	kept := notice()
	if kept == nil {
		t.Fatal("the company lost the notice of its withdrawn offer")
	}

	if kept["targetOpen"] != false {
		t.Errorf("the withdrawn offer's notice still links a page that does not open: %v", kept)
	}

	if code, _ := post(srv, contrA(), "jobs.get", `{"jobId":"`+job+`"}`); code != 404 {
		t.Errorf("the withdrawn offer's job opens for the company: %d", code)
	}
}

// TestExternalTechnicianUnitNotificationScope: an external technician's notice about a unit is listed only while an
// assignment of a job at that unit lets them in — active and inside its work window — as for the unit's other data
// (IR774); an internal technician reads it by scopes.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestExternalTechnicianUnitNotificationScope(t *testing.T) {
	srv := server(t)
	tenant, tech, unit := seed.ID("tenant-a"), seed.ID("tech-external-a"), seed.ID("unit-online-rto")
	nid, jid, aid := uuid.NewString(), uuid.NewString(), uuid.NewString()

	owner(
		t,
		`INSERT INTO notify.notifications (id, tenant_id, recipient_membership_id, scope_version_at_creation, type,
			template_key, target, params, severity, occurred_at)
		VALUES ($1,$2,$3,1,'schedule_change','schedule_change',$4,'{}','normal',$5)`,
		nid,
		tenant,
		tech,
		`{"kind":"unit","id":"`+unit.String()+`"}`,
		clock(),
	)
	// the seed's own assignments of this technician are put aside, so that only the one below counts
	owner(
		t,
		`UPDATE notify.ref_assignments SET status = 'aside' WHERE technician_membership_id = $1 AND status = 'active'`,
		tech,
	)
	owner(t, `INSERT INTO notify.ref_jobs (id, tenant_id, unit_id) VALUES ($1,$2,$3)`, jid, tenant, unit)
	owner(
		t,
		`INSERT INTO notify.ref_assignments (id, tenant_id, job_id, technician_membership_id, status, scheduled)
		VALUES ($1,$2,$3,$4,'active',tstzrange($5,$6))`,
		aid,
		tenant,
		jid,
		tech,
		clock().Add(-time.Hour),
		clock().Add(time.Hour),
	)
	t.Cleanup(func() {
		owner(t, `DELETE FROM notify.notifications WHERE id = $1`, nid)
		owner(t, `DELETE FROM notify.ref_assignments WHERE id = $1`, aid)
		owner(t, `DELETE FROM notify.ref_jobs WHERE id = $1`, jid)
		owner(
			t,
			`UPDATE notify.ref_assignments SET status = 'active' WHERE technician_membership_id = $1 AND status = 'aside'`,
			tech,
		)
	})

	listed := func() bool {
		_, res := post(
			srv,
			techA(),
			"notifications.list",
			`{"filters":{"type":"schedule_change"},"limit":100}`,
		)
		for _, it := range items(t, res) {
			if it["id"] == nid {
				return true
			}
		}

		return false
	}
	if !listed() {
		t.Fatal("assignment inside its window: the unit notice is not listed")
	}

	owner(t, `UPDATE notify.ref_assignments SET status = 'revoked' WHERE id = $1`, aid)

	if listed() {
		t.Error("revoked assignment: the unit notice is still listed")
	}

	owner(
		t,
		`UPDATE notify.ref_assignments SET status = 'active', scheduled = tstzrange($2,$3) WHERE id = $1`,
		aid,
		clock().Add(-3*time.Hour),
		clock().Add(-time.Hour),
	)

	if listed() {
		t.Error("assignment window over: the unit notice is still listed")
	}
}
