package app

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/core/seed"
)

func TestContractsAndInvoices(t *testing.T) {
	s := server(t)
	u := newUnit(t, s, "Billing AC")
	cust := seed.ID("cust-b").String()
	ts := func(h int) string { return clock.Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	body := func(extra map[string]string) string {
		f := map[string]string{"customerId": `"` + cust + `"`, "unitIds": `["` + u + `"]`, "planType": `"rto"`, "startAt": `"` + ts(-24*30) + `"`, "endAt": `"` + ts(24*365) + `"`,
			"priceMinor": "12000", "currency": `"MYR"`, "restrictionEligible": "true", "rulesVersion": `"rules-demo-1"`}
		for k, v := range extra {
			f[k] = v
		}
		parts := []string{}
		for k, v := range f {
			parts = append(parts, `"`+k+`":`+v)
		}
		return "{" + strings.Join(parts, ",") + "}"
	}
	for name, e := range map[string]map[string]string{
		"general restricted":  {"planType": `"general"`},
		"rto without rules":   {"rulesVersion": "null"},
		"rules not eligible":  {"restrictionEligible": "false"},
		"reversed term":       {"startAt": `"` + ts(10) + `"`, "endAt": `"` + ts(1) + `"`},
		"negative price":      {"priceMinor": "-1"},
		"bad currency":        {"currency": `"EUR"`},
		"no units":            {"unitIds": "[]"},
		"other customer unit": {"unitIds": `["` + seed.ID("unit-non-rto").String() + `"]`},
		"duplicate unit":      {"unitIds": `["` + u + `","` + u + `"]`},
	} {
		if code, _ := write(s, &hq, "contracts.save", body(e), 0); code != 422 {
			t.Errorf("contract %s: %d", name, code)
		}
	}
	if code, _ := write(s, &hq, "contracts.save", body(map[string]string{"customerId": `"` + uuid.NewString() + `"`}), 0); code != 404 {
		t.Error("unknown customer")
	}
	code, m := write(s, &hq, "contracts.save", body(nil), 0)
	if code != 200 || data(m)["version"].(float64) != 1 || len(data(m)["unitIds"].([]any)) != 1 || data(m)["customerOrgId"] != seed.ID("org-customer-b").String() {
		t.Fatalf("create: %d %v", code, m)
	}
	k := data(m)["id"].(string)
	if code, m := write(s, &hq, "contracts.save", body(map[string]string{"id": `"` + k + `"`, "priceMinor": "15000"}), 1); code != 200 || data(m)["version"].(float64) != 2 {
		t.Fatalf("revise: %d %v", code, m)
	}
	if code, _ := write(s, &hq, "contracts.save", body(map[string]string{"id": `"` + k + `"`}), 1); code != 409 {
		t.Error("stale contract version")
	}
	if code, _ := write(s, &hq, "contracts.save", body(map[string]string{"id": `"` + k + `"`, "customerId": `"` + seed.ID("cust-a").String() + `"`, "unitIds": `["` + seed.ID("unit-non-rto").String() + `"]`}), 2); code != 422 {
		t.Error("customer fixed")
	}
	// lists
	if _, m := post(s, &customerB, "contracts.list", `{"filters":{"unitId":"`+u+`"}}`); len(items(m)) != 1 || items(m)[0]["priceMinor"].(float64) != 15000 {
		t.Fatalf("client contracts: %v", m)
	}
	if _, m := post(s, &customerA, "contracts.list", `{"filters":{"unitId":"`+u+`"}}`); len(items(m)) != 0 {
		t.Error("other customer's contract")
	}
	if code, _ := post(s, &hq, "contracts.list", `{"filters":{"planType":"vip"}}`); code != 422 {
		t.Error("bad plan filter")
	}
	// a restriction on the contract blocks revisions
	rid := uuid.NewString()
	owner(t, `INSERT INTO restrictions.restrictions (id, tenant_id, contract_id, contract_version, customer_id, rules_version, notice_at, execute_after, reason, policy, state)
		VALUES ($1,$2,$3,2,$4,'r',$5,$6,'demo','{"kind":"power_off"}','scheduled')`, rid, seed.ID("tenant-a"), k, cust, clock, clock.Add(25*time.Hour))
	if _, m := post(s, &hq, "contracts.list", `{"filters":{"customerId":"`+cust+`","unitId":"`+u+`"}}`); len(items(m)[0]["activeRestrictionIds"].([]any)) != 1 {
		t.Fatal("active restriction listed")
	}
	if code, _ := write(s, &hq, "contracts.save", body(map[string]string{"id": `"` + k + `"`}), 2); code != 409 {
		t.Error("restricted contract revision")
	}
	owner(t, `UPDATE restrictions.restrictions SET state = 'cancelled' WHERE id = $1`, rid)

	// invoices
	inv := func(extra map[string]string) (int, map[string]any) {
		f := map[string]string{"amountMinor": "15000", "currency": `"MYR"`, "contractId": `"` + k + `"`, "contractVersion": "2",
			"period": `{"from":"` + ts(-24*14) + `","to":"` + ts(24*16) + `"}`, "dueAt": `"` + ts(48) + `"`}
		for kk, v := range extra {
			f[kk] = v
		}
		parts := []string{}
		for kk, v := range f {
			parts = append(parts, `"`+kk+`":`+v)
		}
		return write(s, &hq, "invoices.create", "{"+strings.Join(parts, ",")+"}", 0)
	}
	for name, tc := range map[string]struct {
		e    map[string]string
		code int
	}{
		"old contract version": {map[string]string{"contractVersion": "1"}, 409},
		"other currency":       {map[string]string{"currency": `"USD"`}, 422},
		"due in the past":      {map[string]string{"dueAt": `"` + ts(-1) + `"`}, 422},
		"zero amount":          {map[string]string{"amountMinor": "0"}, 422},
		"outside term":         {map[string]string{"period": `{"from":"` + ts(-24*60) + `","to":"` + ts(24) + `"}`}, 422},
	} {
		if code, _ := inv(tc.e); code != tc.code {
			t.Errorf("invoice %s: %d want %d", name, code, tc.code)
		}
	}
	code, m = inv(nil)
	if code != 200 || data(m)["status"] != "unpaid" || !strings.HasPrefix(data(m)["number"].(string), "INV-202608-") {
		t.Fatalf("invoice: %d %v", code, m)
	}
	invoice := data(m)["id"].(string)
	if code, _ := inv(nil); code != 409 {
		t.Error("same period twice")
	}
	if code, m := post(s, &customerB, "invoices.get", `{"id":"`+invoice+`"}`); code != 200 || len(data(m)["paymentRefs"].([]any)) != 0 {
		t.Fatalf("client invoice detail: %d %v", code, m)
	}
	if code, _ := post(s, &customerA, "invoices.get", `{"id":"`+invoice+`"}`); code != 404 {
		t.Error("other customer's invoice")
	}
	if _, m := post(s, &customerB, "invoices.list", `{"filters":{"contractId":"`+k+`","status":"unpaid"}}`); len(items(m)) != 1 {
		t.Error("client invoice list")
	}
	if _, m := post(s, &hq, "invoices.list", `{"filters":{"contractId":"`+k+`","overdueOnly":true}}`); len(items(m)) != 0 {
		t.Error("not yet overdue")
	}
	if code, _ := post(s, &hq, "invoices.list", `{"filters":{"status":"late"}}`); code != 422 {
		t.Error("bad invoice status")
	}
	// reminders: only overdue unpaid, active client recipient
	remind := func(recipient, channel string, v int) (int, map[string]any) {
		return write(s, &hq, "invoices.remind", `{"invoiceId":"`+invoice+`","recipientMembershipId":"`+recipient+`","channel":"`+channel+`","reason":"overdue"}`, v)
	}
	if code, _ := remind(seed.ID("customer-b").String(), "inApp", 1); code != 409 {
		t.Error("remind before due")
	}
	owner(t, `UPDATE billing.invoices SET due_at = $2 WHERE id = $1`, invoice, clock.Add(-time.Hour))
	if code, _ := remind(seed.ID("customer-a").String(), "inApp", 1); code != 422 {
		t.Error("recipient of another customer")
	}
	if code, _ := remind(seed.ID("customer-b").String(), "sms", 1); code != 422 {
		t.Error("bad channel")
	}
	code, m = remind(seed.ID("customer-b").String(), "email", 1)
	if code != 200 || data(m)["invoiceVersion"].(float64) != 2 || data(m)["notificationId"] == nil {
		t.Fatalf("remind: %d %v", code, m)
	}
	var channel string
	ownerScan(t, `SELECT channel FROM notify.notifications WHERE id = $1`, []any{data(m)["notificationId"]}, &channel)
	if channel != "email" {
		t.Errorf("notification channel %s", channel)
	}
	if code, _ := remind(seed.ID("customer-b").String(), "inApp", 1); code != 409 {
		t.Error("stale invoice version")
	}
	if _, m := post(s, &hq, "invoices.list", `{"filters":{"contractId":"`+k+`","overdueOnly":true}}`); len(items(m)) != 1 {
		t.Error("overdue list")
	}
}
