// Package writes implements writes.getResult (DDC-03, D04) over the idempotency store.
package writes

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
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
	Result      json.RawMessage `json:"result,omitempty" swaggertype:"object"`
}

// get returns the caller's own stored write (IR145): absent, expired or another operation → not_received;
// in progress → pending; completed → succeeded with the stored result and its id as resourceIds. Rejected writes are
// not stored (zero side effects), so they read as not_received and the same intent may be retried.
//
//	@Summary		writes.getResult (read)
//	@ID				writes.getResult
//	@Description	Authorization: authenticated:original-user-and-current-target-scope
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DDC-03
//	@Tags			writes
//	@Accept			json
//	@Produce		json
//	@Param			idempotencyKey	path		string	true	"input field idempotencyKey"
//	@Param			operation		query		string	false	"input field operation"
//	@Success		200				{object}	ops.Envelope{data=Result}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/writes/{idempotencyKey} [get]
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
		Data json.RawMessage `json:"data" swaggertype:"object"`
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

// @Summary		auth.previewPasswordReset (read)
// @ID				auth.previewPasswordReset
// @Description	Authorization: public:demo-only
// @Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
// @Description	Recovery: D04: retry only UNAVAILABLE, at most twice
// @Description	Design: DDC-07
// @Tags			auth
// @Accept			json
// @Produce		json
// @Param			request	body		ResetInput	true	"input"
// @Success		200		{object}	ops.Envelope{data=ResetPreview}
// @Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504		{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/auth/password-reset-preview [post]
func previewReset(_ context.Context, _ *ops.Call, _ *ResetInput) (ResetPreview, error) {
	return ResetPreview{MessageKey: "auth.reset_generic", DeliveryState: "preview"}, nil
}

// Register binds writes.getResult and auth.previewPasswordReset.
func Register(r *ops.Registry) {
	ops.Register(r, "writes.getResult", get)
	ops.Register(r, "auth.previewPasswordReset", previewReset)
}
