// Audit log rows (FR-A16, DATA_SOURCE=api): audit.list items for the admin audit screen, and the URL filters of the
// screen. Pure code shared by the Server Component and the client view. Texts in the display language and instants in
// the display time zone; the period's days are the display time zone's days too (`i` / `zone`, IR304).
import { EN, relativeTime, showTime, translator, zonedInstant, zonedParts, type I18n, type T } from "@ac/web/lib/i18n";

const en = translator("en");
export type AuditResult = "Success" | "Denied" | "Failed" | "Pending";
export type AuditChange = { field: string; before: string; after: string; changed: boolean };
export type AuditRow = { id: string; op: string; target: string; actor: string; actorId: string; role: string; at: string; occurred: string; corr: string; res: AuditResult; reason: string | null; changes: AuditChange[] };

/** AuditView of service-contracts.ts. */
export type ApiAudit = {
  id: string; actorId: string; actorName: string | null; actorRoleAtTime: string; action: string; targetRef: { kind: string; id: string }; occurredAt: string; correlationId: string;
  result: "success" | "denied" | "failed" | "pending"; maskedBefore: Record<string, string | null>; maskedAfter: Record<string, string | null>; reason: string | null;
};

/** DeviceEvent of service-contracts.ts (fields shown on the device events tab). */
export type ApiDeviceEvent = { id: string; deviceId: string; eventType: string; evidenceSource: string; occurredAt: string; restoredAt: string | null };

export type AuditFilters = { from: string; to: string; correlationId: string; result: AuditResult | "All"; limit: number };

const KL = "Asia/Kuala_Lumpur";
const results: AuditResult[] = ["Success", "Denied", "Failed", "Pending"];
/** A result as the screen words it. */
export const resultWord = (r: AuditResult | string, t: T = en) => ({ Success: t("Success"), Denied: t("Denied"), Failed: t("Failed"), Pending: t("Pending") })[r] ?? r;
/** The role an entry recorded, worded (it is never rewritten). */
export const roleAtTimeWord = (r: string, t: T = en) =>
  ({ admin: t("Admin"), contractor: t("Contractor"), technician: t("Technician"), client: t("Client"), system: t("System") })[r] ?? r;

/** The screen's filters from the URL; the default period is the 30 days up to the Core API business clock, as days of
 * the display time zone. */
export function auditFilters(sp: Record<string, string | string[] | undefined>, now: Date, zone = KL): AuditFilters {
  const one = (k: string) => (typeof sp[k] === "string" ? (sp[k] as string) : "");
  const res = one("result");
  const limit = Math.min(100, Math.max(25, Number(one("limit")) || 25));
  const day = (ms: number) => zonedParts(new Date(ms).toISOString(), zone).date;
  return {
    from: one("from") || day(now.getTime() - 30 * 86_400_000),
    to: one("to") || day(now.getTime() + 86_400_000),
    correlationId: one("corr"),
    result: (results as string[]).includes(res) ? (res as AuditResult) : "All",
    limit,
  };
}

/** A valid period: start < end, at most 366 days (IR143). */
export function periodError(f: Pick<AuditFilters, "from" | "to">, t: T = en): string | undefined {
  const a = Date.parse(f.from), b = Date.parse(f.to);
  return !(a < b) || b - a > 366 * 86_400_000 ? t("Period must be start < end, at most 366 days") : undefined;
}

/** audit.list input for the filters: the days start at midnight in the display time zone (NFR-08). */
export function auditQuery(f: AuditFilters, zone = KL) {
  return {
    limit: f.limit,
    filters: {
      from: zonedInstant(f.from, "00:00", zone),
      to: zonedInstant(f.to, "00:00", zone),
      ...(f.correlationId ? { correlationId: f.correlationId } : {}),
      ...(f.result !== "All" ? { result: f.result.toLowerCase() } : {}),
    },
  };
}

/** One entry of the log: who acted (the user's name, else the actor ID — a system actor has no name, IR305), the list
 * time relative to now, the full time and the role in the display language. */
export function auditRow(a: ApiAudit, i: I18n = EN, nowMs?: number): AuditRow {
  const fields = [...new Set([...Object.keys(a.maskedBefore ?? {}), ...Object.keys(a.maskedAfter ?? {})])].sort();
  return {
    id: a.id,
    op: a.action,
    target: `${a.targetRef.kind} · ${a.targetRef.id}`,
    actor: a.actorName ?? a.actorId,
    actorId: a.actorId,
    role: roleAtTimeWord(a.actorRoleAtTime, i.t),
    at: nowMs === undefined ? showTime(a.occurredAt, i.display) : relativeTime(a.occurredAt, nowMs, i),
    occurred: showTime(a.occurredAt, i.display),
    corr: a.correlationId,
    res: (a.result.charAt(0).toUpperCase() + a.result.slice(1)) as AuditResult,
    reason: a.reason,
    changes: fields.map((f) => {
      const before = a.maskedBefore?.[f] ?? "null", after = a.maskedAfter?.[f] ?? "null";
      return { field: f, before, after, changed: before !== after };
    }),
  };
}

/** An entry as a timeline item (the restriction exception screen's audit card, IR301). */
export const auditItem = (a: ApiAudit, i: I18n = EN) => ({
  time: showTime(a.occurredAt, i.display), title: `${a.action} · ${resultWord(a.result.charAt(0).toUpperCase() + a.result.slice(1), i.t)}`,
  detail: `${a.actorName ?? a.actorId} (${roleAtTimeWord(a.actorRoleAtTime, i.t)})${a.reason ? ` · ${a.reason}` : ""}`,
});

const eventTitle: Record<string, string> = { communication_lost: "communication lost", power_lost: "power_signal lost", tamper: "tamper_signal — cover opened", restored: "restored", operation_failed: "operation failed" };

/** Device events rows: the event in words, its evidence source (a code) and whether it was restored, at its time in
 * the user's time zone with "today / yesterday" when `i` and `nowMs` are given (the HQ devices and audit screens). */
export function deviceEventItem(e: ApiDeviceEvent, i?: I18n, nowMs?: number) {
  const tr = (i ?? EN).t;
  const time = i && nowMs !== undefined ? relativeTime(e.occurredAt, nowMs, i)
    : new Date(e.occurredAt).toLocaleString("en-CA", { timeZone: KL, month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hour12: false }).replace(",", "");
  return { time, title: eventTitle[e.eventType] ? tr(eventTitle[e.eventType]) : e.eventType, detail: `${e.evidenceSource}${e.restoredAt ? ` · ${tr("restored")}` : ""}` };
}
