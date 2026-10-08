package integration

import (
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// newInvoice creates a customer-b contract (rto, restriction eligible) and an unpaid invoice; returns contract and invoice IDs.
func newInvoice(t *testing.T, s *apiserver.Server) (string, string) {
	t.Helper()
	u := newUnit(t, s, "Pay AC")
	ts := func(h int) string { return clock.Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	code, m := write(s, &hq, "contracts.save", `{"customerId":"`+seed.ID("cust-b").String()+`","unitIds":["`+u+`"],"planType":"rto","startAt":"`+ts(-24*30)+`","endAt":"`+ts(24*365)+
		`","priceMinor":5000,"currency":"MYR","restrictionEligible":true,"rulesVersion":"rules-1"}`, 0)
	if code != 200 {
		t.Fatalf("contract: %d %v", code, m)
	}
	k := data(m)["id"].(string)
	code, m = write(s, &hq, "invoices.create", `{"amountMinor":5000,"currency":"MYR","contractId":"`+k+`","contractVersion":1,"period":{"from":"`+ts(-24*14)+`","to":"`+ts(24*16)+`"},"dueAt":"`+ts(48)+`"}`, 0)
	if code != 200 {
		t.Fatalf("invoice: %d %v", code, m)
	}
	return k, data(m)["id"].(string)
}

func TestPaymentSimulation(t *testing.T) {
	s := server(t)
	_, inv := newInvoice(t, s)
	sim := func(body string, v int) (int, map[string]any) {
		return write(s, &customerB, "payments.simulate", body, v)
	}
	for name, b := range map[string]string{
		"bad method":    `{"event":"initiate","invoiceId":"` + inv + `","method":"cash","demoConfirmed":true}`,
		"not confirmed": `{"event":"initiate","invoiceId":"` + inv + `","method":"demo_credit_card","demoConfirmed":false}`,
		"bad event":     `{"event":"refund","invoiceId":"` + inv + `","demoConfirmed":true}`,
	} {
		if code, _ := sim(b, 1); code != 422 {
			t.Errorf("simulate %s: %d", name, code)
		}
	}
	if code, m := sim(`{"event":"instructions","invoiceId":"`+inv+`","demoConfirmed":true}`, 1); code != 200 || data(m)["deliveryState"] != "preview" {
		t.Fatalf("instructions: %d %v", code, m)
	}
	if code, _ := write(s, &customerA, "payments.simulate", `{"event":"initiate","invoiceId":"`+inv+`","method":"demo_credit_card","demoConfirmed":true}`, 1); code != 404 {
		t.Error("other customer pays")
	}
	code, m := sim(`{"event":"initiate","invoiceId":"`+inv+`","method":"demo_debit_card","demoConfirmed":true}`, 1)
	if code != 200 || data(m)["status"] != "initiated" || data(m)["method"] != "demo_debit_card" {
		t.Fatalf("initiate: %d %v", code, m)
	}
	pay := data(m)["id"].(string)
	ev := func(event string) string {
		return `,"eventId":"` + uuid.NewString() + `","paymentReference":"REF-` + pay[:8] + `"`
	}
	if code, _ := sim(`{"event":"confirm","paymentId":"`+pay+`"`+ev("confirm")+`}`, 1); code != 409 {
		t.Error("confirm before processing")
	}
	e1 := uuid.NewString()
	code, m = sim(`{"event":"processing","paymentId":"`+pay+`","eventId":"`+e1+`","paymentReference":"REF-`+pay[:8]+`"}`, 1)
	if code != 200 || data(m)["status"] != "processing" {
		t.Fatalf("processing: %d %v", code, m)
	}
	if code, m := sim(`{"event":"processing","paymentId":"`+pay+`","eventId":"`+e1+`","paymentReference":"REF-`+pay[:8]+`"}`, 2); code != 200 || data(m)["version"].(float64) != 2 {
		t.Fatalf("same event replay: %d %v", code, m)
	}
	if _, m := post(s, &hq, "invoices.get", `{"id":"`+inv+`"}`); data(m)["status"] != "processing" || data(m)["paymentMethod"] != "demo_debit_card" {
		t.Fatalf("invoice processing: %v", m)
	}
	if code, m := sim(`{"event":"fail","paymentId":"`+pay+`"`+ev("fail")+`}`, 2); code != 200 || data(m)["status"] != "failed" {
		t.Fatalf("fail: %d %v", code, m)
	}
	if code, _ := sim(`{"event":"confirm","paymentId":"`+pay+`"`+ev("confirm")+`}`, 3); code != 409 {
		t.Error("confirm after failure")
	}
	if _, m := post(s, &hq, "invoices.get", `{"id":"`+inv+`"}`); data(m)["status"] != "unpaid" || data(m)["paymentStatus"] != "failed" {
		t.Fatalf("invoice after failure: %v", m)
	}
	// second attempt confirms; the invoice becomes paid
	_, m = post(s, &hq, "invoices.get", `{"id":"`+inv+`"}`)
	code, m = sim(`{"event":"initiate","invoiceId":"`+inv+`","method":"demo_credit_card","demoConfirmed":true}`, ver(m))
	if code != 200 {
		t.Fatalf("second initiate: %d %v", code, m)
	}
	pay = data(m)["id"].(string)
	sim(`{"event":"processing","paymentId":"`+pay+`"`+ev("processing")+`}`, 1)
	if code, m := sim(`{"event":"confirm","paymentId":"`+pay+`"`+ev("confirm")+`}`, 2); code != 200 || data(m)["status"] != "confirmed" || data(m)["paymentReference"] == nil {
		t.Fatalf("confirm: %d %v", code, m)
	}
	if code, _ := sim(`{"event":"fail","paymentId":"`+pay+`"`+ev("fail")+`}`, 3); code != 409 {
		t.Error("fail after confirmed")
	}
	if _, m := post(s, &customerB, "invoices.get", `{"id":"`+inv+`"}`); data(m)["status"] != "paid" || len(data(m)["paymentRefs"].([]any)) != 2 {
		t.Fatalf("paid: %v", m)
	}
	_, m = post(s, &hq, "invoices.get", `{"id":"`+inv+`"}`)
	if code, _ := sim(`{"event":"initiate","invoiceId":"`+inv+`","method":"demo_credit_card","demoConfirmed":true}`, ver(m)); code != 409 {
		t.Error("pay a paid invoice")
	}
}

func TestHQPaymentsAndRelease(t *testing.T) {
	s := server(t)
	k, inv := newInvoice(t, s)
	// a restriction caused by this invoice
	rid := uuid.NewString()
	owner(t, `INSERT INTO restrictions.restrictions (id, tenant_id, contract_id, contract_version, customer_id, rules_version, notice_at, execute_after, reason, policy, state)
		VALUES ($1,$2,$3,1,$4,'rules-1',$5,$6,'unpaid','{"kind":"power_off"}','applied')`, rid, seed.ID("tenant-a"), k, seed.ID("cust-b"), clock, clock.Add(25*time.Hour))
	owner(t, `INSERT INTO restrictions.restriction_invoices (tenant_id, restriction_id, invoice_id) VALUES ($1,$2,$3)`, seed.ID("tenant-a"), rid, inv)
	var unit string
	ownerScan(t, `SELECT unit_id::text FROM billing.contract_units WHERE contract_id = $1`, []any{k}, &unit)
	owner(t, `INSERT INTO restrictions.restriction_units (tenant_id, restriction_id, unit_id, apply_state, release_state) VALUES ($1,$2,$3,'applied','none')`, seed.ID("tenant-a"), rid, unit)
	ref := "BANK-" + uuid.NewString()[:8]
	manual := func(amount string, v int) (int, map[string]any) {
		return write(s, &hq, "payments.recordManual", `{"invoiceId":"`+inv+`","paymentReference":"`+ref+`","confirmedAmountMinor":`+amount+`,"currency":"MYR","reason":"bank transfer seen"}`, v)
	}
	if code, _ := manual("4000", 1); code != 422 {
		t.Error("partial amount")
	}
	if code, _ := write(s, &hq, "payments.recordManual", `{"invoiceId":"`+inv+`","paymentReference":" ","confirmedAmountMinor":5000,"currency":"MYR","reason":"x"}`, 1); code != 422 {
		t.Error("blank reference")
	}
	if code, _ := write(s, &customerB, "payments.recordManual", `{"invoiceId":"`+inv+`","paymentReference":"X","confirmedAmountMinor":5000,"currency":"MYR","reason":"x"}`, 1); code != 403 {
		t.Error("client records payment")
	}
	code, m := manual("5000", 1)
	if code != 200 || data(m)["status"] != "confirmed" || data(m)["method"] != nil {
		t.Fatalf("manual: %d %v", code, m)
	}
	if code, m2 := manual("5000", 1); code != 200 || data(m2)["id"] != data(m)["id"] {
		t.Errorf("repeat manual returns the same payment: %d", code)
	}
	// HQ sees the confirmation reason (DD-A08); the client does not
	reason := func(who *actor) any {
		_, d := post(s, who, "invoices.get", `{"id":"`+inv+`"}`)
		return data(d)["paymentRefs"].([]any)[0].(map[string]any)["confirmationReason"]
	}
	if r := reason(&hq); r != "bank transfer seen" {
		t.Errorf("hq confirmation reason %v", r)
	}
	if r := reason(&customerB); r != nil {
		t.Errorf("client sees the confirmation reason %v", r)
	}
	var state string
	var pending *string
	ownerScan(t, `SELECT r.state, ru.pending_reason FROM restrictions.restrictions r JOIN restrictions.restriction_units ru ON ru.restriction_id = r.id WHERE r.id = $1`, []any{rid}, &state, &pending)
	if state != "release_requested" || pending == nil || *pending != "offline" {
		t.Errorf("restriction after payment: %s %v", state, pending)
	}
	owner(t, `UPDATE restrictions.restrictions SET state = 'released' WHERE id = $1`, rid)
	// the reference cannot be reused on another invoice; confirm needs a processing payment
	_, inv2 := newInvoice(t, s)
	if code, _ := write(s, &hq, "payments.recordManual", `{"invoiceId":"`+inv2+`","paymentReference":"`+ref+`","confirmedAmountMinor":5000,"currency":"MYR","reason":"dup"}`, 1); code != 409 {
		t.Error("reference reuse")
	}
	code, m = write(s, &customerB, "payments.simulate", `{"event":"initiate","invoiceId":"`+inv2+`","method":"demo_credit_card","demoConfirmed":true}`, 1)
	pay := data(m)["id"].(string)
	confirm := func(v int) (int, map[string]any) {
		return write(s, &hq, "payments.confirm", `{"paymentId":"`+pay+`","paymentReference":"HQ-`+pay[:8]+`","confirmedAmountMinor":5000,"currency":"MYR","reason":"checked"}`, v)
	}
	if code, _ := confirm(1); code != 409 {
		t.Error("confirm an initiated payment")
	}
	write(s, &customerB, "payments.simulate", `{"event":"processing","paymentId":"`+pay+`","eventId":"`+uuid.NewString()+`","paymentReference":"P-`+pay[:8]+`"}`, 1)
	if code, _ := write(s, &hq, "payments.recordManual", `{"invoiceId":"`+inv2+`","paymentReference":"M-`+pay[:8]+`","confirmedAmountMinor":5000,"currency":"MYR","reason":"x"}`, 3); code != 409 {
		t.Error("manual on an invoice with a payment")
	}
	if code, m := confirm(2); code != 200 || data(m)["status"] != "confirmed" || data(m)["method"] != "demo_credit_card" {
		t.Fatalf("hq confirm: %d %v", code, m)
	}
	if code, _ := confirm(3); code != 409 {
		t.Error("confirm twice")
	}
}
