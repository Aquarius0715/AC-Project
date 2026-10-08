package billing

import "github.com/pradita/ac-project/service/api/internal/platform/events"

// Replicas are billing's reference copies of other domains' rows (IR194), kept by the billing-api consumer: the
// customer organizations of restrictions and inquiries, and the property of each contract unit (invoices.list
// propertyId filter).
var Replicas = []events.Replica{
	{Source: "assets.customers", Table: "billing.ref_customers", Keys: []string{"id"}, Cols: []string{"tenant_id", "organization_id"}},
	{Source: "assets.units", Table: "billing.ref_units", Keys: []string{"id"}, Cols: []string{"tenant_id", "property_id"}},
}
