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

	req, parsed := decodeClientInput(input)
	route := pickRoute(spec.Routes, req)
	path := fillPath(route, req)

	if route.Method == http.MethodGet || route.Method == http.MethodDelete {
		return route.Method, path + queryString(req, input, parsed), nil
	}

	if !parsed {
		return route.Method, path, strings.NewReader(input)
	}

	body, err := json.Marshal(req)
	if err != nil { // decoded JSON always encodes again; the input as it is otherwise
		return route.Method, path, strings.NewReader(input)
	}

	return route.Method, path, bytes.NewReader(body)
}

// decodeClientInput decodes the input object; parsed is false for input that is not one (it is sent as it is).
func decodeClientInput(input string) (map[string]any, bool) {
	var req map[string]any

	dec := json.NewDecoder(strings.NewReader(input))
	dec.UseNumber()

	parsed := strings.TrimSpace(input) == "" || dec.Decode(&req) == nil && req != nil && !dec.More()
	if req == nil {
		req = map[string]any{}
	}

	return req, parsed
}

// fillPath takes the route's path parameters out of the input into the path, percent-encoded, and drops the route's
// fixed field when the input holds its value.
func fillPath(route Route, req map[string]any) string {
	path := route.Path
	for _, param := range PathParams(route.Path) {
		path = strings.Replace(
			path,
			"{"+param+"}",
			url.PathEscape(paramText(takeAt(req, strings.Split(param, ".")))),
			1,
		)
	}

	if f := route.FixedField; f != "" && paramText(req[f]) == route.FixedValue {
		delete(req, f)
	}

	return path
}

// queryString is the query string of a GET or DELETE route: the rest of the input, or input that is not an object as
// the parameter "_".
func queryString(req map[string]any, input string, parsed bool) string {
	params := url.Values{}
	if parsed {
		flattenQuery(params, "", req, true)
	} else {
		params.Set("_", input)
	}

	if len(params) == 0 {
		return ""
	}

	return "?" + params.Encode()
}

// pickRoute prefers the route with the most path parameters the input holds and whose fixed field it matches.
func pickRoute(routes []Route, input map[string]any) Route {
	best, score := routes[0], -1
	for _, route := range routes {
		if route.FixedField != "" && paramText(input[route.FixedField]) != route.FixedValue {
			continue
		}

		held := 0

		for _, p := range PathParams(route.Path) {
			if lookupAt(input, strings.Split(p, ".")) == nil {
				held = -1

				break
			}

			held++
		}

		if held > score {
			best, score = route, held
		}
	}

	return best
}

func lookupAt(obj map[string]any, fieldPath []string) any {
	for _, k := range fieldPath[:len(fieldPath)-1] {
		next, ok := obj[k].(map[string]any)
		if !ok {
			return nil
		}

		obj = next
	}

	return obj[fieldPath[len(fieldPath)-1]]
}

func takeAt(obj map[string]any, fieldPath []string) any {
	for _, k := range fieldPath[:len(fieldPath)-1] {
		next, ok := obj[k].(map[string]any)
		if !ok {
			return nil
		}

		obj = next
	}

	v := obj[fieldPath[len(fieldPath)-1]]
	delete(obj, fieldPath[len(fieldPath)-1])

	return v
}

func paramText(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case bool:
		if typed {
			return "true"
		}

		return "false"
	case json.Number:
		return typed.String()
	}

	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}

	return string(raw)
}

func flattenQuery(params url.Values, prefix string, obj map[string]any, top bool) {
	for key, value := range obj {
		switch typed := value.(type) {
		case nil:
		case map[string]any:
			flattenObject(params, prefix, key, typed, top)
		case []any:
			if len(typed) == 0 { // the empty list
				params.Add(prefix+key, "")
			}

			for _, item := range typed {
				params.Add(prefix+key, paramText(item))
			}
		default:
			params.Add(prefix+key, paramText(typed))
		}
	}
}

// flattenObject adds the fields of a nested object: the paging query and the filters by name at the top, sort as
// field:direction, any other object as object.field.
func flattenObject(params url.Values, prefix, key string, obj map[string]any, top bool) {
	switch {
	case top && (key == "query" || key == "filters"):
		flattenQuery(params, "", obj, true)
	case key == paramSort:
		params.Set(paramSort, paramText(obj["field"])+":"+paramText(obj["direction"]))
	default:
		flattenQuery(params, prefix+key+".", obj, false)
	}
}
