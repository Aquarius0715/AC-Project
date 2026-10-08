// Package apperr implements DomainError from service-contracts.ts and its fixed HTTP mapping (backend Go design §6).
package apperr

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Code string

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
var Codes = []Code{Validation, Unauthenticated, Forbidden, NotFound, Conflict, Offline, Timeout, RateLimited, Unavailable}

var status = map[Code]int{
	Validation:      http.StatusUnprocessableEntity,
	Unauthenticated: http.StatusUnauthorized,
	Forbidden:       http.StatusForbidden,
	NotFound:        http.StatusNotFound,
	Conflict:        http.StatusConflict,
	Offline:         http.StatusConflict,
	Timeout:         http.StatusGatewayTimeout,
	RateLimited:     http.StatusTooManyRequests,
	Unavailable:     http.StatusServiceUnavailable,
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

func caused(de *DomainError, err error) *DomainError { de.cause = err; return de }

// HTTPStatus returns the fixed status for the error code.
func (e *DomainError) HTTPStatus() int { return status[e.Code] }

// StatusCode implements echo.HTTPStatusCoder, so Echo's central error handler and the request logger see the status.
func (e *DomainError) StatusCode() int { return e.HTTPStatus() }

// E creates a DomainError with an empty fieldErrors map.
func E(c Code, messageKey string) *DomainError {
	return &DomainError{Code: c, MessageKey: messageKey, FieldErrors: map[string]string{}}
}

// Fields creates a VALIDATION error with field messages.
func Fields(fe map[string]string) *DomainError {
	return &DomainError{Code: Validation, MessageKey: "error.validation", FieldErrors: fe}
}

// RetryAfter sets retryAfterSeconds.
func (e *DomainError) RetryAfter(s int) *DomainError { e.RetryAfterSeconds = &s; return e }

// From converts any error into a DomainError (PostgreSQL errors, deadlines, unknown errors).
func From(err error) *DomainError {
	if err == nil {
		return nil
	}
	var de *DomainError
	if errors.As(err, &de) {
		return de
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return caused(E(Timeout, "error.timeout"), err)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return caused(E(NotFound, "error.notFound"), err)
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23514", "22P02", "22001", "22003", "22007", "22008":
			return caused(E(Validation, "error.validation"), err)
		case "42501":
			if strings.HasPrefix(pg.Message, "permission denied") { // a missing table privilege is a defect (IR194), not RLS
				return caused(E(Unavailable, "error.unavailable"), err)
			}
			return caused(E(NotFound, "error.notFound"), err) // row-level security: never reveal other tenants
		case "23503":
			return caused(E(NotFound, "error.notFound"), err)
		case "23505", "23P01":
			return caused(E(Conflict, "error.conflict"), err)
		case "40001", "40P01":
			return caused(E(Unavailable, "error.unavailable").RetryAfter(1), err)
		case "57014":
			return caused(E(Timeout, "error.timeout"), err)
		}
	}
	return caused(E(Unavailable, "error.unavailable"), err)
}
