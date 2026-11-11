package integration_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

// A write that waited for a new version of the same record compares its expected version with that version and
// answers CONFLICT error.versionConflict (IR766): it locks the record's rows by id, then reads the current one, so the
// row it waited for no longer being current does not make the record look gone (NOT_FOUND).
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestAWriteThatWaitedForANewVersionIsAVersionConflict(t *testing.T) {
	srv := server(t)
	ctx := context.Background()
	unit := newUnit(t, srv, "Version race AC")
	stamp := func(h int) string { return clock().Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	contract := strings.Join([]string{
		`"customerId":"` + seed.ID("cust-b").String() + `"`,
		`"unitIds":["` + unit + `"]`,
		`"planType":"rto"`,
		`"startAt":"` + stamp(-24*30) + `"`,
		`"endAt":"` + stamp(24*365) + `"`,
		`"priceMinor":12000`,
		`"currency":"MYR"`,
		`"restrictionEligible":true`,
		`"rulesVersion":"rules-demo-1"`,
	}, ",")
	model := "TA-race-" + uuid.NewString()[:6]

	for _, testCase := range []struct {
		name, operation, table, creator, create string
		revise                                  func(id string) string
	}{
		{
			"capability", "capabilities.save", "devices.capabilities", "", capBody("", model, ""),
			func(id string) string { return capBody(id, model, `,"changeReason":"race test"`) },
		},
		{
			"contract", "contracts.save", "billing.contracts", "", "{" + contract + "}",
			func(id string) string { return `{"id":"` + id + `",` + contract + "}" },
		},
		{
			"factor delete", "factors.delete", tableEmissionFactors, opFactorsSave,
			`{"region":"Race del ` + uuid.NewString()[:6] + `","year":2033,"kgCO2ePerKWh":0.4,` +
				`"source":"demo test factor","isDemo":true}`,
			func(id string) string { return `{"id":"` + id + `","reason":"made by mistake"}` },
		},
	} {
		creator := testCase.creator // the operation that makes the record, when it is not the one tested
		if creator == "" {
			creator = testCase.operation
		}

		code, resp := write(srv, hq(), creator, testCase.create, 0)
		if code != 200 {
			t.Fatalf("%s: %d %v", testCase.name, code, resp)
		}

		id := asStr(t, data(resp)["id"])

		conn, err := pgx.Connect(ctx, testDB("postgres"))
		if err != nil {
			t.Skip(err)
		}

		tx, err := conn.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}

		holdNextVersion(ctx, t, tx, testCase.table, id)

		done := make(chan saveResult, 1)

		go func() {
			status, reply := write(srv, hq(), testCase.operation, testCase.revise(id), 1)
			done <- saveResult{status, reply}
		}()

		waited := false

		var got saveResult
		select {
		case got = <-done:
			t.Errorf(
				"%s: the save did not wait for the other one: %d %v",
				testCase.name,
				got.code,
				got.resp,
			)
		case <-time.After(700 * time.Millisecond):
			waited = true
		}

		err = tx.Commit(ctx)
		if err != nil {
			t.Fatal(err)
		}

		_ = conn.Close(ctx)

		if waited {
			got = <-done
		}

		if got.code != 409 || got.resp["messageKey"] != apperr.KeyVersionConflict {
			t.Errorf(
				"%s: a save at version 1 after version 2: %d %v",
				testCase.name,
				got.code,
				got.resp,
			)
		}
	}
}

// holdNextVersion makes, in tx, another save of the record half done: version 1 is no longer current and version 2 is.
// The whole row is copied, whatever the table's columns, to stand for that save's next version; tx holds the locks
// until the caller commits it.
func holdNextVersion(ctx context.Context, t *testing.T, tx pgx.Tx, table, id string) {
	t.Helper()

	for _, step := range []struct {
		stmt string
		byID bool
	}{
		{`CREATE TEMP TABLE next_version ON COMMIT DROP AS SELECT * FROM ` + //nolint:unqueryvet // copies the whole row
			table + ` WHERE id = $1 AND version = 1`, true},
		{`UPDATE next_version SET version = 2`, false},
		{`UPDATE ` + table + ` SET is_current = false WHERE id = $1 AND version = 1`, true},
		{`INSERT INTO ` + table + ` SELECT * FROM next_version`, false}, //nolint:unqueryvet // the copied row as a whole
	} {
		args := []any{}
		if step.byID {
			args = append(args, id)
		}

		_, err := tx.Exec(ctx, step.stmt, args...)
		if err != nil {
			t.Fatalf("%s: %s: %v", table, step.stmt, err)
		}
	}
}
