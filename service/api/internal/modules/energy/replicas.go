package energy

import "github.com/pradita/ac-project/service/api/internal/platform/events"

// Replicas are energy's reference copies of other domains' rows (IR189), kept by the energy-api consumer from
// RowChanged events: unit, property, customer and organization ownership for scope checks, evidence attachments,
// alert and job counts of the monthly report, and the minute-slot power samples energy integrates (D07).
var Replicas = []events.Replica{
	{Source: "assets.units", Table: "energy.ref_units", Keys: []string{"id"}, Cols: []string{"tenant_id", "customer_org_id", "property_id", "archived"}},
	{Source: "assets.customers", Table: "energy.ref_customers", Keys: []string{"id"}, Cols: []string{"tenant_id", "organization_id"}},
	{Source: "assets.properties", Table: "energy.ref_properties", Keys: []string{"id"}, Cols: []string{"tenant_id", "customer_org_id", "archived"}},
	{Source: "identity.organizations", Table: "energy.ref_organizations", Keys: []string{"id"}, Cols: []string{"tenant_id", "kind"}},
	{Source: "maintenance.attachments", Table: "energy.ref_attachments", Keys: []string{"id"}, Cols: []string{"tenant_id"}},
	{Source: "monitoring.alerts", Table: "energy.ref_alerts", Keys: []string{"id"}, Cols: []string{"tenant_id", "unit_id", "detected_at"}},
	{Source: "maintenance.jobs", Table: "energy.ref_jobs", Keys: []string{"id"}, Cols: []string{"tenant_id", "unit_id", "status", "updated_at"}},
	{Source: "monitoring.measurements", Table: "energy.ref_power_samples", Keys: []string{"sensor_id", "observed_at", "sequence"},
		Cols: []string{"tenant_id", "unit_id", "value", "unit", "origin", "quality", "boundary_id", "event_id"}},
}
