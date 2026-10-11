package integration

import (
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"testing"

	"github.com/pradita/ac-project/service/api/internal/ops"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

// resultsDecidedAtRunTime are the operations whose result type cannot be held to the contract by its Go type: the
// answer's shape is picked per request. Their tests check each shape.
var resultsDecidedAtRunTime = map[string]string{
	"jobs.get":                     "the IR42 projection of the job (detail, offer, history, summary): the role and the job pick it",
	"jobs.list":                    "the IR42 projections of the listed jobs",
	"restrictions.get":             "the SR03 projection: the whole restriction or the caller's units",
	"restrictions.list":            "the SR03 projections of the listed restrictions",
	"restrictions.reconcile":       "the SR03 projection of the reconciled restriction",
	"restrictions.retry":           "the SR03 projection of the retried restriction",
	"locations.rename":             "a property or a space, by the input's kind",
	"payments.simulate":            "the event picks the answer: payment instructions, an attempt or a confirmation",
	"policies.get":                 "Policy writes the fields of its kind (alert, default alert, automation)",
	"policies.list":                "the listed policies write the fields of their kinds",
	"policies.save":                "Policy writes the fields of its kind",
	"voice.resolveIntent":          "ResolvedIntent writes the fields of the intent it resolved",
	"demoSession.signIn":           "the BFF answers it; the Core API refuses it (errors.session_via_bff, IR154)",
	"demoSession.switchMembership": "the BFF answers it; the Core API refuses it (IR154)",
	"demoSession.extend":           "the BFF answers it; the Core API refuses it (IR154)",
}

// resultFieldsNeverAnswered are contract fields an operation never answers, with the reason.
var resultFieldsNeverAnswered = map[string]map[string]string{
	"writes.getResult": {"error": "the failed variant: a rejected write leaves no record, so it reads as not_received (IR118)"},
}

// TestResultsMatchContract: every operation's result type answers exactly the fields of its contract result (IR392),
// and for a page or an array the fields of the contract's items. The list is contractResults.gen.json, which the web's
// contractInputs.test.ts derives from the contract with the TypeScript checker and keeps current. A field outside the
// contract is data the contract does not promise (D12: no extra DTO fields), and a missing one breaks a client that
// follows the contract.
func TestResultsMatchContract(t *testing.T) {
	s := serverCfg(t, func(c *apiserver.Config) { c.Domains = ops.Domains })
	raw, err := os.ReadFile("../../../web/shared/lib/contractResults.gen.json")
	if err != nil {
		t.Fatal(err)
	}
	var contract map[string]struct {
		Fields []string  `json:"fields"`
		Items  *[]string `json:"items"`
	}
	if err := json.Unmarshal(raw, &contract); err != nil {
		t.Fatal(err)
	}
	marshaler := reflect.TypeOf((*json.Marshaler)(nil)).Elem()
	plain := func(rt reflect.Type) bool {
		for rt.Kind() == reflect.Pointer {
			rt = rt.Elem()
		}
		return rt.Kind() != reflect.Interface && !rt.Implements(marshaler) && !reflect.PointerTo(rt).Implements(marshaler)
	}
	compare := func(what string, rt reflect.Type, want []string, never map[string]string) {
		got := map[string]bool{}
		inputFields(rt, got)
		var extra, missing []string
		for f := range got {
			if !slices.Contains(want, f) {
				extra = append(extra, f)
			}
		}
		for _, f := range want {
			if _, ok := never[f]; !got[f] && !ok {
				missing = append(missing, f)
			}
		}
		slices.Sort(extra)
		slices.Sort(missing)
		if len(extra) > 0 {
			t.Errorf("%s (%s) answers fields outside the contract: %v", what, rt, extra)
		}
		if len(missing) > 0 {
			t.Errorf("%s (%s) never answers contract fields: %v", what, rt, missing)
		}
	}
	checked := 0
	for _, op := range s.Registry.Operations() {
		name := op.Spec.Name
		want, ok := contract[name]
		if !ok {
			t.Errorf("%s is registered but not in the contract", name)
			continue
		}
		if _, ok := resultsDecidedAtRunTime[name]; ok {
			continue
		}
		rt := op.Result
		for rt.Kind() == reflect.Pointer {
			rt = rt.Elem()
		}
		if !plain(rt) {
			t.Errorf("%s answers %s, whose fields are decided at run time: give it a plain result type or list it with the reason", name, rt)
			continue
		}
		checked++
		if rt.Kind() == reflect.Slice {
			if want.Items == nil {
				t.Errorf("%s answers an array, the contract does not", name)
				continue
			}
			compare(name+" items", rt.Elem(), *want.Items, nil)
			continue
		}
		compare(name, rt, want.Fields, resultFieldsNeverAnswered[name])
		if want.Items != nil {
			f, ok := rt.FieldByName("Items")
			if !ok || f.Type.Kind() != reflect.Slice {
				t.Errorf("%s (%s) has no items, the contract's result does", name, rt)
				continue
			}
			if !plain(f.Type.Elem()) {
				t.Errorf("%s lists %s, whose fields are decided at run time: list the operation with the reason", name, f.Type.Elem())
				continue
			}
			compare(name+" items", f.Type.Elem(), *want.Items, nil)
		}
	}
	if checked < 180 {
		t.Fatalf("only %d operations checked", checked)
	}
}
