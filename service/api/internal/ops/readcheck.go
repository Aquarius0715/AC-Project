package ops

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/authz"
)

// readCheckQuery is the prefix of the internal query of each domain that runs one of its read operations as the
// caller (IR724): readCheckQuery + domain.
const readCheckQuery = "reads.check."

// checkedOperation is the input field of a read check that names its read operation.
const checkedOperation = "operation"

// readCheckIn is the input of a read check: the read operation and its input.
type readCheckIn struct {
	Operation string          `json:"operation"`
	Input     json.RawMessage `json:"input"     swaggertype:"object"`
}

// Validate implements Validator.
func (in *readCheckIn) Validate() map[string]string {
	if strings.TrimSpace(in.Operation) == "" {
		return map[string]string{checkedOperation: apperr.KeyRequired}
	}

	return nil
}

// readCheck is the answer of a read check.
type readCheck struct {
	Readable bool `json:"readable"`
}

// RegisterReadChecks binds the read check of every domain (IR724).
func RegisterReadChecks(reg *Registry) {
	for _, domain := range Domains {
		RegisterQuery(reg, domain, readCheckQuery+domain, readChecker(domain))
	}
}

// Readable reports whether the caller may run the read operation with input now, as a request of its own would: the
// operation's predicates, then its handler's scope checks on the current rows, in the service that owns it.
// writes.getResult asks it before it returns a stored write result (strict review SR14, IR724). A read refused as
// NOT_FOUND or FORBIDDEN is false; any other error is returned.
func Readable(ctx context.Context, call *Call, operation string, input any) (bool, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return false, apperr.Wrap(err, "json.Marshal")
	}

	out, err := Query[readCheck](
		ctx,
		call,
		readCheckQuery+DomainOf(operation),
		readCheckIn{Operation: operation, Input: raw},
	)

	return out.Readable, err
}

// readChecker is the read check of domain: it runs a read operation of that domain with the caller's principal, its
// predicate alternatives and the call's transaction, and keeps only whether it was allowed.
func readChecker(domain string) func(context.Context, *Call, *readCheckIn) (readCheck, error) {
	return func(ctx context.Context, call *Call, check *readCheckIn) (readCheck, error) {
		var refused readCheck

		operation, ok := call.Queries.ops[check.Operation]
		if !ok || operation.Spec.Mode != Read || operation.OwnTx ||
			DomainOf(check.Operation) != domain {
			return refused, apperr.Fields(map[string]string{checkedOperation: apperr.KeyInvalid})
		}

		input, err := decodeInput(operation, check.Input)
		if err != nil {
			return refused, err
		}

		var principal *authz.Principal
		if call.Principal != nil {
			principal = &call.Principal.Principal
		}

		cands := operation.Expr.Candidates(principal)
		if len(cands) == 0 {
			return refused, nil
		}

		read := *call
		read.Candidates = cands

		_, err = operation.Handle(ctx, &read, input)
		if err != nil && readRefused(err) {
			return refused, nil
		}

		return readCheck{Readable: err == nil}, err
	}
}

// readRefused reports whether a read was refused for the caller's access: out of scope (NOT_FOUND, D01) or not
// permitted (FORBIDDEN).
func readRefused(err error) bool {
	code := apperr.From(err).Code

	return code == apperr.NotFound || code == apperr.Forbidden
}
