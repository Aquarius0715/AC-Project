// Package ops holds the operation registry and dispatcher for POST /v1/ops/:operation (backend Go design §4).
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
