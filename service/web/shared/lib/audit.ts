// Audit explorer (FR-A16, DD-A16, SCR-A16, DATA_SOURCE=api): audit.list items for the admin audit screen, the URL
// filters of the screen, the records related to an entry and its correlation trace, and the device events tab. Pure code
// shared by the Server Component and the client view. Texts in the display language and instants in the display time
// zone; the period's days are the display time zone's days too (`i` / `zone`, IR304). Actions, target kinds and IDs,
// correlation IDs, field names, event types and evidence sources stay codes (IR304, IR320).
import { EN, relativeTime, showTime, translator, zonedInstant, zonedParts, type I18n, type T } from "@ac/web/lib/i18n";
import type { OpInput } from "@ac/web/lib/opTypes";
import type { AuditView } from "@ac/web/lib/contracts.gen";
import { stateWord, states, type RestrictionState } from "@ac/web/lib/restrictions";
import { eventCause, eventRows, type ApiAlertLite, type ApiDeviceEventFull } from "@ac/web/lib/techDevices";

const en = translator("en");
export type AuditResult = "Success" | "Denied" | "Failed" | "Pending";
export type AuditChange = { field: string; before: string; after: string; changed: boolean };
export type AuditRow = {
  id: string; op: string; target: string; targetKind: string; targetId: string; versions: string | null; actor: string; actorId: string; role: string; at: string; occurred: string;
  corr: string; res: AuditResult; reason: string | null; changes: AuditChange[];
};

/** AuditView of service-contracts.ts. */
export type ApiAudit = {
  id: string; actorId: string; actorName: string | null; actorRoleAtTime: string; action: string; targetRef: { kind: string; id: string }; previousVersion: number | null; nextVersion: number | null;
  occurredAt: string; correlationId: string; result: "success" | "denied" | "failed" | "pending"; maskedBefore: Record<string, string | null>; maskedAfter: Record<string, string | null>; reason: string | null;
};

/** The URL filters of the log (SCR-A16): the period, actor, target kind and ID, correlation ID, result and page size. */
export type AuditFilters = { from: string; to: string; actorId: string; targetKind: string; targetId: string; correlationId: string; result: AuditResult | "All"; limit: number };

const KL = "Asia/Kuala_Lumpur";
const results: AuditResult[] = ["Success", "Denied", "Failed", "Pending"];
/** A result as the screen words it. */
export const resultWord = (r: AuditResult | string, t: T = en) => ({ Success: t("Success"), Denied: t("Denied"), Failed: t("Failed"), Pending: t("Pending") })[r] ?? r;
/** The badge tone of a result. */
export const resultTone = (r: AuditResult) => (r === "Success" ? "ok" : r === "Denied" ? "warn" : r === "Failed" ? "crit" : "primary");
/** The role an entry recorded, worded (it is never rewritten). */
export const roleAtTimeWord = (r: string, t: T = en) =>
  ({ admin: t("Admin"), contractor: t("Contractor"), technician: t("Technician"), client: t("Client"), system: t("System") })[r] ?? r;
const resultOf = (r: ApiAudit["result"]) => (r.charAt(0).toUpperCase() + r.slice(1)) as AuditResult;

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
    actorId: one("actorId"), targetKind: one("targetKind"), targetId: one("targetId"), correlationId: one("correlationId"),
    result: (results as string[]).includes(res) ? (res as AuditResult) : "All",
    limit,
  };
}

/** A valid period: start < end, at most 366 days (IR143). */
export function periodError(f: Pick<AuditFilters, "from" | "to">, t: T = en): string | undefined {
  const a = Date.parse(f.from), b = Date.parse(f.to);
  return !(a < b) || b - a > 366 * 86_400_000 ? t("Period must be start < end, at most 366 days") : undefined;
}

/** The period of the filters as instants: its days start at midnight in the display time zone (NFR-08). */
export const periodInstants = (f: Pick<AuditFilters, "from" | "to">, zone = KL) => ({ from: zonedInstant(f.from, "00:00", zone), to: zonedInstant(f.to, "00:00", zone) });

/** audit.list input for the filters. */
export function auditQuery(f: AuditFilters, zone = KL): OpInput<"audit.list"> {
  return {
    limit: f.limit,
    filters: {
      ...periodInstants(f, zone),
      ...(f.actorId ? { actorId: f.actorId } : {}),
      ...(f.targetKind ? { targetKind: f.targetKind } : {}),
      ...(f.targetId ? { targetId: f.targetId } : {}),
      ...(f.correlationId ? { correlationId: f.correlationId } : {}),
      ...(f.result !== "All" ? { result: f.result.toLowerCase() as AuditView["result"] } : {}),
    },
  };
}

/** The entries of a correlation ID around one entry (a day either side, inside the 366-day limit): its trace and its
 * related records. */
export function correlationQuery(a: Pick<ApiAudit, "correlationId" | "occurredAt">): OpInput<"audit.list"> {
  const at = Date.parse(a.occurredAt);
  return { limit: 100, sort: { field: "occurredAt", direction: "asc" }, filters: { from: new Date(at - 86_400_000).toISOString(), to: new Date(at + 86_400_000).toISOString(), correlationId: a.correlationId } };
}

/** One entry of the log: who acted (the user's name, else the actor ID — a system actor has no name, IR305), the list
 * time relative to now, the full time, the role in the display language and the versions it moved the target between. */
export function auditRow(a: ApiAudit, i: I18n = EN, nowMs?: number): AuditRow {
  const fields = [...new Set([...Object.keys(a.maskedBefore ?? {}), ...Object.keys(a.maskedAfter ?? {})])].sort();
  const { t } = i;
  return {
    id: a.id,
    op: a.action,
    target: `${a.targetRef.kind} · ${a.targetRef.id}`, targetKind: a.targetRef.kind, targetId: a.targetRef.id,
    versions: a.nextVersion === null ? null : a.previousVersion === null ? t("version {v}", { v: a.nextVersion }) : t("version {from} → {to}", { from: a.previousVersion, to: a.nextVersion }),
    actor: a.actorName ?? a.actorId,
    actorId: a.actorId,
    role: roleAtTimeWord(a.actorRoleAtTime, t),
    at: nowMs === undefined ? showTime(a.occurredAt, i.display) : relativeTime(a.occurredAt, nowMs, i),
    occurred: showTime(a.occurredAt, i.display),
    corr: a.correlationId,
    res: resultOf(a.result),
    reason: a.reason,
    changes: fields.map((f) => {
      const before = a.maskedBefore?.[f] ?? "null", after = a.maskedAfter?.[f] ?? "null";
      return { field: f, before, after, changed: before !== after };
    }),
  };
}

/** An entry as a timeline item (the restriction exception screen's audit card, IR301; the correlation trace, IR320). */
export const auditItem = (a: ApiAudit, i: I18n = EN) => ({
  time: showTime(a.occurredAt, i.display), title: `${a.action} · ${resultWord(resultOf(a.result), i.t)}`,
  detail: `${a.actorName ?? a.actorId} (${roleAtTimeWord(a.actorRoleAtTime, i.t)})${a.reason ? ` · ${a.reason}` : ""}`,
});

/** The entries of one correlation ID in the order they were appended (Figma Admin 338:2): a pending entry stays as it
 * was; the final result is a later entry of its own. Empty when the entry stands alone. */
export const correlationTrace = (group: ApiAudit[], i: I18n = EN) =>
  group.length < 2 ? [] : [...group].sort((a, b) => Date.parse(a.occurredAt) - Date.parse(b.occurredAt) || a.id.localeCompare(b.id)).map((a) => ({ id: a.id, ...auditItem(a, i) }));

/** The target kinds the Target filter always offers, the records HQ looks up most; the kinds of the shown entries and
 * the URL's kind join them (targetKind is a string like AuditView.targetRef.kind and the log records more kinds). */
const KINDS = ["alert", "automation", "command", "contract", "device", "device_event", "invoice", "job", "membership", "payment", "policy", "restriction", "unit", "user"];
export const targetKinds = (rows: Pick<AuditRow, "targetKind">[], current: string) => [...new Set([...KINDS, ...rows.map((r) => r.targetKind), ...(current ? [current] : [])])].sort();

/** The Actor filter's people by name: the users of the memberships the session may read (members.list), the actors of
 * the shown entries and the URL's actor (a system actor by its ID). */
export function actorOptions(members: { userId: string; displayName: string }[], rows: Pick<AuditRow, "actor" | "actorId">[], current: string): { id: string; name: string }[] {
  const m = new Map<string, string>();
  for (const x of members) m.set(x.userId, x.displayName);
  for (const r of rows) if (!m.has(r.actorId)) m.set(r.actorId, r.actor);
  if (current && !m.has(current)) m.set(current, current);
  return [...m].map(([id, name]) => ({ id, name })).sort((a, b) => a.name.localeCompare(b.name) || a.id.localeCompare(b.id));
}

/** A record related to an entry: its own target, or the target of an entry with the same correlation ID. */
export type RelatedRef = { id: string; via: "target" | "correlation"; note: string };
export type Related = { restriction: RelatedRef | null; command: RelatedRef | null; job: RelatedRef | null; unit: RelatedRef | null };

/** The restriction, command, job and unit of an entry and of the entries with its correlation ID (DD-A16 step 1, Figma
 * Admin 337:2 / 338:2), the entry's own target first. A restriction the entry changed shows the state and the exception
 * end it recorded (its masked after values), so the screen reads no restriction; the links open the records' own
 * screens, which check their own permission (SCR-A16). */
export function relatedRecords(sel: ApiAudit, group: ApiAudit[], i: I18n = EN): Related {
  const { t } = i;
  const all = [sel, ...group.filter((g) => g.id !== sel.id)];
  const ref = (kind: string): RelatedRef | null => {
    const a = all.find((x) => x.targetRef.kind === kind);
    if (!a) return null;
    const via = a.id === sel.id ? "target" : "correlation";
    return { id: a.targetRef.id, via, note: via === "target" ? t("this entry’s target") : t("same correlation ID") };
  };
  const restriction = ref("restriction");
  if (restriction && sel.targetRef.kind === "restriction" && sel.targetRef.id === restriction.id) {
    const after = sel.maskedAfter ?? {};
    const state = after.state && (states as string[]).includes(after.state) ? stateWord(after.state as RestrictionState, t) : after.state;
    const until = after["exception.until"] && !Number.isNaN(Date.parse(after["exception.until"])) ? t("exception until {time}", { time: showTime(after["exception.until"], i.display) }) : null;
    const recorded = [state, until].filter(Boolean).join(" · ");
    if (recorded) restriction.note = t("{state} — as this entry recorded it", { state: recorded });
  }
  return { restriction, command: ref("command"), job: ref("job"), unit: ref("unit") };
}

/** DeviceEvent of service-contracts.ts with its sequence. */
export type ApiAuditDeviceEvent = ApiDeviceEventFull & { sequence: number; unitIdAtOccurrence: string };
const EVENT_TITLE: Record<ApiDeviceEventFull["eventType"], string> = {
  tamper: "Tamper · removal suspected", power_lost: "Power lost", communication_lost: "Communication lost", restored: "Restored", operation_failed: "Operation failed",
};
/** One device event of the audit's device tab (Figma Admin 340:2): the list line and the detail with its sequence, the
 * unit it happened on, its alerts and the response notes (added in Devices & models, read-only here). `names` are the
 * users' names, `units` the units' names. */
export type AuditDeviceEvent = {
  id: string; title: string; type: ApiDeviceEventFull["eventType"]; tone: "warn" | "crit" | "ok" | "muted"; meta: string; state: string;
  badge: { text: string; tone: "crit" | "ok" } | null; facts: [string, string][]; alerts: { id: string; text: string; href: string }[]; notes: { text: string; by: string }[];
};
export function auditDeviceEvents(es: ApiAuditDeviceEvent[], alerts: Map<string, ApiAlertLite>, names: Map<string, string>, units: Map<string, string>, nowMs: number, i: I18n = EN): AuditDeviceEvent[] {
  const { t, display } = i;
  const rows = new Map(eventRows(es, alerts, nowMs, i).map((r) => [r.id, r]));
  const seqs = es.map((e) => e.sequence).sort((a, b) => a - b);
  return es.map((e) => {
    const r = rows.get(e.id)!;
    const prev = [...seqs].reverse().find((s) => s < e.sequence);
    const sequence = prev === undefined ? String(e.sequence)
      : prev === e.sequence - 1 ? t("{n} (previous {p} — no gap)", { n: e.sequence, p: prev }) : t("{n} (previous {p} — {missing} missing)", { n: e.sequence, p: prev, missing: e.sequence - prev - 1 });
    const fault = e.eventType !== "restored";
    return {
      id: e.id, title: t(EVENT_TITLE[e.eventType]), type: e.eventType, tone: r.tone,
      meta: t("{source} · seq {n} · {time}", { source: e.evidenceSource, n: e.sequence, time: r.time }), state: r.recovery,
      badge: fault ? (e.restoredAt ? { text: t("Restored"), tone: "ok" } : { text: t("Not restored"), tone: "crit" }) : null,
      facts: [
        [t("Event type"), e.eventType], [t("Evidence"), `${e.evidenceSource} — ${eventCause(e, t)}`], [t("Sequence"), sequence],
        [t("Unit at the time"), units.get(e.unitIdAtOccurrence) ?? e.unitIdAtOccurrence.slice(0, 8)], [t("Occurred"), showTime(e.occurredAt, display)],
        ...(fault ? [[t("Restored at"), e.restoredAt ? showTime(e.restoredAt, display) : t("— not yet")] as [string, string]] : []), [t("Recovery"), r.recovery],
      ],
      alerts: e.alertIds.map((id) => {
        const a = alerts.get(id);
        return { id, text: a ? `${a.type} · ${t(a.status === "open" ? "open" : a.status === "acknowledged" ? "acknowledged" : "resolved")}` : t("alert {id}", { id: id.slice(0, 8) }), href: `/admin/alerts?alertId=${id}${a && a.status !== "open" ? `&status=${a.status}` : ""}` };
      }),
      notes: e.responseNotes.map((n) => ({ text: n.message, by: `${names.get(n.actorId) ?? n.actorId.slice(0, 8)} · ${showTime(n.at, display)}` })),
    };
  });
}
