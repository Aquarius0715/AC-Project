// HQ restrictions (FR-A09, FR-A10, DATA_SOURCE=api): restrictions with their units, cause invoices and commands
// projected for /admin/restrictions and /admin/restrictions/[id], with the transition rules of IR35 / IR96 / IR141 for
// the action availability. Pure code shared by the Server Components and the client views.
import { klStamp } from "@ac/web/lib/energy";
import { translator, type T } from "@ac/web/lib/i18n";

const en = translator("en");

export type RestrictionState = "scheduled" | "requested" | "applied" | "release_requested" | "released" | "cancelled";
export type Policy = { kind: "temperature_limit"; minimumCoolingSetpoint: number } | { kind: "power_off" };
export type PerUnit = {
  unitId: string; applyState: "not_sent" | "sent_unknown" | "applied" | "not_applied"; releaseState: "none" | "waiting_reconcile" | "requested" | "released" | "not_required" | "failed";
  applyCommandIds: string[]; releaseCommandIds: string[]; observedRestriction: { restrictionId: string; rulesVersion: string } | null; observedAt: string | null; pendingReason: string | null;
};
type Recovery = { id: string; unitId: string; state: "pending" | "removing" | "resolved"; successorRestrictionId: string | null };
type ReleaseIntent = { source: "payment" | "exception" | "override" | "manual" | "cancel"; at: string; actorMembershipId?: string } | null;
/** Restriction (canonical) or RestrictionReleaseView (override-only callers, IR03) of service-contracts.ts. */
export type ApiRestriction = {
  id: string; version: number; createdAt: string; unitIds: string[]; rulesVersion: string; policy: Policy; state: RestrictionState; perUnit: PerUnit[]; recoveryCases: Recovery[];
  noticeAt: string; executeAfter: string; releaseIntent: ReleaseIntent; projection?: "release";
  contractId?: string; contractVersion?: number; causeInvoiceIds?: string[]; reason?: string; exception?: { until: string; reason: string | null } | null; graceUntil?: string | null;
  noticeNotificationIds?: string[]; events?: { action: string; occurredAt: string; actorId: string; actorRoleAtTime: string; reason: string | null; result: string }[];
};
export type ApiCommand = { id: string; status: "requested" | "sent" | "acknowledged" | "failed" | "expired" | "cancelled"; delivery: string; requestedAt: string; failureCode: string | null };

export const states: RestrictionState[] = ["scheduled", "requested", "applied", "release_requested", "released", "cancelled"];
export const lifecycle = ["Scheduled", "Requested", "Applied", "Release requested", "Released"];
export const stateLabel: Record<RestrictionState, string> = { scheduled: "Scheduled", requested: "Requested", applied: "Applied", release_requested: "Release requested", released: "Released", cancelled: "Cancelled" };
export const stateTone = (s: RestrictionState) => (s === "applied" ? "warn" : s === "released" ? "ok" : s === "cancelled" ? "muted" : s === "release_requested" ? "unknown" : "primary") as "warn" | "ok" | "muted" | "unknown" | "primary";
/** The policy in the display language (the customer's notice, IR267); HQ's screens keep English. */
export const policyText = (p: Policy, t: T = en) => (p.kind === "power_off" ? t("Power off") : t("Temperature limit — cooling setpoint ≥ {value} °C", { value: p.minimumCoolingSetpoint }));

/** An active grace period or exception at now (execute and apply retries wait; release may proceed). */
export function activePeriod(r: ApiRestriction, now: Date): string | null {
  const t = now.getTime();
  if (r.exception && Date.parse(r.exception.until) > t) return `Exception until ${klStamp(r.exception.until)}`;
  if (r.graceUntil && Date.parse(r.graceUntil) > t) return `Grace until ${klStamp(r.graceUntil)}`;
  return null;
}

export type RestrictionRow = { id: string; version: number; state: RestrictionState; label: string; policy: string; units: number; progress: string };

export function progressText(r: ApiRestriction): string {
  const n = r.perUnit.length;
  const applied = r.perUnit.filter((u) => u.applyState === "applied").length;
  const released = r.perUnit.filter((u) => u.releaseState === "released" || u.releaseState === "not_required").length;
  const offline = r.perUnit.filter((u) => u.pendingReason === "offline").length;
  const tail = offline ? ` · ${offline} offline` : "";
  if (r.state === "scheduled") return `executes after ${klStamp(r.executeAfter)}`;
  if (r.state === "requested" || r.state === "applied") return `${applied}/${n} applied${tail}`;
  if (r.state === "release_requested") return `${released}/${n} released${tail}`;
  return r.state === "released" ? `${n} released` : "cancelled";
}

export function restrictionRows(rs: ApiRestriction[], contractLabel: (id?: string) => string): RestrictionRow[] {
  return rs.map((r) => ({ id: r.id, version: r.version, state: r.state, label: contractLabel(r.contractId), policy: policyText(r.policy), units: r.unitIds.length, progress: progressText(r) }));
}

/** Per-unit rows of the detail with the actions each unit allows (IR141 items 4–5, SR26). */
export type UnitRow = { unitId: string; name: string; apply: string; release: string; observed: string; pending: string | null; retryApply: boolean; retryRelease: boolean; reconcile: boolean; commands: string };
export function unitRows(r: ApiRestriction, names: Map<string, string>, commands: Map<string, ApiCommand>): UnitRow[] {
  return r.perUnit.map((u) => {
    const last = (ids: string[]) => (ids.length ? commands.get(ids[ids.length - 1]) : undefined);
    const a = last(u.applyCommandIds);
    const rel = last(u.releaseCommandIds);
    return {
      unitId: u.unitId, name: names.get(u.unitId) ?? u.unitId.slice(0, 8), apply: u.applyState, release: u.releaseState, pending: u.pendingReason,
      observed: u.observedRestriction ? `${u.observedRestriction.restrictionId === r.id ? "this restriction" : "another restriction"} · ${u.observedAt ? klStamp(u.observedAt) : ""}` : "none observed",
      retryApply: r.state === "requested" && (u.applyState === "not_sent" || u.applyState === "not_applied"),
      retryRelease: r.state === "release_requested" && (u.releaseState === "failed" || (u.releaseState === "none" && u.applyState === "applied")),
      reconcile: ["requested", "applied", "release_requested"].includes(r.state) && u.applyState === "sent_unknown",
      commands: [a && `apply ${a.status}${a.failureCode ? ` (${a.failureCode})` : ""}`, rel && `remove ${rel.status}${rel.failureCode ? ` (${rel.failureCode})` : ""}`].filter(Boolean).join(" · ") || "—",
    };
  });
}

/** Execute is enabled only for scheduled restrictions at or after executeAfter and 24 h after the notice, without an
 * active grace period or exception (IR140 item 2); otherwise the reason. */
export function executeBlocker(r: ApiRestriction, now: Date): string | null {
  if (r.state !== "scheduled") return "Only a scheduled restriction can be executed";
  const t = now.getTime();
  if (t < Date.parse(r.executeAfter)) return `Executes after ${klStamp(r.executeAfter)}`;
  if (t < Date.parse(r.noticeAt) + 24 * 3600 * 1000) return "Wait 24 hours after the notice";
  const p = activePeriod(r, now);
  return p ? `${p} — execution waits` : null;
}

/** An explicit release request needs requested/applied and all causes paid or an active grace/exception (IR35). */
export function releaseBlocker(r: ApiRestriction, allPaid: boolean, now: Date): string | null {
  if (r.state === "release_requested") return "Release already requested";
  if (r.state !== "requested" && r.state !== "applied") return "Only a requested or applied restriction can be released";
  if (!allPaid && !activePeriod(r, now)) return "Cause invoices are unpaid and no grace period or exception is active — use the exception screen or an override";
  return null;
}

/** The outcome of each exception action for the current state (IR96, IR35 ②③, IR141 items 1 and 3). */
export type ActionKind = "defer" | "exempt" | "cancel" | "override";
export function actionOutcome(kind: ActionKind, s: RestrictionState): { allowed: boolean; result: string } {
  const terminal = s === "released" || s === "cancelled";
  if (terminal) return { allowed: false, result: `Not possible — the restriction is ${stateLabel[s].toLowerCase()}` };
  switch (kind) {
    case "defer":
    case "exempt":
      return s === "scheduled" ? { allowed: true, result: "Stays scheduled; execution waits until the date passes" }
        : s === "release_requested" ? { allowed: true, result: "Records the period only (release already requested)" }
        : { allowed: true, result: `${stateLabel[s]} → Release requested (source: exception)` };
    case "cancel":
      return s === "scheduled" ? { allowed: true, result: "Scheduled → Cancelled; no commands are sent" }
        : s === "release_requested" ? { allowed: true, result: "No change — release already requested (idempotent)" }
        : { allowed: true, result: `${stateLabel[s]} → Release requested (source: cancel); application may already have happened` };
    case "override":
      return s === "scheduled" ? { allowed: false, result: "Not possible for a scheduled restriction — cancel it instead" }
        : s === "release_requested" ? { allowed: true, result: "No change — release already requested (idempotent)" }
        : { allowed: true, result: `${stateLabel[s]} → Release requested (source: override); the invoices stay unpaid` };
  }
}
