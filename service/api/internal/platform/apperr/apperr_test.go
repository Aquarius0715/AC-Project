package apperr_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

var errBoom = errors.New("boom")

func TestStatusMapping(t *testing.T) {
	t.Parallel()

	want := map[apperr.Code]int{
		apperr.Validation:      422,
		apperr.Unauthenticated: 401,
		apperr.Forbidden:       403,
		apperr.NotFound:        404,
		apperr.Conflict:        409,
		apperr.Offline:         409,
		apperr.Timeout:         504,
		apperr.RateLimited:     429,
		apperr.Unavailable:     503,
	}
	for _, code := range apperr.Codes() {
		if got := apperr.E(code, "k").HTTPStatus(); got != want[code] {
			t.Errorf("%s: got %d want %d", code, got, want[code])
		}

		if got := apperr.E(
			code,
			"k",
		).StatusCode(); got != want[code] { // what Echo's error handler and the request log read
			t.Errorf("%s: StatusCode %d want %d", code, got, want[code])
		}
	}
}

// The code list must equal the ErrorCode union in service-contracts.ts.
func TestCodesMatchServiceContracts(t *testing.T) {
	t.Parallel()

	contracts, err := os.ReadFile("../../../../../docs/02-design/service-contracts.ts")
	if err != nil {
		t.Skip("contracts not available:", err)
	}

	match := regexp.MustCompile(`export type ErrorCode = ([^;]+);`).FindSubmatch(contracts)
	if match == nil {
		t.Fatal("ErrorCode not found")
	}

	got := make([]string, 0, len(apperr.Codes()))
	for _, c := range apperr.Codes() {
		got = append(got, "'"+string(c)+"'")
	}

	if strings.Join(got, "|") != string(match[1]) {
		t.Fatalf("codes drift: %s vs %s", strings.Join(got, "|"), match[1])
	}
}

// pgError builds the PostgreSQL error the driver returns for a failed statement.
func pgError(code, message string) *pgconn.PgError {
	return &pgconn.PgError{
		Code:                code,
		Message:             message,
		Severity:            "",
		SeverityUnlocalized: "",
		Detail:              "",
		Hint:                "",
		Position:            0,
		InternalPosition:    0,
		InternalQuery:       "",
		Where:               "",
		SchemaName:          "",
		TableName:           "",
		ColumnName:          "",
		DataTypeName:        "",
		ConstraintName:      "",
		File:                "",
		Line:                0,
		Routine:             "",
	}
}

func TestFrom(t *testing.T) {
	t.Parallel()

	cases := []struct {
		err  error
		want apperr.Code
	}{
		{pgError("23514", ""), apperr.Validation},
		{pgError("22P02", ""), apperr.Validation},
		{pgError("42501", "new row violates row-level security policy"), apperr.NotFound},
		{pgError("42501", "permission denied for table units"), apperr.Unavailable},
		{pgError("23503", ""), apperr.NotFound},
		{pgError("23505", ""), apperr.Conflict},
		{pgError("23P01", ""), apperr.Conflict},
		{pgError("40001", ""), apperr.Unavailable},
		{pgError("40P01", ""), apperr.Unavailable},
		{pgError("57014", ""), apperr.Timeout},
		{pgError("XX000", ""), apperr.Unavailable},
		{context.DeadlineExceeded, apperr.Timeout},
		{pgx.ErrNoRows, apperr.NotFound},
		{fmt.Errorf("wrap: %w", apperr.E(apperr.Offline, "error.offline")), apperr.Offline},
		{errBoom, apperr.Unavailable},
	}
	for _, c := range cases {
		if got := apperr.From(c.err).Code; got != c.want {
			t.Errorf("%v: got %s want %s", c.err, got, c.want)
		}
	}

	if apperr.From(nil) != nil {
		t.Error("nil must stay nil")
	}

	if retryAfter := apperr.From(pgError("40001", "")).RetryAfterSeconds; retryAfter == nil ||
		*retryAfter != 1 {
		t.Error("serialization failure must carry retryAfterSeconds")
	}
}

func TestFieldsAndError(t *testing.T) {
	t.Parallel()

	domainError := apperr.Fields(map[string]string{"name": apperr.KeyRequired})
	if domainError.Code != apperr.Validation ||
		domainError.FieldErrors["name"] != apperr.KeyRequired ||
		domainError.HTTPStatus() != http.StatusUnprocessableEntity {
		t.Fatal("Fields")
	}

	if domainError.Error() != "VALIDATION: error.validation" {
		t.Fatal(domainError.Error())
	}
}

func TestWrap(t *testing.T) {
	t.Parallel()

	if apperr.Wrap(nil, "select assets.units") != nil {
		t.Fatal("a nil error stays nil")
	}

	err := apperr.Wrap(pgx.ErrNoRows, "select assets.units")
	if err.Error() != "select assets.units: no rows in result set" {
		t.Fatalf("the context comes first: %q", err.Error())
	}

	if !errors.Is(err, pgx.ErrNoRows) || apperr.From(err).Code != apperr.NotFound {
		t.Fatal("the wrapped error is still found inside")
	}

	de := apperr.E(apperr.Conflict, apperr.KeyVersionConflict)

	got := apperr.From(apperr.Wrap(apperr.Wrap(de, "jobs.save"), "transaction"))
	if got != de {
		t.Fatalf("From returns the DomainError itself through any number of wraps: %v", got)
	}

	if apperr.From(apperr.Wrap(context.DeadlineExceeded, "tx.Exec")).Code != apperr.Timeout {
		t.Fatal("a wrapped deadline is still a timeout")
	}
}
