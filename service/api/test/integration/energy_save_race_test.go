package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// A new version of a factor or a baseline is saved only under the lock of the record's versions (IR765): a delete of
// the record half done (it locks the current version and removes them all) makes the save wait and then answer
// NOT_FOUND, instead of leaving the new version as a record that survived its own delete (DEC-103).
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestEnergyRecordSaveWaitsForItsDelete(t *testing.T) {
	srv := server(t)
	ctx := context.Background()
	unit, _ := isolatedUnit(t, srv)
	region := "Race save " + uuid.NewString()[:6]
	factorBody := `"region":"` + region + `","year":2032,"kgCO2ePerKWh":0.41,"source":"demo test factor","isDemo":true}`
	baselineBody := `"unitIds":["` + unit + `"],"period":{"from":"2026-08-01T00:00:00Z","to":"2026-08-01T00:05:00Z"},` +
		`"boundaryId":"ac_input_electricity","boundary":"AC input","assumptions":"demo","source":"test",` +
		`"method":"demo_fixed","baselineKWh":10}`

	for _, testCase := range []struct {
		operation, table, body string
	}{
		{"factors.save", "energy.emission_factors", factorBody},
		{"baselines.save", "energy.baselines", baselineBody},
	} {
		code, resp := write(srv, hq(), testCase.operation, "{"+testCase.body, 0)
		if code != 200 {
			t.Fatalf("%s: %d %v", testCase.operation, code, resp)
		}

		id := asStr(t, data(resp)["id"])

		t.Cleanup(func() { owner(t, `DELETE FROM `+testCase.table+` WHERE id = $1`, id) })

		conn, err := pgx.Connect(ctx, testDB("postgres"))
		if err != nil {
			t.Skip(err)
		}

		tx, err := conn.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		// the delete half done: it locked the current version and removed every version (deletes.go)
		_, err = tx.Exec(
			ctx,
			`SELECT 1 FROM `+testCase.table+` WHERE id = $1 AND is_current FOR UPDATE`,
			id,
		)
		if err == nil {
			_, err = tx.Exec(ctx, `DELETE FROM `+testCase.table+` WHERE id = $1`, id)
		}

		if err != nil {
			t.Fatal(err)
		}

		done := make(chan saveResult, 1)

		go func() {
			status, reply := write(
				srv,
				hq(),
				testCase.operation,
				`{"id":"`+id+`",`+testCase.body,
				1,
			)
			done <- saveResult{status, reply}
		}()

		waited := false

		var got saveResult
		select { // the save reads the record while the delete holds it
		case got = <-done:
			t.Errorf(
				"%s: the save did not wait for the delete: %d %v",
				testCase.operation,
				got.code,
				got.resp,
			)
		case <-time.After(700 * time.Millisecond):
			waited = true
		}

		commitErr := tx.Commit(ctx)
		if commitErr != nil {
			t.Fatal(commitErr)
		}

		_ = conn.Close(ctx)

		if waited {
			got = <-done
		}

		if got.code != 404 {
			t.Errorf(
				"%s: a new version of a record deleted meanwhile: %d %v",
				testCase.operation,
				got.code,
				got.resp,
			)
		}
	}
}
