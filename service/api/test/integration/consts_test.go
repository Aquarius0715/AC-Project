package integration_test

import "github.com/pradita/ac-project/service/api/internal/platform/apperr"

// The operations the tables and lists name.
const (
	opAlertsList                  = "alerts.list"
	opAutomationsNextRuns         = "automations.nextRuns"
	opAutomationsSave             = "automations.save"
	opClientUsersList             = "clientUsers.list"
	opClientUsersRemove           = "clientUsers.remove"
	opClientUsersSave             = "clientUsers.save"
	opCommandsCreate              = "commands.create"
	opCommandsGet                 = "commands.get"
	opCommandsList                = "commands.list"
	opContractorsList             = "contractors.list"
	opDemoSessionExtend           = "demoSession.extend"
	opDemoSessionSignIn           = "demoSession.signIn"
	opDemoSessionSwitchMembership = "demoSession.switchMembership"
	opDevicesCalibrations         = "devices.calibrations"
	opDevicesEvents               = "devices.events"
	opDevicesList                 = "devices.list"
	opDevicesOperations           = "devices.operations"
	opEnergyExportReport          = "energy.exportReport"
	opFilterCareList              = "filterCare.list"
	opInquiriesList               = "inquiries.list"
	opJobsAccept                  = "jobs.accept"
	opJobsAcknowledgeAssignment   = "jobs.acknowledgeAssignment"
	opJobsAddNote                 = "jobs.addNote"
	opJobsAssign                  = "jobs.assign"
	opJobsCheckIn                 = "jobs.checkIn"
	opJobsDecline                 = "jobs.decline"
	opJobsEvents                  = "jobs.events"
	opJobsGet                     = "jobs.get"
	opJobsList                    = "jobs.list"
	opJobsSaveDraft               = "jobs.saveDraft"
	opLocationsRename             = "locations.rename"
	opMembersEligible             = "members.eligible"
	opOffsetsPreview              = "offsets.preview"
	opOffsetsSimulate             = "offsets.simulate"
	opPartnerUsersRemove          = "partnerUsers.remove"
	opPartnerUsersResendInvite    = "partnerUsers.resendInvite"
	opPartnerUsersSave            = "partnerUsers.save"
	opPartsList                   = "parts.list"
	opPaymentsRecordManual        = "payments.recordManual"
	opPaymentsSimulate            = "payments.simulate"
	opPoliciesGet                 = "policies.get"
	opPoliciesList                = "policies.list"
	opPoliciesSave                = "policies.save"
	opPoliciesSetDefaultRule      = "policies.setDefaultRule"
	opPreferencesUpdate           = "preferences.update"
	opReportsGet                  = "reports.get"
	opReportsSignOff              = "reports.signOff"
	opRestrictionsForInvoice      = "restrictions.forInvoice"
	opRestrictionsRetry           = "restrictions.retry"
	opSessionGet                  = "session.get"
	opSummariesGet                = "summaries.get"
	opTelemetrySeries             = "telemetry.series"
	opUnitsList                   = "units.list"
	opWritesGetResult             = "writes.getResult"
)

// Input and result field names.
const (
	fieldAccessValidUntil        = "accessValidUntil"
	fieldAction                  = "action"
	fieldActiveWindow            = "activeWindow"
	fieldAlternativeSlots        = "alternativeSlots"
	fieldAttachmentIDs           = "attachmentIds"
	fieldBoundary                = "boundary"
	fieldBoundaryID              = "boundaryId"
	fieldCode                    = "code"
	fieldCondition               = "condition"
	fieldContractID              = "contractId"
	fieldContractorOrgID         = "contractorOrgId"
	fieldCurrency                = "currency"
	fieldCustomerID              = "customerId"
	fieldDeviceID                = "deviceId"
	fieldDeviceIDs               = "deviceIds"
	fieldDueAt                   = "dueAt"
	fieldDurationSeconds         = "durationSeconds"
	fieldEmail                   = "email"
	fieldEmployment              = "employment"
	fieldEnabled                 = "enabled"
	fieldEndAt                   = "endAt"
	fieldEndLocal                = "endLocal"
	fieldEventID                 = "eventId"
	fieldExecuteAfter            = "executeAfter"
	fieldExpectedContractVersion = "expectedContractVersion"
	fieldFilters                 = "filters"
	fieldFrom                    = "from"
	fieldItems                   = "items"
	fieldJobID                   = "jobId"
	fieldKind                    = "kind"
	fieldMeasurements            = "measurements"
	fieldMetric                  = "metric"
	fieldModelID                 = "modelId"
	fieldName                    = "name"
	fieldNextAction              = "nextAction"
	fieldOccurredAt              = "occurredAt"
	fieldOperator                = "operator"
	fieldOrganizationID          = "organizationId"
	fieldPeriod                  = "period"
	fieldPermissions             = "permissions"
	fieldPhase                   = "phase"
	fieldPlanType                = "planType"
	fieldPriority                = "priority"
	fieldReason                  = "reason"
	fieldRecipientMembershipIDs  = "recipientMembershipIds"
	fieldRecoveryThreshold       = "recoveryThreshold"
	fieldReportID                = "reportId"
	fieldRequestedEnd            = "requestedEnd"
	fieldRequestedStart          = "requestedStart"
	fieldRestrictionID           = "restrictionId"
	fieldRole                    = "role"
	fieldRulesVersion            = "rulesVersion"
	fieldScopes                  = "scopes"
	fieldSeverity                = "severity"
	fieldStartAction             = "startAction"
	fieldStartAt                 = "startAt"
	fieldStartLocal              = "startLocal"
	fieldStatus                  = "status"
	fieldTargetVersion           = "targetVersion"
	fieldTermsVersion            = "termsVersion"
	fieldThreshold               = "threshold"
	fieldTimezone                = "timezone"
	fieldType                    = "type"
	fieldUnitID                  = "unitId"
	fieldUnitIDs                 = "unitIds"
	fieldUserID                  = "userId"
	fieldValidFrom               = "validFrom"
	fieldValidUntil              = "validUntil"
	fieldWeekdays                = "weekdays"
)

// The field errors of a VALIDATION refusal (field=message key).
const (
	refusedActionInvalid         = "action=" + apperr.KeyInvalid
	refusedActionUnsupported     = "action=" + apperr.KeyUnsupportedAction
	refusedChannelsInvalid       = "channels=" + apperr.KeyInvalid
	refusedCodeInvalid           = "code=" + apperr.KeyInvalid
	refusedConditionInvalid      = "condition=" + apperr.KeyInvalid
	refusedDurationMinutesRange  = "durationMinutes=" + apperr.KeyRange
	refusedEmailImmutable        = "email=" + apperr.KeyImmutable
	refusedEmailTaken            = "email=errors.email_taken"
	refusedEnergyRange           = "from=errors.energy_range"
	refusedFileInvalid           = "file=" + apperr.KeyInvalidFile
	refusedFilterKindInvalid     = "filters.kind=" + apperr.KeyInvalid
	refusedFilterStatusInvalid   = "filters.status=" + apperr.KeyInvalid
	refusedFilterStatusesInvalid = "filters.statuses=" + apperr.KeyInvalid
	refusedFilterToRange         = "filters.to=" + apperr.KeyRange
	refusedFiltersInvalid        = "filters=" + apperr.KeyInvalid
	refusedInvoiceIDRequired     = "invoiceId=" + apperr.KeyRequired
	refusedKindFixed             = "kind=" + apperr.KeyKindFixed
	refusedKindInvalid           = "kind=" + apperr.KeyInvalid
	refusedMalformedInput        = "_=" + apperr.KeyMalformedInput
	refusedMessageLength         = "message=" + apperr.KeyLength
	refusedMethodInvalid         = "method=" + apperr.KeyInvalid
	refusedMetricInvalid         = "metric=" + apperr.KeyInvalid
	refusedNameLength            = "name=" + apperr.KeyLength
	refusedPriorityRange         = "priority=" + apperr.KeyRange
	refusedReasonLength          = "reason=" + apperr.KeyLength
	refusedReasonRequired        = "reason=" + apperr.KeyRequired
	refusedSortFieldInvalid      = "sort.field=" + apperr.KeyInvalid
	refusedStatusNotAllowed      = "status=" + apperr.KeyNotAllowed
	refusedTimezoneInvalid       = "timezone=" + apperr.KeyInvalid
	refusedToRange               = "to=" + apperr.KeyRange
	refusedUnitArchived          = "unitId=" + apperr.KeyUnitArchived
	refusedUnitIDsInvalid        = "unitIds=" + apperr.KeyInvalid
	refusedUnknownFieldX         = "x=" + apperr.KeyNotAllowed
)

// Module message keys.
const (
	keyAssignmentEnded      = "errors.assignment_ended"
	keyAssignmentNotStarted = "errors.assignment_not_started"
	keyOwnAccount           = "errors.own_account"
	keyAccessExpired        = "errors.access_expired"
	keyUnitBusy             = "errors.unit_busy"
)

// Statuses and states.
const (
	statusAccepted         = "accepted"
	statusAcknowledged     = "acknowledged"
	statusActive           = "active"
	statusApplied          = "applied"
	statusAssigned         = "assigned"
	statusCancelled        = "cancelled"
	statusCompleted        = "completed"
	statusDisabled         = "disabled"
	statusDraft            = "draft"
	statusExpired          = "expired"
	statusExpiring         = "expiring"
	statusFailed           = "failed"
	statusInProgress       = "in_progress"
	statusInvited          = "invited"
	statusNotSent          = "not_sent"
	statusOffered          = "offered"
	statusOnHold           = "on_hold"
	statusOpen             = "open"
	statusPaused           = "paused"
	statusPending          = "pending"
	statusQueued           = "queued"
	statusRejected         = "rejected"
	statusReleaseRequested = "release_requested"
	statusReleased         = "released"
	statusRemoving         = "removing"
	statusRequested        = "requested"
	statusResolved         = "resolved"
	statusRunning          = "running"
	statusScheduled        = "scheduled"
	statusSent             = "sent"
	statusUnpaid           = "unpaid"
	statusWithdrawn        = "withdrawn"
)

// Evaluation decisions, their reasons and the outcomes of runs.
const (
	decisionRequested    = "requested"
	decisionSelected     = "selected"
	decisionSuppressed   = "suppressed"
	reasonConsentRevoked = "consent_revoked"
	reasonDisabled       = "disabled"
	reasonNoMatch        = "no_match"
	reasonOffline        = "offline"
)

// Roles.
const (
	clientRoleMember = "member"
	clientRoleOwner  = "owner"
	roleAdmin        = "admin"
	roleClient       = "client"
	roleContractor   = "contractor"
	roleTechnician   = "technician"
)

// The names of table cases that several tests share.
const (
	caseBadAction       = "bad action"
	caseBadCode         = "bad code"
	caseBadEmail        = "bad email"
	caseBadMethod       = "bad method"
	caseBadMode         = "bad mode"
	caseBadOperator     = "bad operator"
	caseBadRole         = "bad role"
	caseBadStatus       = "bad status"
	caseBadType         = "bad type"
	caseBadZone         = "bad zone"
	caseBlankName       = "blank name"
	caseBlankReason     = "blank reason"
	caseBothTargets     = "both targets"
	caseDuplicateUnit   = "duplicate unit"
	caseDuplicateUnits  = "duplicate units"
	caseFuture          = "future"
	caseNoName          = "no name"
	caseNoTarget        = "no target"
	caseNoUnits         = "no units"
	caseNoWeekdays      = "no weekdays"
	caseOtherCustomer   = "other customer"
	casePNGAsJPEG       = "png as jpeg"
	casePast            = "past"
	casePixelFlood      = "pixel flood"
	casePriorityOver100 = "priority 101"
	caseSizeMismatch    = "size mismatch"
	caseUnitMismatch    = "unit mismatch"
	caseUnknownField    = "unknown field"
	caseUnknownMetric   = "unknown metric"
	caseUnknownModel    = "unknown model"
	caseUnknownUnit     = "unknown unit"
	caseWeekdayEight    = "weekday 8"
	caseZeroDuration    = "duration 0"
)

// Request bodies.
const (
	bodyEmptyQuery     = `{"query":{}}`
	bodyFirstHundred   = `{"limit":100}`
	bodyInfoSeverity   = `{"filters":{"severity":"info"}}`
	bodyReversedPeriod = `{"filters":{"from":"2026-09-14T01:00:00Z","to":"2026-09-14T00:00:00Z"}}`
	bodySortByName     = `{"sort":{"field":"name","direction":"asc"}}`
	bodyUnknownFilter  = `{"filters":{"x":1}}`
	bodyUnknownFilterY = `{"filters":{"y":1}}`
	bodyUnknownKind    = `{"filters":{"kind":"x"}}`
	bodyUnknownStatus  = `{"filters":{"status":"x"}}`
)

// Command actions and policies (JSON).
const (
	actionCoolTo25     = `{"kind":"set_temperature","celsius":25}`
	actionPowerOff     = `{"kind":"set_power","power":false}`
	actionPowerOn      = `{"kind":"set_power","power":true}`
	actionVentilateLow = `{"kind":"ventilate","level":"low"}`
	policyPowerOff     = `{"kind":"power_off"}`
)

// Raw JSON values of builder overrides.
const (
	jsonBlank          = `" "`
	jsonFalse          = "false"
	jsonKindRTO        = `"rto"`
	jsonKualaLumpur    = `"Asia/Kuala_Lumpur"`
	jsonMondays        = "[1]"
	jsonNull           = "null"
	jsonRoleTechnician = `"technician"`
	jsonTenOClock      = `"10:00"`
	jsonTrue           = "true"
)

// API error codes and the failure codes of device operations.
const (
	codeConflict       = "CONFLICT"
	codeDeviceRejected = "DEVICE_REJECTED"
	codeOffline        = "OFFLINE"
	codeTimeout        = "TIMEOUT"
	codeUnavailable    = "UNAVAILABLE"
)

// States of a diagnostic test run.
const stateEndFailed = "end_failed"

// Units and their readings: connection, power, metrics, measurement quality and alert severity.
const (
	connectionOffline = "offline"
	connectionOnline  = "online"
	metricCO2         = "co2"
	metricHumidity    = "humidity"
	metricPM25        = "pm25"
	metricTemperature = "temperature"
	operatorAtLeast   = "gte"
	powerOff          = "off"
	powerUnknown      = "unknown"
	qualitySuspect    = "suspect"
	qualityValid      = "valid"
	severityNormal    = "normal"
	severityWarning   = "warning"
	stateAvailable    = "available"
)

// The fixture's memberships and the internal token of the split services.
const (
	internalToken           = "test-internal"
	membershipCustomerA     = "customer-a"
	membershipCustomerB     = "customer-b"
	membershipHQOperator    = "hq-operator"
	membershipTechInternalA = "tech-internal-a"
)

// Projections, kinds and other answer values.
const (
	deliveryPreview   = "preview"
	kindAutomation    = "automation"
	kindProperty      = "property"
	mimePNG           = "image/png"
	projectionHistory = "history"
	projectionOffer   = "offer"
	projectionSummary = "summary"
)

// oneSlotSQL is the SQL of an offer's candidate_slots holding one time, from two timestamp expressions (IR636).
func oneSlotSQL(start, end string) string {
	return "jsonb_build_array(jsonb_build_object('startAt'," + start + "::timestamptz,'endAt'," + end + "::timestamptz))"
}
