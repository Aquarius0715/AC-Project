// Audit explorer (FR-A16, DD-A16, SCR-A16, DATA_SOURCE=API): audit.list items for the admin audit screen, the URL
// filters of the screen, the records related to an entry and its correlation trace, and the device events tab. Pure code
// shared by the Server Component and the client view. Texts in the display language and instants in the display time
// zone; the period's days are the display time zone's days too (`i` / `zone`, IR304). Actions, target kinds and IDs,
// correlation IDs, field names, event types and evidence sources stay codes (IR304, IR320).
import { EN, relativeTime, showTime, translator, zonedInstant, zonedParts, type I18n, type T, textOf } from "@ac/web/lib/i18n";
import type { OpInput } from "@ac/web/lib/opTypes";
import { stateWord, states } from "@ac/web/lib/restrictions";
import { oneOf } from "@ac/web/lib/oneOf";
import { eventCause, eventRows, type ApiAlertLite, type ApiDeviceEventFull } from "@ac/web/lib/techDevices";
import { byText } from "@ac/web/lib/order";
import { wrapped } from "@ac/web/lib/text";
import { lookup } from "@ac/web/lib/lookup";

const RESTRICTION = "restriction";
const KL = "Asia/Kuala_Lumpur";

const en = translator("en");
export type AuditResult = "Success" | "Denied" | "Failed" | "Pending";
export interface AuditChange { field: string; before: string; after: string; changed: boolean }
export interface AuditRow {
  id: string; op: string; target: string; targetKind: string; targetId: string; versions: string | null; actor: string; actorId: string; role: string; at: string; occurred: string;
  corr: string; res: AuditResult; reason: string | null; changes: AuditChange[];
}

/** AuditView of service-contracts.ts. */
export interface ApiAudit {
  id: string; actorId: string; actorName: string | null; actorRoleAtTime: string; action: string; targetRef: { kind: string; id: string }; previousVersion: number | null; nextVersion: number | null;
  occurredAt: string; correlationId: string; result: "success" | "denied" | "failed" | "pending"; maskedBefore: Record<string, string | null>; maskedAfter: Record<string, string | null>; reason: string | null;
}

/** The URL filters of the log (SCR-A16): the period, actor, target kind and ID, correlation ID, result and page size. */
export interface AuditFilters { from: string; to: string; actorId: string; targetKind: string; targetId: string; correlationId: string; result: AuditResult | "All"; limit: number }

/** The API's result and the word the screen files it under. */
const RESULT_WORDS: [ApiAudit["result"], AuditResult][] = [["success", "Success"], ["denied", "Denied"], ["failed", "Failed"], ["pending", "Pending"]];
const results = RESULT_WORDS.map(([, word]) => word);
/** A result as the screen words it. */
export const resultWord = (r: string, t: T = en) => lookup({ Success: t("Success"), Denied: t("Denied"), Failed: t("Failed"), Pending: t("Pending") }, r) ?? r;
/** The badge tone of a result. */
const RESULT_TONE = new Map<AuditResult, "ok" | "warn" | "crit">([["Success", "ok"], ["Denied", "warn"], ["Failed", "crit"]]);
export const resultTone = (r: AuditResult) => RESULT_TONE.get(r) ?? "primary";
/** The role an entry recorded, worded (it is never rewritten). */
export const roleAtTimeWord = (r: string, t: T = en) =>
  lookup({ admin: t("Admin"), contractor: t("Contractor"), technician: t("Technician"), client: t("Client"), system: t("System") }, r) ?? r;
const resultOf = (r: ApiAudit["result"]): AuditResult => RESULT_WORDS.find(([wire]) => wire === r)?.[1] ?? "Pending";

/** The screen's filters from the URL; the default period is the 30 days up to the Core API business clock, as days of the display time zone. */
export function auditFilters(sp: Record<string, string | string[] | undefined>, now: Date, zone = KL): AuditFilters {
  const one = (k: string) => (typeof sp[k] === "string" ? (sp[k]) : "");
  const res = one("result");
  const limit = Math.min(100, Math.max(25, Number(one("limit")) || 25));
  const day = (ms: number) => zonedParts(new Date(ms).toISOString(), zone).date;
  return {
    from: one("from") || day(now.getTime() - 30 * 86_400_000),
    to: one("to") || day(now.getTime() + 86_400_000),
    actorId: one("actorId"), targetKind: one("targetKind"), targetId: one("targetId"), correlationId: one("correlationId"),
    result: oneOf(results, res, "All"),
    limit,
  };
}

/** A valid period: start < end, at most 366 days (IR143). */
export function periodError(f: Pick<AuditFilters, "from" | "to">, t: T = en): string | undefined {
  const a = Date.parse(f.from), b = Date.parse(f.to);
  const isOrdered = !Number.isNaN(a) && !Number.isNaN(b) && a < b; // an unreadable date is not ordered
  return !isOrdered || b - a > 366 * 86_400_000 ? t("Period must be start < end, at most 366 days") : undefined;
}

/** The period of the filters as instants: its days start at midnight in the display time zone (NFR-08). */
export const periodInstants = (f: Pick<AuditFilters, "from" | "to">, zone = KL) => ({ from: zonedInstant(f.from, "00:00", zone), to: zonedInstant(f.to, "00:00", zone) });

/** audit.list input for the filters. */
export function auditQuery(f: AuditFilters, zone = KL): OpInput<"audit.list"> {
  const wire = RESULT_WORDS.find(([, word]) => word === f.result)?.[0];
  return {
    limit: f.limit,
    filters: {
      ...periodInstants(f, zone),
      ...(f.actorId && { actorId: f.actorId }),
      ...(f.targetKind && { targetKind: f.targetKind }),
      ...(f.targetId && { targetId: f.targetId }),
      ...(f.correlationId && { correlationId: f.correlationId }),
      ...(wire && { result: wire }),
    },
  };
}

/** The entries of a correlation ID around one entry (a day either side, inside the 366-day limit): its trace and its
related records. */
export function correlationQuery(a: Pick<ApiAudit, "correlationId" | "occurredAt">): OpInput<"audit.list"> {
  const at = Date.parse(a.occurredAt);
  return { limit: 100, sort: { field: "occurredAt", direction: "asc" }, filters: { from: new Date(at - 86_400_000).toISOString(), to: new Date(at + 86_400_000).toISOString(), correlationId: a.correlationId } };
}

/** The versions a write moved between: “version 3”, “version 2 → 3”, or none. */
function versionsText(previous: number | null, next: number | null, t: T): string | null {
  if (next === null) return null;
  return previous === null ? t("version {v}", { v: next }) : t("version {from} → {to}", { from: previous, to: next });
}
/** One entry of the log: who acted (the user's name, else the actor ID — a system actor has no name, IR305), the list
time relative to now, the full time, the role in the display language and the versions it moved the target between. */
export function auditRow(a: ApiAudit, i: I18n = EN, nowMs?: number): AuditRow {
  const fields = [...new Set([...Object.keys(a.maskedBefore ?? {}), ...Object.keys(a.maskedAfter ?? {})])].toSorted(byText);
  const { t } = i;
  return {
    id: a.id,
    op: a.action,
    target: `${a.targetRef.kind} · ${a.targetRef.id}`, targetKind: a.targetRef.kind, targetId: a.targetRef.id,
    versions: versionsText(a.previousVersion, a.nextVersion, t),
    actor: a.actorName ?? a.actorId,
    actorId: a.actorId,
    role: roleAtTimeWord(a.actorRoleAtTime, t),
    at: nowMs === undefined ? showTime(a.occurredAt, i.display) : relativeTime(a.occurredAt, nowMs, i),
    occurred: showTime(a.occurredAt, i.display),
    corr: a.correlationId,
    res: resultOf(a.result),
    reason: reasonText(a, t),
    changes: fields.map((f) => {
      const before = a.maskedBefore?.[f] ?? "null", after = a.maskedAfter?.[f] ?? "null";
      return { field: f, changed: before !== after, before, after };
    }),
  };
}

/** A denied write records the refusal's message key as its reason (IR603): it is said in words; any other reason stays as
recorded. */
const DENIAL: Record<string, string> = {
  "error.forbidden": "Refused — the role may not do this",
  "error.notFound": "Refused — not found in the actor's scope",
};
export const reasonText = (a: Pick<ApiAudit, "result" | "reason">, t: T = en): string | null => ((a.result === "denied" && a.reason ? textOf(DENIAL, a.reason, t) : undefined) ?? a.reason);

/** An entry as a timeline item (the restriction exception screen's audit card, IR301; the correlation trace, IR320). */
export const auditItem = (a: ApiAudit, i: I18n = EN) => {
  const reason = reasonText(a, i.t);
  return {
    time: showTime(a.occurredAt, i.display), title: `${a.action} · ${resultWord(resultOf(a.result), i.t)}`,
    detail: `${a.actorName ?? a.actorId} (${roleAtTimeWord(a.actorRoleAtTime, i.t)})${wrapped(reason, " · ")}`,
  };
};

/** The entries of one correlation ID in the order they were appended (Figma Admin 338:2): a pending entry stays as it
was; the final result is a later entry of its own. Empty when the entry stands alone. */
export const correlationTrace = (group: ApiAudit[], i: I18n = EN) =>
  group.length < 2 ? [] : [...group].toSorted((a, b) => Date.parse(a.occurredAt) - Date.parse(b.occurredAt) || a.id.localeCompare(b.id)).map((a) => ({ id: a.id, ...auditItem(a, i) }));

/** The target kinds the Target filter always offers, the records HQ looks up most; the kinds of the shown entries and
the URL's kind join them (targetKind is a string like AuditView.targetRef.kind and the log records more kinds). */
const KINDS = ["alert", "automation", "command", "contract", "device", "device_event", "invoice", "job", "membership", "payment", "policy", RESTRICTION, "unit", "user"];
export const targetKinds = (rows: Pick<AuditRow, "targetKind">[], current: string) => [...new Set([...KINDS, ...rows.map((r) => r.targetKind), ...(current ? [current] : [])])].toSorted(byText);

/** The Actor filter's people by name: the users of the memberships the session may read (members.list), the actors of
the shown entries and the URL's actor (a system actor by its ID). A name that several people share carries the start
of each one's ID, as the entry's detail shows it, so the auditor can tell them apart (IR599). */
export function actorOptions(members: { userId: string; displayName: string }[], rows: Pick<AuditRow, "actor" | "actorId">[], current: string): { id: string; name: string }[] {
  const m = new Map<string, string>();
  for (const x of members) m.set(x.userId, x.displayName);
  for (const r of rows) if (!m.has(r.actorId)) m.set(r.actorId, r.actor);
  if (current && !m.has(current)) m.set(current, current);
  const shared = new Map<string, number>();
  for (const name of m.values()) shared.set(name, (shared.get(name) ?? 0) + 1);
  return [...m].map(([id, name]) => ({ id, name: (shared.get(name) ?? 0) > 1 ? `${name} · ${id.slice(0, 8)}` : name })).toSorted((a, b) => a.name.localeCompare(b.name) || a.id.localeCompare(b.id));
}

/** A record related to an entry: its own target, or the target of an entry with the same correlation ID. */
export interface RelatedRef { id: string; via: "target" | "correlation"; note: string }
export interface Related { restriction: RelatedRef | null; command: RelatedRef | null; job: RelatedRef | null; unit: RelatedRef | null }

/** The restriction, command, job and unit of an entry and of the entries with its correlation ID (DD-A16 step 1, Figma
Admin 337:2 / 338:2), the entry's own target first. A restriction the entry changed shows the state and the exception
end it recorded (its masked after values), so the screen reads no restriction; the links open the records' own
screens, which check their own permission (SCR-A16). */
export function relatedRecords(sel: ApiAudit, group: ApiAudit[], i: I18n = EN): Related {
  const { t } = i;
  const all = [sel, ...group.filter((g) => g.id !== sel.id)];
  const ref = (kind: string): RelatedRef | null => {
    const a = all.find((x) => x.targetRef.kind === kind);
    if (!a) return null;
    const via = a.id === sel.id ? "target" : "correlation";
    return { id: a.targetRef.id, note: t(via === "target" ? "this entry’s target" : "same correlation ID"), via };
  };
  const restriction = ref(RESTRICTION);
  if (restriction && sel.targetRef.kind === RESTRICTION && sel.targetRef.id === restriction.id) {
    const after = sel.maskedAfter ?? {};
    const known = after.state ? oneOf(states, after.state, undefined) : undefined;
    const state = known ? stateWord(known, t) : after.state;
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
unit it happened on, its alerts and the response notes (added in Devices & models, read-only here). `names` are the
users' names, `units` the units' names. */
export interface AuditDeviceEvent {
  id: string; title: string; type: ApiDeviceEventFull["eventType"]; tone: "warn" | "crit" | "ok" | "muted"; meta: string; state: string;
  badge: { text: string; tone: "crit" | "ok" } | null; facts: [string, string][]; alerts: { id: string; text: string; href: string }[]; notes: { text: string; by: string }[];
}
/** An event's sequence number against the one before it: no gap, or how many are missing. */
function sequenceText(n: number, prev: number | undefined, t: T): string {
  if (prev === undefined) return String(n);
  return prev === n - 1 ? t("{n} (previous {p} — no gap)", { n, p: prev }) : t("{n} (previous {p} — {missing} missing)", { n, p: prev, missing: n - prev - 1 });
}
/** A fault's badge (whether it was restored); a restore event has none. */
function faultBadge(e: ApiAuditDeviceEvent, t: T): { text: string; tone: "ok" | "crit" } | null {
  if (e.eventType === "restored") return null;
  return e.restoredAt ? { text: t("Restored"), tone: "ok" } : { text: t("Not restored"), tone: "crit" };
}
export function auditDeviceEvents(es: ApiAuditDeviceEvent[], alerts: Map<string, ApiAlertLite>, names: Map<string, string>, units: Map<string, string>, nowMs: number, i: I18n = EN): AuditDeviceEvent[] {
  const { t, display } = i;
  const rows = new Map(eventRows(es, alerts, nowMs, i).map((r) => [r.id, r]));
  const seqs = es.map((e) => e.sequence).toSorted((a, b) => a - b);
  const withRows = es.map((e) => ({ e, r: rows.get(e.id) })).filter((x): x is { e: ApiAuditDeviceEvent; r: NonNullable<typeof x.r> } => x.r !== undefined);
  return withRows.map(({ e, r }) => {
    const prev = seqs.findLast((s) => s < e.sequence);
    const sequence = sequenceText(e.sequence, prev, t);
    const isFault = e.eventType !== "restored";
    return {
      id: e.id, title: t(EVENT_TITLE[e.eventType]), type: e.eventType, tone: r.tone,
      meta: t("{source} · seq {n} · {time}", { source: e.evidenceSource, n: e.sequence, time: r.time }), state: r.recovery,
      badge: faultBadge(e, t),
      facts: [
        [t("Event type"), e.eventType], [t("Evidence"), `${e.evidenceSource} — ${eventCause(e, t)}`], [t("Sequence"), sequence],
        [t("Unit at the time"), units.get(e.unitIdAtOccurrence) ?? e.unitIdAtOccurrence.slice(0, 8)], [t("Occurred"), showTime(e.occurredAt, display)],
        ...(isFault ? [[t("Restored at"), e.restoredAt ? showTime(e.restoredAt, display) : t("— not yet")] as [string, string]] : []), [t("Recovery"), r.recovery],
      ],
      alerts: e.alertIds.map((id) => {
        const a = alerts.get(id);
        return { id, text: a ? `${a.type} · ${t(a.status)}` : t("alert {id}", { id: id.slice(0, 8) }), href: `/admin/alerts?alertId=${id}${a && a.status !== "open" ? `&status=${a.status}` : ""}` };
      }),
      notes: e.responseNotes.map((n) => ({ text: n.message, by: `${names.get(n.actorId) ?? n.actorId.slice(0, 8)} · ${showTime(n.at, display)}` })),
    };
  });
}
