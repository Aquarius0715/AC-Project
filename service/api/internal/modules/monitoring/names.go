package monitoring

// The values the monitoring module compares and writes by name: statuses, kinds, metrics and keys.
const (
	keyCreatedAt              = "createdAt"
	keyCustomerID             = "customerId"
	keyFilters                = "filters"
	keyKind                   = "kind"
	keyPolicyID               = "policyId"
	keyUnitID                 = "unitId"
	keyUpdatedAt              = "updatedAt"
	kindAlert                 = "alert"
	kindAutomation            = "automation"
	kindDefaultAlert          = "default_alert"
	kindPolicy                = "policy"
	metricCO2                 = "co2"
	metricHumidity            = "humidity"
	metricPM25                = "pm25"
	metricPower               = "power"
	metricRefrigerantPressure = "refrigerant_pressure"
	metricTemperature         = "temperature"
	metricVibration           = "vibration"
	opGTE                     = "gte"
	outcomeFailed             = "failed"
	qualityValid              = "valid"
	reasonNoRecipient         = "no_recipient"
	statusOpen                = "open"
)
