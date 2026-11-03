package ops

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// ClientRequest is the request a client of the Core API makes for one operation call — its name and JSON input — at the
// operation's REST routes (IR222); the Go counterpart of the web apps' coreRequest (service/web/shared/lib/rest.ts),
// used by the tests:
//   - the route whose path parameters the input holds (an item route before a collection route) and whose fixed
//     field it matches;
//   - path parameters are taken out of the input and percent-encoded;
//   - GET and DELETE send the rest as the query string (the paging query and the filters by name, sort as
//     field:direction, nested fields as object.field, lists as repeated parameters, an empty list as one empty value);
//   - POST, PUT and PATCH send the rest as the JSON body.
//
// Input that is not a JSON object is sent as it is (malformed-input cases still reach the API); an operation outside
// the catalog goes to POST /v1/<name>, which no route serves.
func ClientRequest(operation, input string) (string, string, io.Reader) {
	spec, ok := SpecByName()[operation]
	if !ok || len(spec.Routes) == 0 {
		return http.MethodPost, "/v1/" + operation, strings.NewReader(input)
	}

	var req map[string]any

	dec := json.NewDecoder(strings.NewReader(input))
	dec.UseNumber()

	parsed := strings.TrimSpace(input) == "" || dec.Decode(&req) == nil && req != nil && !dec.More()
	if req == nil {
		req = map[string]any{}
	}

	route := pickRoute(spec.Routes, req)

	path := route.Path
	for _, p := range PathParams(route.Path) {
		path = strings.Replace(
			path,
			"{"+p+"}",
			url.PathEscape(paramText(takeAt(req, strings.Split(p, ".")))),
			1,
		)
	}

	if f := route.FixedField; f != "" && paramText(req[f]) == route.FixedValue {
		delete(req, f)
	}

	if route.Method == http.MethodGet || route.Method == http.MethodDelete {
		params := url.Values{}
		if parsed {
			flattenQuery(params, "", req, true)
		} else {
			params.Set("_", input)
		}

		if len(params) > 0 {
			path += "?" + params.Encode()
		}

		return route.Method, path, nil
	}

	if !parsed {
		return route.Method, path, strings.NewReader(input)
	}

	b, _ := json.Marshal(req)

	return route.Method, path, bytes.NewReader(b)
}

// pickRoute prefers the route with the most path parameters the input holds and whose fixed field it matches.
func pickRoute(routes []Route, input map[string]any) Route {
	best, score := routes[0], -1
	for _, route := range routes {
		if route.FixedField != "" && paramText(input[route.FixedField]) != route.FixedValue {
			continue
		}

		n := 0

		for _, p := range PathParams(route.Path) {
			if lookupAt(input, strings.Split(p, ".")) == nil {
				n = -1

				break
			}

			n++
		}

		if n > score {
			best, score = route, n
		}
	}

	return best
}

func lookupAt(obj map[string]any, at []string) any {
	for _, k := range at[:len(at)-1] {
		next, ok := obj[k].(map[string]any)
		if !ok {
			return nil
		}

		obj = next
	}

	return obj[at[len(at)-1]]
}

func takeAt(obj map[string]any, at []string) any {
	for _, k := range at[:len(at)-1] {
		next, ok := obj[k].(map[string]any)
		if !ok {
			return nil
		}

		obj = next
	}

	v := obj[at[len(at)-1]]
	delete(obj, at[len(at)-1])

	return v
}

func paramText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "true"
		}

		return "false"
	case json.Number:
		return x.String()
	}

	b, _ := json.Marshal(v)

	return string(b)
}

func flattenQuery(params url.Values, prefix string, obj map[string]any, top bool) {
	for key, v := range obj {
		switch x := v.(type) {
		case nil:
		case map[string]any:
			switch {
			case top && (key == "query" || key == "filters"):
				flattenQuery(params, "", x, true)
			case key == "sort":
				params.Set("sort", paramText(x["field"])+":"+paramText(x["direction"]))
			default:
				flattenQuery(params, prefix+key+".", x, false)
			}
		case []any:
			if len(x) == 0 { // the empty list
				params.Add(prefix+key, "")
			}

			for _, e := range x {
				params.Add(prefix+key, paramText(e))
			}
		default:
			params.Add(prefix+key, paramText(x))
		}
	}
}
