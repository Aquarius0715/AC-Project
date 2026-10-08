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

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pradita/ac-project/service/core/ops"
	"github.com/pradita/ac-project/service/core/platform/apperr"
)

// TxManager runs one transaction per operation on the writer or reader pool.
type TxManager struct {
	Writer *pgxpool.Pool
	Reader *pgxpool.Pool
}

// Open connects both pools (the reader URL may equal the writer URL locally).
func Open(ctx context.Context, writerURL, readerURL string) (*TxManager, error) {
	w, err := newPool(ctx, writerURL)
	if err != nil {
		return nil, err
	}
	if readerURL == "" || readerURL == writerURL {
		return &TxManager{Writer: w, Reader: w}, nil
	}
	r, err := newPool(ctx, readerURL)
	if err != nil {
		w.Close()
		return nil, err
	}
	return &TxManager{Writer: w, Reader: r}, nil
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
func (m *TxManager) Run(ctx context.Context, readOnly bool, p *ops.Principal, fn func(pgx.Tx) error) error {
	pool, opts := m.Writer, pgx.TxOptions{IsoLevel: pgx.ReadCommitted}
	if readOnly {
		pool, opts = m.Reader, pgx.TxOptions{AccessMode: pgx.ReadOnly}
	}
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		err = pgx.BeginTxFunc(ctx, pool, opts, func(tx pgx.Tx) error {
			if p != nil {
				if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true), set_config('app.membership_id', $2, true)",
					p.TenantID.String(), p.MembershipID.String()); err != nil {
					return err
				}
			}
			return fn(tx)
		})
		var pg *pgconn.PgError
		if !(errors.As(err, &pg) && (pg.Code == "40001" || pg.Code == "40P01")) {
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
func (s Idempotency) Begin(ctx context.Context, p *ops.Principal, op, key, hash string) (json.RawMessage, error) {
	if p == nil {
		return nil, nil // public demo operations are not keyed per membership
	}
	h, err := hex.DecodeString(hash)
	if err != nil {
		return nil, fmt.Errorf("hash: %w", err)
	}
	var stored json.RawMessage
	err = s.tx(ctx, p, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `INSERT INTO platform.idempotency_keys (tenant_id, membership_id, key, operation, request_hash, status)
			VALUES ($1,$2,$3,$4,$5,'in_progress') ON CONFLICT DO NOTHING`, p.TenantID, p.MembershipID, key, op, h)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 1 {
			return nil
		}
		var gotOp, status string
		var gotHash []byte
		var resp []byte
		var expired bool
		if err := tx.QueryRow(ctx, `SELECT operation, request_hash, status, response, expires_at < platform.app_now() FROM platform.idempotency_keys
			WHERE tenant_id=$1 AND membership_id=$2 AND key=$3 FOR UPDATE`, p.TenantID, p.MembershipID, key).Scan(&gotOp, &gotHash, &status, &resp, &expired); err != nil {
			return err
		}
		if expired {
			_, err := tx.Exec(ctx, `UPDATE platform.idempotency_keys SET operation=$4, request_hash=$5, status='in_progress', response=NULL,
				created_at=platform.app_now(), expires_at=platform.app_now()+interval '24 hours' WHERE tenant_id=$1 AND membership_id=$2 AND key=$3`,
				p.TenantID, p.MembershipID, key, op, h)
			return err
		}
		if gotOp != op || hex.EncodeToString(gotHash) != hash {
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
func (s Idempotency) Complete(ctx context.Context, p *ops.Principal, op, key string, resp json.RawMessage) error {
	if p == nil {
		return nil
	}
	return s.tx(ctx, p, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE platform.idempotency_keys SET status='completed', http_status=200, response=$4
			WHERE tenant_id=$1 AND membership_id=$2 AND key=$3`, p.TenantID, p.MembershipID, key, resp)
		return err
	})
}

// Abort releases a key after a failed attempt so the client may retry with the same key.
func (s Idempotency) Abort(ctx context.Context, p *ops.Principal, op, key string) {
	if p == nil {
		return
	}
	_ = s.tx(ctx, p, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM platform.idempotency_keys WHERE tenant_id=$1 AND membership_id=$2 AND key=$3 AND status='in_progress'`,
			p.TenantID, p.MembershipID, key)
		return err
	})
}

// Recorder writes audit rows and outbox events in the operation's transaction (ops.Recorder).
type Recorder struct{}

// Record implements ops.Recorder.
func (Recorder) Record(ctx context.Context, tx pgx.Tx, c *ops.Call, op string) error {
	p := c.Principal
	if p == nil {
		return nil
	}
	for _, a := range c.Audits {
		if _, err := tx.Exec(ctx, `INSERT INTO audit.audit_log (tenant_id, actor_id, actor_role_at_time, membership_id, action, target_kind, target_id,
			previous_version, next_version, occurred_at, correlation_id, result, reason)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'success',NULLIF($12,''))`,
			p.TenantID, p.UserID.String(), p.Role, p.MembershipID, a.Action, a.TargetKind, a.TargetID,
			a.PreviousVersion, a.NextVersion, c.Now, c.CorrelationID, a.Reason); err != nil {
			return err
		}
	}
	for _, e := range c.Events {
		payload, err := json.Marshal(e.Payload)
		if err != nil {
			return err
		}
		if e.Payload == nil {
			payload = []byte("{}")
		}
		if _, err := tx.Exec(ctx, `INSERT INTO platform.outbox (tenant_id, aggregate_type, aggregate_id, event_type, payload, correlation_id, occurred_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7)`, p.TenantID, e.AggregateType, e.AggregateID, e.Type, payload, c.CorrelationID, c.Now); err != nil {
			return err
		}
	}
	return nil
}

// newPool opens a pool whose timestamptz values scan in UTC, so every Instant leaves the API with a Z offset
// regardless of the server's local time zone.
func newPool(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	cfg.AfterConnect = func(_ context.Context, conn *pgx.Conn) error {
		conn.TypeMap().RegisterType(&pgtype.Type{Name: "timestamptz", OID: pgtype.TimestamptzOID, Codec: &pgtype.TimestamptzCodec{ScanLocation: time.UTC}})
		return nil
	}
	return pgxpool.NewWithConfig(ctx, cfg)
}
