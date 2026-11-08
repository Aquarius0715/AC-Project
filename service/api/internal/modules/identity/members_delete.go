package identity

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// MemberDeleteInput is members.delete input.
type MemberDeleteInput struct {
	ID     uuid.UUID `json:"id"`
	Reason string    `json:"reason"`
}

// Validate implements ops.Validator.
func (in *MemberDeleteInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.ID == uuid.Nil {
		fieldErrs["id"] = apperr.KeyRequired
	}

	in.Reason = strings.TrimSpace(in.Reason)
	if n := utf8.RuneCountInString(in.Reason); n < 1 || n > 1000 {
		fieldErrs[keyReason] = apperr.KeyLength
	}

	return fieldErrs
}

// membersDelete deletes an account made by mistake, with its person (DEC-103, IR657): the account has never acted
// (no audit entry names it), has no assignment or certificate, is not the caller's own, and is the person's only
// account. The account's entry in a customer's or a contractor company's user list goes with it. The person's
// provider account is deleted afterwards (event AccountDeleted). Otherwise CONFLICT with the reason; end the access
// instead.
//
//	@Summary		members.delete (write)
//	@ID				members.delete
//	@Description	Authorization: admin:identity.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR657 only an account that never acted (no audit entry names it) and has no assignment or \
//	@Description	certificate (CONFLICT error.memberUsed); not the caller own (error.memberSelf); the only account of \
//	@Description	its person (error.memberHasOthers); deletes the person, its entry in a user list and the provider \
//	@Description	account
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A03
//	@Tags			members
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string	true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer	true	"all: required (target members, read members.list)"
//	@Param			id					path		string	true	"input field id"
//	@Param			reason				query		string	false	"input field reason"
//	@Success		200					{object}	ops.Envelope{data=Deleted}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/members/{id} [delete]
func membersDelete(ctx context.Context, call *ops.Call, input *MemberDeleteInput) (Deleted, error) {
	var (
		user    uuid.UUID
		version int
	)

	err := call.Tx.QueryRow(
		ctx,
		`SELECT user_id, version FROM identity.memberships WHERE id = $1 FOR UPDATE`,
		input.ID,
	).Scan(&user, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return Deleted{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return Deleted{}, apperr.Wrap(err, "select identity.memberships")
	}

	if call.ExpectedVersion != nil && version != *call.ExpectedVersion {
		return Deleted{}, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	refusal := refuseMemberDelete(ctx, call, input.ID, user)
	if refusal != nil {
		return Deleted{}, refusal
	}

	var subject *string

	err = call.Tx.QueryRow(ctx, `SELECT cognito_sub FROM identity.users WHERE id = $1`, user).
		Scan(&subject)
	if err != nil {
		return Deleted{}, apperr.Wrap(err, "select identity.users")
	}

	for _, stmt := range []string{
		`DELETE FROM identity.consents WHERE membership_id = $1`,
		`DELETE FROM identity.client_users WHERE membership_id = $1`,
		`DELETE FROM identity.partner_users WHERE membership_id = $1`,
		`DELETE FROM identity.memberships WHERE id = $1`,
	} {
		_, execErr := call.Tx.Exec(ctx, stmt, input.ID)
		if execErr != nil {
			return Deleted{}, apperr.Wrap(execErr, "delete identity.memberships")
		}
	}

	for _, stmt := range []string{
		`DELETE FROM identity.preferences WHERE user_id = $1`,
		`DELETE FROM identity.two_factor WHERE user_id = $1`,
		`DELETE FROM identity.users WHERE id = $1`,
	} {
		_, execErr := call.Tx.Exec(ctx, stmt, user)
		if execErr != nil {
			return Deleted{}, apperr.Wrap(execErr, "delete identity.users")
		}
	}

	if subject != nil {
		call.Emit(
			ops.Event{
				AggregateType: kindMembership,
				AggregateID:   input.ID,
				Type:          EventAccountDeleted,
				Payload:       map[string]any{"subject": *subject},
			},
		)
	}

	call.Emit(
		ops.Event{
			AggregateType: kindMembership,
			AggregateID:   input.ID,
			Type:          "MembershipDeleted",
			Payload:       nil,
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:          "members.delete",
			TargetKind:      kindMembership,
			TargetID:        input.ID.String(),
			PreviousVersion: call.ExpectedVersion,
			Reason:          input.Reason, NextVersion: nil, Result: "", Before: nil, After: nil,
		},
	)

	return Deleted{ID: input.ID, Deleted: true}, nil
}

// refuseMemberDelete is the CONFLICT for an account that is not deleted, or nil.
func refuseMemberDelete(ctx context.Context, call *ops.Call, membership, user uuid.UUID) error {
	if membership == call.Principal.MembershipID {
		return apperr.E(apperr.Conflict, "error.memberSelf")
	}

	var others, acted bool

	err := call.Tx.QueryRow(
		ctx,
		`SELECT EXISTS (SELECT 1 FROM identity.memberships WHERE user_id = $2 AND id <> $1),
		EXISTS (SELECT 1 FROM audit.audit_log WHERE membership_id = $1)`,
		membership,
		user,
	).Scan(&others, &acted)
	if err != nil {
		return apperr.Wrap(err, "select identity.memberships")
	}

	switch {
	case others:
		return apperr.E(apperr.Conflict, "error.memberHasOthers")
	case acted:
		return apperr.E(apperr.Conflict, "error.memberUsed")
	}

	held, err := ops.Query[bool](
		ctx,
		call,
		ops.QueryMembershipRecords,
		ops.MembershipRecordsInput{MembershipID: membership},
	)
	if err != nil {
		return apperr.Wrap(err, "ask "+ops.QueryMembershipRecords)
	}

	if held {
		return apperr.E(apperr.Conflict, "error.memberUsed")
	}

	return nil
}
