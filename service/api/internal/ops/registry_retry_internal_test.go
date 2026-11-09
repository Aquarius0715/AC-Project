package ops

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// retryingDB runs the transaction again when the callback fails with the retryable error, as platform/db does for a
// deadlock or serialization failure (40P01, 40001): the same callback, and so the same Call, runs once more.
type retryingDB struct{ attempts *int }

func (r retryingDB) Run(
	_ context.Context,
	_ bool,
	_ *Principal,
	callback func(pgx.Tx) error,
) error {
	var err error

	for range 3 {
		*r.attempts++

		err = callback(nil)
		if err == nil || apperr.From(err).Code != apperr.Unavailable {
			return err
		}
	}

	return err
}

// countingRec counts the events and audit entries the transaction that committed carried.
type countingRec struct{ events, audits int }

func (c *countingRec) Record(_ context.Context, _ pgx.Tx, call *Call, _ string) error {
	c.events, c.audits = len(call.Events), len(call.Audits)

	return nil
}

// A write that is run again after a deadlock records the events and audit entries of the attempt that committed, not
// those of the attempt that rolled back as well (IR663).
func TestRetriedWriteRecordsOneAttemptOnly(t *testing.T) {
	t.Parallel()

	attempts := 0
	rec := &countingRec{events: 0, audits: 0}
	reg := NewRegistry()
	reg.DB = retryingDB{attempts: &attempts}
	reg.Rec = rec
	reg.Idem = &fakeIdem{
		stored:  map[string]json.RawMessage{},
		hash:    map[string]string{},
		aborted: 0,
	}
	reg.Clock = func() time.Time { return time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC) }

	handled := 0

	Register(reg, opUnitsSave, func(_ context.Context, call *Call, input *unitIn) (out, error) {
		handled++

		call.Emit(
			Event{Type: "UnitChanged", AggregateType: "", AggregateID: [16]byte{}, Payload: nil},
		)
		call.Audit(AuditEntry{
			Action:          opUnitsSave,
			TargetKind:      "",
			TargetID:        "",
			PreviousVersion: nil,
			NextVersion:     nil,
			Reason:          "",
			Result:          "",
			Before:          nil,
			After:           nil,
		})

		if handled == 1 { // the first attempt is rolled back by the database after it has emitted
			return out{}, apperr.E(apperr.Unavailable, apperr.KeyUnavailable)
		}

		return out{OK: true, Name: input.Name, Calls: handled}, nil
	})

	app := echo.New()
	app.HTTPErrorHandler = HTTPErrorHandler

	err := reg.MountREST(app.Group("/v1"))
	if err != nil {
		t.Fatal(err)
	}

	response := do(t, app, adminPrincipal(), opUnitsSave, bodyNameA, "key-retry-1")
	if response.Code != 200 || attempts != 2 {
		t.Fatalf("status %d after %d attempts: %s", response.Code, attempts, response.Body.String())
	}

	if rec.events != 1 || rec.audits != 1 {
		t.Errorf(
			"recorded %d events and %d audit entries after a retry, want 1 and 1",
			rec.events,
			rec.audits,
		)
	}
}
