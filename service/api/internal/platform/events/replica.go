package events

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// RowChanged is the event type prefix of the generic change capture (IR188): platform.capture_row() publishes
// "RowChanged:<schema>.<table>" with {op, row} for the columns named in the trigger.
const RowChanged = "RowChanged:"

// RowChange is the payload of a RowChanged event.
type RowChange struct {
	Op  string          `json:"op"` // INSERT, UPDATE, DELETE
	Row json.RawMessage `json:"row" swaggertype:"object"`
}

// Replica keeps a reference copy (same column names) of a source table captured by platform.capture_row().
type Replica struct {
	Source string   // "<schema>.<table>" of the owner
	Table  string   // the consumer's copy, for example "notify.ref_units"
	Keys   []string // primary key columns
	Cols   []string // the other copied columns
}

// Handlers returns the event handlers that keep the replicas: upsert on INSERT / UPDATE, delete on DELETE.
func Handlers(replicas ...Replica) map[string]Handler {
	out := map[string]Handler{}
	for _, r := range replicas {
		out[RowChanged+r.Source] = r.handle
	}

	return out
}

func (r Replica) handle(ctx context.Context, txn pgx.Tx, e Event) error {
	var rowChange RowChange

	decodeErr := e.Decode(&rowChange)
	if decodeErr != nil {
		return decodeErr
	}

	keyMatch := make([]string, len(r.Keys))
	for i, k := range r.Keys {
		keyMatch[i] = fmt.Sprintf("t.%[1]s = s.%[1]s", k)
	}

	if rowChange.Op == "DELETE" {
		_, err := txn.Exec(
			ctx,
			fmt.Sprintf(
				`DELETE FROM %[1]s t USING jsonb_populate_record(NULL::%[1]s, $1) s WHERE %[2]s`,
				r.Table,
				strings.Join(keyMatch, " AND "),
			),
			[]byte(rowChange.Row),
		)

		return apperr.Wrap(err, "delete (handle)")
	}

	all := append(append([]string{}, r.Keys...), r.Cols...)

	set := make([]string, len(r.Cols))
	for i, c := range r.Cols {
		set[i] = c + " = EXCLUDED." + c
	}

	conflict := "DO NOTHING"
	if len(set) > 0 {
		conflict = "DO UPDATE SET " + strings.Join(set, ", ")
	}

	cols := strings.Join(all, ", ")
	_, err := txn.Exec(
		ctx,
		fmt.Sprintf(
			`INSERT INTO %[1]s (%[2]s) SELECT %[2]s FROM jsonb_populate_record(NULL::%[1]s, $1) ON CONFLICT (%[3]s) %[4]s`,
			r.Table,
			cols,
			strings.Join(r.Keys, ", "),
			conflict,
		),
		[]byte(rowChange.Row),
	)

	return apperr.Wrap(err, "insert (handle)")
}
