package integration_test

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

// IR190: in the split services the read models of equipment-api ask maintenance, billing, energy and identity over
// the internal query endpoint; the answers equal the single-process composition.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestReadModelsAcrossServices(t *testing.T) {
	all := server(t)
	cl := newCluster(t)
	urls := cl.urls
	equipment := cl.srv[ops.DomainEquipment]
	from := clock().Add(-24 * time.Hour).Truncate(time.Minute)

	cases := []struct {
		who  *actor
		op   string
		body string
	}{
		{
			hq(),
			"admin.summary",
			`{"from":"` + from.Format(
				time.RFC3339,
			) + `","to":"` + clock().Truncate(time.Minute).
				Format(time.RFC3339) +
				`"}`,
		},
		{contrA(), opSummariesGet, `{"kind":"partner","filters":{}}`},
		{techA(), opSummariesGet, `{"kind":"technician","filters":{}}`},
		{customerA(), opSummariesGet, `{"kind":"customer","filters":{}}`},
	}
	for _, testCase := range cases {
		code1, want := post(all, testCase.who, testCase.op, testCase.body)
		code2, got := post(equipment, testCase.who, testCase.op, testCase.body)
		wantJSON := jsonBytes(t, want["data"])

		gotJSON := jsonBytes(t, got["data"])
		if code1 != 200 || code2 != 200 || !bytes.Equal(wantJSON, gotJSON) {
			t.Fatalf(
				"%s %s: single %d %s\nsplit %d %s",
				testCase.op,
				testCase.who.membership,
				code1,
				wantJSON,
				code2,
				gotJSON,
			)
		}
	}
	// the internal endpoint needs the internal token, and serves only the service's own domain
	ask := func(base, name, token string) int {
		req, _ := http.NewRequestWithContext(
			t.Context(),
			http.MethodPost,
			base+"/internal/v1/queries/"+name,
			strings.NewReader(`{"unitIds":[]}`),
		)
		req.Header.Set("Authorization", "Bearer "+hq().token)
		req.Header.Set("X-Tenant-Id", seed.ID("tenant-a").String())
		req.Header.Set("X-Membership-Id", seed.ID(hq().membership).String())

		if token != "" {
			req.Header.Set(ops.InternalTokenHeader, token)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}

		_ = resp.Body.Close()

		return resp.StatusCode
	}
	if code := ask(urls[ops.DomainBilling], "billing.overdue", ""); code != 401 {
		t.Fatalf("without the internal token: %d", code)
	}

	if code := ask(urls[ops.DomainBilling], "billing.overdue", "test-internal"); code != 200 {
		t.Fatalf("billing.overdue: %d", code)
	}

	if code := ask(urls[ops.DomainBilling], "energy.actuals", "test-internal"); code != 404 {
		t.Fatalf("another domain's query: %d", code)
	}
}

// IR191: billing-api alone notifies through identity — recipients come from identity.members over HTTP and the
// notification is stored by identity-api's consumer from NotificationRequested.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestNotificationsAcrossServices(t *testing.T) {
	s := server(t)
	u := boundUnit(t, s, "online")
	_, inv := overdueContract(t, s, u)
	cl := newCluster(t)
	identitySrv, billingSrv := cl.srv[ops.DomainIdentity], cl.srv[ops.DomainBilling]

	code, res := write(
		billingSrv,
		customerB(),
		"inquiries.create",
		`{"subjectType":"payment","invoiceId":"`+inv+`","message":"split services"}`,
		0,
	)
	if code != 200 {
		t.Fatalf("create on billing-api: %d %v", code, res)
	}

	inquiryID := asStr(t, data(res)["id"])

	count := func() int {
		var total int

		ownerScan(
			t,
			`SELECT count(*) FROM notify.notifications WHERE target->>'id' = $1 AND recipient_membership_id = $2`,
			[]any{inquiryID, seed.ID("hq-operator")},
			&total,
		)

		return total
	}
	if count() != 0 {
		t.Fatal("billing-api must not store notifications itself")
	}

	err := identitySrv.DrainEvents(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	if count() != 1 {
		t.Fatal("identity-api stores the requested notification")
	}
}

// IR192: maintenance-api alone checks technicians through identity.technician over HTTP, and a verified certificate
// reaches identity's qualification grants through QualificationGranted.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestDirectoryAcrossServices(t *testing.T) {
	_ = server(t)
	cl := newCluster(t)
	identitySrv, maint := cl.srv[ops.DomainIdentity], cl.srv[ops.DomainMaintenance]
	tech := seed.ID("tech-external-a").String()
	until := time.Date(2027, 3, 31, 0, 0, 0, 0, time.UTC)
	body := `{"membershipId":"` + tech + `","code":"demo_electrical","name":"Electrical","number":"E-split",` +
		`"issuedAt":"` + clock().Add(-24*time.Hour).
		Format(time.RFC3339) +
		`","expiresAt":"` + until.Format(
		time.RFC3339,
	) + `","file":` + blobJSON(
		"e.pdf",
		"application/pdf",
		[]byte("%PDF-1.4 split"),
		14,
	) + `}`

	code, res := write(maint, contrA(), "certificates.submit", body, 0)
	if code != 200 {
		t.Fatalf("submit on maintenance-api: %d %v", code, res)
	}

	if code, m := write(maint, contrB(), "certificates.submit", body, 0); code != 404 {
		t.Fatalf("another contractor's technician (identity.technician over HTTP): %d %v", code, m)
	}

	cert := asStr(t, data(res)["id"])
	if code, resp := write(
		maint,
		hq(),
		"certificates.verify",
		`{"certificateId":"`+cert+`","decision":"approve"}`,
		1,
	); code != 200 {
		t.Fatalf("verify on maintenance-api: %d %v", code, resp)
	}

	err := identitySrv.DrainEvents(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	var got time.Time
	ownerScan(
		t,
		`SELECT valid_until FROM identity.qualification_grants WHERE membership_id = $1 AND code = 'demo_electrical'`,
		[]any{tech},
		&got,
	)

	if !got.Equal(until) {
		t.Fatalf("grant valid until %v, want %v", got, until)
	}
}

// IR195: workers ask other domains without a user through the system route: internal token and tenant only.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestSystemQueries(t *testing.T) {
	cluster := newCluster(t)
	unit := seed.ID("unit-online-rto").String()
	ask := func(token, tenant string) int {
		req, _ := http.NewRequestWithContext(
			t.Context(),
			http.MethodPost,
			cluster.urls[ops.DomainBilling]+"/internal/v1/system/queries/restrictions.unitPolicy",
			strings.NewReader(`{"unitId":"`+unit+`"}`),
		)
		if token != "" {
			req.Header.Set(ops.InternalTokenHeader, token)
		}

		if tenant != "" {
			req.Header.Set("X-Tenant-Id", tenant)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}

		_ = resp.Body.Close()

		return resp.StatusCode
	}

	tenant := seed.ID("tenant-a").String()
	if code := ask("", tenant); code != 401 {
		t.Fatalf("without the internal token: %d", code)
	}

	if code := ask("test-internal", ""); code != 401 {
		t.Fatalf("without a tenant: %d", code)
	}

	if code := ask("wrong", tenant); code != 401 {
		t.Fatalf("wrong token: %d", code)
	}

	if code := ask("test-internal", tenant); code != 200 {
		t.Fatalf("system query: %d", code)
	}
}
