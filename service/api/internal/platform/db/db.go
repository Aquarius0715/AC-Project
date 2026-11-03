// Package db provides the PostgreSQL pools, the transaction manager with row-level-security context, the
// idempotency store and the audit/outbox recorder (backend Go design §4, database design §7–§8).
package db

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/events"
)

// TxManager runs one transaction per operation on the writer or reader pool.
type TxManager struct {
	Writer *pgxpool.Pool
	Reader *pgxpool.Pool
}

// Open connects both pools (the reader URL may equal the writer URL locally).
func Open(ctx context.Context, writerURL, readerURL string) (*TxManager, error) {
	pool, err := newPool(ctx, writerURL)
	if err != nil {
		return nil, err
	}

	if readerURL == "" || readerURL == writerURL {
		return &TxManager{Writer: pool, Reader: pool}, nil
	}

	r, err := newPool(ctx, readerURL)
	if err != nil {
		pool.Close()

		return nil, err
	}

	return &TxManager{Writer: pool, Reader: r}, nil
}

// Close closes the pools.
func (m *TxManager) Close() {
	m.Writer.Close()

	if m.Reader != m.Writer {
		m.Reader.Close()
	}
}

// Run implements ops.Runner: RLS context via set_config(..., true) and up to three attempts on serialization
// failure or deadlock.
func (m *TxManager) Run(
	ctx context.Context,
	readOnly bool,
	principal *ops.Principal,
	callback func(pgx.Tx) error,
) error {
	pool, opts := m.Writer, pgx.TxOptions{
		IsoLevel:       pgx.ReadCommitted,
		AccessMode:     "",
		DeferrableMode: "",
		BeginQuery:     "",
		CommitQuery:    "",
	}
	if readOnly {
		pool, opts = m.Reader, pgx.TxOptions{
			AccessMode:     pgx.ReadOnly,
			IsoLevel:       "",
			DeferrableMode: "",
			BeginQuery:     "",
			CommitQuery:    "",
		}
	}

	var err error
	for range 3 {
		err = pgx.BeginTxFunc(ctx, pool, opts, func(txn pgx.Tx) error {
			if principal != nil {
				_, err := txn.Exec(
					ctx,
					"SELECT set_config('app.tenant_id', $1, true), set_config('app.membership_id', $2, true)",
					principal.TenantID.String(),
					principal.MembershipID.String(),
				)
				if err != nil {
					return apperr.Wrap(err, "select set_config")
				}
			}

			return callback(txn)
		})

		var pg *pgconn.PgError
		if !errors.As(err, &pg) ||
			(pg.Code != "40001" && pg.Code != "40P01") { // only a serialization failure retries
			break
		}
	}

	if err != nil {
		return apperr.From(err)
	}

	return nil
}

// Idempotency implements ops.Idempotency on platform.idempotency_keys.
type Idempotency struct{ M *TxManager }

func (s Idempotency) tx(ctx context.Context, p *ops.Principal, fn func(pgx.Tx) error) error {
	return s.M.Run(ctx, false, p, fn)
}

// Begin claims a key or returns the stored response.
func (s Idempotency) Begin(
	ctx context.Context,
	principal *ops.Principal,
	operation, key, hash string,
) (json.RawMessage, error) {
	if principal == nil {
		return nil, nil // public demo operations are not keyed per membership
	}

	h, err := hex.DecodeString(hash)
	if err != nil {
		return nil, fmt.Errorf("hash: %w", err)
	}

	var stored json.RawMessage

	err = s.tx(ctx, principal, func(txn pgx.Tx) error {
		tag, err := txn.Exec(
			ctx,
			`INSERT INTO platform.idempotency_keys (tenant_id, membership_id, key, operation, request_hash, status)
			VALUES ($1,$2,$3,$4,$5,'in_progress') ON CONFLICT DO NOTHING`,
			principal.TenantID,
			principal.MembershipID,
			key,
			operation,
			h,
		)
		if err != nil {
			return apperr.Wrap(err, "insert platform.idempotency_keys")
		}

		if tag.RowsAffected() == 1 {
			return nil
		}

		var (
			gotOp, status string
			gotHash       []byte
			resp          []byte
			expired       bool
		)

		scanErr := txn.QueryRow(ctx, `SELECT operation, request_hash, status, response, expires_at < platform.app_now() FROM
			platform.idempotency_keys
			WHERE tenant_id=$1 AND membership_id=$2 AND key=$3 FOR UPDATE`, principal.TenantID, principal.MembershipID, key).
			Scan(&gotOp, &gotHash, &status, &resp, &expired)
		if scanErr != nil {
			return apperr.Wrap(scanErr, "select platform.idempotency_keys")
		}

		if expired {
			_, err := txn.Exec(
				ctx,
				`UPDATE platform.idempotency_keys SET operation=$4, request_hash=$5, status='in_progress', response=NULL,
				created_at=platform.app_now(), expires_at=platform.app_now()+interval '24 hours' WHERE tenant_id=$1 AND
				membership_id=$2 AND key=$3`,
				principal.TenantID,
				principal.MembershipID,
				key,
				operation,
				h,
			)

			return apperr.Wrap(err, "update platform.idempotency_keys")
		}

		if gotOp != operation || hex.EncodeToString(gotHash) != hash {
			return apperr.E(apperr.Conflict, "error.idempotencyKeyReused")
		}

		if status == "in_progress" {
			return apperr.E(apperr.Conflict, "error.requestInProgress").RetryAfter(1)
		}

		stored = resp

		return nil
	})

	return stored, err
}

// Complete stores the response.
func (s Idempotency) Complete(
	ctx context.Context,
	principal *ops.Principal,
	op, key string,
	resp json.RawMessage,
) error {
	if principal == nil {
		return nil
	}

	return s.tx(ctx, principal, func(tx pgx.Tx) error {
		_, err := tx.Exec(
			ctx,
			`UPDATE platform.idempotency_keys SET status='completed', http_status=200, response=$4
			WHERE tenant_id=$1 AND membership_id=$2 AND key=$3`,
			principal.TenantID,
			principal.MembershipID,
			key,
			resp,
		)

		return apperr.Wrap(err, "update platform.idempotency_keys")
	})
}

// Abort releases a key after a failed attempt so the client may retry with the same key.
func (s Idempotency) Abort(ctx context.Context, principal *ops.Principal, op, key string) {
	if principal == nil {
		return
	}

	_ = s.tx(ctx, principal, func(tx pgx.Tx) error {
		_, err := tx.Exec(
			ctx,
			`DELETE FROM platform.idempotency_keys WHERE tenant_id=$1 AND membership_id=$2 AND key=$3 AND status='in_progress'`,
			principal.TenantID,
			principal.MembershipID,
			key,
		)

		return apperr.Wrap(err, "delete platform.idempotency_keys")
	})
}

// Recorder writes audit rows and outbox events in the operation's transaction (ops.Recorder).
type Recorder struct{}

// RecordDenied implements ops.DeniedRecorder: the audit entry of a refused write, result denied — the actor, the
// operation, the target it asked for and the refusal's message key as the reason; no input values (IR603).
func (Recorder) RecordDenied(
	ctx context.Context,
	txn pgx.Tx,
	call *ops.Call,
	operation string,
	denial ops.Denial,
) error {
	principal := call.Principal
	id := uuid.Must(uuid.NewV7())

	var membership *uuid.UUID

	if principal.MembershipID != uuid.Nil {
		m := principal.MembershipID
		membership = &m
	}

	return apperr.Wrap(
		events.Publish(
			ctx,
			txn,
			principal.TenantID,
			"audit",
			id,
			events.AuditRecorded,
			events.Audit{
				ID:              id,
				ActorID:         principal.UserID.String(),
				ActorRole:       principal.Role,
				MembershipID:    membership,
				Action:          operation,
				TargetKind:      denial.TargetKind,
				TargetID:        denial.TargetID,
				OccurredAt:      call.Now,
				CorrelationID:   call.CorrelationID,
				Result:          "denied",
				Reason:          denial.Reason,
				PreviousVersion: nil,
				NextVersion:     nil,
				MaskedBefore:    nil,
				MaskedAfter:     nil,
			},
		),
		"events.Publish",
	)
}

// Record implements ops.Recorder.
func (Recorder) Record(ctx context.Context, txn pgx.Tx, c *ops.Call, op string) error {
	principal := c.Principal
	if principal == nil {
		return nil
	}

	for _, auditEntry := range c.Audits {
		// the audit log is identity's: published in this transaction, stored by its consumer (IR196)
		id := uuid.Must(uuid.NewV7())

		result := auditEntry.Result
		if result == "" {
			result = "success"
		}

		var membership *uuid.UUID

		if principal.MembershipID != uuid.Nil {
			m := principal.MembershipID
			membership = &m
		}

		err := events.Publish(
			ctx,
			txn,
			principal.TenantID,
			"audit",
			id,
			events.AuditRecorded,
			events.Audit{
				ID:              id,
				ActorID:         principal.UserID.String(),
				ActorRole:       principal.Role,
				MembershipID:    membership,
				Action:          auditEntry.Action,
				TargetKind:      auditEntry.TargetKind,
				TargetID:        auditEntry.TargetID,
				PreviousVersion: auditEntry.PreviousVersion,
				NextVersion:     auditEntry.NextVersion,
				OccurredAt:      c.Now,
				CorrelationID:   c.CorrelationID,
				Result:          result,
				Reason:          auditEntry.Reason,
				MaskedBefore:    ops.Masked(auditEntry.Before),
				MaskedAfter:     ops.Masked(auditEntry.After),
			},
		)
		if err != nil {
			return apperr.Wrap(err, "events.Publish")
		}
	}

	for _, event := range c.Events {
		payload, err := json.Marshal(event.Payload)
		if err != nil {
			return apperr.Wrap(err, "json.Marshal")
		}

		if event.Payload == nil {
			payload = []byte("{}")
		}

		_, execErr := txn.Exec(
			ctx,
			`INSERT INTO platform.outbox (tenant_id, aggregate_type, aggregate_id, event_type, payload, correlation_id,
				occurred_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			principal.TenantID,
			event.AggregateType,
			event.AggregateID,
			event.Type,
			payload,
			c.CorrelationID,
			c.Now,
		)
		if execErr != nil {
			return apperr.Wrap(execErr, "insert platform.outbox")
		}
	}

	return nil
}

// newPool opens a pool whose timestamptz values scan in UTC, so every Instant leaves the API with a Z offset
// regardless of the server's local time zone.
func newPool(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, apperr.Wrap(err, "pgxpool.ParseConfig")
	}

	cfg.AfterConnect = func(_ context.Context, conn *pgx.Conn) error {
		conn.TypeMap().
			RegisterType(
				&pgtype.Type{
					Name:  "timestamptz",
					OID:   pgtype.TimestamptzOID,
					Codec: &pgtype.TimestamptzCodec{ScanLocation: time.UTC},
				})

		return nil
	}

	out, err := pgxpool.NewWithConfig(ctx, cfg)

	return out, apperr.Wrap(err, "pgxpool.NewWithConfig")
}
