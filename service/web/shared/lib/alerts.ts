// Customer alert rows (DATA_SOURCE=api): alerts.list items projected for the alerts screen (IR51: unresolved
// critical / warning alerts need attention, the rest are information). Pure code shared by server and client.
export type Alert = { id: string; title: string; sev: "warning" | "normal"; kind: string; icon: string; where: string; ev: string; group: "attn" | "info"; read: boolean };

/** Alert of service-contracts.ts (fields shown on this screen). */
export type ApiAlert = { id: string; unitId: string; type: string; severity: "critical" | "warning" | "normal"; status: string; causeCode: string; evidenceKind: string; evidenceText: string; detectedAt: string };

const causeTitle: Record<string, string> = { window_open: "Possible open window", insulation_loss: "Poor insulation suspected" };
const typeTitle: Record<string, string> = { maintenance: "Maintenance reminder", quality: "Air quality alert", tamper: "Device tamper", reconciliation_required: "Restriction check needed", sensor: "Sensor alert" };
const evidenceLabel: Record<string, string> = { inferred: "Evidence (inferred)", inspection: "Inspection record", demo_observation: "Evidence (demo observation)" };

/** The display title of an alert: its suspected cause, else its type. */
export function alertTitle(a: Pick<ApiAlert, "causeCode" | "type">): string {
  return causeTitle[a.causeCode] ?? typeTitle[a.type] ?? "Alert";
}

// DATA_SOURCE=api: one row per alert; unresolved critical/warning alerts need attention (IR51), the rest are information
export function alertRow(a: ApiAlert): Alert {
  const attn = a.status !== "resolved" && a.severity !== "normal";
  const at = new Date(a.detectedAt).toLocaleString("en-MY", { dateStyle: "medium", timeStyle: "short", timeZone: "Asia/Kuala_Lumpur" });
  return {
    id: a.id,
    title: alertTitle(a),
    sev: a.severity === "normal" ? "normal" : "warning",
    kind: a.evidenceKind === "inspection" ? "✎ Inspection record" : a.type === "maintenance" ? "◷ Maintenance reminder" : "✕ Fault",
    icon: attn ? "⚠" : "ⓘ",
    where: `detected ${at}${a.status === "acknowledged" ? " · acknowledged" : ""}`,
    ev: `${evidenceLabel[a.evidenceKind] ?? "Evidence"}: ${a.evidenceText}`,
    group: attn ? "attn" : "info",
    read: a.status !== "open",
  };
}

