// Customer alerts (DATA_SOURCE=api): the alert titles every role shows and the customer's alert inbox rows (IR51:
// unresolved critical / warning alerts need attention, the rest are information; DD-C08 read state on the
// notifications). Pure code shared by server and client.
export type Alert = { id: string; title: string; sev: "warning" | "normal"; kind: string; icon: string; where: string; ev: string; group: "attn" | "info"; read: boolean };

/** Alert of service-contracts.ts (fields shown on this screen). */
export type ApiAlert = {
  id: string; unitId: string; type: string; severity: "critical" | "warning" | "normal"; status: string; causeCode: string; evidenceKind: string; evidenceText: string; detectedAt: string;
  acknowledgedAt?: string | null; resolvedAt?: string | null; resolutionReason?: string | null;
};

const causeTitle: Record<string, string> = { window_open: "Possible open window", insulation_loss: "Poor insulation suspected" };
const typeTitle: Record<string, string> = { maintenance: "Filter cleaning reminder", quality: "Air quality alert", tamper: "Device tamper", reconciliation_required: "Restriction check needed", sensor: "Sensor alert" };
const evidenceLabel: Record<string, string> = { inferred: "Evidence (inferred)", inspection: "Inspection record", demo_observation: "Evidence (demo observation)" };

/** The display title of an alert: its suspected cause, else its type. */
export function alertTitle(a: Pick<ApiAlert, "causeCode" | "type">): string {
  return causeTitle[a.causeCode] ?? typeTitle[a.type] ?? "Alert";
}

// ---- the customer alert inbox (DD-C08, IR242) ----

const KL = "Asia/Kuala_Lumpur";
const stamp = (iso: string) => new Date(iso).toLocaleString("en-MY", { dateStyle: "medium", timeStyle: "short", timeZone: KL });
/** A notification about an alert (Notification of service-contracts.ts: the fields the inbox needs). */
export type AlertNote = { id: string; version: number; sourceAlertId: string | null; readAt: string | null };
export type InboxAlert = {
  id: string; title: string; severity: "critical" | "warning" | "normal"; kind: string; where: string; evidence: string; group: "attn" | "info";
  status: { text: string; tone: "warn" | "primary" | "ok" | "muted"; detail: string };
  /** the signed-in membership's notifications about the alert: unread ones (marked read on opening) and how many exist */
  unread: { id: string; version: number }[]; notes: number;
};
const rank = { critical: 0, warning: 1, normal: 2 };
/** One row per alert: the unit and its place, the status with its time and resolution, Needs attention for unresolved
 * critical / warning alerts (IR51), and the read state that lives on the notifications, never on the alert (DD-C08). */
export function inboxAlerts(alerts: ApiAlert[], notes: AlertNote[], unit: (id: string) => { name: string; place: string } | undefined): InboxAlert[] {
  const attn = (a: ApiAlert) => a.status !== "resolved" && a.severity !== "normal";
  const order = [...alerts].sort((x, y) => Number(attn(y)) - Number(attn(x)) || rank[x.severity] - rank[y.severity] || y.detectedAt.localeCompare(x.detectedAt));
  return order.map((a): InboxAlert => {
    const u = unit(a.unitId);
    const mine = notes.filter((n) => n.sourceAlertId === a.id);
    const resolved = a.status === "resolved";
    const status = resolved ? { text: "Resolved", tone: "ok" as const, detail: `Resolved ${a.resolvedAt ? stamp(a.resolvedAt) : ""}${a.resolutionReason ? ` · ${a.resolutionReason}` : ""}`.trim() }
      : a.status === "acknowledged" ? { text: "Acknowledged", tone: "primary" as const, detail: `Acknowledged ${a.acknowledgedAt ? stamp(a.acknowledgedAt) : ""} · still unresolved`.trim() }
      : { text: "Unresolved", tone: a.severity === "normal" ? "muted" as const : "warn" as const, detail: "Open — not resolved yet" };
    return {
      id: a.id, title: alertTitle(a), severity: a.severity,
      kind: a.evidenceKind === "inspection" ? "✎ Inspection record" : a.type === "maintenance" ? "◷ Maintenance reminder" : a.type === "quality" ? "≋ Air quality" : "✕ Fault",
      where: `${u ? `${u.name}${u.place ? ` · ${u.place}` : ""}` : "AC"} · detected ${stamp(a.detectedAt)}`,
      evidence: `${evidenceLabel[a.evidenceKind] ?? "Evidence"}: ${a.evidenceText}`,
      group: attn(a) ? "attn" : "info", status,
      unread: mine.filter((n) => n.readAt === null).map((n) => ({ id: n.id, version: n.version })), notes: mine.length,
    };
  });
}
