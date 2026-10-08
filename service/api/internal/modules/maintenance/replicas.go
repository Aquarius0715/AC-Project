package maintenance

import "github.com/pradita/ac-project/service/api/internal/platform/events"

// Replicas are maintenance's reference copies of other domains' rows (IR192), kept by the maintenance-api consumer
// from RowChanged events: memberships for technician planning, partner checks and the worker's history snapshots.
var Replicas = []events.Replica{
	{Source: "identity.memberships", Table: "maintenance.ref_memberships", Keys: []string{"id"}, Cols: []string{"tenant_id", "version", "created_at", "updated_at",
		"user_id", "organization_id", "role", "employment", "scope_version", "valid_from", "valid_until", "client_role"}},
}
