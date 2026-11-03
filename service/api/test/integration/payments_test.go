package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

const statusConfirmed = "confirmed"

// newInvoice creates a customer-b contract (rto, restriction eligible) and an unpaid invoice; returns contract and
// invoice IDs.
func newInvoice(t *testing.T, srv *apiserver.Server) (string, string) {
	t.Helper()
	unit := newUnit(t, srv, "Pay AC")
	stamp := func(h int) string { return clock().Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }

	code, res := write(
		srv,
		hq(),
		"contracts.save",
		`{"customerId":"`+seed.ID("cust-b").
			String()+
			`","unitIds":["`+unit+`"],"planType":"rto","startAt":"`+stamp(
			-24*30,
		)+`","endAt":"`+stamp(
			24*365,
		)+
			`","priceMinor":5000,"currency":"MYR","restrictionEligible":true,"rulesVersion":"rules-1"}`,
		0,
	)
	if code != 200 {
		t.Fatalf("contract: %d %v", code, res)
	}

	key := asStr(t, data(res)["id"])

	code, res = write(
		srv,
		hq(),
		"invoices.create",
		`{"amountMinor":5000,"currency":"MYR","contractId":"`+key+`","contractVersion":1,"period":{"from":"`+stamp(
			-24*14,
		)+`","to":"`+stamp(
			24*16,
		)+`"},"dueAt":"`+stamp(
			48,
		)+`"}`,
		0,
	)
	if code != 200 {
		t.Fatalf("invoice: %d %v", code, res)
	}

	return key, asStr(t, data(res)["id"])
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestPaymentSimulation(t *testing.T) {
	srv := server(t)
	_, inv := newInvoice(t, srv)
	sim := func(body string, v int) (int, map[string]any) {
		return write(srv, customerB(), "payments.simulate", body, v)
	}
	want := map[string]string{ // the field each case is refused for
		"bad event":     "event=error.invalid",
		caseBadMethod:   refusedMethodInvalid,
		"not confirmed": refusedInvoiceIDRequired,
	}

	for name, b := range map[string]string{
		caseBadMethod:   `{"event":"initiate","invoiceId":"` + inv + `","method":"cash","demoConfirmed":true}`,
		"not confirmed": `{"event":"initiate","invoiceId":"` + inv + `","method":"demo_credit_card","demoConfirmed":false}`,
		"bad event":     `{"event":"refund","invoiceId":"` + inv + `","demoConfirmed":true}`,
	} {
		code, m := sim(b, 1)
		refusal(t, name, code, m, want[name])
	}

	if code, m := sim(
		`{"event":"instructions","invoiceId":"`+inv+`","demoConfirmed":true}`,
		1,
	); code != 200 ||
		data(m)["deliveryState"] != deliveryPreview {
		t.Fatalf("instructions: %d %v", code, m)
	}

	if code, _ := write(
		srv,
		customerA(),
		"payments.simulate",
		`{"event":"initiate","invoiceId":"`+inv+`","method":"demo_credit_card","demoConfirmed":true}`,
		1,
	); code != 404 {
		t.Error("other customer pays")
	}

	if code, m := sim(
		`{"event":"initiate","invoiceId":"`+inv+`","method":"demo_credit_card","demoConfirmed":true}`,
		9,
	); code != 409 ||
		m["messageKey"] != apperr.KeyVersionConflict {
		t.Errorf("initiate on a stale invoice version: %d %v", code, m)
	}

	code, resp := sim(
		`{"event":"initiate","invoiceId":"`+inv+`","method":"demo_debit_card","demoConfirmed":true}`,
		1,
	)
	if code != 200 || data(resp)["status"] != "initiated" ||
		data(resp)["method"] != "demo_debit_card" {
		t.Fatalf("initiate: %d %v", code, resp)
	}

	pay := asStr(t, data(resp)["id"])

	ev := func(_ string) string { // the event's name only labels the call
		return `,"eventId":"` + uuid.NewString() + `","paymentReference":"REF-` + pay[:8] + `"`
	}
	if code, _ := sim(
		`{"event":"confirm","paymentId":"`+pay+`"`+ev("confirm")+`}`,
		1,
	); code != 409 {
		t.Error("confirm before processing")
	}

	e1 := uuid.NewString()

	code, resp = sim(
		`{"event":"processing","paymentId":"`+pay+`","eventId":"`+e1+`","paymentReference":"REF-`+pay[:8]+`"}`,
		1,
	)
	if code != 200 || data(resp)["status"] != "processing" {
		t.Fatalf("processing: %d %v", code, resp)
	}

	if code, m := sim(
		`{"event":"processing","paymentId":"`+pay+`","eventId":"`+e1+`","paymentReference":"REF-`+pay[:8]+`"}`,
		2,
	); code != 200 ||
		asNum(t, data(m)["version"]) != 2 {
		t.Fatalf("same event replay: %d %v", code, m)
	}

	if code, m := sim(
		`{"event":"processing","paymentId":"`+pay+`"`+ev("processing")+`}`,
		2,
	); code != 409 ||
		m["messageKey"] != apperr.KeyInvalidState {
		t.Errorf("a second processing event: %d %v", code, m)
	}

	if code, m := sim(
		`{"event":"fail","paymentId":"`+pay+`"`+ev("fail")+`}`,
		1,
	); code != 409 ||
		m["messageKey"] != apperr.KeyVersionConflict {
		t.Errorf("an event on a stale payment version: %d %v", code, m)
	}

	if _, res := post(
		srv,
		hq(),
		"invoices.get",
		`{"id":"`+inv+`"}`,
	); data(res)["status"] != "processing" ||
		data(res)["paymentMethod"] != "demo_debit_card" {
		t.Fatalf("invoice processing: %v", res)
	}

	if code, m := sim(
		`{"event":"fail","paymentId":"`+pay+`"`+ev("fail")+`}`,
		2,
	); code != 200 ||
		data(m)["status"] != statusFailed {
		t.Fatalf("fail: %d %v", code, m)
	}

	if code, _ := sim(
		`{"event":"confirm","paymentId":"`+pay+`"`+ev("confirm")+`}`,
		3,
	); code != 409 {
		t.Error("confirm after failure")
	}

	if _, res := post(
		srv,
		hq(),
		"invoices.get",
		`{"id":"`+inv+`"}`,
	); data(res)["status"] != statusUnpaid ||
		data(res)["paymentStatus"] != statusFailed {
		t.Fatalf("invoice after failure: %v", res)
	}
	// second attempt confirms; the invoice becomes paid
	_, resp = post(srv, hq(), "invoices.get", `{"id":"`+inv+`"}`)

	code, resp = sim(
		`{"event":"initiate","invoiceId":"`+inv+`","method":"demo_credit_card","demoConfirmed":true}`,
		ver(t, resp),
	)
	if code != 200 {
		t.Fatalf("second initiate: %d %v", code, resp)
	}

	pay = asStr(t, data(resp)["id"])
	sim(`{"event":"processing","paymentId":"`+pay+`"`+ev("processing")+`}`, 1)

	if code, m := sim(
		`{"event":"confirm","paymentId":"`+pay+`"`+ev("confirm")+`}`,
		2,
	); code != 200 || data(m)["status"] != statusConfirmed ||
		data(m)["paymentReference"] == nil {
		t.Fatalf("confirm: %d %v", code, m)
	}

	if code, _ := sim(`{"event":"fail","paymentId":"`+pay+`"`+ev("fail")+`}`, 3); code != 409 {
		t.Error("fail after confirmed")
	}

	if _, res := post(
		srv,
		customerB(),
		"invoices.get",
		`{"id":"`+inv+`"}`,
	); data(res)["status"] != "paid" ||
		len(asArr(t, data(res)["paymentRefs"])) != 2 {
		t.Fatalf("paid: %v", res)
	}

	_, resp = post(srv, hq(), "invoices.get", `{"id":"`+inv+`"}`)
	if code, _ := sim(
		`{"event":"initiate","invoiceId":"`+inv+`","method":"demo_credit_card","demoConfirmed":true}`,
		ver(t, resp),
	); code != 409 {
		t.Error("pay a paid invoice")
	}
	// the confirmed payment's reference cannot confirm another invoice's payment
	_, inv2 := newInvoice(t, srv)

	code, resp = sim(
		`{"event":"initiate","invoiceId":"`+inv2+`","method":"demo_credit_card","demoConfirmed":true}`,
		1,
	)
	if code != 200 {
		t.Fatalf("initiate another invoice: %d %v", code, resp)
	}

	pay2 := asStr(t, data(resp)["id"])
	sim(
		`{"event":"processing","paymentId":"`+pay2+`","eventId":"`+uuid.NewString()+`","paymentReference":"P-`+pay2[:8]+`"}`,
		1,
	)

	if code, m := sim(
		`{"event":"confirm","paymentId":"`+pay2+`","eventId":"`+uuid.NewString()+`","paymentReference":"REF-`+pay[:8]+`"}`,
		2,
	); code != 409 ||
		m["messageKey"] != "errors.reference_used" {
		t.Errorf("confirm with a used reference: %d %v", code, m)
	}
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestHQPaymentsAndRelease(t *testing.T) {
	srv := server(t)
	key, inv := newInvoice(t, srv)
	// a restriction caused by this invoice
	rid := uuid.NewString()
	owner(
		t,
		`INSERT INTO restrictions.restrictions (id, tenant_id, contract_id, contract_version, customer_id, rules_version,
			notice_at, execute_after, reason, policy, state)
		VALUES ($1,$2,$3,1,$4,'rules-1',$5,$6,'unpaid','{"kind":"power_off"}','applied')`,
		rid,
		seed.ID("tenant-a"),
		key,
		seed.ID("cust-b"),
		clock(),
		clock().Add(25*time.Hour),
	)
	owner(
		t,
		`INSERT INTO restrictions.restriction_invoices (tenant_id, restriction_id, invoice_id) VALUES ($1,$2,$3)`,
		seed.ID("tenant-a"),
		rid,
		inv,
	)

	var unit string
	ownerScan(
		t,
		`SELECT unit_id::text FROM billing.contract_units WHERE contract_id = $1`,
		[]any{key},
		&unit,
	)
	owner(
		t,
		`INSERT INTO restrictions.restriction_units (tenant_id, restriction_id, unit_id, apply_state, release_state) VALUES
			($1,$2,$3,'applied','none')`,
		seed.ID("tenant-a"),
		rid,
		unit,
	)

	ref := "BANK-" + uuid.NewString()[:8]

	manual := func(amount string, v int) (int, map[string]any) {
		return write(
			srv,
			hq(),
			"payments.recordManual",
			`{"invoiceId":"`+inv+`","paymentReference":"`+ref+`","confirmedAmountMinor":`+amount+`,"currency":"MYR",`+
				`"reason":"bank transfer seen"}`,
			v,
		)
	}
	if code, m := manual(
		"4000",
		1,
	); code != 422 ||
		fields(m) != "confirmedAmountMinor=errors.full_amount_only" {
		t.Errorf("partial amount: %d %s", code, fields(m))
	}

	if code, resp := write(
		srv,
		hq(),
		"payments.recordManual",
		`{"invoiceId":"`+inv+`","paymentReference":" ","confirmedAmountMinor":5000,"currency":"MYR","reason":"x"}`,
		1,
	); code != 422 ||
		fields(resp) != "paymentReference=error.length" {
		t.Errorf("blank reference: %d %s", code, fields(resp))
	}

	if code, _ := write(
		srv,
		customerB(),
		"payments.recordManual",
		`{"invoiceId":"`+inv+`","paymentReference":"X","confirmedAmountMinor":5000,"currency":"MYR","reason":"x"}`,
		1,
	); code != 403 {
		t.Error("client records payment")
	}
	// each operation takes its own ID only (IR390): the other one is an unknown field, and recordManual names invoiceId
	entry := `"paymentReference":"X","confirmedAmountMinor":5000,"currency":"MYR","reason":"x"`
	for _, c := range []struct{ op, body, want string }{
		{opPaymentsRecordManual, `{` + entry + `}`, refusedInvoiceIDRequired},
		{opPaymentsRecordManual, `{"paymentId":"` + inv + `",` + entry + `}`, refusedMalformedInput},
		{
			opPaymentsRecordManual, `{"invoiceId":"` + inv + `","paymentId":"` + inv + `",` + entry + `}`,
			refusedMalformedInput,
		},
		{"payments.confirm", `{"invoiceId":"` + inv + `",` + entry + `}`, refusedMalformedInput},
	} {
		code, m := write(srv, hq(), c.op, c.body, 1)
		refusal(t, c.op+" "+c.body, code, m, c.want)
	}

	if code, m := manual("5000", 9); code != 409 || m["messageKey"] != apperr.KeyVersionConflict {
		t.Errorf("manual on a stale invoice version: %d %v", code, m)
	}

	code, res := manual("5000", 1)
	if code != 200 || data(res)["status"] != statusConfirmed || data(res)["method"] != nil {
		t.Fatalf("manual: %d %v", code, res)
	}

	if code, m2 := manual("5000", 1); code != 200 || data(m2)["id"] != data(res)["id"] {
		t.Errorf("repeat manual returns the same payment: %d", code)
	}
	// HQ sees the confirmation reason (DD-A08); the client does not
	reason := func(who *actor) any {
		_, d := post(srv, who, "invoices.get", `{"id":"`+inv+`"}`)

		return asObj(t, asArr(t, data(d)["paymentRefs"])[0])["confirmationReason"]
	}
	if r := reason(hq()); r != "bank transfer seen" {
		t.Errorf("hq confirmation reason %v", r)
	}

	if r := reason(customerB()); r != nil {
		t.Errorf("client sees the confirmation reason %v", r)
	}

	var (
		state   string
		pending *string
	)
	ownerScan(
		t,
		`SELECT r.state, ru.pending_reason FROM restrictions.restrictions r JOIN restrictions.restriction_units ru ON
			ru.restriction_id = r.id WHERE r.id = $1`,
		[]any{rid},
		&state,
		&pending,
	)

	if state != statusReleaseRequested || pending == nil || *pending != reasonOffline {
		t.Errorf("restriction after payment: %s %v", state, pending)
	}

	owner(t, `UPDATE restrictions.restrictions SET state = 'released' WHERE id = $1`, rid)
	// the reference cannot be reused on another invoice; confirm needs a processing payment
	_, inv2 := newInvoice(t, srv)
	if code, _ := write(
		srv,
		hq(),
		"payments.recordManual",
		`{"invoiceId":"`+inv2+`","paymentReference":"`+ref+`","confirmedAmountMinor":5000,"currency":"MYR","reason":"dup"}`,
		1,
	); code != 409 {
		t.Error("reference reuse")
	}

	code, res = write(
		srv,
		customerB(),
		"payments.simulate",
		`{"event":"initiate","invoiceId":"`+inv2+`","method":"demo_credit_card","demoConfirmed":true}`,
		1,
	)
	if code != 200 {
		t.Fatalf("initiate: %d %v", code, res)
	}

	pay := asStr(t, data(res)["id"])

	confirm := func(v int) (int, map[string]any) {
		return write(
			srv,
			hq(),
			"payments.confirm",
			`{"paymentId":"`+pay+`","paymentReference":"HQ-`+pay[:8]+`","confirmedAmountMinor":5000,"currency":"MYR",`+
				`"reason":"checked"}`,
			v,
		)
	}
	if code, _ := confirm(1); code != 409 {
		t.Error("confirm an initiated payment")
	}

	write(
		srv,
		customerB(),
		"payments.simulate",
		`{"event":"processing","paymentId":"`+pay+`","eventId":"`+uuid.NewString()+`","paymentReference":"P-`+pay[:8]+`"}`,
		1,
	)

	if code, _ := write(
		srv,
		hq(),
		"payments.recordManual",
		`{"invoiceId":"`+inv2+`","paymentReference":"M-`+pay[:8]+`","confirmedAmountMinor":5000,"currency":"MYR",`+
			`"reason":"x"}`,
		3,
	); code != 409 {
		t.Error("manual on an invoice with a payment")
	}

	if code, m := confirm(9); code != 409 || m["messageKey"] != apperr.KeyVersionConflict {
		t.Errorf("hq confirm on a stale version: %d %v", code, m)
	}

	if code, resp := write(
		srv,
		hq(),
		"payments.confirm",
		`{"paymentId":"`+pay+`","paymentReference":"`+ref+`","confirmedAmountMinor":5000,"currency":"MYR",`+
			`"reason":"checked"}`,
		2,
	); code != 409 ||
		resp["messageKey"] != "errors.reference_used" {
		t.Errorf("hq confirm with the manual payment's reference: %d %v", code, resp)
	}

	if code, m := confirm(
		2,
	); code != 200 || data(m)["status"] != statusConfirmed ||
		data(m)["method"] != "demo_credit_card" {
		t.Fatalf("hq confirm: %d %v", code, m)
	}

	if code, _ := confirm(3); code != 409 {
		t.Error("confirm twice")
	}
}

// TestManualAfterFailedPayment (IR447): after the customer's card payment failed, HQ records the bank transfer it
// received; an attempt still in flight keeps refusing it.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestManualAfterFailedPayment(t *testing.T) {
	srv := server(t)
	_, inv := newInvoice(t, srv)
	sim := func(body string, v int) (int, map[string]any) {
		return write(srv, customerB(), "payments.simulate", body, v)
	}

	code, res := sim(
		`{"event":"initiate","invoiceId":"`+inv+`","method":"demo_credit_card","demoConfirmed":true}`,
		1,
	)
	if code != 200 {
		t.Fatalf("initiate: %d %v", code, res)
	}

	pay := asStr(t, data(res)["id"])

	manual := func(v int) (int, map[string]any) {
		return write(
			srv,
			hq(),
			"payments.recordManual",
			`{"invoiceId":"`+inv+`","paymentReference":"BANK-`+pay[:8]+`","confirmedAmountMinor":5000,"currency":"MYR",`+
				`"reason":"bank transfer seen"}`,
			v,
		)
	}
	if code, m := manual(2); code != 409 || m["messageKey"] != "errors.invoice_has_payment" {
		t.Errorf("manual while a card payment is in flight: %d %v", code, m)
	}

	if code, m := sim(
		`{"event":"fail","paymentId":"`+pay+`","eventId":"`+uuid.NewString()+`","paymentReference":"REF-`+pay[:8]+`"}`,
		1,
	); code != 200 ||
		data(m)["status"] != statusFailed {
		t.Fatalf("fail: %d %v", code, m)
	}

	_, res = post(srv, hq(), "invoices.get", `{"id":"`+inv+`"}`)
	if code, m := manual(ver(t, res)); code != 200 || data(m)["status"] != statusConfirmed {
		t.Fatalf("manual after the failed card payment: %d %v", code, m)
	}

	if _, m := post(srv, hq(), "invoices.get", `{"id":"`+inv+`"}`); data(m)["status"] != "paid" {
		t.Errorf("invoice after the manual payment: %v", data(m)["status"])
	}
}

// IR474: an invoice has at most one payment in flight or confirmed (payments are for the full amount): a second card
// attempt while one is initiated is refused, and the database refuses a second open payment too. Before, the second
// initiate passed (the invoice stays unpaid until processing), so two attempts could both be confirmed. After the first
// attempt fails, a new one starts.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestInvoiceOnePaymentAtATime(t *testing.T) {
	srv := server(t)
	_, inv := newInvoice(t, srv)
	sim := func(body string, v int) (int, map[string]any) {
		return write(srv, customerB(), "payments.simulate", body, v)
	}
	invoiceVersion := func() int {
		_, m := post(srv, customerB(), "invoices.get", `{"id":"`+inv+`"}`)

		return ver(t, m)
	}
	initiate := `{"event":"initiate","invoiceId":"` + inv + `","method":"demo_credit_card","demoConfirmed":true}`

	code, m := sim(initiate, invoiceVersion())
	if code != 200 {
		t.Fatalf("initiate: %d %v", code, m)
	}

	pay := asStr(t, data(m)["id"])
	if code, m := sim(
		initiate,
		invoiceVersion(),
	); code != 409 ||
		m["messageKey"] != "errors.invoice_has_payment" {
		t.Fatalf("a second attempt while one is initiated: %d %v", code, m)
	}

	conn, err := pgx.Connect(context.Background(), testDB("postgres"))
	if err != nil {
		t.Skip(err)
	}
	defer func() { _ = conn.Close(context.Background()) }()

	_, err = conn.Exec(
		context.Background(),
		`INSERT INTO billing.payments (tenant_id, invoice_id, amount_minor, currency, method, channel, status, created_at,
			updated_at)
		SELECT tenant_id, id, amount_minor, currency, 'demo_credit_card', 'demo', 'processing', $2, $2 FROM billing.invoices
		WHERE id = $1`,
		inv,
		clock(),
	)

	pe := (*pgconn.PgError)(nil)
	if !errors.As(err, &pe) || pe.Code != "23505" {
		t.Fatalf("a second open payment in the database: %v", err)
	}

	if code, m := sim(
		`{"event":"fail","paymentId":"`+pay+`","eventId":"`+uuid.NewString()+`","paymentReference":"REF-`+pay[:8]+`"}`,
		1,
	); code != 200 ||
		data(m)["status"] != statusFailed {
		t.Fatalf("fail: %d %v", code, m)
	}

	if code, m := sim(initiate, invoiceVersion()); code != 200 {
		t.Fatalf("a new attempt after a failed one: %d %v", code, m)
	}
}
