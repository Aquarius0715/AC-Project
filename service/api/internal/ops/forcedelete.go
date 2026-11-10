package ops

import (
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// ForceDeletePermission is the permission that allows a forced delete (IR690): a delete that does not stop at the
// accounts a customer or a contractor company still has, even those that acted. Nobody holds it by default.
const ForceDeletePermission = "data.forceDelete"

// KeyForceNotAllowed is the FORBIDDEN of a forced delete asked by a caller without ForceDeletePermission.
const KeyForceNotAllowed = "errors.force_delete_not_allowed"

// CheckForce is FORBIDDEN when force is asked and the caller does not hold ForceDeletePermission.
func CheckForce(call *Call, force bool) error {
	if force && !call.Principal.Permissions[ForceDeletePermission] {
		return apperr.E(apperr.Forbidden, KeyForceNotAllowed)
	}

	return nil
}

// ForcedReason is the reason as the audit log records it: a forced delete says so.
func ForcedReason(reason string, force bool) string {
	if force {
		return "[forced] " + reason
	}

	return reason
}
