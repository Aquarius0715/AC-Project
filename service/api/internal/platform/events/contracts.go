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

	// any domain → identity (IR191)
	NotificationRequested = "NotificationRequested" // store one notification (ID assigned by the producer)
	QualificationGranted  = "QualificationGranted"  // maintenance → identity: a verified certificate covers a grant (IR192)
	JobNoteRequested      = "JobNoteRequested"      // billing → maintenance: an internal job note (payout questions, IR193)
	AuditRecorded         = "AuditRecorded"         // any domain → identity: one audit log entry (IR196)

	// maintenance → equipment, written by row triggers on maintenance.offers / maintenance.assignments (IR186)
	OfferAccessChanged      = "OfferAccessChanged"      // a contractor's access window to a unit (accepted Offers)
	AssignmentAccessChanged = "AssignmentAccessChanged" // a technician's viewing and work window on a unit

	// identity → equipment (IR53, IR214)
	ConsentRevoked = "ConsentRevoked" // a membership withdrew location consent: disable its location automations

	// maintenance → equipment: the source Alert of a filter cleaning reminder (IR134 item 5, IR239)
	FilterCleaningDue     = "FilterCleaningDue"     // open the maintenance Alert of the unit's cleaning cycle
	FilterCleaningCleared = "FilterCleaningCleared" // resolve it: a later cleaning ended the cycle
)

// FilterReminder is the payload of FilterCleaningDue / FilterCleaningCleared; maintenance assigns the Alert ID so the
// reminder's notifications can name their source Alert before equipment stores it.
type FilterReminder struct {
	AlertID       uuid.UUID `json:"alertId"`
	UnitID        uuid.UUID `json:"unitId"`
	CustomerOrgID uuid.UUID `json:"customerOrgId"`
	Evidence      string    `json:"evidence,omitempty"`
	NoRecipient   bool      `json:"noRecipient,omitempty"` // no active recipient: the Alert records a no_recipient DeliveryFailure (SR12)
	At            time.Time `json:"at"`
}

// ConsentRevocation is the payload of ConsentRevoked.
type ConsentRevocation struct {
	MembershipID uuid.UUID `json:"membershipId"`
	Purpose      string    `json:"purpose"`
	At           time.Time `json:"at"`
}

// OfferAccess is the payload of OfferAccessChanged: the Offer's current snapshot; Accepted false removes it.
type OfferAccess struct {
	OfferID          uuid.UUID  `json:"offerId"`
	JobID            uuid.UUID  `json:"jobId"`
	UnitID           *uuid.UUID `json:"unitId"`
	ContractorOrgID  uuid.UUID  `json:"contractorOrgId"`
	Accepted         bool       `json:"accepted"`
	AccessValidFrom  time.Time  `json:"accessValidFrom"`
	AccessValidUntil time.Time  `json:"accessValidUntil"`
}

// AssignmentAccess is the payload of AssignmentAccessChanged; Deleted removes it, revoked ones stay with Active false
// (IR187).
type AssignmentAccess struct {
	AssignmentID           uuid.UUID  `json:"assignmentId"`
	JobID                  uuid.UUID  `json:"jobId"`
	UnitID                 *uuid.UUID `json:"unitId"`
	TechnicianMembershipID uuid.UUID  `json:"technicianMembershipId"`
	Deleted                bool       `json:"deleted"`
	Active                 bool       `json:"active"`
	CreatedAt              time.Time  `json:"createdAt"`
	ScheduledFrom          *time.Time `json:"scheduledFrom"`
	ScheduledUntil         *time.Time `json:"scheduledUntil"`
}

// UnitRestriction is the payload of UnitRestrictionApplied / UnitRestrictionCleared.
type UnitRestriction struct {
	UnitID        uuid.UUID       `json:"unitId"`
	RestrictionID uuid.UUID       `json:"restrictionId"`
	Observed      json.RawMessage `json:"observed,omitempty" swaggertype:"object"` // ObservedRestriction for Applied
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
	Action        json.RawMessage `json:"action" swaggertype:"object"`
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
	Observed json.RawMessage `json:"observed" swaggertype:"object"`
	EventID  uuid.UUID       `json:"eventId"`
	At       time.Time       `json:"at"`
}

// Commands is the payload of CommandAcknowledged / CommandsEnded.
type Commands struct {
	CommandIDs []uuid.UUID `json:"commandIds"`
	At         time.Time   `json:"at"`
	// Statuses are the commands' final statuses for CommandsEnded (failed / expired / cancelled, IR194).
	Statuses map[uuid.UUID]string `json:"statuses,omitempty"`
}

// Target is a notification target.
type Target struct {
	Kind string    `json:"kind"`
	ID   uuid.UUID `json:"id"`
}

// Notification is the payload of NotificationRequested.
type Notification struct {
	NotificationID        uuid.UUID      `json:"notificationId"`
	RecipientMembershipID uuid.UUID      `json:"recipientMembershipId"`
	Channel               string         `json:"channel"`
	Type                  string         `json:"type"`
	TemplateKey           string         `json:"templateKey"`
	Target                Target         `json:"target"`
	Params                map[string]any `json:"params"`
	Severity              string         `json:"severity"`
	SourceAlertID         *uuid.UUID     `json:"sourceAlertId"`
	OccurredAt            time.Time      `json:"occurredAt"`
}

// Grant is the payload of QualificationGranted.
type Grant struct {
	MembershipID uuid.UUID `json:"membershipId"`
	Code         string    `json:"code"`
	ValidFrom    time.Time `json:"validFrom"`
	ValidUntil   time.Time `json:"validUntil"`
}

// JobNote is the payload of JobNoteRequested.
type JobNote struct {
	JobID        uuid.UUID `json:"jobId"`
	AuthorUserID uuid.UUID `json:"authorUserId"`
	Message      string    `json:"message"`
	At           time.Time `json:"at"`
}

// Audit is the payload of AuditRecorded: one audit_log row (the ID is assigned by the writer).
type Audit struct {
	ID              uuid.UUID  `json:"id"`
	ActorID         string     `json:"actorId"`
	ActorRole       string     `json:"actorRole"`
	MembershipID    *uuid.UUID `json:"membershipId"`
	Action          string     `json:"action"`
	TargetKind      string     `json:"targetKind"`
	TargetID        string     `json:"targetId"`
	PreviousVersion *int       `json:"previousVersion"`
	NextVersion     *int       `json:"nextVersion"`
	OccurredAt      time.Time  `json:"occurredAt"`
	CorrelationID   string     `json:"correlationId"`
	Result          string     `json:"result"`
	Reason          string     `json:"reason,omitempty"`
	// MaskedBefore / MaskedAfter are the changed fields, already masked (ops.Masked).
	MaskedBefore map[string]*string `json:"maskedBefore,omitempty"`
	MaskedAfter  map[string]*string `json:"maskedAfter,omitempty"`
}
