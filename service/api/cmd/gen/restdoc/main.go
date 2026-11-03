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
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/pradita/ac-project/service/api/internal/modules/identity"
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

func typeName(parameter ops.Param) string {
	switch {
	case parameter.Sort:
		return "string"
	case parameter.Kind == ops.KindInteger:
		return "integer"
	case parameter.Kind == ops.KindNumber:
		return "number"
	case parameter.Kind == ops.KindBool:
		return "boolean"
	}

	return "string"
}

func main() {
	srv, err := server.New(
		context.Background(),
		server.Config{
			DatabaseURL:       "postgres://doc:doc@127.0.0.1:1/doc",
			DatabaseReaderURL: "",
			Addr:              "",
			Clock:             nil,
			DemoOps:           false,
			DemoStart:         time.Time{},
			Logger:            nil,
			Domains:           nil,
			IdentityURL:       "",
			InternalToken:     "",
			ServiceURLs:       nil,
			PrincipalTTL:      0,
			DemoClock:         nil,
			WriteLimit:        nil,
			DenyLimit:         nil,
			IdentityProvider:  nil,
			AccountApps: identity.AccountApps{
				Customer:   "",
				Partner:    "",
				Technician: "",
				Admin:      "",
			},
			AccountsPerDay: 0,
			WrapTx:         nil,
		},
		nil,
	)
	if err != nil {
		log.Fatal(err)
	}
	defer srv.DB.Close()

	out := map[string][]route{}

	for _, operation := range srv.Registry.Operations() {
		for _, catalogRoute := range operation.Spec.Routes {
			binding, err := ops.NewBinding(operation, catalogRoute)
			if err != nil {
				log.Fatal(err)
			}

			described := route{
				Method:     catalogRoute.Method,
				Path:       catalogRoute.Path,
				Body:       binding.Body,
				FixedField: catalogRoute.FixedField,
				FixedValue: catalogRoute.FixedValue,
				Params:     []param{},
			}
			for _, name := range binding.Path {
				parameter := binding.Params[name]
				described.Params = append(
					described.Params,
					param{
						Name:     name,
						In:       "path",
						Type:     typeName(parameter),
						Role:     "path",
						Field:    strings.Join(parameter.At, "."),
						Required: true, List: false,
					},
				)
			}

			if !binding.Body {
				var query []ops.Param

				for name, p := range binding.Params {
					if name != catalogRoute.FixedField && !contains(binding.Path, name) {
						query = append(query, p)
					}
				}

				sort.Slice(query, func(i, j int) bool { return query[i].Order < query[j].Order })

				for _, parameter := range query {
					described.Params = append(
						described.Params,
						param{
							Name:  parameter.Name,
							In:    "query",
							Type:  typeName(parameter),
							List:  parameter.List,
							Role:  parameter.Role,
							Field: strings.Join(parameter.At, "."), Required: false,
						},
					)
				}
			}

			out[operation.Spec.Name] = append(out[operation.Spec.Name], described)
		}
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", " ")

	encodeErr := enc.Encode(out)
	if encodeErr != nil {
		log.Fatal(encodeErr)
	}
}

func contains(list []string, s string) bool {
	return slices.Contains(list, s)
}
