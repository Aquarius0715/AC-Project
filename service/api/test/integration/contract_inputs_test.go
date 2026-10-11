package integration

import (
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/pradita/ac-project/service/api/internal/ops"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

// inputFields are the JSON fields a decoder fills in a value of type t: the json names of its exported fields, with
// the fields of embedded structs promoted.
func inputFields(t reflect.Type, out map[string]bool) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name := strings.Split(f.Tag.Get("json"), ",")[0]
		if name == "-" {
			continue
		}
		ft := f.Type
		for ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if f.Anonymous && name == "" && ft.Kind() == reflect.Struct {
			inputFields(ft, out)
			continue
		}
		if !f.IsExported() {
			continue
		}
		if name == "" {
			name = f.Name
		}
		out[name] = true
	}
}

// TestInputsMatchContract: every operation's input type takes exactly the fields of its contract input (IR391). The
// list is contractInputs.gen.json, which the web's contractInputs.test.ts derives from the contract with the TypeScript
// checker and keeps current. A field outside the contract would be accepted where D12 says an unknown field is
// VALIDATION, and a contract field the type lacks would refuse a request that follows the contract. Two operations may
// share an input type only when their contract inputs are the same.
func TestInputsMatchContract(t *testing.T) {
	s := serverCfg(t, func(c *apiserver.Config) { c.Domains = ops.Domains })
	raw, err := os.ReadFile("../../../web/shared/lib/contractInputs.gen.json")
	if err != nil {
		t.Fatal(err)
	}
	var contract map[string][]string
	if err := json.Unmarshal(raw, &contract); err != nil {
		t.Fatal(err)
	}
	registered := map[string]bool{}
	for _, op := range s.Registry.Operations() {
		name := op.Spec.Name
		registered[name] = true
		want, ok := contract[name]
		if !ok {
			t.Errorf("%s is registered but not in the contract", name)
			continue
		}
		fields := map[string]bool{}
		inputFields(reflect.TypeOf(op.NewInput()), fields)
		var extra, missing []string
		for f := range fields {
			if !slices.Contains(want, f) {
				extra = append(extra, f)
			}
		}
		for _, f := range want {
			if !fields[f] {
				missing = append(missing, f)
			}
		}
		slices.Sort(extra)
		slices.Sort(missing)
		if len(extra) > 0 {
			t.Errorf("%s (%T) takes fields outside its contract input: %v", name, op.NewInput(), extra)
		}
		if len(missing) > 0 {
			t.Errorf("%s (%T) cannot read contract fields: %v", name, op.NewInput(), missing)
		}
	}
	for name := range contract {
		if !registered[name] {
			t.Errorf("%s is in the contract but not registered", name)
		}
	}
	if len(registered) < 200 {
		t.Fatalf("only %d operations registered", len(registered))
	}
}
