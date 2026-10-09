package ops

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

// REST routes (IR222): every operation is served at the routes of its catalog entry. A route builds the same JSON
// input the operation reads from POST /v1/ops/<operation> and runs the same pipeline (validation, authorization,
// idempotency, expected version, transaction, ServiceResult). The input is made of
//   - the path parameters, written {field} after the input field they fill ({target.kind} fills a nested field);
//   - GET and DELETE: the query string — top-level fields by name (lists repeat the parameter or separate the values
//     with commas and an empty value is the empty list, fields of a nested object as object.field), the paging
//     fields cursor, limit and sort=field:direction, and the operation's catalogued filters by their own names;
//   - POST, PUT and PATCH: the JSON object body (no query parameters);
//   - the route's fixed field (payouts.transition at /approve sets action=approve).
// A path parameter or the fixed field that the body names with another value is VALIDATION error.pathMismatch.

// Param describes one query or path parameter of a route (also used for the API description).
type Param struct {
	Name  string
	At    []string // JSON path of the input field it fills
	Kind  ParamKind
	List  bool
	Sort  bool   // sort=field:direction → {field, direction}
	Role  string // field, cursor, limit, sort or filter
	Order int    // input order: fields, then the paging fields, then the filters in catalog order
}

// Binding turns the requests of one REST route into operation input.
type Binding struct {
	Route  Route
	Body   bool     // POST, PUT, PATCH read a JSON object body; GET and DELETE the query string
	Path   []string // path parameter names in order
	Params map[string]Param
	// Opaque lists the input fields a query string cannot carry (lists of objects, maps, raw JSON): a route that
	// reads the query string must have none.
	Opaque []string
}

var (
	pathParam = regexp.MustCompile(`\{([^}]+)\}`)
	timeType  = reflect.TypeFor[time.Time]()
	uuidType  = reflect.TypeFor[uuid.UUID]()
	rawType   = reflect.TypeFor[json.RawMessage]()
	queryType = reflect.TypeFor[paging.Query]()
)

// EchoPath turns a catalog path (/v1/jobs/{jobId}) into Echo's notation (/v1/jobs/:jobId).
func EchoPath(p string) string { return pathParam.ReplaceAllString(p, ":$1") }

// PathParams lists the {field} parameters of a catalog path in order.
func PathParams(p string) []string {
	var names []string
	for _, m := range pathParam.FindAllStringSubmatch(p, -1) {
		names = append(names, m[1])
	}
	return names
}

// NewBinding checks a route against its operation's input type.
func NewBinding(op *Operation, rt Route) (*Binding, error) {
	b := &Binding{Route: rt, Path: PathParams(rt.Path)}
	switch rt.Method {
	case "GET", "DELETE":
	case "POST", "PUT", "PATCH":
		b.Body = true
	default:
		return nil, fmt.Errorf("ops: %s: method %s", op.Spec.Name, rt.Method)
	}
	if rt.Method == "GET" && op.Spec.Mode != Read || rt.Method == "DELETE" && op.Spec.Mode != Write {
		return nil, fmt.Errorf("ops: %s: %s does not fit a %s", op.Spec.Name, rt.Method, op.Spec.Mode)
	}
	t := reflect.TypeOf(op.NewInput())
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	params, opaque, err := parameters(t, op.Spec.Filters)
	if err != nil {
		return nil, fmt.Errorf("ops: %s: %w", op.Spec.Name, err)
	}
	b.Params, b.Opaque = params, opaque
	for _, name := range append(slices.Clone(b.Path), rt.FixedField) {
		if name == "" {
			continue
		}
		if p, ok := params[name]; !ok || p.List || p.Sort || slices.Contains(p.At, "filters") {
			return nil, fmt.Errorf("ops: %s: %s is not a scalar input field", op.Spec.Name, name)
		}
	}
	if !b.Body && len(opaque) > 0 {
		return nil, fmt.Errorf("ops: %s: %s %s cannot carry %s in a query string", op.Spec.Name, rt.Method, rt.Path, strings.Join(opaque, ", "))
	}
	return b, nil
}

// parameters lists the input fields a query string or path can name: scalars and lists of scalars by their JSON
// name, fields of a nested object as object.field, the paging query (embedded or as a field) as cursor, limit and
// sort, and the catalogued filters by their own names (a filter named like an input field is left out).
func parameters(t reflect.Type, filters []string) (map[string]Param, []string, error) {
	ps := map[string]Param{}
	var opaque []string
	var queries, filterAt [][]string
	var err error
	add := func(p Param) {
		if _, dup := ps[p.Name]; dup && err == nil {
			err = fmt.Errorf("parameter %s twice", p.Name)
		}
		if p.Role == "" {
			p.Role = "field"
		}
		p.Order = len(ps)
		ps[p.Name] = p
	}
	var walk func(t reflect.Type, at []string, prefix string, nested bool)
	walk = func(t reflect.Type, at []string, prefix string, nested bool) {
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if name == "-" {
				continue
			}
			ft := deref(f.Type)
			if f.Anonymous && name == "" {
				if ft == queryType {
					queries = append(queries, at)
				} else if ft.Kind() == reflect.Struct {
					walk(ft, at, prefix, nested)
				}
				continue
			}
			if name == "" {
				name = f.Name
			}
			key, here := prefix+name, append(slices.Clone(at), name)
			switch {
			case ft == queryType && !nested:
				queries = append(queries, here)
			case name == "filters" && ft == rawType && !nested:
				filterAt = append(filterAt, here)
			case scalar(ft):
				add(Param{Name: key, At: here, Kind: kindOf(ft)})
			case ft.Kind() == reflect.Slice && ft != rawType && scalar(deref(ft.Elem())):
				add(Param{Name: key, At: here, Kind: kindOf(deref(ft.Elem())), List: true})
			case ft.Kind() == reflect.Struct && !nested:
				walk(ft, here, key+".", true)
			default:
				opaque = append(opaque, key)
			}
		}
	}
	if t == queryType { // list reads whose input is the paging query itself
		queries = append(queries, nil)
	} else {
		walk(t, nil, "", false)
	}
	for _, at := range queries {
		add(Param{Name: "cursor", At: append(slices.Clone(at), "cursor"), Role: "cursor"})
		add(Param{Name: "limit", At: append(slices.Clone(at), "limit"), Kind: KindInteger, Role: "limit"})
		add(Param{Name: "sort", At: append(slices.Clone(at), "sort"), Sort: true, Role: "sort"})
		filterAt = append(filterAt, append(slices.Clone(at), "filters"))
	}
	for _, at := range filterAt {
		for _, name := range filters {
			if _, taken := ps[name]; taken {
				continue
			}
			k := FilterKinds[name]
			ps[name] = Param{Name: name, At: append(slices.Clone(at), name), Kind: k.Kind, List: k.List, Role: "filter", Order: len(ps)}
		}
	}
	return ps, opaque, err
}

func deref(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

func scalar(t reflect.Type) bool {
	if t == timeType || t == uuidType {
		return true
	}
	switch t.Kind() {
	case reflect.String, reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

func kindOf(t reflect.Type) ParamKind {
	if t == timeType || t == uuidType {
		return KindString
	}
	switch t.Kind() {
	case reflect.Bool:
		return KindBool
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return KindInteger
	case reflect.Float32, reflect.Float64:
		return KindNumber
	}
	return KindString
}

// value reads the parameter's text; nil means absent (an empty value).
func (p Param) value(values []string) (any, bool) {
	if p.Sort {
		if len(values) != 1 || values[0] == "" {
			return nil, len(values) == 1
		}
		field, dir, _ := strings.Cut(values[0], ":")
		if dir == "" {
			dir = "asc"
		}
		if field == "" {
			return nil, false
		}
		return map[string]any{"field": field, "direction": dir}, true
	}
	if p.List {
		var items []any
		for _, v := range values {
			for _, s := range strings.Split(v, ",") {
				if s = strings.TrimSpace(s); s == "" {
					continue
				}
				x, ok := convert(p.Kind, s)
				if !ok {
					return nil, false
				}
				items = append(items, x)
			}
		}
		if items == nil { // a list parameter with only empty values is the empty list (unitIds= → [])
			return []any{}, true
		}
		return items, true
	}
	if len(values) != 1 {
		return nil, false
	}
	if values[0] == "" {
		return nil, true
	}
	return convert(p.Kind, values[0])
}

func convert(k ParamKind, s string) (any, bool) {
	switch k {
	case KindInteger:
		n, err := strconv.ParseInt(s, 10, 64)
		return json.Number(strconv.FormatInt(n, 10)), err == nil
	case KindNumber:
		f, err := strconv.ParseFloat(s, 64)
		if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
			return nil, false
		}
		return json.Number(strconv.FormatFloat(f, 'g', -1, 64)), true
	case KindBool:
		switch s {
		case "true":
			return true, true
		case "false":
			return false, true
		}
		return nil, false
	}
	return s, true
}

// place sets v at the path unless the object already holds another value there.
func place(obj map[string]any, at []string, v any) bool {
	for _, key := range at[:len(at)-1] {
		next, ok := obj[key]
		if !ok || next == nil {
			m := map[string]any{}
			obj[key] = m
			obj = m
			continue
		}
		if obj, ok = next.(map[string]any); !ok {
			return false
		}
	}
	leaf := at[len(at)-1]
	if old, ok := obj[leaf]; ok && old != nil {
		a, _ := json.Marshal(old)
		b, _ := json.Marshal(v)
		return bytes.Equal(a, b)
	}
	obj[leaf] = v
	return true
}

// Input builds the operation input of a request on this route; body is the request body of POST, PUT and PATCH.
func (b *Binding) Input(c *echo.Context, body []byte) ([]byte, error) {
	obj := map[string]any{}
	fe := map[string]string{}
	query := c.QueryParams()
	if b.Body {
		if len(bytes.TrimSpace(body)) > 0 {
			dec := json.NewDecoder(bytes.NewReader(body))
			dec.UseNumber()
			if err := dec.Decode(&obj); err != nil || obj == nil || dec.More() {
				return nil, apperr.Fields(map[string]string{"_": "error.malformedInput"})
			}
		}
		for name := range query {
			fe[name] = "error.notAllowed" // a body route reads its input from the body
		}
	} else {
		for name, values := range query {
			p, ok := b.Params[name]
			if !ok || slices.Contains(b.Path, name) || name == b.Route.FixedField {
				fe[name] = "error.notAllowed"
				continue
			}
			v, ok := p.value(values)
			if !ok {
				fe[name] = "error.invalid"
			} else if v != nil {
				place(obj, p.At, v)
			}
		}
	}
	for _, name := range b.Path {
		raw, err := url.PathUnescape(c.Param(name))
		v, ok := convert(b.Params[name].Kind, raw)
		switch {
		case err != nil || raw == "" || !ok:
			fe[name] = "error.invalid"
		case !place(obj, b.Params[name].At, v):
			fe[name] = "error.pathMismatch"
		}
	}
	if f := b.Route.FixedField; f != "" && !place(obj, b.Params[f].At, b.Route.FixedValue) {
		fe[f] = "error.pathMismatch"
	}
	if len(fe) > 0 {
		return nil, apperr.Fields(fe)
	}
	return json.Marshal(obj)
}

// MountREST registers the REST routes of the operations this process serves on the /v1 group (IR222).
func (r *Registry) MountREST(g *echo.Group) error {
	for _, op := range r.Operations() {
		if !r.serves(op.Spec.Name) {
			continue
		}
		for _, rt := range op.Spec.Routes {
			b, err := NewBinding(op, rt)
			if err != nil {
				return err
			}
			path, ok := strings.CutPrefix(rt.Path, "/v1/")
			if !ok {
				return fmt.Errorf("ops: %s: route %s outside /v1", op.Spec.Name, rt.Path)
			}
			if _, err := g.AddRoute(echo.Route{Method: rt.Method, Path: "/" + EchoPath(path), Name: op.Spec.Name, Handler: r.restHandler(op, b)}); err != nil {
				return fmt.Errorf("ops: %s: %w", op.Spec.Name, err)
			}
		}
	}
	return nil
}

func (r *Registry) restHandler(op *Operation, b *Binding) echo.HandlerFunc {
	return func(c *echo.Context) error {
		corr := correlationID(c)
		var body []byte
		if b.Body {
			var err error
			if body, err = readBody(c, op.Spec.Name); err != nil {
				return fail(c, err, corr)
			}
		}
		in, err := b.Input(c, body)
		if err != nil {
			return fail(c, err, corr)
		}
		return r.run(c, op, in, corr)
	}
}
