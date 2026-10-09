// Package ops holds the operation registry, the REST bindings of the catalog routes (IR222) and the pipeline every
// operation runs (backend Go design §4).
package ops

import "github.com/google/uuid"

// Mode is read or write (operation-catalog.csv mode column).
type Mode int

const (
	Read Mode = iota
	Write
)

func (m Mode) String() string {
	if m == Write {
		return "write"
	}
	return "read"
}

// Spec is one generated catalog entry.
type Spec struct {
	Name          string
	Mode          Mode
	Module        string
	Authorization string
	Versions      []VersionRule // write-version-catalog.csv rows (empty for reads)
	DesignIDs     string
	Routes        []Route  // operation-catalog.csv rest_routes (IR222)
	Filters       []string // query-catalog.csv allowed_filters: the filter query parameters of a list read
}

// Route is one REST route of an operation (IR222). Path parameters are written {field} after the input field they
// fill (a dotted name fills a field of a nested object); FixedField / FixedValue set one more input field, so one
// operation can serve several verb routes (payouts.transition: /approve with action=approve).
type Route struct {
	Method     string
	Path       string
	FixedField string
	FixedValue string
}

// ParamKind is how the text of a query or path parameter becomes a JSON value (IR222).
type ParamKind int

const (
	KindString  ParamKind = iota // strings, IDs, instants, enums
	KindInteger                  // integers
	KindNumber                   // decimals
	KindBool                     // true or false
)

// FilterKind is the query parameter type of one Query.filters field (generated FilterKinds).
type FilterKind struct {
	Kind ParamKind
	List bool
}

// VersionRule is one write-version-catalog.csv branch: Branch is "all", "<field> present", "<field> omitted",
// "event=a|b" or "branch"; Rule is "required" or "omit".
type VersionRule struct {
	Branch string
	Rule   string
}

// SpecByName indexes Catalog.
func SpecByName() map[string]Spec {
	m := make(map[string]Spec, len(Catalog))
	for _, s := range Catalog {
		m[s.Name] = s
	}
	return m
}

// UnitBrief is the minimal unit view modules exchange (Assets → Maintenance filter care).
type UnitBrief struct {
	ID, OrgID  uuid.UUID
	Connection string
}
