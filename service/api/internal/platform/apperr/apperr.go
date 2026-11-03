// Package apperr implements DomainError from service-contracts.ts and its fixed HTTP mapping (backend Go design §6).
package apperr

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Code is the ErrorCode union of service-contracts.ts.
type Code string

// The error codes, in the order of the ErrorCode union.
const (
	Validation      Code = "VALIDATION"
	Unauthenticated Code = "UNAUTHENTICATED"
	Forbidden       Code = "FORBIDDEN"
	NotFound        Code = "NOT_FOUND"
	Conflict        Code = "CONFLICT"
	Offline         Code = "OFFLINE"
	Timeout         Code = "TIMEOUT"
	RateLimited     Code = "RATE_LIMITED"
	Unavailable     Code = "UNAVAILABLE"
)

// Codes lists every ErrorCode of service-contracts.ts in declaration order.
func Codes() []Code {
	return []Code{
		Validation,
		Unauthenticated,
		Forbidden,
		NotFound,
		Conflict,
		Offline,
		Timeout,
		RateLimited,
		Unavailable,
	}
}

// DomainError is the wire shape {code, messageKey, fieldErrors, correlationId, retryAfterSeconds}.
type DomainError struct {
	Code              Code              `json:"code"`
	MessageKey        string            `json:"messageKey"`
	FieldErrors       map[string]string `json:"fieldErrors"`
	CorrelationID     string            `json:"correlationId"`
	RetryAfterSeconds *int              `json:"retryAfterSeconds"`

	cause error // the converted error (database, deadline, unknown); logged for 5xx, never sent
}

func (e *DomainError) Error() string { return string(e.Code) + ": " + e.MessageKey }

// Cause returns the error this DomainError was converted from (nil for errors created by E / Fields).
func (e *DomainError) Cause() error { return e.cause }

func caused(de *DomainError, err error) *DomainError {
	de.cause = err

	return de
}

// HTTPStatus returns the fixed status for the error code.
func (e *DomainError) HTTPStatus() int {
	switch e.Code {
	case Validation:
		return http.StatusUnprocessableEntity
	case Unauthenticated:
		return http.StatusUnauthorized
	case Forbidden:
		return http.StatusForbidden
	case NotFound:
		return http.StatusNotFound
	case Conflict, Offline:
		return http.StatusConflict
	case Timeout:
		return http.StatusGatewayTimeout
	case RateLimited:
		return http.StatusTooManyRequests
	case Unavailable:
		return http.StatusServiceUnavailable
	}

	return 0
}

// StatusCode implements echo.HTTPStatusCoder, so Echo's central error handler and the request logger see the status.
func (e *DomainError) StatusCode() int { return e.HTTPStatus() }

// E creates a DomainError with an empty fieldErrors map.
func E(c Code, messageKey string) *DomainError {
	return &DomainError{
		Code:              c,
		MessageKey:        messageKey,
		FieldErrors:       map[string]string{},
		CorrelationID:     "",
		RetryAfterSeconds: nil,
		cause:             nil,
	}
}

// Fields creates a VALIDATION error with field messages.
func Fields(fe map[string]string) *DomainError {
	return &DomainError{
		Code:              Validation,
		MessageKey:        KeyValidation,
		FieldErrors:       fe,
		CorrelationID:     "",
		RetryAfterSeconds: nil,
		cause:             nil,
	}
}

// RetryAfter sets retryAfterSeconds.
func (e *DomainError) RetryAfter(s int) *DomainError {
	e.RetryAfterSeconds = &s

	return e
}

// Wrap says where err happened (the statement or the call that failed) for the log of a 5xx: every error from
// another package carries that context (wrapcheck). Nil stays nil, and From still finds the DomainError, the
// PostgreSQL error or the deadline inside.
func Wrap(err error, where string) error {
	if err == nil {
		return nil
	}

	return fmt.Errorf("%s: %w", where, err)
}

// From converts any error into a DomainError (PostgreSQL errors, deadlines, unknown errors).
func From(err error) *DomainError {
	if err == nil {
		return nil
	}

	de, matched := errors.AsType[*DomainError](err)
	if matched {
		return de
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return caused(E(Timeout, KeyTimeout), err)
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return caused(E(NotFound, KeyNotFound), err)
	}

	pgError, matched := errors.AsType[*pgconn.PgError](err)
	if matched {
		converted := fromPostgres(pgError)
		if converted != nil {
			return caused(converted, err)
		}
	}

	return caused(E(Unavailable, KeyUnavailable), err)
}

// fromPostgres maps the PostgreSQL error codes with a meaning of their own; nil for any other code.
func fromPostgres(pgError *pgconn.PgError) *DomainError {
	switch pgError.Code {
	case "23514", "22P02", "22001", "22003", "22007", "22008":
		return E(Validation, KeyValidation)
	case "42501":
		if strings.HasPrefix(
			pgError.Message,
			"permission denied",
		) { // a missing table privilege is a defect (IR194), not RLS
			return E(Unavailable, KeyUnavailable)
		}

		return E(NotFound, KeyNotFound) // row-level security: never reveal other tenants
	case "23503":
		return E(NotFound, KeyNotFound)
	case "23505", "23P01":
		return E(Conflict, KeyConflict)
	case "40001", "40P01":
		return E(Unavailable, KeyUnavailable).RetryAfter(1)
	case "57014":
		return E(Timeout, KeyTimeout)
	}

	return nil
}
