package identity

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
)

// QueryActiveOrganizations returns which of the given organizations are active (IR190; IR40 customer counts).
const QueryActiveOrganizations = "identity.activeOrganizations"

// OrganizationsInput is QueryActiveOrganizations input.
type OrganizationsInput struct {
	IDs []uuid.UUID `json:"ids"`
}

// RegisterQueries binds identity's internal queries.
func RegisterQueries(r *ops.Registry) {
	ops.RegisterQuery(r, ops.DomainIdentity, QueryActiveOrganizations, func(ctx context.Context, c *ops.Call, in *OrganizationsInput) ([]uuid.UUID, error) {
		rows, err := c.Tx.Query(ctx, `SELECT id FROM identity.organizations WHERE id = ANY($1) AND status = 'active' ORDER BY id`, in.IDs)
		if err != nil {
			return nil, err
		}
		return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	})
}
