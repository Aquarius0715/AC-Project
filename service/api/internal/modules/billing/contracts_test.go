package billing

import (
	"testing"
	"time"
)

func TestInvoiceNumber(t *testing.T) {
	for _, tc := range []struct {
		from string
		seq  int
		want string
	}{
		{"2026-08-01T00:00:00Z", 1, "INV-202608-0001"},       // 08:00 in Kuala Lumpur
		{"2026-08-31T16:00:00Z", 7, "INV-202609-0007"},       // local midnight of 1 Sep, still August in UTC
		{"2026-09-01T00:00:00+08:00", 12, "INV-202609-0012"}, // the web's local-day input
		{"2026-12-31T16:00:00Z", 10000, "INV-202701-10000"},  // the year rolls over locally; sequences past 9999 widen
		{"2026-09-30T15:59:59Z", 3, "INV-202609-0003"},       // the last local second of September
	} {
		from, err := time.Parse(time.RFC3339, tc.from)
		if err != nil {
			t.Fatal(err)
		}
		if got := InvoiceNumber(from, tc.seq); got != tc.want {
			t.Errorf("InvoiceNumber(%s, %d) = %s, want %s", tc.from, tc.seq, got, tc.want)
		}
	}
}
