// Package authz parses the authorization column of operation-catalog.csv and evaluates it against a Principal
// (backend Go design §3). Grammar of the main clause (text before the first ';'):
//
//	expr        = alternative { "|" alternative }
//	alternative = role ":" [ permission ":" ] predicate { ":" predicate }   (role-only "role:predicate" is allowed)
//	            | "IR" digits ":" note { ":" note }                           (replay rule annotation: never grants)
//
// A permission is one of the 38 Permission values; anything else after the role is a named predicate. Predicates
// are scope rules evaluated by the module with the target loaded (for example self, assigned-valid-job); the
// dispatcher only checks role and permission, and the handler must call Check for the predicates.
package authz

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

var (
	errEmptyAlternative       = errors.New("empty alternative")
	errUnknownRole            = errors.New("unknown role")
	errBadPredicate           = errors.New("bad permission or predicate")
	errAdminWithoutPermission = errors.New("admin alternative without permission")
	errNoGrantingAlternative  = errors.New("no granting alternative")
)

// The roles of a Membership (Membership.role of service-contracts.ts), and RoleSystem, the role of the system's own
// work: scheduler jobs, event consumers and internal queries without a user.
const (
	RoleClient     = "client"
	RoleContractor = "contractor"
	RoleTechnician = "technician"
	RoleAdmin      = "admin"
	RoleSystem     = "system"
)

// isRole reports whether name is a Membership.role or one of the two pseudo roles used in the catalog.
func isRole(name string) bool {
	switch name {
	case RoleClient, RoleContractor, RoleTechnician, RoleAdmin, "authenticated", "public":
		return true
	default:
		return false
	}
}

// permissionNames is the Permission union of service-contracts.ts (38 values).
const permissionNames = "dashboard.read asset.read asset.write identity.read identity.write " +
	"device.read device.write device.maintain control.execute control.diagnose alert.read alert.resolve " +
	"alert.policy.read alert.policy.write job.read job.write contract.read contract.write billing.read " +
	"billing.write billing.payment restriction.read restriction.write restriction.override automation.policy.read " +
	"automation.policy.write energy.read energy.write mrv.read mrv.write mrv.review mrv.factors offset.read " +
	"offset.write audit.read partner.accept partner.assign partner.review"

// Permissions are the Permission union of service-contracts.ts (38 values), in the contract's order.
func Permissions() []string { return strings.Fields(permissionNames) }

// IsPermission reports whether name is one of the contract's permissions.
func IsPermission(name string) bool { return slices.Contains(Permissions(), name) }

var predicateRe = regexp.MustCompile(`^[a-z][a-z0-9_-]*(=[a-z0-9_-]+)?$`)

// Alternative is one role (+ optional permission) with predicates.
type Alternative struct {
	Role       string
	Permission string
	Predicates []string
}

// Expr is a parsed authorization column.
type Expr struct {
	Alternatives []Alternative
	Notes        []string // replay annotations such as IR01:same-key-receipt
	Remark       string   // text after the first ';' (e.g. "billing fields require billing.read")
}

// Parse parses an authorization column value.
func Parse(column string) (Expr, error) {
	var expr Expr

	main := column
	if before, after, ok := strings.Cut(column, ";"); ok {
		main, expr.Remark = before, strings.TrimSpace(after)
	}

	for raw := range strings.SplitSeq(main, "|") {
		alt := strings.TrimSpace(raw)
		if alt == "" {
			return expr, fmt.Errorf("%w in %q", errEmptyAlternative, column)
		}

		parts := strings.Split(alt, ":")
		if regexp.MustCompile(`^IR\d+$`).MatchString(parts[0]) {
			expr.Notes = append(expr.Notes, alt)

			continue
		}

		alternative, err := parseAlternative(parts, column)
		if err != nil {
			return expr, err
		}

		expr.Alternatives = append(expr.Alternatives, alternative)
	}

	if len(expr.Alternatives) == 0 {
		return expr, fmt.Errorf("%w in %q", errNoGrantingAlternative, column)
	}

	return expr, nil
}

// parseAlternative parses one granting alternative of the column: a role, an optional permission, then predicates.
func parseAlternative(parts []string, column string) (Alternative, error) {
	if !isRole(parts[0]) {
		return Alternative{}, fmt.Errorf("%w %q in %q", errUnknownRole, parts[0], column)
	}

	alternative := Alternative{Role: parts[0], Permission: "", Predicates: nil}

	rest := parts[1:]
	if len(rest) > 0 && IsPermission(rest[0]) {
		alternative.Permission, rest = rest[0], rest[1:]
	}

	for _, predicate := range rest {
		if strings.Contains(predicate, ".") || !predicateRe.MatchString(predicate) {
			return Alternative{}, fmt.Errorf("%w %q in %q", errBadPredicate, predicate, column)
		}

		alternative.Predicates = append(alternative.Predicates, predicate)
	}

	if alternative.Role == "admin" && alternative.Permission == "" {
		return Alternative{}, fmt.Errorf("%w in %q", errAdminWithoutPermission, column)
	}

	return alternative, nil
}

// Principal is the authenticated caller (selected Membership).
type Principal struct {
	Role        string // client | contractor | technician | admin
	Permissions map[string]bool
	ClientRole  string // owner | member (client only)
}

// Candidates returns the alternatives whose role and permission the principal satisfies; predicates still have to
// be checked by the handler. An empty result means FORBIDDEN.
func (e Expr) Candidates(principal *Principal) []Alternative {
	out := make([]Alternative, 0, len(e.Alternatives))

	for _, alternative := range e.Alternatives {
		switch alternative.Role {
		case "public":
		case "authenticated":
			if principal == nil {
				continue
			}
		default:
			if principal == nil || principal.Role != alternative.Role {
				continue
			}

			if alternative.Permission != "" && !principal.Permissions[alternative.Permission] {
				continue
			}
		}

		out = append(out, alternative)
	}

	return out
}

// Facts are the target facts a handler computes for predicate evaluation.
type Facts map[string]bool

// Allowed reports whether any candidate alternative has all predicates true in facts. Predicates of the form
// key=value are looked up verbatim (for example "kind=alert").
func Allowed(cands []Alternative, f Facts) bool {
	for _, a := range cands {
		matched := true

		for _, pr := range a.Predicates {
			if !f[pr] {
				matched = false

				break
			}
		}

		if matched {
			return true
		}
	}

	return false
}
