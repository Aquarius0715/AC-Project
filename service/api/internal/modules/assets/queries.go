package assets

import (
	"context"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// RegisterQueries binds assets' internal queries.
func RegisterQueries(r *ops.Registry) {
	ops.RegisterQuery(r, ops.DomainEquipment, ops.QueryOrgHasCustomer, orgHasCustomer)
}

// orgHasCustomer reports a customer profile of the organization: the organization is deleted after it (DEC-103).
func orgHasCustomer(ctx context.Context, c *ops.Call, input *ops.OrgRecordsInput) (bool, error) {
	var found bool

	err := c.Tx.QueryRow(
		ctx,
		`SELECT EXISTS (SELECT 1 FROM assets.customers WHERE organization_id = $1)`,
		input.OrgID,
	).Scan(&found)

	return found, apperr.Wrap(err, "select assets.customers")
}
