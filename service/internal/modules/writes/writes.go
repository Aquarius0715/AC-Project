// Package writes implements writes.getResult (DDC-03, D04) over the idempotency store.
package writes

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/internal/ops"
	"github.com/pradita/ac-project/service/internal/platform/apperr"
)

// Input is writes.getResult input.
type Input struct {
	Operation      string `json:"operation"`
	IdempotencyKey string `json:"idempotencyKey"`
}

// Validate implements ops.Validator.
func (in *Input) Validate() map[string]string {
	fe := map[string]string{}
	if strings.TrimSpace(in.Operation) == "" {
		fe["operation"] = "error.required"
	}
	if n := len(in.IdempotencyKey); n < 8 || n > 128 {
		fe["idempotencyKey"] = "error.invalid"
	}
	return fe
}

// Result is WriteResult.
type Result struct {
	State       string          `json:"state"`
	Operation   string          `json:"operation,omitempty"`
	ResourceIDs []string        `json:"resourceIds,omitempty"`
	Result      json.RawMessage `json:"result,omitempty"`
}

// get returns the caller's own stored write (IR145): absent, expired or another operation → not_received;
// in progress → pending; completed → succeeded with the stored result and its id as resourceIds. Rejected writes are
// not stored (zero side effects), so they read as not_received and the same intent may be retried.
func get(ctx context.Context, c *ops.Call, in *Input) (Result, error) {
	if _, ok := ops.SpecByName()[in.Operation]; !ok {
		return Result{}, apperr.Fields(map[string]string{"operation": "error.invalid"})
	}
	var status string
	var resp []byte
	err := c.Tx.QueryRow(ctx, `SELECT status, response FROM platform.idempotency_keys
		WHERE membership_id = $1 AND key = $2 AND operation = $3 AND expires_at > $4`, c.Principal.MembershipID, in.IdempotencyKey, in.Operation, c.Now).Scan(&status, &resp)
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{State: "not_received"}, nil
	}
	if err != nil {
		return Result{}, err
	}
	if status != "completed" {
		return Result{State: "pending"}, nil
	}
	var stored struct {
		Data json.RawMessage `json:"data"`
	}
	_ = json.Unmarshal(resp, &stored)
	var ids struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(stored.Data, &ids)
	out := Result{State: "succeeded", Operation: in.Operation, ResourceIDs: []string{}, Result: stored.Data}
	if ids.ID != "" {
		out.ResourceIDs = []string{ids.ID}
	}
	return out, nil
}

// ResetInput is auth.previewPasswordReset input.
type ResetInput struct {
	DemoEmail string `json:"demoEmail"`
}

// Validate implements ops.Validator.
func (in *ResetInput) Validate() map[string]string {
	in.DemoEmail = strings.TrimSpace(in.DemoEmail)
	if !strings.Contains(in.DemoEmail, "@") || len(in.DemoEmail) > 254 {
		return map[string]string{"demoEmail": "error.invalid"}
	}
	return nil
}

// ResetPreview is ResetPreview: the same generic message for every address (no account enumeration).
type ResetPreview struct {
	MessageKey    string `json:"messageKey"`
	DeliveryState string `json:"deliveryState"`
}

func previewReset(_ context.Context, _ *ops.Call, _ *ResetInput) (ResetPreview, error) {
	return ResetPreview{MessageKey: "auth.reset_generic", DeliveryState: "preview"}, nil
}

// Register binds writes.getResult and auth.previewPasswordReset.
func Register(r *ops.Registry) {
	ops.Register(r, "writes.getResult", get)
	ops.Register(r, "auth.previewPasswordReset", previewReset)
}
