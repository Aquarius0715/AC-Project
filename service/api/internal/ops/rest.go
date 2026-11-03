package ops

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
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

// REST routes (IR222): every operation is served at the routes of its catalog entry. A route builds the operation's
// JSON input and runs the pipeline every operation shares (strict decoding, validation, authorization, idempotency,
// expected version, transaction, ServiceResult). The input is made of
//   - the path parameters, written {field} after the input field they fill ({target.kind} fills a nested field);
//   - GET and DELETE: the query string — top-level fields by name (lists repeat the parameter or separate the values
//     with commas and an empty value is the empty list, fields of a nested object as object.field), the paging
//     fields cursor, limit and sort=field:direction, and the operation's catalogued filters by their own names;
//   - POST, PUT and PATCH: the JSON object body (no query parameters);
//   - the route's fixed field (payouts.transition at /approve sets action=approve).
// A path parameter or the fixed field that the body names with another value is VALIDATION error.pathMismatch; an ID
// path parameter that is not a UUID is NOT_FOUND (no resource has it).

// Param describes one query or path parameter of a route (also used for the API description).
type Param struct {
	Name  string
	At    []string // JSON path of the input field it fills
	Kind  ParamKind
	List  bool
	Sort  bool   // sort=field:direction → {field, direction}
	UUID  bool   // the field is an ID: a path value that is not a UUID names no resource (NOT_FOUND)
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

// The paging parameters of a list read (paging.Query) and the other parameter roles.
const (
	paramCursor = "cursor"
	paramLimit  = "limit"
	paramSort   = "sort"
	roleField   = "field"
	roleFilter  = "filter"
)

// fieldFilters names the filters object of the paging query and of an input that has its own (raw JSON).
const fieldFilters = "filters"

var (
	errRouteMethod    = errors.New("unsupported method")
	errRouteMode      = errors.New("method does not fit the operation's mode")
	errNotScalar      = errors.New("not a scalar input field")
	errOpaqueQuery    = errors.New("input fields a query string cannot carry")
	errDuplicateParam = errors.New("parameter named twice")
	errOutsideV1      = errors.New("route outside /v1")
)

var pathParam = regexp.MustCompile(`\{([^}]+)\}`)

// The input field types a binding treats apart from their kind.
func timeType() reflect.Type  { return reflect.TypeFor[time.Time]() }
func uuidType() reflect.Type  { return reflect.TypeFor[uuid.UUID]() }
func rawType() reflect.Type   { return reflect.TypeFor[json.RawMessage]() }
func queryType() reflect.Type { return reflect.TypeFor[paging.Query]() }

// EchoPath turns a catalog path (/v1/jobs/{jobId}) into Echo's notation (/v1/jobs/:jobId).
func EchoPath(p string) string { return pathParam.ReplaceAllString(p, ":$1") }

// PathParams lists the {field} parameters of a catalog path in order.
func PathParams(p string) []string {
	matches := pathParam.FindAllStringSubmatch(p, -1)

	names := make([]string, 0, len(matches))
	for _, match := range matches {
		names = append(names, match[1])
	}

	return names
}

// NewBinding checks a route against its operation's input type.
func NewBinding(operation *Operation, route Route) (*Binding, error) {
	body, err := readsBody(operation.Spec.Mode, route.Method)
	if err != nil {
		return nil, fmt.Errorf("ops: %s: %w", operation.Spec.Name, err)
	}

	params, opaque, err := parameters(
		deref(reflect.TypeOf(operation.NewInput())),
		operation.Spec.Filters,
	)
	if err != nil {
		return nil, fmt.Errorf("ops: %s: %w", operation.Spec.Name, err)
	}

	binding := &Binding{
		Route:  route,
		Path:   PathParams(route.Path),
		Body:   body,
		Params: params,
		Opaque: opaque,
	}

	err = binding.check()
	if err != nil {
		return nil, fmt.Errorf("ops: %s: %w", operation.Spec.Name, err)
	}

	return binding, nil
}

// readsBody says whether a route of the method reads a JSON body (POST, PUT, PATCH) or the query string (GET,
// DELETE); another method, a GET of a write and a DELETE of a read are refused.
func readsBody(mode Mode, method string) (bool, error) {
	switch method {
	case http.MethodGet, http.MethodDelete:
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		return true, nil
	default:
		return false, fmt.Errorf("%w %s", errRouteMethod, method)
	}

	if method == http.MethodGet && mode != Read || method == http.MethodDelete && mode != Write {
		return false, fmt.Errorf("%w: %s for a %s", errRouteMode, method, mode)
	}

	return false, nil
}

// check refuses path parameters and a fixed field that are not scalar input fields, and input fields a query string
// cannot carry on a route that reads one.
func (b *Binding) check() error {
	for _, name := range append(slices.Clone(b.Path), b.Route.FixedField) {
		if name == "" {
			continue
		}

		if param, ok := b.Params[name]; !ok || param.List || param.Sort ||
			slices.Contains(param.At, fieldFilters) {
			return fmt.Errorf("%w: %s", errNotScalar, name)
		}
	}

	if !b.Body && len(b.Opaque) > 0 {
		return fmt.Errorf(
			"%w: %s %s: %s",
			errOpaqueQuery,
			b.Route.Method,
			b.Route.Path,
			strings.Join(b.Opaque, ", "),
		)
	}

	return nil
}

// parameters lists the input fields a query string or path can name: scalars and lists of scalars by their JSON
// name, fields of a nested object as object.field, the paging query (embedded or as a field) as cursor, limit and
// sort, and the catalogued filters by their own names (a filter named like an input field is left out).
func parameters(inputType reflect.Type, filters []string) (map[string]Param, []string, error) {
	set := &paramSet{params: map[string]Param{}, opaque: nil, queries: nil, filterAt: nil, err: nil}
	if inputType == queryType() { // list reads whose input is the paging query itself
		set.queries = append(set.queries, nil)
	} else {
		set.walk(inputType, nil, "", false)
	}

	for _, queryPath := range set.queries {
		set.addPaging(queryPath)
	}

	for _, filtersPath := range set.filterAt {
		set.addFilters(filtersPath, filters)
	}

	return set.params, set.opaque, set.err
}

// paramSet collects the parameters of an input type (see parameters).
type paramSet struct {
	params   map[string]Param
	opaque   []string   // the input fields a query string cannot carry
	queries  [][]string // where the paging queries are
	filterAt [][]string // where the filters objects are
	err      error      // the first parameter named twice
}

// add records a parameter in input order; one without a role is a field.
func (set *paramSet) add(param Param) {
	if _, dup := set.params[param.Name]; dup && set.err == nil {
		set.err = fmt.Errorf("%w: %s", errDuplicateParam, param.Name)
	}

	if param.Role == "" {
		param.Role = roleField
	}

	param.Order = len(set.params)
	set.params[param.Name] = param
}

// walk adds the fields of a struct at fieldPath; nested is true inside a field's own struct.
func (set *paramSet) walk(structType reflect.Type, fieldPath []string, prefix string, nested bool) {
	for structField := range structType.Fields() {
		if !structField.IsExported() {
			continue
		}

		name, _, _ := strings.Cut(structField.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}

		fieldType := deref(structField.Type)
		if structField.Anonymous && name == "" {
			set.embedded(fieldType, fieldPath, prefix, nested)

			continue
		}

		if name == "" {
			name = structField.Name
		}

		set.field(fieldType, prefix, name, append(slices.Clone(fieldPath), name), nested)
	}
}

// embedded adds an embedded struct: the paging query, or its fields as the embedding struct's own.
func (set *paramSet) embedded(
	fieldType reflect.Type,
	fieldPath []string,
	prefix string,
	nested bool,
) {
	if fieldType == queryType() {
		set.queries = append(set.queries, fieldPath)
	} else if fieldType.Kind() == reflect.Struct {
		set.walk(fieldType, fieldPath, prefix, nested)
	}
}

// field adds the field name at here: a scalar or a list of scalars as a parameter, a top-level struct's fields as
// object.field, anything else as opaque.
func (set *paramSet) field(
	fieldType reflect.Type,
	prefix, name string,
	here []string,
	nested bool,
) {
	if !nested && set.topLevel(fieldType, name, here) {
		return
	}

	key := prefix + name
	switch elemType := scalarListElem(fieldType); {
	case scalar(fieldType):
		set.add(
			Param{
				Name:  key,
				At:    here,
				Kind:  kindOf(fieldType),
				UUID:  fieldType == uuidType(),
				List:  false,
				Sort:  false,
				Role:  "",
				Order: 0,
			},
		)
	case elemType != nil:
		set.add(
			Param{
				Name: key,
				At:   here,
				Kind: kindOf(elemType),
				List: true,
				UUID: elemType == uuidType(), Sort: false, Role: "", Order: 0,
			},
		)
	case fieldType.Kind() == reflect.Struct && !nested:
		set.walk(fieldType, here, key+".", true)
	default:
		set.opaque = append(set.opaque, key)
	}
}

// topLevel takes a top-level field that is the paging query or the input's own filters object.
func (set *paramSet) topLevel(fieldType reflect.Type, name string, here []string) bool {
	switch {
	case fieldType == queryType():
		set.queries = append(set.queries, here)
	case name == fieldFilters && fieldType == rawType():
		set.filterAt = append(set.filterAt, here)
	default:
		return false
	}

	return true
}

// addPaging adds cursor, limit and sort of the paging query at queryPath, and its filters object.
func (set *paramSet) addPaging(queryPath []string) {
	set.add(
		Param{
			Name:  paramCursor,
			At:    append(slices.Clone(queryPath), paramCursor),
			Role:  paramCursor,
			Kind:  0,
			List:  false,
			Sort:  false,
			UUID:  false,
			Order: 0,
		},
	)
	set.add(
		Param{
			Name: paramLimit,
			At:   append(slices.Clone(queryPath), paramLimit),
			Kind: KindInteger,
			Role: paramLimit, List: false, Sort: false, UUID: false, Order: 0,
		},
	)
	set.add(
		Param{
			Name:  paramSort,
			At:    append(slices.Clone(queryPath), paramSort),
			Sort:  true,
			Role:  paramSort,
			Kind:  0,
			List:  false,
			UUID:  false,
			Order: 0,
		},
	)
	set.filterAt = append(set.filterAt, append(slices.Clone(queryPath), fieldFilters))
}

// addFilters adds the catalogued filters of the filters object at filtersPath, except those an input field names.
func (set *paramSet) addFilters(filtersPath, filters []string) {
	for _, name := range filters {
		if _, taken := set.params[name]; taken {
			continue
		}

		filterKind := FilterKinds[name]
		set.params[name] = Param{
			Name:  name,
			At:    append(slices.Clone(filtersPath), name),
			Kind:  filterKind.Kind,
			List:  filterKind.List,
			Role:  roleFilter,
			Order: len(set.params), Sort: false, UUID: false,
		}
	}
}

func deref(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	return t
}

// scalarListElem is the element type of a list of scalars, nil for any other type.
func scalarListElem(t reflect.Type) reflect.Type {
	if t.Kind() != reflect.Slice || t == rawType() {
		return nil
	}

	if elemType := deref(t.Elem()); scalar(elemType) {
		return elemType
	}

	return nil
}

func scalar(t reflect.Type) bool {
	if t == timeType() || t == uuidType() {
		return true
	}

	switch t.Kind() {
	case reflect.String,
		reflect.Bool,
		reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64,
		reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
		reflect.Float32,
		reflect.Float64:
		return true
	case reflect.Invalid,
		reflect.Uintptr,
		reflect.Complex64,
		reflect.Complex128,
		reflect.Array,
		reflect.Chan,
		reflect.Func,
		reflect.Interface,
		reflect.Map,
		reflect.Pointer,
		reflect.Slice,
		reflect.Struct,
		reflect.UnsafePointer:
		return false
	}

	return false
}

func kindOf(t reflect.Type) ParamKind {
	if t == timeType() || t == uuidType() {
		return KindString
	}

	switch t.Kind() {
	case reflect.Bool:
		return KindBool
	case reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64,
		reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64:
		return KindInteger
	case reflect.Float32, reflect.Float64:
		return KindNumber
	case reflect.Invalid,
		reflect.Uintptr,
		reflect.Complex64,
		reflect.Complex128,
		reflect.Array,
		reflect.Chan,
		reflect.Func,
		reflect.Interface,
		reflect.Map,
		reflect.Pointer,
		reflect.Slice,
		reflect.String,
		reflect.Struct,
		reflect.UnsafePointer:
		return KindString
	}

	return KindString
}

// value reads the parameter's text; nil means absent (an empty value).
func (p Param) value(values []string) (any, bool) {
	if p.Sort {
		return sortValue(values)
	}

	if p.List {
		return listValue(p.Kind, values)
	}

	if len(values) != 1 {
		return nil, false
	}

	if values[0] == "" {
		return nil, true
	}

	return convert(p.Kind, values[0])
}

// sortValue reads sort=field:direction; without a direction the order is ascending.
func sortValue(values []string) (any, bool) {
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

// listValue reads a list parameter: repeated, or its values separated by commas.
func listValue(kind ParamKind, values []string) (any, bool) {
	var items []any

	for _, value := range values {
		for text := range strings.SplitSeq(value, ",") {
			if text = strings.TrimSpace(text); text == "" {
				continue
			}

			item, ok := convert(kind, text)
			if !ok {
				return nil, false
			}

			items = append(items, item)
		}
	}

	if items == nil { // a list parameter with only empty values is the empty list (unitIds= → [])
		return []any{}, true
	}

	return items, true
}

func convert(k ParamKind, text string) (any, bool) {
	switch k {
	case KindString:
	case KindInteger:
		n, err := strconv.ParseInt(text, 10, 64)

		return json.Number(strconv.FormatInt(n, 10)), err == nil
	case KindNumber:
		f, err := strconv.ParseFloat(text, 64)
		if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
			return nil, false
		}

		return json.Number(strconv.FormatFloat(f, 'g', -1, 64)), true
	case KindBool:
		switch text {
		case "true":
			return true, true
		case "false":
			return false, true
		}

		return nil, false
	}

	return text, true
}

// place sets value at the path unless the object already holds another value there.
func place(obj map[string]any, fieldPath []string, value any) bool {
	for _, key := range fieldPath[:len(fieldPath)-1] {
		next, present := obj[key]
		if !present || next == nil {
			nested := map[string]any{}
			obj[key] = nested
			obj = nested

			continue
		}

		nested, isObject := next.(map[string]any)
		if !isObject {
			return false
		}

		obj = nested
	}

	leaf := fieldPath[len(fieldPath)-1]
	if old, present := obj[leaf]; present && old != nil {
		oldJSON, oldErr := json.Marshal(old)
		newJSON, newErr := json.Marshal(value)

		return oldErr == nil && newErr == nil && bytes.Equal(oldJSON, newJSON)
	}

	obj[leaf] = value

	return true
}

// Input builds the operation input of a request on this route; body is the request body of POST, PUT and PATCH.
func (b *Binding) Input(ectx *echo.Context, body []byte) ([]byte, error) {
	obj := map[string]any{}
	fieldErrs := map[string]string{}
	query := ectx.QueryParams()

	if b.Body {
		var err error

		obj, err = bodyObject(body)
		if err != nil {
			return nil, err
		}

		for name := range query {
			fieldErrs[name] = apperr.KeyNotAllowed // a body route reads its input from the body
		}
	} else {
		b.queryInput(obj, query, fieldErrs)
	}

	err := b.pathInput(ectx, obj, fieldErrs)
	if err != nil {
		return nil, err
	}

	if f := b.Route.FixedField; f != "" && !place(obj, b.Params[f].At, b.Route.FixedValue) {
		fieldErrs[f] = apperr.KeyPathMismatch
	}

	if len(fieldErrs) > 0 {
		return nil, apperr.Fields(fieldErrs)
	}

	out, err := json.Marshal(obj)

	return out, apperr.Wrap(err, "json.Marshal")
}

// bodyObject decodes the JSON object body with numbers kept as their text; an empty body is the empty object.
func bodyObject(body []byte) (map[string]any, error) {
	obj := map[string]any{}
	if len(bytes.TrimSpace(body)) == 0 {
		return obj, nil
	}

	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()

	err := dec.Decode(&obj)
	if err != nil || obj == nil || dec.More() {
		return nil, apperr.Fields(map[string]string{"_": apperr.KeyMalformedInput})
	}

	return obj, nil
}

// queryInput places the query parameters of a GET or DELETE route; an unknown parameter, a path parameter and the
// fixed field are not allowed.
func (b *Binding) queryInput(obj map[string]any, query url.Values, fieldErrs map[string]string) {
	for name, values := range query {
		param, ok := b.Params[name]
		if !ok || slices.Contains(b.Path, name) || name == b.Route.FixedField {
			fieldErrs[name] = apperr.KeyNotAllowed

			continue
		}

		value, ok := param.value(values)
		if !ok {
			fieldErrs[name] = apperr.KeyInvalid
		} else if value != nil {
			place(obj, param.At, value)
		}
	}
}

// pathInput places the path parameters; an ID that is not a UUID names no resource (NOT_FOUND).
func (b *Binding) pathInput(
	ectx *echo.Context,
	obj map[string]any,
	fieldErrs map[string]string,
) error {
	for _, name := range b.Path {
		raw, err := url.PathUnescape(ectx.Param(name))
		if param := b.Params[name]; param.UUID && err == nil && raw != "" {
			_, perr := uuid.Parse(
				raw,
			)
			if perr != nil { // no resource has this ID: the path names nothing
				return apperr.E(apperr.NotFound, apperr.KeyNotFound)
			}
		}

		value, ok := convert(b.Params[name].Kind, raw)
		switch {
		case err != nil || raw == "" || !ok:
			fieldErrs[name] = apperr.KeyInvalid
		case !place(obj, b.Params[name].At, value):
			fieldErrs[name] = apperr.KeyPathMismatch
		}
	}

	return nil
}

// MountREST registers the REST routes of the operations this process serves on the /v1 group (IR222).
func (r *Registry) MountREST(group *echo.Group) error {
	for _, operation := range r.Operations() {
		if !r.serves(operation.Spec.Name) {
			continue
		}

		for _, route := range operation.Spec.Routes {
			binding, err := NewBinding(operation, route)
			if err != nil {
				return err
			}

			path, ok := strings.CutPrefix(route.Path, "/v1/")
			if !ok {
				return fmt.Errorf("ops: %s: %w: %s", operation.Spec.Name, errOutsideV1, route.Path)
			}

			_, addRouteErr := group.AddRoute(
				echo.Route{
					Method:  route.Method,
					Path:    "/" + EchoPath(path),
					Name:    operation.Spec.Name,
					Handler: r.restHandler(operation, binding), Middlewares: nil,
				},
			)
			if addRouteErr != nil {
				return fmt.Errorf("ops: %s: %w", operation.Spec.Name, addRouteErr)
			}
		}
	}

	return nil
}

func (r *Registry) restHandler(operation *Operation, binding *Binding) echo.HandlerFunc {
	return func(ectx *echo.Context) error {
		corr := correlationID(ectx)

		var body []byte

		if binding.Body {
			var err error

			body, err = readBody(ectx, operation.Spec.Name)
			if err != nil {
				return fail(ectx, err, corr)
			}
		}

		in, err := binding.Input(ectx, body)
		if err != nil {
			return fail(ectx, err, corr)
		}

		return r.run(ectx, operation, in, corr)
	}
}
