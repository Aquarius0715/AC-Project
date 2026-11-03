package integration_test

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
func resultsDecidedAtRunTime() map[string]string {
	return map[string]string{
		opJobsGet: "the IR42 projection of the job (detail, offer, history, summary): the role and the " +
			"job pick it",
		opJobsList:                    "the IR42 projections of the listed jobs",
		"restrictions.get":            "the SR03 projection: the whole restriction or the caller's units",
		"restrictions.list":           "the SR03 projections of the listed restrictions",
		"restrictions.reconcile":      "the SR03 projection of the reconciled restriction",
		opRestrictionsRetry:           "the SR03 projection of the retried restriction",
		opLocationsRename:             "a property or a space, by the input's kind",
		opPaymentsSimulate:            "the event picks the answer: payment instructions, an attempt or a confirmation",
		opPoliciesGet:                 "Policy writes the fields of its kind (alert, default alert, automation)",
		opPoliciesList:                "the listed policies write the fields of their kinds",
		opPoliciesSave:                "Policy writes the fields of its kind",
		"voice.resolveIntent":         "ResolvedIntent writes the fields of the intent it resolved",
		opDemoSessionSignIn:           "the BFF answers it; the Core API refuses it (errors.session_via_bff, IR154)",
		opDemoSessionSwitchMembership: "the BFF answers it; the Core API refuses it (IR154)",
		opDemoSessionExtend:           "the BFF answers it; the Core API refuses it (IR154)",
	}
}

// resultFieldsNeverAnswered are contract fields an operation never answers, with the reason.
func resultFieldsNeverAnswered() map[string]map[string]string {
	return map[string]map[string]string{
		opWritesGetResult: {
			"error": "the failed variant: a rejected write leaves no record, so it reads as not_received (IR118)",
		},
	}
}

// TestResultsMatchContract: every operation's result type answers exactly the fields of its contract result (IR392),
// and for a page or an array the fields of the contract's items. The list is contractResults.gen.json, which the web's
// contractInputs.test.ts derives from the contract with the TypeScript checker and keeps current. A field outside the
// contract is data the contract does not promise (D12: no extra DTO fields), and a missing one breaks a client that
// follows the contract.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestResultsMatchContract(t *testing.T) {
	srv := serverCfg(t, func(c *apiserver.Config) { c.Domains = ops.Domains })

	raw, err := os.ReadFile("../../../web/shared/lib/contractResults.gen.json")
	if err != nil {
		t.Fatal(err)
	}

	var contract map[string]struct {
		Fields []string  `json:"fields"`
		Items  *[]string `json:"items"`
	}

	unmarshalErr := json.Unmarshal(raw, &contract)
	if unmarshalErr != nil {
		t.Fatal(unmarshalErr)
	}

	marshaler := reflect.TypeFor[json.Marshaler]()
	plain := func(resultType reflect.Type) bool {
		for resultType.Kind() == reflect.Pointer {
			resultType = resultType.Elem()
		}

		return resultType.Kind() != reflect.Interface && !resultType.Implements(marshaler) &&
			!reflect.PointerTo(resultType).Implements(marshaler)
	}
	compare := func(what string, resultType reflect.Type, want []string, never map[string]string) {
		got := map[string]bool{}
		inputFields(resultType, got)

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
			t.Errorf("%s (%s) answers fields outside the contract: %v", what, resultType, extra)
		}

		if len(missing) > 0 {
			t.Errorf("%s (%s) never answers contract fields: %v", what, resultType, missing)
		}
	}
	checked := 0

	for _, operation := range srv.Registry.Operations() {
		name := operation.Spec.Name

		want, ok := contract[name]
		if !ok {
			t.Errorf("%s is registered but not in the contract", name)

			continue
		}

		if _, ok := resultsDecidedAtRunTime()[name]; ok {
			continue
		}

		resultType := operation.Result
		for resultType.Kind() == reflect.Pointer {
			resultType = resultType.Elem()
		}

		if !plain(resultType) {
			t.Errorf(
				"%s answers %s, whose fields are decided at run time: give it a plain result type or list it with the reason",
				name,
				resultType,
			)

			continue
		}

		checked++

		if resultType.Kind() == reflect.Slice {
			if want.Items == nil {
				t.Errorf("%s answers an array, the contract does not", name)

				continue
			}

			compare(name+" items", resultType.Elem(), *want.Items, nil)

			continue
		}

		compare(name, resultType, want.Fields, resultFieldsNeverAnswered()[name])

		if want.Items != nil {
			structField, ok := resultType.FieldByName("Items")
			if !ok || structField.Type.Kind() != reflect.Slice {
				t.Errorf("%s (%s) has no items, the contract's result does", name, resultType)

				continue
			}

			if !plain(structField.Type.Elem()) {
				t.Errorf(
					"%s lists %s, whose fields are decided at run time: list the operation with the reason",
					name,
					structField.Type.Elem(),
				)

				continue
			}

			compare(name+" items", structField.Type.Elem(), *want.Items, nil)
		}
	}

	if checked < 180 {
		t.Fatalf("only %d operations checked", checked)
	}
}
