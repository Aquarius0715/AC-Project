// Package authz parses the authorization column of operation-catalog.csv and evaluates it against a Principal
// (backend Go design §3). Grammar of the main clause (text before the first ';'):
//
//	expr        = alternative { "|" alternative }
//	alternative = role ":" [ permission ":" ] predicate { ":" predicate }   (role-only "role:predicate" is allowed)
//	            | "IR" digits ":" note { ":" note }                           (replay rule annotation, never grants access)
//
// A permission is one of the 38 Permission values; anything else after the role is a named predicate. Predicates
// are scope rules evaluated by the module with the target loaded (for example self, assigned-valid-job); the
// dispatcher only checks role and permission, and the handler must call Check for the predicates.
package authz

import (
	"fmt"
	"regexp"
	"strings"
)

// Roles of Membership.role plus the two pseudo roles used in the catalog.
var Roles = map[string]bool{"client": true, "contractor": true, "technician": true, "admin": true, "authenticated": true, "public": true}

// Permissions is the Permission union of service-contracts.ts (38 values).
var Permissions = map[string]bool{}

func init() {
	for _, p := range strings.Split("dashboard.read asset.read asset.write identity.read identity.write device.read device.write device.maintain control.execute control.diagnose alert.read alert.resolve alert.policy.read alert.policy.write job.read job.write contract.read contract.write billing.read billing.write billing.payment restriction.read restriction.write restriction.override automation.policy.read automation.policy.write energy.read energy.write mrv.read mrv.write mrv.review mrv.factors offset.read offset.write audit.read partner.accept partner.assign partner.review", " ") {
		Permissions[p] = true
	}
}

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
func Parse(s string) (Expr, error) {
	var e Expr
	main := s
	if i := strings.Index(s, ";"); i >= 0 {
		main, e.Remark = s[:i], strings.TrimSpace(s[i+1:])
	}
	for _, raw := range strings.Split(main, "|") {
		alt := strings.TrimSpace(raw)
		if alt == "" {
			return e, fmt.Errorf("empty alternative in %q", s)
		}
		parts := strings.Split(alt, ":")
		if regexp.MustCompile(`^IR\d+$`).MatchString(parts[0]) {
			e.Notes = append(e.Notes, alt)
			continue
		}
		if !Roles[parts[0]] {
			return e, fmt.Errorf("unknown role %q in %q", parts[0], s)
		}
		a := Alternative{Role: parts[0]}
		rest := parts[1:]
		if len(rest) > 0 && Permissions[rest[0]] {
			a.Permission, rest = rest[0], rest[1:]
		}
		for _, p := range rest {
			if strings.Contains(p, ".") || !predicateRe.MatchString(p) {
				return e, fmt.Errorf("bad permission or predicate %q in %q", p, s)
			}
			a.Predicates = append(a.Predicates, p)
		}
		if a.Role == "admin" && a.Permission == "" {
			return e, fmt.Errorf("admin alternative without permission in %q", s)
		}
		e.Alternatives = append(e.Alternatives, a)
	}
	if len(e.Alternatives) == 0 {
		return e, fmt.Errorf("no granting alternative in %q", s)
	}
	return e, nil
}

// Principal is the authenticated caller (selected Membership).
type Principal struct {
	Role        string // client | contractor | technician | admin
	Permissions map[string]bool
	ClientRole  string // owner | member (client only)
}

// Candidates returns the alternatives whose role and permission the principal satisfies; predicates still have to
// be checked by the handler. An empty result means FORBIDDEN.
func (e Expr) Candidates(p *Principal) []Alternative {
	var out []Alternative
	for _, a := range e.Alternatives {
		switch a.Role {
		case "public":
		case "authenticated":
			if p == nil {
				continue
			}
		default:
			if p == nil || p.Role != a.Role {
				continue
			}
			if a.Permission != "" && !p.Permissions[a.Permission] {
				continue
			}
		}
		out = append(out, a)
	}
	return out
}

// Facts are the target facts a handler computes for predicate evaluation.
type Facts map[string]bool

// Allowed reports whether any candidate alternative has all predicates true in facts. Predicates of the form
// key=value are looked up verbatim (for example "kind=alert").
func Allowed(cands []Alternative, f Facts) bool {
	for _, a := range cands {
		ok := true
		for _, pr := range a.Predicates {
			if !f[pr] {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}
