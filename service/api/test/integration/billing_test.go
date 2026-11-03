package integration

import (
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

func TestContractsAndInvoices(t *testing.T) {
	srv := server(t)
	unit := newUnit(t, srv, "Billing AC")
	cust := seed.ID("cust-b").String()
	stamp := func(h int) string { return clock.Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	body := func(extra map[string]string) string {
		f := map[string]string{
			"customerId":          `"` + cust + `"`,
			"unitIds":             `["` + unit + `"]`,
			"planType":            `"rto"`,
			"startAt":             `"` + stamp(-24*30) + `"`,
			"endAt":               `"` + stamp(24*365) + `"`,
			"priceMinor":          "12000",
			"currency":            `"MYR"`,
			"restrictionEligible": "true",
			"rulesVersion":        `"rules-demo-1"`,
		}
		maps.Copy(f, extra)

		parts := []string{}
		for k, v := range f {
			parts = append(parts, `"`+k+`":`+v)
		}

		return "{" + strings.Join(parts, ",") + "}"
	}
	// the field each case is refused for
	want := map[string]string{
		"bad currency":        "currency=error.invalid",
		"duplicate unit":      "unitIds=error.duplicate",
		"general restricted":  "restrictionEligible=errors.rto_only",
		"negative price":      "priceMinor=error.range",
		"no units":            "unitIds=error.count",
		"other customer unit": "unitIds=error.otherCustomerUnit",
		"reversed term":       "endAt=error.range",
		"rto without rules":   "rulesVersion=error.required",
		"rules not eligible":  "rulesVersion=error.notAllowed",
	}

	for name, e := range map[string]map[string]string{
		"general restricted":  {"planType": `"general"`},
		"rto without rules":   {"rulesVersion": "null"},
		"rules not eligible":  {"restrictionEligible": "false"},
		"reversed term":       {"startAt": `"` + stamp(10) + `"`, "endAt": `"` + stamp(1) + `"`},
		"negative price":      {"priceMinor": "-1"},
		"bad currency":        {"currency": `"EUR"`},
		"no units":            {"unitIds": "[]"},
		"other customer unit": {"unitIds": `["` + seed.ID("unit-non-rto").String() + `"]`},
		"duplicate unit":      {"unitIds": `["` + unit + `","` + unit + `"]`},
	} {
		code, m := write(srv, &hq, "contracts.save", body(e), 0)
		refusal(t, name, code, m, want[name])
	}

	if code, _ := write(
		srv,
		&hq,
		"contracts.save",
		body(map[string]string{"customerId": `"` + uuid.NewString() + `"`}),
		0,
	); code != 404 {
		t.Error("unknown customer")
	}

	code, resp := write(srv, &hq, "contracts.save", body(nil), 0)
	if code != 200 || data(resp)["version"].(float64) != 1 ||
		len(data(resp)["unitIds"].([]any)) != 1 ||
		data(resp)["customerOrgId"] != seed.ID("org-customer-b").String() {
		t.Fatalf("create: %d %v", code, resp)
	}

	key := data(resp)["id"].(string)
	if code, res := write(
		srv,
		&hq,
		"contracts.save",
		body(map[string]string{"id": `"` + key + `"`, "priceMinor": "15000"}),
		1,
	); code != 200 ||
		data(res)["version"].(float64) != 2 {
		t.Fatalf("revise: %d %v", code, res)
	}

	day := `"from":"` + clock.Add(-time.Hour).
		Format(time.RFC3339) +
		`","to":"` + clock.Add(time.Hour).
		Format(time.RFC3339) +
		`"`
	if _, am := post(
		srv,
		&hq,
		"audit.list",
		`{"filters":{`+day+`,"targetKind":"contract","targetId":"`+key+`"},"limit":10}`,
	); !hasChange(
		items(am),
		2,
		"priceMinor",
		"12000",
		"15000",
	) {
		t.Errorf("contract audit before/after: %v", am)
	}

	if code, _ := write(
		srv,
		&hq,
		"contracts.save",
		body(map[string]string{"id": `"` + key + `"`}),
		1,
	); code != 409 {
		t.Error("stale contract version")
	}

	if code, res := write(
		srv,
		&hq,
		"contracts.save",
		body(
			map[string]string{
				"id":         `"` + key + `"`,
				"customerId": `"` + seed.ID("cust-a").String() + `"`,
				"unitIds":    `["` + seed.ID("unit-non-rto").String() + `"]`,
			},
		),
		2,
	); code != 422 ||
		fields(res) != "customerId=error.customerFixed" {
		t.Errorf("customer fixed: %d %s", code, fields(res))
	}
	// lists
	if _, res := post(
		srv,
		&customerB,
		"contracts.list",
		`{"filters":{"unitId":"`+unit+`"}}`,
	); len(items(res)) != 1 ||
		items(res)[0]["priceMinor"].(float64) != 15000 {
		t.Fatalf("client contracts: %v", res)
	}

	if code, res := post(
		srv,
		&customerA,
		"contracts.list",
		`{"filters":{"unitId":"`+unit+`"}}`,
	); code != 200 ||
		len(items(res)) != 0 {
		t.Error("other customer's contract")
	}

	if code, res := post(
		srv,
		&hq,
		"contracts.list",
		`{"filters":{"kind":"vip"}}`,
	); code != 422 ||
		fields(res) != "filters=error.invalid" {
		t.Errorf("bad plan filter: %d %s", code, fields(res))
	}
	// a restriction on the contract blocks revisions
	rid := uuid.NewString()
	owner(
		t,
		`INSERT INTO restrictions.restrictions (id, tenant_id, contract_id, contract_version, customer_id, rules_version,
			notice_at, execute_after, reason, policy, state)
		VALUES ($1,$2,$3,2,$4,'r',$5,$6,'demo','{"kind":"power_off"}','scheduled')`,
		rid,
		seed.ID("tenant-a"),
		key,
		cust,
		clock,
		clock.Add(25*time.Hour),
	)

	if _, res := post(
		srv,
		&hq,
		"contracts.list",
		`{"filters":{"customerId":"`+cust+`","unitId":"`+unit+`"}}`,
	); len(
		items(res)[0]["activeRestrictionIds"].([]any),
	) != 1 {
		t.Fatal("active restriction listed")
	}

	if code, _ := write(
		srv,
		&hq,
		"contracts.save",
		body(map[string]string{"id": `"` + key + `"`}),
		2,
	); code != 409 {
		t.Error("restricted contract revision")
	}

	owner(t, `UPDATE restrictions.restrictions SET state = 'cancelled' WHERE id = $1`, rid)

	// invoices
	inv := func(extra map[string]string) (int, map[string]any) {
		f := map[string]string{
			"amountMinor":     "15000",
			"currency":        `"MYR"`,
			"contractId":      `"` + key + `"`,
			"contractVersion": "2",
			"period": `{"from":"` + stamp(
				-24*14,
			) + `","to":"` + stamp(
				24*16,
			) + `"}`,
			"dueAt": `"` + stamp(48) + `"`,
		}
		maps.Copy(f, extra)

		parts := []string{}
		for kk, v := range f {
			parts = append(parts, `"`+kk+`":`+v)
		}

		return write(srv, &hq, "invoices.create", "{"+strings.Join(parts, ",")+"}", 0)
	}
	refused := map[string]string{ // the field each VALIDATION case is refused for
		"due in the past": "dueAt=error.past",
		"other currency":  "currency=errors.contract_currency",
		"outside term":    "period=errors.outside_contract",
		"zero amount":     "amountMinor=error.range",
	}

	for name, tc := range map[string]struct {
		e    map[string]string
		code int
	}{
		"old contract version": {map[string]string{"contractVersion": "1"}, 409},
		"other currency":       {map[string]string{"currency": `"USD"`}, 422},
		"due in the past":      {map[string]string{"dueAt": `"` + stamp(-1) + `"`}, 422},
		"zero amount":          {map[string]string{"amountMinor": "0"}, 422},
		"outside term": {
			map[string]string{"period": `{"from":"` + stamp(-24*60) + `","to":"` + stamp(24) + `"}`},
			422,
		},
	} {
		code, m := inv(tc.e)
		if w, ok := refused[name]; ok {
			refusal(t, name, code, m, w)
		} else if code != tc.code {
			t.Errorf("invoice %s: %d want %d", name, code, tc.code)
		}
	}

	code, resp = inv(nil)
	if code != 200 || data(resp)["status"] != "unpaid" ||
		!strings.HasPrefix(data(resp)["number"].(string), "INV-202608-") {
		t.Fatalf("invoice: %d %v", code, resp)
	}

	invoice := data(resp)["id"].(string)

	if code, _ := inv(nil); code != 409 {
		t.Error("same period twice")
	}
	// IR469: a period overlapping an invoiced one would bill those days twice; the next period may start where it ends
	overlap := `{"from":"` + stamp(24*10) + `","to":"` + stamp(24*40) + `"}`
	if code, m := inv(
		map[string]string{"period": overlap, "dueAt": `"` + stamp(24*11) + `"`},
	); code != 409 ||
		m["messageKey"] != "errors.invoice_period_exists" {
		t.Errorf("overlapping period: %d %v", code, m)
	}

	if code, res := post(
		srv,
		&customerB,
		"invoices.get",
		`{"id":"`+invoice+`"}`,
	); code != 200 ||
		len(data(res)["paymentRefs"].([]any)) != 0 {
		t.Fatalf("client invoice detail: %d %v", code, res)
	}

	if code, _ := post(srv, &customerA, "invoices.get", `{"id":"`+invoice+`"}`); code != 404 {
		t.Error("other customer's invoice")
	}

	if _, res := post(
		srv,
		&customerB,
		"invoices.list",
		`{"filters":{"contractId":"`+key+`","status":"unpaid"}}`,
	); len(
		items(res),
	) != 1 {
		t.Error("client invoice list")
	}

	if code, res := post(
		srv,
		&hq,
		"invoices.list",
		`{"filters":{"contractId":"`+key+`","overdueOnly":true}}`,
	); code != 200 ||
		len(items(res)) != 0 {
		t.Error("not yet overdue")
	}

	if code, res := post(
		srv,
		&hq,
		"invoices.list",
		`{"filters":{"status":"late"}}`,
	); code != 422 ||
		fields(res) != "filters=error.invalid" {
		t.Errorf("bad invoice status: %d %s", code, fields(res))
	}
	// scope filters: the property of the contract version's units, [from, to) on the period start
	for name, tc := range map[string]struct {
		f string
		n int
	}{
		"property":           {`"propertyId":"` + seed.ID("property-home-b").String() + `"`, 1},
		"other property":     {`"propertyId":"` + seed.ID("property-home-a").String() + `"`, 0},
		"period start in":    {`"from":"` + stamp(-24*15) + `","to":"` + stamp(-24*13) + `"`, 1},
		"period start at to": {`"to":"` + stamp(-24*14) + `"`, 0},
		"period start after": {`"from":"` + stamp(-24*13) + `"`, 0},
	} {
		if code, res := post(
			srv,
			&hq,
			"invoices.list",
			`{"filters":{"contractId":"`+key+`",`+tc.f+`}}`,
		); code != 200 ||
			len(items(res)) != tc.n {
			t.Errorf("invoice filter %s: %v", name, res)
		}
	}

	if code, res := post(
		srv,
		&hq,
		"invoices.list",
		`{"filters":{"from":"`+stamp(1)+`","to":"`+stamp(1)+`"}}`,
	); code != 422 ||
		fields(res) != "filters.to=error.range" {
		t.Errorf("empty invoice range: %d %s", code, fields(res))
	}
	// reminders: only overdue unpaid, active client recipient
	remind := func(recipient, channel string, v int) (int, map[string]any) {
		return write(
			srv,
			&hq,
			"invoices.remind",
			`{"invoiceId":"`+invoice+`","recipientMembershipId":"`+recipient+`","channel":"`+channel+`","reason":"overdue"}`,
			v,
		)
	}
	if code, _ := remind(seed.ID("customer-b").String(), "inApp", 1); code != 409 {
		t.Error("remind before due")
	}

	owner(
		t,
		`UPDATE billing.invoices SET due_at = $2 WHERE id = $1`,
		invoice,
		clock.Add(-time.Hour),
	)

	if code, res := remind(
		seed.ID("customer-a").String(),
		"inApp",
		1,
	); code != 422 ||
		fields(res) != "recipientMembershipId=errors.recipient_not_eligible" {
		t.Errorf("recipient of another customer: %d %s", code, fields(res))
	}

	if code, res := remind(
		seed.ID("customer-b").String(),
		"sms",
		1,
	); code != 422 ||
		fields(res) != "channel=error.invalid" {
		t.Errorf("bad channel: %d %s", code, fields(res))
	}

	code, resp = remind(seed.ID("customer-b").String(), "email", 1)
	if code != 200 || data(resp)["invoiceVersion"].(float64) != 2 ||
		data(resp)["notificationId"] == nil {
		t.Fatalf("remind: %d %v", code, resp)
	}

	var channel string
	ownerScan(
		t,
		`SELECT channel FROM notify.notifications WHERE id = $1`,
		[]any{data(resp)["notificationId"]},
		&channel,
	)

	if channel != "email" {
		t.Errorf("notification channel %s", channel)
	}

	if code, _ := remind(seed.ID("customer-b").String(), "inApp", 1); code != 409 {
		t.Error("stale invoice version")
	}

	if _, res := post(
		srv,
		&hq,
		"invoices.list",
		`{"filters":{"contractId":"`+key+`","overdueOnly":true}}`,
	); len(
		items(res),
	) != 1 {
		t.Error("overdue list")
	}
}

// hasChange reports whether an audit entry for nextVersion records field changing from before to after.
func hasChange(entries []map[string]any, nextVersion int, field, before, after string) bool {
	for _, a := range entries {
		b, _ := a["maskedBefore"].(map[string]any)

		f, _ := a["maskedAfter"].(map[string]any)
		if a["nextVersion"] == float64(nextVersion) && b[field] == before && f[field] == after {
			return true
		}
	}

	return false
}
