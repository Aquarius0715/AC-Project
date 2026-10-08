package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/seed"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

// splitCluster is one server per business domain, each serving only its domain and asking the others over HTTP
// (internal queries), like the compose services; principals come from the shared test verifier.
type splitCluster struct {
	srv  map[string]*apiserver.Server
	urls map[string]string
}

func newCluster(t *testing.T) splitCluster {
	t.Helper()
	cl := splitCluster{srv: map[string]*apiserver.Server{}, urls: map[string]string{}}
	for _, d := range ops.Domains {
		s := serverCfg(t, func(c *apiserver.Config) { c.Domains, c.InternalToken = []string{d}, "test-internal" })
		hs := httptest.NewServer(s.Echo)
		t.Cleanup(hs.Close)
		cl.srv[d], cl.urls[d] = s, hs.URL
	}
	for d, s := range cl.srv {
		s.Registry.Remote = &ops.HTTPQueries{Targets: cl.targets(d), Token: "test-internal"}
	}
	return cl
}

// targets are the other services' URLs as seen from domain d.
func (cl splitCluster) targets(d string) map[string]*url.URL {
	out := map[string]*url.URL{}
	for o, raw := range cl.urls {
		if o != d {
			out[o], _ = url.Parse(raw)
		}
	}
	return out
}

// others are the other services' URLs as configuration (Config.ServiceURLs).
func (cl splitCluster) others(d string) map[string]string {
	out := map[string]string{}
	for o, raw := range cl.urls {
		if o != d {
			out[o] = raw
		}
	}
	return out
}

// drain applies pending events in every service until nothing is left (chains cross services).
func (cl splitCluster) drain(t *testing.T) {
	t.Helper()
	for range 3 {
		for _, d := range ops.Domains {
			if err := cl.srv[d].DrainEvents(t.Context()); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// IR190: in the split services the read models of equipment-api ask maintenance, billing, energy and identity over
// the internal query endpoint; the answers equal the single-process composition.
func TestReadModelsAcrossServices(t *testing.T) {
	all := server(t)
	cl := newCluster(t)
	urls := cl.urls
	equipment := cl.srv[ops.DomainEquipment]
	from := clock.Add(-24 * time.Hour).Truncate(time.Minute)
	cases := []struct {
		who  *actor
		op   string
		body string
	}{
		{&hq, "admin.summary", `{"from":"` + from.Format(time.RFC3339) + `","to":"` + clock.Truncate(time.Minute).Format(time.RFC3339) + `"}`},
		{&contrA, "summaries.get", `{"kind":"partner","filters":{}}`},
		{&techA, "summaries.get", `{"kind":"technician","filters":{}}`},
		{&customerA, "summaries.get", `{"kind":"customer","filters":{}}`},
	}
	for _, tc := range cases {
		code1, want := post(all, tc.who, tc.op, tc.body)
		code2, got := post(equipment, tc.who, tc.op, tc.body)
		w, _ := json.Marshal(want["data"])
		g, _ := json.Marshal(got["data"])
		if code1 != 200 || code2 != 200 || string(w) != string(g) {
			t.Fatalf("%s %s: single %d %s\nsplit %d %s", tc.op, tc.who.membership, code1, w, code2, g)
		}
	}
	// the internal endpoint needs the internal token, and serves only the service's own domain
	ask := func(base, name, token string) int {
		req, _ := http.NewRequest(http.MethodPost, base+"/internal/v1/queries/"+name, strings.NewReader(`{"unitIds":[]}`))
		req.Header.Set("Authorization", "Bearer "+hq.token)
		req.Header.Set("X-Tenant-Id", seed.ID("tenant-a").String())
		req.Header.Set("X-Membership-Id", seed.ID(hq.membership).String())
		if token != "" {
			req.Header.Set(ops.InternalTokenHeader, token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
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
func TestNotificationsAcrossServices(t *testing.T) {
	s := server(t)
	u := boundUnit(t, s, "online")
	_, inv := overdueContract(t, s, u)
	cl := newCluster(t)
	identitySrv, billingSrv := cl.srv[ops.DomainIdentity], cl.srv[ops.DomainBilling]
	code, m := write(billingSrv, &customerB, "inquiries.create", `{"subjectType":"payment","invoiceId":"`+inv+`","message":"split services"}`, 0)
	if code != 200 {
		t.Fatalf("create on billing-api: %d %v", code, m)
	}
	id := data(m)["id"].(string)
	count := func() (n int) {
		ownerScan(t, `SELECT count(*) FROM notify.notifications WHERE target->>'id' = $1 AND recipient_membership_id = $2`, []any{id, seed.ID("hq-operator")}, &n)
		return n
	}
	if count() != 0 {
		t.Fatal("billing-api must not store notifications itself")
	}
	if err := identitySrv.DrainEvents(t.Context()); err != nil {
		t.Fatal(err)
	}
	if count() != 1 {
		t.Fatal("identity-api stores the requested notification")
	}
}

// IR192: maintenance-api alone checks technicians through identity.technician over HTTP, and a verified certificate
// reaches identity's qualification grants through QualificationGranted.
func TestDirectoryAcrossServices(t *testing.T) {
	_ = server(t)
	cl := newCluster(t)
	identitySrv, maint := cl.srv[ops.DomainIdentity], cl.srv[ops.DomainMaintenance]
	tech := seed.ID("tech-external-a").String()
	until := time.Date(2027, 3, 31, 0, 0, 0, 0, time.UTC)
	body := `{"membershipId":"` + tech + `","code":"demo_electrical","name":"Electrical","number":"E-split","issuedAt":"` + clock.Add(-24*time.Hour).Format(time.RFC3339) +
		`","expiresAt":"` + until.Format(time.RFC3339) + `","file":` + blobJSON("e.pdf", "application/pdf", []byte("%PDF-1.4 split"), 14) + `}`
	code, m := write(maint, &contrA, "certificates.submit", body, 0)
	if code != 200 {
		t.Fatalf("submit on maintenance-api: %d %v", code, m)
	}
	if code, m := write(maint, &contrB, "certificates.submit", body, 0); code != 404 {
		t.Fatalf("another contractor's technician (identity.technician over HTTP): %d %v", code, m)
	}
	cert := data(m)["id"].(string)
	if code, m := write(maint, &hq, "certificates.verify", `{"certificateId":"`+cert+`","decision":"approve"}`, 1); code != 200 {
		t.Fatalf("verify on maintenance-api: %d %v", code, m)
	}
	if err := identitySrv.DrainEvents(t.Context()); err != nil {
		t.Fatal(err)
	}
	var got time.Time
	ownerScan(t, `SELECT valid_until FROM identity.qualification_grants WHERE membership_id = $1 AND code = 'demo_electrical'`, []any{tech}, &got)
	if !got.Equal(until) {
		t.Fatalf("grant valid until %v, want %v", got, until)
	}
}
