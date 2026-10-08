package events

import (
	"encoding/json"
	"time"

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

	RestrictionCommandRequested  = "RestrictionCommandRequested"  // create the device command billing decided (IR185)
	RestrictionCommandsCancelled = "RestrictionCommandsCancelled" // cancel undelivered restriction commands

	// equipment → billing (restrictions)
	UnitRestrictionObserved = "UnitRestrictionObserved" // a device reported the restriction it enforces (SR26)
	CommandAcknowledged     = "CommandAcknowledged"     // a restriction command was acknowledged in time
	CommandsEnded           = "CommandsEnded"           // restriction commands failed or expired
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

// RestrictionCommand is the payload of RestrictionCommandRequested: the full command row billing decided (the ID is
// assigned by billing so restriction records can reference it before equipment stores it).
type RestrictionCommand struct {
	CommandID     uuid.UUID       `json:"commandId"`
	UnitID        uuid.UUID       `json:"unitId"`
	DeviceID      *uuid.UUID      `json:"deviceId"`
	ActorID       uuid.UUID       `json:"actorMembershipId"`
	Action        json.RawMessage `json:"action"`
	RestrictionID uuid.UUID       `json:"restrictionId"`
	Status        string          `json:"status"`
	Delivery      string          `json:"delivery"`
	RequestedAt   time.Time       `json:"requestedAt"`
	SentAt        *time.Time      `json:"sentAt"`
	ExpiresAt     time.Time       `json:"expiresAt"`
	CorrelationID string          `json:"correlationId"`
}

// CommandsCancelled is the payload of RestrictionCommandsCancelled: either explicit IDs, or every undelivered intent
// of the restriction on the unit.
type CommandsCancelled struct {
	CommandIDs    []uuid.UUID `json:"commandIds,omitempty"`
	RestrictionID uuid.UUID   `json:"restrictionId"`
	UnitID        *uuid.UUID  `json:"unitId,omitempty"`
}

// Observation is the payload of UnitRestrictionObserved (observed is null when the device enforces none).
type Observation struct {
	UnitID   uuid.UUID       `json:"unitId"`
	Observed json.RawMessage `json:"observed"`
	EventID  uuid.UUID       `json:"eventId"`
	At       time.Time       `json:"at"`
}

// Commands is the payload of CommandAcknowledged / CommandsEnded.
type Commands struct {
	CommandIDs []uuid.UUID `json:"commandIds"`
	At         time.Time   `json:"at"`
}
