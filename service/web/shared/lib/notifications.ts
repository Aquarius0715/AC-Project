// Notification rows (DATA_SOURCE=api): notifications.list items projected into the inbox row shape. Pure code,
// shared by the Server Component that reads the list and the client view that renders it (FR-X07).
import type { Role } from "@ac/web/lib/nav";
import { evidenceText } from "@ac/web/lib/evidence";
import { money } from "@ac/web/lib/money";
import { DEFAULT_DISPLAY, showDate, showTime, translate, translator, type Display } from "@ac/web/lib/i18n";

type TargetKind = "unit" | "job" | "invoice" | "restriction" | "device" | "inquiry" | "client_user" | "partner_user" | "certificate";

/** Notification of service-contracts.ts (fields shown in the inbox). */
export type ApiNotification = {
  id: string;
  version: number;
  type: string;
  sourceAlertId: string | null;
  target: { kind: TargetKind; id: string };
  templateKey: string;
  params: { targetName: string; at: string; status: string; reason: string | null; message: string | null; amountMinor?: number | null; currency?: string | null };
  channel: string;
  occurredAt: string;
  readAt: string | null;
  severity?: "critical" | "warning" | "normal";
};

export type InboxRow = { id: string; version: number; t: string; d: string; w: string; read: boolean; href: string };

// titles by template, {name} the target (English keys of the display-language dictionary, IR258)
const titles: Record<string, string> = {
  alert: "Alert on {name}",
  quality: "Air quality alert on {name}",
  schedule_change: "Schedule changed — {name}",
  report_return: "Work report returned — {name}",
  completion: "Work completed — {name}",
  payment: "Payment update — {name}",
  payment_reminder: "Payment reminder — {name}",
  restriction: "Service restriction — {name}",
  inquiry: "Inquiry update — {name}",
  job_update: "Job update — {name}",
  device_operation: "Device operation — {name}",
  invite: "Invitation — {name}",
  certificate_expiry: "Certificate expiring — {name}",
  warranty_expiring: "Warranty ending — {name}",
};

/** Templates whose detail line comes from the params in the user's language: a certificate reminder (BR-P09) names
 * the day the certificate expires (IR336), a warranty reminder (BR-A19) the day the warranty ends (IR340), a payment
 * reminder the invoice's amount (IR488). */
const details: Record<string, (n: ApiNotification, display: Display) => string> = {
  certificate_expiry: (n, display) => translate(display.locale, "Expires {date}", { date: showDate(n.params.at, display) }),
  warranty_expiring: (n, display) => translate(display.locale, "Warranty ends {date} · no maintenance contract", { date: showDate(n.params.at, display) }),
  payment_reminder: (n, display) => (n.params.amountMinor != null && n.params.currency ? translate(display.locale, "Amount due {amount}", { amount: money(n.params.amountMinor, n.params.currency) }) : ""),
};

/** Types whose title and link come from the type rather than the template: a filter cleaning reminder (alert template,
 * IR104 maintenance → cleaning_due) opens the customer's Filter care tab (IR239). */
const typeTitles: Record<string, string> = { cleaning_due: "Filter cleaning due on {name}" };
const typeRoutes: Partial<Record<Role, Record<string, string>>> = {
  client: { cleaning_due: "/customer/maintenance?tab=filter-care" },
  admin: { warranty_expiring: "/admin/units?tab=warranty" }, // Customers & units › Warranty & coverage (FR-A19)
};

const routes: Record<Role, Partial<Record<TargetKind, (id: string) => string>>> = {
  client: { unit: (id) => `/customer/units/${id}`, job: (id) => `/customer/maintenance?jobId=${id}`, invoice: (id) => `/customer/payments/${id}`, restriction: () => "/customer/payments", inquiry: () => "/customer/maintenance", client_user: () => "/customer/users" },
  admin: { unit: (id) => `/admin/units?unitId=${id}`, job: (id) => `/admin/jobs?jobId=${id}`, invoice: (id) => `/admin/billing?invoiceId=${id}`, inquiry: (id) => `/admin/billing?inquiryId=${id}`, restriction: (id) => `/admin/restrictions/${id}`, device: (id) => `/admin/devices?deviceId=${id}`, client_user: () => "/admin/settings/access" },
  contractor: { unit: (id) => `/partner/units/${id}`, job: (id) => `/partner/jobs/${id}`, certificate: () => "/partner/team?tab=certifications", partner_user: () => "/partner/users" },
  technician: { unit: (id) => `/technician/units/${id}`, job: (id) => `/technician/jobs/${id}`, device: (id) => `/technician/devices/${id}` },
};

const alertsPage: Record<Role, string> = { client: "/customer/alerts", admin: "/admin/alerts", contractor: "", technician: "" };

/** The status codes the sources store (IR95 events, the job status after a cancel, hold or resume, a restriction's state, an
 * inquiry's state), in words: English keys of the display-language dictionary (IR481). */
const statusWords: Record<string, string> = {
  open: "Unresolved", requested: "Requested", time_proposed: "Time proposed", offered: "Offered", accepted: "Accepted", declined: "Declined", assigned: "Assigned",
  in_progress: "In progress", submitted: "Submitted", returned: "Returned for rework", rework_requested: "Rework requested", completed: "Completed", cancelled: "Cancelled",
  on_hold: "On hold", scheduled: "Scheduled", applied: "Applied", release_requested: "Release requested", released: "Released", received: "Received", answered: "Answered",
  confirmed: "Payment confirmed",
};
/** Why an assignment was released (the access-release notice, IR439). */
const reasonWords: Record<string, string> = {
  technician_access_ended: "Technician’s access ended — to be reassigned",
  technician_qualification_ended: "Technician’s qualification ended — to be reassigned",
};
/** The firmware campaign's advance notice (IR339): its day is dated again in the display language. */
const FIRMWARE_NOTICE = "Firmware update from {date}, {from}–{to} local time. The AC may restart for about 2 minutes.";
const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
const firmwareNotice = /^Firmware update from (\d{1,2}) ([A-Z][a-z]{2}) (\d{4}), (\S+)–(\S+) local time\. The AC may restart for about 2 minutes\.$/;

/** The dictionary keys of the detail line (the key check of the tests). */
export const NOTIFICATION_KEYS = [...Object.values(statusWords), ...Object.values(reasonWords), FIRMWARE_NOTICE];

function messageText(message: string, display: Display): string {
  const m = message.match(firmwareNotice);
  const month = m ? MONTHS.indexOf(m[2]) : -1;
  if (!m || month < 0) return evidenceText(message, translator(display.locale)); // a filter reminder's message is its evidence (IR480)
  const day = `${m[3]}-${String(month + 1).padStart(2, "0")}-${m[1].padStart(2, "0")}T00:00:00Z`;
  return translate(display.locale, FIRMWARE_NOTICE, { date: showDate(day, { locale: display.locale, timeZone: "UTC" }), from: m[4], to: m[5] });
}

/** One inbox row in the user's display language and time zone (FR-X01, IR258). The message, the reason and the status
 * are worded in that language when a source above wrote them (IR481); a status the API filled in from the type (the
 * notification had none) is left out, and any other text stays as recorded. */
export function inboxRow(n: ApiNotification, role: Role, display: Display = DEFAULT_DISPLAY): InboxRow {
  const title = typeTitles[n.type] ?? titles[n.templateKey] ?? "Notification — {name}";
  const word = (words: Record<string, string>, code: string | null) => (code && words[code] ? translate(display.locale, words[code]) : code);
  const status = n.params.status === n.type ? null : word(statusWords, n.params.status);
  const message = n.params.message ? messageText(n.params.message, display) : word(reasonWords, n.params.reason);
  const text = details[n.templateKey]?.(n, display) ?? [message, status].filter(Boolean).join(" · ");
  const detail = [text, n.channel === "inApp" ? null : n.channel].filter(Boolean).join(" · ");
  const href = typeRoutes[role]?.[n.type] ?? (n.sourceAlertId && alertsPage[role] ? alertsPage[role] : (routes[role][n.target.kind]?.(n.target.id) ?? ""));
  return { id: n.id, version: n.version, t: translate(display.locale, title, { name: n.params.targetName }), d: detail, w: showTime(n.occurredAt, display), read: n.readAt !== null, href };
}
