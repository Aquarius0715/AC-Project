package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/seed"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

// IR190: in the split services the read models of equipment-api ask maintenance, billing, energy and identity over
// the internal query endpoint; the answers equal the single-process composition.
func TestReadModelsAcrossServices(t *testing.T) {
	all := server(t)
	urls := map[string]string{}
	for _, d := range []string{ops.DomainIdentity, ops.DomainMaintenance, ops.DomainBilling, ops.DomainEnergy} {
		s := serverCfg(t, func(c *apiserver.Config) { c.Domains, c.InternalToken = []string{d}, "test-internal" })
		hs := httptest.NewServer(s.Echo)
		t.Cleanup(hs.Close)
		urls[d] = hs.URL
	}
	equipment := serverCfg(t, func(c *apiserver.Config) {
		c.Domains, c.InternalToken, c.ServiceURLs = []string{ops.DomainEquipment}, "test-internal", urls
	})
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
	identitySrv := serverCfg(t, func(c *apiserver.Config) { c.Domains, c.InternalToken = []string{ops.DomainIdentity}, "test-internal" })
	hs := httptest.NewServer(identitySrv.Echo)
	t.Cleanup(hs.Close)
	billingSrv := serverCfg(t, func(c *apiserver.Config) {
		c.Domains, c.InternalToken, c.ServiceURLs = []string{ops.DomainBilling}, "test-internal", map[string]string{ops.DomainIdentity: hs.URL}
	})
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
