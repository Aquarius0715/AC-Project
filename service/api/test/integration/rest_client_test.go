package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/pradita/ac-project/service/api/internal/ops"
)

// restRequest turns an operation call (name and JSON input) into its REST request (IR222), the way a client of the
// Core API calls it: the route whose path parameters the input holds (an item route before a collection route, the
// route of the input's fixed field), the path filled from the input, and the rest of the input as the query string
// (GET, DELETE: cursor, limit, sort=field:direction and the filters by name, lists as repeated parameters, nested
// fields as object.field) or as the JSON body. Input that is not a JSON object is sent as it is, so malformed-input
// cases still reach the API; an operation outside the catalog goes to POST /v1/<name>, which is not routed.
func restRequest(op, body string) (string, string, io.Reader) {
	spec, ok := ops.SpecByName()[op]
	if !ok {
		return http.MethodPost, "/v1/" + op, strings.NewReader(body)
	}
	var in map[string]any
	dec := json.NewDecoder(strings.NewReader(body))
	dec.UseNumber()
	parsed := strings.TrimSpace(body) == "" || dec.Decode(&in) == nil && in != nil && !dec.More()
	if in == nil {
		in = map[string]any{}
	}
	rt := pickRoute(spec.Routes, in)
	path := rt.Path
	for _, p := range ops.PathParams(rt.Path) {
		path = strings.Replace(path, "{"+p+"}", url.PathEscape(paramText(take(in, strings.Split(p, ".")))), 1)
	}
	if f := rt.FixedField; f != "" && paramText(in[f]) == rt.FixedValue {
		delete(in, f)
	}
	if rt.Method == http.MethodGet || rt.Method == http.MethodDelete {
		q := url.Values{}
		if parsed {
			flatten(q, "", in, true)
		} else {
			q.Set("_", body)
		}
		if len(q) > 0 {
			path += "?" + q.Encode()
		}
		return rt.Method, path, nil
	}
	if !parsed {
		return rt.Method, path, strings.NewReader(body)
	}
	b, _ := json.Marshal(in)
	return rt.Method, path, bytes.NewReader(b)
}

// pickRoute prefers the route with the most path parameters the input holds and whose fixed field it matches.
func pickRoute(routes []ops.Route, in map[string]any) ops.Route {
	best, score := routes[0], -1
	for _, rt := range routes {
		if rt.FixedField != "" && paramText(in[rt.FixedField]) != rt.FixedValue {
			continue
		}
		n := 0
		for _, p := range ops.PathParams(rt.Path) {
			if lookup(in, strings.Split(p, ".")) == nil {
				n = -1
				break
			}
			n++
		}
		if n > score {
			best, score = rt, n
		}
	}
	return best
}

func lookup(obj map[string]any, at []string) any {
	for _, k := range at[:len(at)-1] {
		next, ok := obj[k].(map[string]any)
		if !ok {
			return nil
		}
		obj = next
	}
	return obj[at[len(at)-1]]
}

// take removes and returns the value at the path.
func take(obj map[string]any, at []string) any {
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

func flatten(q url.Values, prefix string, obj map[string]any, top bool) {
	for k, v := range obj {
		switch x := v.(type) {
		case nil:
		case map[string]any:
			switch {
			case top && (k == "query" || k == "filters"):
				flatten(q, "", x, true)
			case k == "sort":
				q.Set("sort", paramText(x["field"])+":"+paramText(x["direction"]))
			default:
				flatten(q, prefix+k+".", x, false)
			}
		case []any:
			if len(x) == 0 { // the empty list
				q.Add(prefix+k, "")
			}
			for _, e := range x {
				q.Add(prefix+k, paramText(e))
			}
		default:
			q.Add(prefix+k, paramText(x))
		}
	}
}
