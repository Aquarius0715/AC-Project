"use client";

import Link from "next/link";
import { useState } from "react";
import { Badge, Banner, Btn, Card, Field, Input, Page, SummaryList, Textarea, Timeline } from "@ac/web/components/ui";
import { useAction } from "@ac/web/lib/useAction";
import type { AuditRow } from "@ac/web/lib/audit";
import { klInstant, klLocal, klStamp } from "@ac/web/lib/energy";
import { actionOutcome, activePeriod, policyText, stateLabel, stateTone, type ActionKind, type ApiRestriction, type UnitRow } from "@ac/web/lib/restrictions";
import { cancelRestriction, deferRestriction, exemptRestriction, overrideRestriction, reconcileUnits, retryUnits } from "../../actions";

type Live = { now: string; r: ApiRestriction; units: UnitRow[]; canWrite: boolean; canOverride: boolean; canAudit: boolean; audit: AuditRow[] };
const cards: [ActionKind, string, string][] = [
  ["defer", "Grace period", "restrictions.defer"], ["exempt", "Exception", "restrictions.exempt"], ["cancel", "Cancel", "restrictions.cancel"], ["override", "Override release", "restrictions.override · restriction.override"],
];

/** Grace periods, exceptions, cancellation and manual release (FR-A10) in API mode. Each action card states the
 * transition for the current state before saving (IR96 / IR35); every action needs a reason and the version read. */
export function ExceptionView({ live }: { live: Live }) {
  const [pending, run] = useAction();
  const { r } = live;
  const now = new Date(live.now);
  const allowedFor = (k: ActionKind) => (k === "override" ? live.canOverride : live.canWrite);
  const first = cards.find(([k]) => allowedFor(k) && actionOutcome(k, r.state).allowed)?.[0] ?? "cancel";
  const [act, setAct] = useState<ActionKind>(first);
  const [until, setUntil] = useState(klLocal(new Date(now.getTime() + 7 * 24 * 3600 * 1000).toISOString()));
  const [reason, setReason] = useState("");
  const [tried, setTried] = useState(false);
  const needsDate = act === "defer" || act === "exempt";
  const outcome = actionOutcome(act, r.state);
  const untilMs = Date.parse(klInstant(until));
  const dateErr = needsDate && !(untilMs > now.getTime() && untilMs <= now.getTime() + 90 * 24 * 3600 * 1000) ? "A future date within 90 days" : undefined;
  const reasonErr = reason.trim().length < 1 || reason.length > 1000 ? "A reason is required (1–1000 characters)" : undefined;
  const period = activePeriod(r, now);
  const isRelease = r.projection === "release";
  const save = () => {
    setTried(true);
    if (!outcome.allowed || reasonErr || (needsDate && dateErr)) return;
    const why = reason.trim();
    const fn = act === "defer" ? () => deferRestriction(r.id, r.version, klInstant(until), why)
      : act === "exempt" ? () => exemptRestriction(r.id, r.version, klInstant(until), why)
      : act === "cancel" ? () => cancelRestriction(r.id, r.version, why) : () => overrideRestriction(r.id, r.version, why);
    run(fn, `${cards.find((c) => c[0] === act)![1]} recorded`, () => { setReason(""); setTried(false); });
  };
  // IR96: override-only callers see reconcile / release retry only for an override release intent or unresolved recovery
  const followUp = live.canWrite || (live.canOverride && (r.releaseIntent?.source === "override" || r.recoveryCases.some((c) => c.state !== "resolved")));
  return (
    <Page>
      <div className="flex flex-wrap items-center gap-2 text-[13px] text-muted"><Link href={`/admin/restrictions?restrictionId=${r.id}`} className="font-semibold text-primary">← Restrictions</Link>/ <b className="text-ink">{r.id.slice(0, 8)}</b> <Badge tone={stateTone(r.state)}>{stateLabel[r.state]}</Badge></div>
      <div className="split">
        <div className="flex min-w-0 flex-col gap-4">
          <Card title="Choose an action" sub={`Version ${r.version} · the result for the current state is shown before saving`}>
            <div className="grid-fluid" style={{ ["--min" as string]: "220px" }}>{cards.map(([k, label, op]) => {
              const o = actionOutcome(k, r.state);
              const permitted = allowedFor(k);
              return (
                <button key={k} disabled={!permitted || !o.allowed} onClick={() => setAct(k)} aria-pressed={act === k} className={`rounded-xl border p-3 text-left disabled:opacity-50 ${act === k ? "border-primary bg-primary-soft/60" : "border-line hover:bg-surface2"}`}>
                  <b className="text-[13px]">{label}</b><div className="font-mono text-[10px] text-muted">{op}</div><div className="mt-1 text-xs text-muted">{permitted ? o.result : k === "override" ? "Needs restriction.override" : "Needs restriction.write"}</div>
                </button>
              );
            })}</div>
            {needsDate && <div className="mt-4 max-w-xs"><Field label="Until (Kuala Lumpur) · future, at most 90 days" error={tried ? dateErr : undefined} hint="Expiry never reapplies the restriction — conditions are checked again"><Input type="datetime-local" value={until} onChange={(e) => setUntil(e.target.value)} /></Field></div>}
            <div className="mt-3"><Field label="Reason · required" error={tried ? reasonErr : undefined} hint={`${reason.length} / 1000`}><Textarea value={reason} maxLength={1000} onChange={(e) => setReason(e.target.value)} /></Field></div>
            <div className="mt-3 flex justify-end"><Btn variant={act === "override" ? "danger" : "primary"} disabled={pending || !outcome.allowed || !allowedFor(act)} onClick={save}>Apply {cards.find((c) => c[0] === act)![1].toLowerCase()}</Btn></div>
          </Card>
          <Card title="What happens"><ul className="list-disc pl-5 text-[13px]"><li>{outcome.result}</li><li>A release still needs per-unit evidence (released or not required)</li><li>Cause invoices stay unpaid — no action here settles billing</li><li>Before/after, expiry, reason and your name are recorded in the audit log</li></ul></Card>
          {followUp && live.units.some((u) => u.reconcile || u.retryRelease) && (
            <Card title="Units needing follow-up">{live.units.filter((u) => u.reconcile || u.retryRelease).map((u) => (
              <div key={u.unitId} className="flex items-center justify-between gap-2 border-t border-line py-1.5 text-[13px] first:border-0"><span>{u.name} <span className="text-xs text-muted">apply {u.apply} · release {u.release}{u.pending ? ` · ${u.pending}` : ""}</span></span><span className="flex gap-1">{u.reconcile && <Btn size="sm" disabled={pending} onClick={() => run(() => reconcileUnits(r.id, r.version, [u.unitId]), "Reconciled")}>Reconcile</Btn>}{u.retryRelease && <Btn size="sm" disabled={pending} onClick={() => run(() => retryUnits(r.id, r.version, [u.unitId], "release", r.rulesVersion, "Retry release from the exception screen"), "Release retried")}>Retry release</Btn>}</span></div>
            ))}</Card>
          )}
        </div>
        <div className="flex min-w-0 flex-col gap-4">
          <Card title="Summary">
            {isRelease && <Banner>Release view (restriction.override only, IR03): billing fields are not shown.</Banner>}
            <SummaryList items={[
              ["Policy", policyText(r.policy)], ["Units", live.units.map((u) => `${u.name} · ${u.apply}/${u.release}`).join(", ") || "—"],
              ...(isRelease ? [] : [["Cause invoices", `${r.causeInvoiceIds?.length ?? 0}`] as [string, string]]),
              ["Grace / exception", period ?? "none"], ["Release intent", r.releaseIntent ? `${r.releaseIntent.source} · ${klStamp(r.releaseIntent.at)}` : "none"],
              ["Your permissions", [live.canWrite && "restriction.write", live.canOverride && "restriction.override", live.canAudit && "audit.read"].filter(Boolean).join(" · ") || "read only"],
            ]} />
          </Card>
          {live.canAudit && <Card title="Audit" sub="audit.read">{live.audit.length === 0 ? <p className="text-xs text-muted">No audit entries yet.</p> : <Timeline items={live.audit.map((a) => ({ time: a.at, title: `${a.op} · ${a.res}`, detail: `${a.actor} (${a.role})${a.reason ? ` · ${a.reason}` : ""}` }))} />}</Card>}
        </div>
      </div>
    </Page>
  );
}
