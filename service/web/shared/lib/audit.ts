// Audit log rows (FR-A16, DATA_SOURCE=api): audit.list items for the admin audit screen, and the URL filters of the
// screen. Pure code shared by the Server Component and the client view.
export type AuditResult = "Success" | "Denied" | "Failed" | "Pending";
export type AuditChange = { field: string; before: string; after: string; changed: boolean };
export type AuditRow = { id: string; op: string; target: string; actor: string; role: string; at: string; occurred: string; corr: string; res: AuditResult; reason: string | null; changes: AuditChange[] };

/** AuditView of service-contracts.ts. */
export type ApiAudit = {
  id: string; actorId: string; actorRoleAtTime: string; action: string; targetRef: { kind: string; id: string }; occurredAt: string; correlationId: string;
  result: "success" | "denied" | "failed" | "pending"; maskedBefore: Record<string, string | null>; maskedAfter: Record<string, string | null>; reason: string | null;
};

/** DeviceEvent of service-contracts.ts (fields shown on the device events tab). */
export type ApiDeviceEvent = { id: string; deviceId: string; eventType: string; evidenceSource: string; occurredAt: string; restoredAt: string | null };

export type AuditFilters = { from: string; to: string; correlationId: string; result: AuditResult | "All"; limit: number };

const KL = "Asia/Kuala_Lumpur";
const ymd = (d: Date) => d.toLocaleDateString("en-CA", { timeZone: KL }); // YYYY-MM-DD in Kuala Lumpur
const results: AuditResult[] = ["Success", "Denied", "Failed", "Pending"];

/** The screen's filters from the URL; the default period is the 30 days up to the Core API business clock. */
export function auditFilters(sp: Record<string, string | string[] | undefined>, now: Date): AuditFilters {
  const one = (k: string) => (typeof sp[k] === "string" ? (sp[k] as string) : "");
  const res = one("result");
  const limit = Math.min(100, Math.max(25, Number(one("limit")) || 25));
  return {
    from: one("from") || ymd(new Date(now.getTime() - 30 * 86_400_000)),
    to: one("to") || ymd(new Date(now.getTime() + 86_400_000)),
    correlationId: one("corr"),
    result: (results as string[]).includes(res) ? (res as AuditResult) : "All",
    limit,
  };
}

/** A valid period: start < end, at most 366 days (IR143). */
export function periodError(f: Pick<AuditFilters, "from" | "to">): string | undefined {
  const a = Date.parse(f.from), b = Date.parse(f.to);
  return !(a < b) || b - a > 366 * 86_400_000 ? "Period must be start < end, at most 366 days" : undefined;
}

/** audit.list input for the filters (dates are Kuala Lumpur days). */
export function auditQuery(f: AuditFilters) {
  return {
    limit: f.limit,
    filters: {
      from: `${f.from}T00:00:00+08:00`,
      to: `${f.to}T00:00:00+08:00`,
      ...(f.correlationId ? { correlationId: f.correlationId } : {}),
      ...(f.result !== "All" ? { result: f.result.toLowerCase() } : {}),
    },
  };
}

export function auditRow(a: ApiAudit): AuditRow {
  const d = new Date(a.occurredAt);
  const fields = [...new Set([...Object.keys(a.maskedBefore ?? {}), ...Object.keys(a.maskedAfter ?? {})])].sort();
  return {
    id: a.id,
    op: a.action,
    target: `${a.targetRef.kind} · ${a.targetRef.id}`,
    actor: a.actorId,
    role: a.actorRoleAtTime,
    at: d.toLocaleString("en-CA", { timeZone: KL, month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hour12: false }).replace(",", ""),
    occurred: `${d.toLocaleString("en-CA", { timeZone: KL, hour12: false }).replace(",", "")} (Asia/Kuala_Lumpur)`,
    corr: a.correlationId,
    res: (a.result.charAt(0).toUpperCase() + a.result.slice(1)) as AuditResult,
    reason: a.reason,
    changes: fields.map((f) => {
      const before = a.maskedBefore?.[f] ?? "null", after = a.maskedAfter?.[f] ?? "null";
      return { field: f, before, after, changed: before !== after };
    }),
  };
}

const eventTitle: Record<string, string> = { communication_lost: "communication lost", power_lost: "power_signal lost", tamper: "tamper_signal — cover opened", restored: "restored", operation_failed: "operation failed" };

/** Device events tab rows. */
export function deviceEventItem(e: ApiDeviceEvent) {
  const t = new Date(e.occurredAt).toLocaleString("en-CA", { timeZone: KL, month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hour12: false }).replace(",", "");
  return { time: t, title: eventTitle[e.eventType] ?? e.eventType, detail: `${e.evidenceSource}${e.restoredAt ? " · restored" : ""}` };
}
