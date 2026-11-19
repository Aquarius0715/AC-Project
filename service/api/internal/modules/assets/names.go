package assets

// The values the assets module compares and writes by name: states, kinds, events, field and sort keys.
const (
	connectionOnline     = "online"
	coverageExpiring     = "expiring"
	coverageNone         = "no_coverage"
	eventLocationChanged = "LocationChanged"
	eventUnitChanged     = "UnitChanged"
	everyRow             = "TRUE"
	keyCSVText           = "csvText"
	keyDueAt             = "dueAt"
	keyFiltersKind       = "filters.kind"
	keyInstalledAt       = "installedAt"
	keyInstallState      = "installState"
	installPlanned       = "planned"
	installInstalled     = "installed"
	keyModelID           = "modelId"
	keyIoTModelID        = "iotModelId"
	keyName              = "name"
	keyPropertyID        = "propertyId"
	keySpaceID           = "spaceId"
	keyWarrantyEndsAt    = "warrantyEndsAt"
	kindCustomer         = "customer"
	kindProperty         = "property"
	kindSpace            = "space"
	kindUnit             = "unit"
	levelFloor           = "floor"
	powerOff             = "off"
	rowError             = "error"
	rowReady             = "ready"
	rowWarning           = "warning"
	severityWarning      = "warning"
	stateError           = "error"
	stateUnknown         = "unknown"
	statusActive         = "active"
	unarchivedUnits      = "NOT u.archived"
)

// The values this package compares and writes by name.
const (
	levelRoom      = "room"
	statusInactive = "inactive"
)

// The tables of the places units stand in.
const (
	tableProperties = "assets.properties"
	tableSpaces     = "assets.spaces"
)
