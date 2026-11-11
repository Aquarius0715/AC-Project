package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// An MRV draft reads the factor and the baseline it cites under a share lock (IR763, B-29): a delete of either one
// half done (it locks the record and has removed its versions) makes the draft wait and then answer NOT_FOUND, instead
// of saving a report that cites a record that is gone (DEC-103, IR656).
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestMRVDraftWaitsForADeleteOfWhatItCites(t *testing.T) {
	srv := server(t)
	ctx := context.Background()
	unit, _ := isolatedUnit(t, srv)
	start := time.Date(2026, 9, 12, 8, 0, 0, 0, time.UTC)

	saved := func(operation, body string) string {
		code, resp := write(srv, hq(), operation, body, 0)
		if code != 200 {
			t.Fatalf("%s: %d %v", operation, code, resp)
		}

		return asStr(t, data(resp)["id"])
	}
	baseline := func() string {
		return saved(
			"baselines.save",
			`{"unitIds":["`+unit+`"],"period":{"from":"2026-08-01T00:00:00Z",`+
				`"to":"2026-08-01T00:05:00Z"},"boundaryId":"ac_input_electricity","boundary":"AC input",`+
				`"assumptions":"demo","source":"test","method":"demo_fixed","baselineKWh":10}`,
		)
	}
	factor := func() string {
		return saved(
			"factors.save",
			`{"region":"Race region `+uuid.NewString()[:6]+`","year":2031,`+
				`"kgCO2ePerKWh":0.41,"source":"demo test factor","isDemo":true}`,
		)
	}
	draftBody := func(baselineID, factorID string) string {
		return `{"conditions":{"from":"` + start.Format(time.RFC3339) + `","to":"` +
			start.Add(5*time.Minute).
				Format(time.RFC3339) +
			`","unitIds":["` + unit + `"],"baselineId":"` + baselineID +
			`","baselineVersion":1,"factorId":"` + factorID + `","factorVersion":1,"boundaryId":"ac_input_electricity",` +
			`"boundary":"AC input electricity","organizationId":"` + seed.ID("org-customer-b").String() +
			`"},"evidenceIds":[]}`
	}

	for _, testCase := range []struct {
		name, table string
		cited       func() (string, string, string) // the deleted record, then the draft's baseline and factor
	}{
		{"factor", "energy.emission_factors", func() (string, string, string) {
			f := factor()

			return f, baseline(), f
		}},
		{"baseline", "energy.baselines", func() (string, string, string) {
			b := baseline()

			return b, b, factor()
		}},
	} {
		gone, baselineID, factorID := testCase.cited()

		t.Cleanup(func() {
			owner(
				t,
				`DELETE FROM energy.mrv_reports WHERE conditions->>'factorId' = $1 OR conditions->>'baselineId' = $1`,
				gone,
			)
		})

		conn, err := pgx.Connect(ctx, testDB("postgres"))
		if err != nil {
			t.Skip(err)
		}

		tx, err := conn.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}

		_, err = tx.Exec(ctx, `DELETE FROM `+testCase.table+` WHERE id = $1`, gone)
		if err != nil {
			t.Fatal(err)
		}

		done := make(chan saveResult, 1)

		go func() {
			status, reply := write(srv, hq(), "mrv.saveDraft", draftBody(baselineID, factorID), 0)
			done <- saveResult{status, reply}
		}()

		waited := false

		var got saveResult
		select {
		case got = <-done:
			t.Errorf(
				"%s: the draft did not wait for the delete: %d %v",
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

		if got.code != 404 {
			t.Errorf(
				"%s: a draft citing a deleted record: %d %v",
				testCase.name,
				got.code,
				got.resp,
			)
		}
	}
}

// The writes of an MRV report lock its latest version (IR764): a delete of the draft half done makes a new version of
// it wait and then answer NOT_FOUND, instead of leaving that version as the whole report after the delete.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestMRVDraftVersionWaitsForTheDraftDelete(t *testing.T) {
	srv := server(t)
	ctx := context.Background()
	unit, _ := isolatedUnit(t, srv)
	start := time.Date(2026, 9, 12, 8, 0, 0, 0, time.UTC)

	code, resp := write(
		srv,
		hq(),
		"baselines.save",
		`{"unitIds":["`+unit+`"],"period":{"from":"2026-08-01T00:00:00Z",`+
			`"to":"2026-08-01T00:05:00Z"},"boundaryId":"ac_input_electricity","boundary":"AC input","assumptions":"demo",`+
			`"source":"test","method":"demo_fixed","baselineKWh":10}`,
		0,
	)
	if code != 200 {
		t.Fatalf("baseline: %d %v", code, resp)
	}

	conditions := `{"from":"` + start.Format(
		time.RFC3339,
	) + `","to":"` + start.Add(5*time.Minute).
		Format(time.RFC3339) +
		`","unitIds":["` + unit + `"],"baselineId":"` + asStr(
		t,
		data(resp)["id"],
	) +
		`","baselineVersion":1,"factorId":"` + seed.ID("factor-demo-2026").
		String() +
		`","factorVersion":1,"boundaryId":"ac_input_electricity","boundary":"AC input electricity",` +
		`"organizationId":"` + seed.ID("org-customer-b").
		String() +
		`"}`

	code, resp = write(
		srv,
		hq(),
		"mrv.saveDraft",
		`{"conditions":`+conditions+`,"evidenceIds":[]}`,
		0,
	)
	if code != 200 {
		t.Fatalf("draft: %d %v", code, resp)
	}

	report := asStr(t, data(resp)["id"])

	t.Cleanup(func() { owner(t, `DELETE FROM energy.mrv_reports WHERE id = $1`, report) })

	conn, err := pgx.Connect(ctx, testDB("postgres"))
	if err != nil {
		t.Skip(err)
	}
	defer func() { _ = conn.Close(ctx) }()

	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, `DELETE FROM energy.mrv_reports WHERE id = $1`, report)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan saveResult, 1)

	go func() {
		status, reply := write(srv, hq(), "mrv.saveDraft",
			`{"id":"`+report+`","conditions":`+conditions+`,"evidenceIds":[]}`, 1)
		done <- saveResult{status, reply}
	}()

	waited := false

	var got saveResult
	select {
	case got = <-done:
		t.Errorf("the new version did not wait for the delete: %d %v", got.code, got.resp)
	case <-time.After(700 * time.Millisecond):
		waited = true
	}

	err = tx.Commit(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if waited {
		got = <-done
	}

	if got.code != 404 {
		t.Errorf("a new version of a deleted draft: %d %v", got.code, got.resp)
	}
}
