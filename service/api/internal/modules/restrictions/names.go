package restrictions

// The values the restrictions module compares and writes by name: states, phases, kinds and keys.
const (
	actionApplyRestriction  = "apply_restriction"
	applyNotSent            = "not_sent"
	applySentUnknown        = "sent_unknown"
	caseRemoving            = "removing"
	caseResolved            = "resolved"
	keyExecuteAfter         = "executeAfter"
	keyKind                 = "kind"
	keyPolicy               = "policy"
	keyRestrictionID        = "restrictionId"
	keyRulesVersion         = "rulesVersion"
	keyRulesVersionMismatch = "errors.rules_version_mismatch"
	keySource               = "source"
	keyUnitIDs              = "unitIds"
	kindRestriction         = "restriction"
	opExecute               = "restrictions.execute"
	phaseApply              = "apply"
	phaseRelease            = "release"
	policyTemperatureLimit  = "temperature_limit"
	powerOff                = "off"
	stateApplied            = "applied"
	stateCancelled          = "cancelled"
	stateReleaseRequested   = "release_requested"
	stateReleased           = "released"
	stateRequested          = "requested"
	stateScheduled          = "scheduled"
)

// The values this package compares and writes by name.
const (
	actionRemoveRestriction = "remove_restriction"
	connectionOnline        = "online"
)
