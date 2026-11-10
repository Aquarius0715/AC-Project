// Package writes implements writes.getResult (DDC-03, D04) over the idempotency store.
package writes

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

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
	fieldErrs := map[string]string{}
	if strings.TrimSpace(in.Operation) == "" {
		fieldErrs["operation"] = apperr.KeyRequired
	}

	if n := utf8.RuneCountInString(
		in.IdempotencyKey,
	); n < 8 ||
		n > 128 { // 8–128 characters (review-resolution-contracts writes.getResult)
		fieldErrs["idempotencyKey"] = apperr.KeyInvalid
	}

	return fieldErrs
}

// Result is WriteResult.
type Result struct {
	State       string          `json:"state"`
	Operation   string          `json:"operation,omitempty"`
	ResourceIDs []string        `json:"resourceIds,omitempty"`
	Result      json.RawMessage `json:"result,omitempty"      swaggertype:"object"`
}

// get returns the caller's own stored write (IR145): absent, expired or another operation → not_received;
// in progress → pending; completed → succeeded with the stored result and its id as resourceIds. Rejected writes are
// not stored (zero side effects), so they read as not_received and the same intent may be retried. A write whose
// target the caller can no longer read (an assignment, a delegation or a unit scope that ended) is succeeded without
// the result (IR724): the write happened, so it is not sent again, but what it answered then is not shown.
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
func get(ctx context.Context, call *ops.Call, input *Input) (Result, error) {
	if _, ok := ops.SpecByName()[input.Operation]; !ok {
		return Result{}, apperr.Fields(map[string]string{"operation": apperr.KeyInvalid})
	}

	var (
		status string
		resp   []byte
	)

	err := call.Tx.QueryRow(ctx, `SELECT status, response FROM platform.idempotency_keys
		WHERE membership_id = $1 AND key = $2 AND operation = $3 AND expires_at >
		$4`, call.Principal.MembershipID, input.IdempotencyKey, input.Operation, call.Now).Scan(&status, &resp)
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{State: "not_received", Operation: "", ResourceIDs: nil, Result: nil}, nil
	}

	if err != nil {
		return Result{}, apperr.Wrap(err, "select platform.idempotency_keys")
	}

	if status != "completed" {
		return Result{State: "pending", Operation: "", ResourceIDs: nil, Result: nil}, nil
	}

	var stored struct {
		Data json.RawMessage `json:"data" swaggertype:"object"`
	}

	_ = json.Unmarshal(resp, &stored)

	var ids struct {
		ID string `json:"id"`
	}

	_ = json.Unmarshal(stored.Data, &ids)

	out := Result{
		State:       "succeeded",
		Operation:   input.Operation,
		ResourceIDs: []string{},
		Result:      stored.Data,
	}
	if ids.ID != "" {
		out.ResourceIDs = []string{ids.ID}
	}

	readable, err := targetReadable(ctx, call, input.Operation, stored.Data)
	if err != nil {
		return Result{}, err
	}

	if !readable {
		out.Result = nil
	}

	return out, nil
}

// target is the read that rechecks a write's target as the caller: the read operation and its ID field, filled from
// the field of the stored result.
type target struct {
	read, field, from string
}

// The ID fields of the targets.
const (
	fieldID     = "id"
	fieldJobID  = "jobId"
	fieldDevice = "deviceId"
)

// byID is the target read with {id} from the result's id.
func byID(read string) target { return target{read: read, field: fieldID, from: fieldID} }

// job is the job read with jobs.get from the result's field from.
func job(from string) target { return target{read: "jobs.get", field: fieldJobID, from: from} }

// device is the device read with devices.get from the result's field from.
func device(from string) target { return target{read: "devices.get", field: fieldID, from: from} }

// targets are the writes whose target is held to the caller's current scope: assignments, delegations, unit scopes
// and the all-target-units condition of restrictions (strict review SR14, SR03/IR19). A stored result of one of them
// is returned only while the caller may still read the target (IR724). Other writes keep returning it: the caller's
// own records (preferences, consents, notifications, sessions), the receipts the original user keeps (jobs.accept and
// jobs.decline, IR01), deletions, and HQ settings without a unit, job or device scope.
//
//nolint:gochecknoglobals // a fixed table, read only
var targets = map[string]target{
	"alerts.acknowledge":      byID("alerts.get"),
	"alerts.resolve":          byID("alerts.get"),
	"attachments.add":         job(fieldJobID),
	"commands.create":         byID("commands.get"),
	"devices.addResponseNote": device(fieldDevice),
	"devices.bind":            device(fieldID),
	"devices.calibrate":       device(fieldDevice),
	"devices.check":           device(fieldDevice),
	"devices.register":        device(fieldID),
	"devices.setMqttNode":     device(fieldID),
	"devices.unbind":          device(fieldID),
	"devices.updateFirmware":  device(fieldDevice),
	"diagnosticRuns.create": {
		read:  "diagnosticRuns.get",
		field: "diagnosticRunId",
		from:  "id",
	},
	"filterCare.markCleaned":     {read: "units.get", field: fieldID, from: "unitId"},
	"jobs.acknowledgeAssignment": job(fieldJobID),
	"jobs.addNote":               job(fieldJobID),
	"jobs.assign":                job(fieldID),
	"jobs.cancel":                job(fieldID),
	"jobs.checkIn":               job(fieldID),
	"jobs.classifyFollowUp":      job(fieldID),
	"jobs.create":                job(fieldID),
	"jobs.extendAccess":          job(fieldID),
	"jobs.hold":                  job(fieldID),
	"jobs.offer":                 job(fieldID),
	"jobs.pauseWork":             job(fieldID),
	"jobs.proposePartnerSlot":    job(fieldID),
	"jobs.proposeSlot":           job(fieldID),
	"jobs.rate":                  job(fieldID),
	"jobs.recordWarrantyClaim":   job(fieldID),
	"jobs.reportProblem":         job(fieldID),
	"jobs.requestReschedule":     job(fieldID),
	"jobs.resolvePartnerSlot":    job(fieldID),
	"jobs.respondProposal":       job(fieldID),
	"jobs.resumeHold":            job(fieldID),
	"jobs.resumeRework":          job(fieldID),
	"jobs.review":                job(fieldID),
	"jobs.saveCost":              job(fieldID),
	"jobs.saveDraft":             job(fieldJobID),
	"jobs.start":                 job(fieldID),
	"jobs.submit":                job(fieldID),
	"jobs.withdrawPartnerSlot":   job(fieldID),
	"jobs.withdrawProposal":      job(fieldID),
	"plans.generateNext":         job(fieldID),
	"reports.signOff":            job(fieldJobID),
	"restrictions.cancel":        byID("restrictions.get"),
	"restrictions.defer":         byID("restrictions.get"),
	"restrictions.execute":       byID("restrictions.get"),
	"restrictions.exempt":        byID("restrictions.get"),
	"restrictions.override":      byID("restrictions.get"),
	"restrictions.reconcile":     byID("restrictions.get"),
	"restrictions.release":       byID("restrictions.get"),
	"restrictions.retry":         byID("restrictions.get"),
	"restrictions.schedule":      byID("restrictions.get"),
	"units.save":                 byID("units.get"),
	"units.setAlertPolicies":     byID("units.get"),
}

// targetReadable reports whether the caller may still read the target of a stored result of operation (IR724): a
// write outside targets is always readable; one whose stored result does not name its target is not.
func targetReadable(
	ctx context.Context,
	call *ops.Call,
	operation string,
	data json.RawMessage,
) (bool, error) {
	check, ok := targets[operation]
	if !ok {
		return true, nil
	}

	var fields map[string]any

	err := json.Unmarshal(data, &fields)
	if err != nil {
		return false, apperr.Wrap(err, "decode the stored result")
	}

	id, _ := fields[check.from].(string)
	if id == "" {
		return false, nil
	}

	readable, err := ops.Readable(ctx, call, check.read, map[string]string{check.field: id})

	return readable, apperr.Wrap(err, "ops.Readable "+check.read)
}

// ResetInput is auth.previewPasswordReset input.
type ResetInput struct {
	DemoEmail string `json:"demoEmail"`
}

// Validate implements ops.Validator.
func (in *ResetInput) Validate() map[string]string {
	in.DemoEmail = strings.TrimSpace(in.DemoEmail)
	if !strings.Contains(in.DemoEmail, "@") || len(in.DemoEmail) > 254 {
		return map[string]string{"demoEmail": apperr.KeyInvalid}
	}

	return nil
}

// ResetPreview is ResetPreview: the same generic message for every address (no account enumeration).
type ResetPreview struct {
	MessageKey    string `json:"messageKey"`
	DeliveryState string `json:"deliveryState"`
}

// previewReset serves auth.previewPasswordReset.
//
//	@Summary		auth.previewPasswordReset (read)
//	@ID				auth.previewPasswordReset
//	@Description	Authorization: public:demo-only
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DDC-07
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			request	body		ResetInput	true	"input"
//	@Success		200		{object}	ops.Envelope{data=ResetPreview}
//	@Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504		{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/auth/password-reset-preview [post]
func previewReset(_ context.Context, _ *ops.Call, _ *ResetInput) (ResetPreview, error) {
	return ResetPreview{MessageKey: "auth.reset_generic", DeliveryState: "preview"}, nil
}

// Register binds writes.getResult and auth.previewPasswordReset.
func Register(r *ops.Registry) {
	ops.Register(r, "writes.getResult", get)
	ops.Register(r, "auth.previewPasswordReset", previewReset)
	ops.RegisterReadChecks(r)
}
