package maintenance

import (
	"context"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// QrAccess answers units.resolveQr's assignment question for equipment (IR145, IR194).
type QrAccess struct{}

// QueryQrAssignment is the internal query behind QrAccess.
const QueryQrAssignment = "maintenance.qrAssignment"

// QrAssignmentResult is QueryQrAssignment output.
type QrAssignmentResult struct {
	Assigned bool       `json:"assigned"`
	JobID    *uuid.UUID `json:"jobId"`
}

// QrAssignment reports the caller's active Assignment on the unit and the open job whose scheduled window ends first
// after now.
func (QrAccess) QrAssignment(
	ctx context.Context,
	c *ops.Call,
	unit uuid.UUID,
) (bool, *uuid.UUID, error) {
	r, err := ops.Delegate(ctx, c, QueryQrAssignment, UnitInput{UnitID: unit}, qrAssignment)

	return r.Assigned, r.JobID, apperr.Wrap(err, "delegate maintenance.qrAssignment")
}

func qrAssignment(
	ctx context.Context,
	call *ops.Call,
	input *UnitInput,
) (QrAssignmentResult, error) {
	var qrAssignmentResult QrAssignmentResult

	err := call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM maintenance.assignments a JOIN maintenance.jobs j ON j.id =
		a.job_id
			WHERE j.unit_id = $1 AND a.technician_membership_id = $2 AND a.status = 'active'),
		(SELECT j.id FROM maintenance.assignments a JOIN maintenance.jobs j ON j.id = a.job_id
			WHERE j.unit_id = $1 AND a.technician_membership_id = $2 AND a.status = 'active' AND j.status IN ('assigned',
			'in_progress','on_hold','rework_requested')
			  AND upper(a.scheduled) > $3 ORDER BY upper(a.scheduled), j.id LIMIT
			  1)`, input.UnitID, call.Principal.MembershipID, call.Now).
		Scan(&qrAssignmentResult.Assigned, &qrAssignmentResult.JobID)

	return qrAssignmentResult, apperr.Wrap(err, "select maintenance.assignments")
}
