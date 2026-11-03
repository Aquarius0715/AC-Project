package energy

// The values the energy module compares and writes by name: states, events, field, sort and column keys.
const (
	colTenantID            = "tenant_id"
	colUnitID              = "unit_id"
	eventRequest           = "request"
	eventRetry             = "retry"
	keyCreatedAt           = "createdAt"
	keyEnergyRange         = "errors.energy_range"
	keyFilters             = "filters"
	keyUnitIDs             = "unitIds"
	keyUpdatedAt           = "updatedAt"
	methodPeriodComparison = "demo_period_comparison"
	stateDemoPurchased     = "demo_purchased"
	stateDemoRequested     = "demo_requested"
	statusPending          = "pending"
)

// The values this package compares and writes by name.
const (
	colCreatedAt = "r.created_at"
)
