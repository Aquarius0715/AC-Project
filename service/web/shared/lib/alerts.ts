// Customer alerts (DATA_SOURCE=api): the alert titles every role shows and the customer's alert inbox rows (IR51:
// unresolved critical / warning alerts need attention, the rest are information; DD-C08 read state on the
// notifications). Pure code shared by server and client.
import { EN, showTime, translator, type I18n, type T } from "@ac/web/lib/i18n";
export type Alert = { id: string; title: string; sev: "warning" | "normal"; kind: string; icon: string; where: string; ev: string; group: "attn" | "info"; read: boolean };

/** Alert of service-contracts.ts (fields shown on this screen). */
export type ApiAlert = {
  id: string; unitId: string; type: string; severity: "critical" | "warning" | "normal"; status: string; causeCode: string; evidenceKind: string; evidenceText: string; detectedAt: string;
  acknowledgedAt?: string | null; resolvedAt?: string | null; resolutionReason?: string | null;
};

const causeTitle: Record<string, string> = { window_open: "Possible open window", insulation_loss: "Poor insulation suspected" };
const typeTitle: Record<string, string> = { maintenance: "Filter cleaning reminder", quality: "Air quality alert", tamper: "Device tamper", reconciliation_required: "Restriction check needed", sensor: "Sensor alert" };
const evidenceLabel: Record<string, string> = { inferred: "Evidence (inferred)", inspection: "Inspection record", demo_observation: "Evidence (demo observation)" };
/** The kind of an alert (Figma Client 06a, DD-C08): an inspection record; a load cause — an open window or poor
 * insulation — that is only suspected, never a fault (BIZ-17, IR315); a maintenance reminder; an air-quality alert;
 * else a fault. */
export type AlertKind = "inspection" | "load" | "maintenance" | "quality" | "fault";
const kindOf = (a: Pick<ApiAlert, "type" | "causeCode" | "evidenceKind">): AlertKind =>
  a.evidenceKind === "inspection" ? "inspection" : a.causeCode === "window_open" || a.causeCode === "insulation_loss" ? "load"
    : a.type === "maintenance" ? "maintenance" : a.type === "quality" ? "quality" : "fault";
const kindLabel: Record<AlertKind, string> = { inspection: "✎ Inspection record", load: "⌂ Load cause (possible)", maintenance: "◷ Maintenance reminder", quality: "≋ Air quality", fault: "✕ Fault" };

/** The display title of an alert: its suspected cause, else its type — in the display language (IR260). */
export function alertTitle(a: Pick<ApiAlert, "causeCode" | "type">, t: T = translator("en")): string {
  return t(causeTitle[a.causeCode] ?? typeTitle[a.type] ?? "Alert");
}

// ---- the customer alert inbox (DD-C08, IR242) ----

/** A notification about an alert (Notification of service-contracts.ts: the fields the inbox needs). */
export type AlertNote = { id: string; version: number; sourceAlertId: string | null; readAt: string | null };
export type InboxAlert = {
  id: string; unitId: string; type: string; title: string; severity: "critical" | "warning" | "normal";
  /** cause is the kind code; kind is its badge in the display language */
  cause: AlertKind; kind: string; where: string; evidence: string; group: "attn" | "info";
  /** state is the alert's status (open / acknowledged / resolved); text and detail are in the display language */
  status: { state: "open" | "acknowledged" | "resolved"; text: string; tone: "warn" | "primary" | "ok" | "muted"; detail: string };
  /** the signed-in membership's notifications about the alert: unread ones (marked read on opening) and how many exist */
  unread: { id: string; version: number }[]; notes: number;
};
const rank = { critical: 0, warning: 1, normal: 2 };
/** One row per alert: the unit and its place, the status with its time and resolution, Needs attention for unresolved
 * critical / warning alerts (IR51), and the read state that lives on the notifications, never on the alert (DD-C08).
 * Texts in the display language, times IR44 in the user's display time zone (IR261). */
export function inboxAlerts(alerts: ApiAlert[], notes: AlertNote[], unit: (id: string) => { name: string; place: string } | undefined, i: I18n = EN): InboxAlert[] {
  const { t, display } = i;
  const stamp = (iso: string) => showTime(iso, display);
  const attn = (a: ApiAlert) => a.status !== "resolved" && a.severity !== "normal";
  const order = [...alerts].sort((x, y) => Number(attn(y)) - Number(attn(x)) || rank[x.severity] - rank[y.severity] || y.detectedAt.localeCompare(x.detectedAt));
  return order.map((a): InboxAlert => {
    const u = unit(a.unitId);
    const mine = notes.filter((n) => n.sourceAlertId === a.id);
    const resolved = a.status === "resolved";
    const status = resolved ? { state: "resolved" as const, text: t("Resolved"), tone: "ok" as const, detail: [a.resolvedAt ? t("Resolved {when}", { when: stamp(a.resolvedAt) }) : t("Resolved"), a.resolutionReason].filter(Boolean).join(" · ") }
      : a.status === "acknowledged" ? { state: "acknowledged" as const, text: t("Acknowledged"), tone: "primary" as const, detail: a.acknowledgedAt ? t("Acknowledged {when} · still unresolved", { when: stamp(a.acknowledgedAt) }) : t("Acknowledged · still unresolved") }
      : { state: "open" as const, text: t("Unresolved"), tone: a.severity === "normal" ? "muted" as const : "warn" as const, detail: t("Open — not resolved yet") };
    return {
      id: a.id, unitId: a.unitId, type: a.type, title: alertTitle(a, t), severity: a.severity,
      cause: kindOf(a), kind: t(kindLabel[kindOf(a)]),
      where: t("{where} · detected {when}", { where: u ? `${u.name}${u.place ? ` · ${u.place}` : ""}` : "AC", when: stamp(a.detectedAt) }),
      evidence: `${t(evidenceLabel[a.evidenceKind] ?? "Evidence")}: ${a.evidenceText}`,
      group: attn(a) ? "attn" : "info", status,
      unread: mine.filter((n) => n.readAt === null).map((n) => ({ id: n.id, version: n.version })), notes: mine.length,
    };
  });
}
