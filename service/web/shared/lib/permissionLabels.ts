// Every permission in words (IR821): the screens name a permission by what it lets a person do, not by its code
// ("alert.resolve"). The user decided on 2026-11-16 to replace the earlier rule that kept the codes. The English label
// is also the dictionary key. Access & roles shows the label with the code beside it, so an administrator can still
// match the two.
import { translator, type T } from "@ac/web/lib/i18n";
import { lookup } from "@ac/web/lib/lookup";

const en = translator("en");

const LABEL: Record<string, string> = {
  "dashboard.read": "View the dashboard",
  "asset.read": "View customers & units",
  "asset.write": "Edit customers & units",
  "identity.read": "View users & roles",
  "identity.write": "Edit users & roles",
  "device.read": "View devices & models",
  "device.write": "Edit devices & models",
  "device.maintain": "Maintain devices",
  "control.execute": "Control ACs",
  "control.diagnose": "Run test runs",
  "alert.read": "View alerts",
  "alert.resolve": "Resolve alerts",
  "alert.policy.read": "View alert policies",
  "alert.policy.write": "Edit alert policies",
  "job.read": "View maintenance jobs",
  "job.write": "Edit maintenance jobs",
  "contract.read": "View contracts",
  "contract.write": "Edit contracts",
  "billing.read": "View billing",
  "billing.write": "Edit billing",
  "billing.payment": "Confirm payments",
  "restriction.read": "View restrictions",
  "restriction.write": "Edit restrictions",
  "restriction.override": "Override restrictions",
  "automation.policy.read": "View automation policies",
  "automation.policy.write": "Edit automation policies",
  "energy.read": "View energy",
  "energy.write": "Edit energy",
  "mrv.read": "View MRV reports",
  "mrv.write": "Edit MRV reports",
  "mrv.review": "Review MRV reports",
  "mrv.factors": "Change emission factors",
  "offset.read": "View offsets",
  "offset.write": "Edit offsets",
  "audit.read": "View the audit log",
  "partner.accept": "Accept job offers",
  "partner.assign": "Assign technicians",
  "partner.review": "Review technician reports",
  "data.forceDelete": "Force-delete data",
};

/** The permission in the display language; a code this list does not know stays as it is. */
export const permissionLabel = (code: string, t: T = en): string => {
  const label = lookup(LABEL, code);
  return label ? t(label) : code;
};

/** The dictionary keys this module uses (the key check of the tests). */
export const PERMISSION_LABEL_KEYS = Object.values(LABEL);
