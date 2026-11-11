// HQ restrictions (FR-A09, FR-A10, DATA_SOURCE=API): restrictions with their units, cause invoices and commands
// projected for /admin/restrictions and /admin/restrictions/[id], with the transition rules of IR35 / IR96 / IR141 for
// the action availability. Pure code shared by the Server Components and the client views. Texts in the display
// language and instants in the display time zone (`t` / `i`, IR301); the customer's notice uses the same words (IR267).
import { EN, showTime, translator, type I18n, type T } from "@ac/web/lib/i18n";
import { wrapped } from "@ac/web/lib/text";
import { lookup } from "@ac/web/lib/lookup";

const RELEASE_REQUESTED = "release_requested";

const en = translator("en");

export type RestrictionState = "scheduled" | "requested" | "applied" | "release_requested" | "released" | "cancelled";
export type Policy = { kind: "temperature_limit"; minimumCoolingSetpoint: number } | { kind: "power_off" };
export interface PerUnit {
  unitId: string; applyState: "not_sent" | "sent_unknown" | "applied" | "not_applied"; releaseState: "none" | "waiting_reconcile" | "requested" | "released" | "not_required" | "failed";
  applyCommandIds: string[]; releaseCommandIds: string[]; observedRestriction: { restrictionId: string; rulesVersion: string } | null; observedAt: string | null; pendingReason: string | null;
}
interface Recovery { id: string; unitId: string; state: "pending" | "removing" | "resolved"; successorRestrictionId: string | null }
type ReleaseIntent = { source: "payment" | "exception" | "override" | "manual" | "cancel"; at: string; actorMembershipId?: string } | null;
/** Restriction (canonical) or RestrictionReleaseView (override-only callers, IR03) of service-contracts.ts. */
export interface ApiRestriction {
  id: string; version: number; createdAt: string; unitIds: string[]; rulesVersion: string; policy: Policy; state: RestrictionState; perUnit: PerUnit[]; recoveryCases: Recovery[];
  noticeAt: string; executeAfter: string; releaseIntent: ReleaseIntent; projection?: "release";
  contractId?: string; contractVersion?: number; causeInvoiceIds?: string[]; reason?: string; exception?: { until: string; reason: string | null } | null; graceUntil?: string | null;
  noticeNotificationIds?: string[]; events?: { action: string; occurredAt: string; actorId: string; actorRoleAtTime: string; reason: string | null; result: string }[];
}
export interface ApiCommand { id: string; status: "requested" | "sent" | "acknowledged" | "failed" | "expired" | "cancelled"; delivery: string; requestedAt: string; failureCode: string | null }

export const states: RestrictionState[] = ["scheduled", "requested", "applied", RELEASE_REQUESTED, "released", "cancelled"];
/** The steps of the lifecycle (cancelled is off the path). */
export const lifecycleStates: RestrictionState[] = ["scheduled", "requested", "applied", RELEASE_REQUESTED, "released"];
export const stateLabel: Record<RestrictionState, string> = { scheduled: "Scheduled", requested: "Requested", applied: "Applied", release_requested: "Release requested", released: "Released", cancelled: "Cancelled" };
/** A state as the screens word it. */
export const stateWord = (s: RestrictionState, t: T = en) => {
  const words = new Map(Object.entries({ scheduled: t("Scheduled"), requested: t("Requested"), applied: t("Applied"), release_requested: t("Release requested"), released: t("Released"), cancelled: t("Cancelled") }));
  return words.get(s) ?? s; // a value the API adds before this table does
};
/** The colour of a restriction state's badge. */
export type StateTone = "warn" | "ok" | "muted" | "unknown" | "primary";
const STATE_TONE = new Map<RestrictionState, StateTone>([["applied", "warn"], ["released", "ok"], ["cancelled", "muted"], [RELEASE_REQUESTED, "unknown"]]);
export const stateTone = (s: RestrictionState): StateTone => STATE_TONE.get(s) ?? "primary";
/** Which restriction the unit reports it is under, and when it said so. */
function observedText(u: ApiRestriction["perUnit"][number], restrictionId: string, i: I18n): string {
  const { t } = i;
  if (!u.observedRestriction) return t("none observed");
  const which = t(u.observedRestriction.restrictionId === restrictionId ? "this restriction" : "another restriction");
  return u.observedAt ? `${which} · ${showTime(u.observedAt, i.display)}` : which;
}
export const policyText = (p: Policy, t: T = en) => (p.kind === "power_off" ? t("Power off") : t("Temperature limit — cooling setpoint ≥ {value} °C", { value: p.minimumCoolingSetpoint }));
/** Where a release intent came from, worded. */
const sourceWord = (s: NonNullable<ReleaseIntent>["source"], t: T = en) => {
  const words: Partial<Record<NonNullable<ReleaseIntent>["source"], string>> = { payment: t("payment"), exception: t("exception"), override: t("override"), manual: t("manual"), cancel: t("cancellation") };
  return lookup(words, s) ?? s; // a value the API adds before this table does
};
/** The release intent with its time in the display time zone, or "none". */
export const intentText = (r: Pick<ApiRestriction, "releaseIntent">, i: I18n = EN) => (r.releaseIntent ? `${sourceWord(r.releaseIntent.source, i.t)} · ${showTime(r.releaseIntent.at, i.display)}` : i.t("none"));

/** An active grace period or exception at now (execute and apply retries wait; release may proceed). */
export function activePeriod(r: ApiRestriction, now: Date, i: I18n = EN): string | null {
  const ms = now.getTime();
  if (r.exception && Date.parse(r.exception.until) > ms) return i.t("Exception until {time}", { time: showTime(r.exception.until, i.display) });
  if (r.graceUntil && Date.parse(r.graceUntil) > ms) return i.t("Grace until {time}", { time: showTime(r.graceUntil, i.display) });
  return null;
}

export interface RestrictionRow { id: string; version: number; state: RestrictionState; label: string; policy: string; units: number; progress: string }

export function progressText(r: ApiRestriction, i: I18n = EN): string {
  const { t } = i;
  const n = r.perUnit.length;
  const applied = r.perUnit.filter((u) => u.applyState === "applied").length;
  const released = r.perUnit.filter((u) => u.releaseState === "released" || u.releaseState === "not_required").length;
  const offline = r.perUnit.filter((u) => u.pendingReason === "offline").length;
  const busy = r.perUnit.filter((u) => u.pendingReason === "device_operation_running").length; // waiting for a check or firmware update (D03)
  if (r.state === "scheduled") return t("executes after {time}", { time: showTime(r.executeAfter, i.display) });
  const notes = [...(offline > 0 ? [t("{n} offline", { n: offline })] : []), ...(busy > 0 ? [t("{n} waiting for a device operation", { n: busy })] : [])];
  const tail = notes.map((x) => ` · ${x}`).join("");
  if (r.state === "requested" || r.state === "applied") return `${t("{done}/{n} applied", { done: applied, n })}${tail}`;
  if (r.state === RELEASE_REQUESTED) return `${t("{done}/{n} released", { done: released, n })}${tail}`;
  return r.state === "released" ? t("{n} released", { n }) : t("cancelled");
}

export function restrictionRows(rs: ApiRestriction[], contractLabel: (id?: string) => string, i: I18n = EN): RestrictionRow[] {
  return rs.map((r) => ({ id: r.id, version: r.version, state: r.state, label: contractLabel(r.contractId), policy: policyText(r.policy, i.t), units: r.unitIds.length, progress: progressText(r, i) }));
}

/** Per-unit rows of the detail with the actions each unit allows (IR141 items 4–5, SR26). */
export interface UnitRow { unitId: string; name: string; apply: string; release: string; observed: string; pending: string | null; retryApply: boolean; retryRelease: boolean; reconcile: boolean; commands: string }
export function unitRows(r: ApiRestriction, names: Map<string, string>, commands: Map<string, ApiCommand>, i: I18n = EN): UnitRow[] {
  const { t } = i;
  const apply: Partial<Record<PerUnit["applyState"], string>> = { not_sent: t("not sent"), sent_unknown: t("sent, result unknown"), applied: t("applied"), not_applied: t("not applied") };
  const release: Partial<Record<PerUnit["releaseState"], string>> = {
    none: t("none"), waiting_reconcile: t("waiting for reconciliation"), requested: t("requested"), released: t("released"), not_required: t("not required"), failed: t("failed"),
  };
  const command: Partial<Record<ApiCommand["status"], string>> = { requested: t("requested"), sent: t("sent"), acknowledged: t("acknowledged"), failed: t("failed"), expired: t("expired"), cancelled: t("cancelled") };
  const pendingText = new Map([["offline", t("offline")], ["device_operation_running", t("a device operation is running")]]);
  const pending = (p: string | null) => (p === null ? null : pendingText.get(p) ?? p);
  return r.perUnit.map((u) => {
    const last = (ids: string[]) => { const id = ids.at(-1); return id ? commands.get(id) : undefined; };
    const a = last(u.applyCommandIds);
    const releaseId = last(u.releaseCommandIds);
    const status = (c: ApiCommand) => `${lookup(command, c.status) ?? c.status}${wrapped(c.failureCode, " (", ")")}`;
    return {
      unitId: u.unitId, name: names.get(u.unitId) ?? u.unitId.slice(0, 8), apply: lookup(apply, u.applyState) ?? u.applyState, release: lookup(release, u.releaseState) ?? u.releaseState, pending: pending(u.pendingReason),
      observed: observedText(u, r.id, i),
      retryApply: r.state === "requested" && (u.applyState === "not_sent" || u.applyState === "not_applied"),
      retryRelease: r.state === RELEASE_REQUESTED && (u.releaseState === "failed" || (u.releaseState === "none" && u.applyState === "applied")),
      reconcile: ["requested", "applied", RELEASE_REQUESTED].includes(r.state) && u.applyState === "sent_unknown",
      commands: [a && t("apply {status}", { status: status(a) }), releaseId && t("remove {status}", { status: status(releaseId) })].filter(Boolean).join(" · ") || "—",
    };
  });
}

/** Execute is enabled only for scheduled restrictions at or after executeAfter and 24 h after the notice, without an
active grace period or exception (IR140 item 2); otherwise the reason. */
export function executeBlocker(r: ApiRestriction, now: Date, i: I18n = EN, processing = false): string | null {
  const { t } = i;
  if (r.state !== "scheduled") return t("Only a scheduled restriction can be executed");
  const ms = now.getTime();
  if (ms < Date.parse(r.executeAfter)) return t("Executes after {time}", { time: showTime(r.executeAfter, i.display) });
  if (ms < Date.parse(r.noticeAt) + 24 * 3600 * 1000) return t("Wait 24 hours after the notice");
  const p = activePeriod(r, now, i);
  if (p) return t("{period} — execution waits", { period: p });
  return processing ? t("A payment of a cause invoice is in progress — wait for it to be confirmed or to fail") : null; // the Core API refuses it too (IR773)
}

/** An explicit release request needs requested/applied and all causes paid or an active grace/exception (IR35). */
export function releaseBlocker(r: ApiRestriction, allPaid: boolean, now: Date, i: I18n = EN): string | null {
  const { t } = i;
  if (r.state === RELEASE_REQUESTED) return t("Release already requested");
  if (r.state !== "requested" && r.state !== "applied") return t("Only a requested or applied restriction can be released");
  if (!allPaid && !activePeriod(r, now, i)) return t("Cause invoices are unpaid and no grace period or exception is active — use the exception screen or an override");
  return null;
}

/** The outcome of each exception action for the current state (IR96, IR35 items 2 and 3, IR141 items 1 and 3). */
export type ActionKind = "defer" | "exempt" | "cancel" | "override";
export function actionOutcome(kind: ActionKind, s: RestrictionState, t: T = en): { allowed: boolean; result: string } {
  const isTerminal = s === "released" || s === "cancelled";
  if (isTerminal) return { allowed: false, result: t("Not possible — the restriction is {state}", { state: stateWord(s, t).toLowerCase() }) };
  const state = stateWord(s, t);
  switch (kind) {
    case "defer":
    case "exempt": {
      return s === "scheduled" ? { allowed: true, result: t("Stays scheduled; execution waits until the date passes") }
        : (({ allowed: true, result: s === RELEASE_REQUESTED ? t("Records the period only (release already requested)") : t("{state} → Release requested (source: exception)", { state }) }));
    }
    case "cancel": {
      return s === "scheduled" ? { allowed: true, result: t("Scheduled → Cancelled; no commands are sent") }
        : (({ allowed: true, result: s === RELEASE_REQUESTED ? t("No change — release already requested (idempotent)") : t("{state} → Release requested (source: cancel); application may already have happened", { state }) }));
    }
    case "override": {
      return s === "scheduled" ? { allowed: false, result: t("Not possible for a scheduled restriction — cancel it instead") }
        : (({ allowed: true, result: s === RELEASE_REQUESTED ? t("No change — release already requested (idempotent)") : t("{state} → Release requested (source: override); the invoices stay unpaid", { state }) }));
    }
  }
}
