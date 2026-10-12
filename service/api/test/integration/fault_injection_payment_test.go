package integration

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
)

// TestManualPaymentRollsBackWhenTheInvoiceCannotBePaid: a manual payment whose invoice cannot be marked paid is not
// recorded. The operation fails as a server error, the invoice stays as it was, and the same payment reference is then
// still free: the same request succeeds once the database accepts the write (payments.recordManual; markPaid fails).
func TestManualPaymentRollsBackWhenTheInvoiceCannotBePaid(t *testing.T) {
	s := server(t)
	_, inv := newInvoice(t, s)
	_, before := post(s, &hq, "invoices.get", `{"id":"`+inv+`"}`)
	amount := data(before)["amountMinor"]
	ref := "BANK-" + uuid.NewString()[:8]
	body := `{"invoiceId":"` + inv + `","paymentReference":"` + ref + `","confirmedAmountMinor":` + fmt.Sprintf("%.0f", amount.(float64)) + `,"currency":"MYR","reason":"bank transfer seen"}`

	broken := injectAt(t, "UPDATE billing.invoices SET status = 'paid'")
	code, m := write(broken, &hq, "payments.recordManual", body, ver(before))
	if code < 500 {
		t.Fatalf("manual payment with a failing invoice update: %d %v, want a server error", code, m)
	}
	_, after := post(s, &hq, "invoices.get", `{"id":"`+inv+`"}`)
	if data(after)["status"] != data(before)["status"] || ver(after) != ver(before) {
		t.Errorf("a failed payment changed the invoice: before %v, after %v", before, after)
	}
	if code, m := write(s, &hq, "payments.recordManual", body, ver(after)); code != 200 {
		t.Errorf("the reference is not free after a failed payment: %d %v", code, m)
	}
}
