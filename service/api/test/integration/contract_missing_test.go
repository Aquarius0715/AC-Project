package integration_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

// TestContractUpdateOfAMissingContractIsNotFound: revising a contract that does not exist is NOT_FOUND, read under the
// row lock before any rule of the revision is checked (billing contracts.save with an id; loadContract).
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestContractUpdateOfAMissingContractIsNotFound(t *testing.T) {
	srv := server(t)
	u := newUnit(t, srv, "Missing contract AC")
	stamp := func(h int) string { return clock().Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	body := strings.Join([]string{
		`"id":"` + uuid.NewString() + `"`,
		`"customerId":"` + seed.ID("cust-b").String() + `"`,
		`"unitIds":["` + u + `"]`,
		`"planType":"rto"`,
		`"startAt":"` + stamp(-24*30) + `"`,
		`"endAt":"` + stamp(24*365) + `"`,
		`"priceMinor":12000`,
		`"currency":"MYR"`,
		`"restrictionEligible":true`,
		`"rulesVersion":"rules-demo-1"`,
	}, ",")

	code, m := write(srv, hq(), "contracts.save", "{"+body+"}", 1)
	if code != 404 || m["messageKey"] != apperr.KeyNotFound {
		t.Fatalf("revision of a missing contract: %d %v, want 404 NOT_FOUND", code, m)
	}
}
