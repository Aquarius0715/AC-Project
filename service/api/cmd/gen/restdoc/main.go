// Command restdoc prints the REST routes of the Core API (IR222) with their path and query parameters as JSON, for
// scripts/swag_annotate.py: the parameters come from the bindings the server mounts (the handlers' input types, the
// catalog filters). It assembles the server without a database (the pool connects lazily). Run from service/api:
//
//	go run ./cmd/gen/restdoc | python3 scripts/swag_annotate.py --routes -
package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/server"
)

type param struct {
	Name     string `json:"name"`
	In       string `json:"in"`   // path or query
	Type     string `json:"type"` // string, integer, number, boolean
	List     bool   `json:"list,omitempty"`
	Role     string `json:"role"` // path, field, cursor, limit, sort, filter
	Field    string `json:"field"`
	Required bool   `json:"required,omitempty"`
}

type route struct {
	Method     string  `json:"method"`
	Path       string  `json:"path"`
	Body       bool    `json:"body"`
	FixedField string  `json:"fixedField,omitempty"`
	FixedValue string  `json:"fixedValue,omitempty"`
	Params     []param `json:"params"`
}

func typeName(p ops.Param) string {
	switch {
	case p.Sort:
		return "string"
	case p.Kind == ops.KindInteger:
		return "integer"
	case p.Kind == ops.KindNumber:
		return "number"
	case p.Kind == ops.KindBool:
		return "boolean"
	}
	return "string"
}

func main() {
	srv, err := server.New(context.Background(), server.Config{DatabaseURL: "postgres://doc:doc@127.0.0.1:1/doc"}, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer srv.DB.Close()
	out := map[string][]route{}
	for _, op := range srv.Registry.Operations() {
		for _, rt := range op.Spec.Routes {
			b, err := ops.NewBinding(op, rt)
			if err != nil {
				log.Fatal(err)
			}
			r := route{Method: rt.Method, Path: rt.Path, Body: b.Body, FixedField: rt.FixedField, FixedValue: rt.FixedValue, Params: []param{}}
			for _, name := range b.Path {
				p := b.Params[name]
				r.Params = append(r.Params, param{Name: name, In: "path", Type: typeName(p), Role: "path", Field: strings.Join(p.At, "."), Required: true})
			}
			if !b.Body {
				var query []ops.Param
				for name, p := range b.Params {
					if name != rt.FixedField && !contains(b.Path, name) {
						query = append(query, p)
					}
				}
				sort.Slice(query, func(i, j int) bool { return query[i].Order < query[j].Order })
				for _, p := range query {
					r.Params = append(r.Params, param{Name: p.Name, In: "query", Type: typeName(p), List: p.List, Role: p.Role, Field: strings.Join(p.At, ".")})
				}
			}
			out[op.Spec.Name] = append(out[op.Spec.Name], r)
		}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", " ")
	if err := enc.Encode(out); err != nil {
		log.Fatal(err)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
