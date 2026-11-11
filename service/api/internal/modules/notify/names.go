package notify

// The values the notify module compares and writes by name: target kinds, notice types, channels and columns.
const (
	channelInApp          = "inApp"
	colCustomerID         = "customer_id"
	colCustomerOrgID      = "customer_org_id"
	colJobID              = "job_id"
	colUnitID             = "unit_id"
	keyIDs                = "ids"
	keyStatus             = "status"
	noticeAlert           = "alert"
	noticeCompletion      = "completion"
	noticeDeviceOperation = "device_operation"
	noticeJobUpdate       = "job_update"
	noticePayment         = "payment"
	noticePaymentReminder = "payment_reminder"
	noticeQuality         = "quality"
	noticeReportReturn    = "report_return"
	noticeScheduleChange  = "schedule_change"
	severityNormal        = "normal"
	targetInquiry         = "inquiry"
	targetInvoice         = "invoice"
	targetJob             = "job"
	targetRestriction     = "restriction"
	targetUnit            = "unit"
	employmentInternal    = "internal" // a technician employed by the operator (Principal.Employment)
)

// The values this package compares and writes by name.
const (
	colTenantID = "tenant_id"
)
