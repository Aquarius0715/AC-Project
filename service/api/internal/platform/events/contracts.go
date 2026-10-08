package events

import (
	"encoding/json"

	"github.com/google/uuid"
)

// Event contracts between business-domain services (IR184). Producers publish these types; the owning service of
// the affected data consumes them. Payloads are the only shared shape.
const (
	// billing (restrictions) → equipment
	UnitRestrictionApplied = "UnitRestrictionApplied" // set assets.units.observed_restriction
	UnitRestrictionCleared = "UnitRestrictionCleared" // clear it when it still names the restriction
	RecoveryCasesChanged   = "RecoveryCasesChanged"   // SR29: bump the version of the restriction's units
	ReconciliationRequired = "ReconciliationRequired" // open one reconciliation_required alert per unit
	ReconciliationResolved = "ReconciliationResolved" // resolve it (units without unresolved recovery cases)
)

// UnitRestriction is the payload of UnitRestrictionApplied / UnitRestrictionCleared.
type UnitRestriction struct {
	UnitID        uuid.UUID       `json:"unitId"`
	RestrictionID uuid.UUID       `json:"restrictionId"`
	Observed      json.RawMessage `json:"observed,omitempty"` // ObservedRestriction for Applied
}

// Units is the payload of RecoveryCasesChanged / ReconciliationRequired / ReconciliationResolved.
type Units struct {
	RestrictionID uuid.UUID   `json:"restrictionId"`
	UnitIDs       []uuid.UUID `json:"unitIds"`
}
