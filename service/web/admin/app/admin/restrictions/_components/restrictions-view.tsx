"use client";

import Link from "next/link";
import { useState } from "react";
import { Badge, Banner, Btn, Card, Check, DataTable, EmptyState, Field, Input, ListRow, Modal, Page, Select, Steps, SummaryList, Textarea } from "@ac/web/components/ui";
import { useAction } from "@ac/web/lib/useAction";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { amount, klInstant, klLocal, klStamp } from "@ac/web/lib/energy";
import {
  activePeriod, executeBlocker, lifecycle, policyText, releaseBlocker, stateLabel, states, stateTone,
  type ApiRestriction, type Policy, type RestrictionRow, type RestrictionState, type UnitRow,
} from "@ac/web/lib/restrictions";
import { executeRestriction, reconcileUnits, requestRelease, retryUnits, scheduleRestriction } from "../actions";

type ScheduleContract = {
  id: string; version: number; label: string; rulesVersion: string; recipients: number; invoices: { id: string; number: string }[];
  units: { id: string; name: string; blocked: string | null; temperature: { min: number; max: number; step: number } | null }[];
};
export type RestrictionsLive = {
  now: string; full: boolean; canWrite: boolean; scope: { contractId?: string; invoiceId?: string; status?: RestrictionState };
  rows: RestrictionRow[]; counts: Record<RestrictionState, number>; contracts: { id: string; label: string }[];
  selected?: { r: ApiRestriction; contract: string; causes: { id: string; number: string; paid: boolean; amountMinor: number | null; currency: string }[]; allPaid: boolean; units: UnitRow[] };
  schedule?: ScheduleContract[];
} | null;

/** The restriction manager (FR-A09) in API mode: scope, state and selection live in the URL; schedule, execute,
 * release requests, retries and reconciliation are Server Actions with the restriction version. */
export function RestrictionsView({ live }: { live: RestrictionsLive }) {
  if (!live) return <Page><EmptyState title="No access">Restrictions need restriction.read, restriction.write or restriction.override.</EmptyState></Page>;
  return <Manager live={live} />;
}

function Manager({ live }: { live: NonNullable<RestrictionsLive> }) {
  const nav = useUrlPatch();
  const [pending, run] = useAction();
  const now = new Date(live.now);
  const sel = live.selected;
  const r = sel?.r;
  const [confirmRules, setConfirmRules] = useState(false);
  const [retry, setRetry] = useState<null | { phase: "apply" | "release"; unitIds: string[]; reason: string }>(null);
  const [scheduling, setScheduling] = useState(false);
  const step = r ? (r.state === "cancelled" ? -1 : lifecycle.indexOf(stateLabel[r.state])) : 0;
  const exec = r ? executeBlocker(r, now) : null;
  const release = r && sel ? releaseBlocker(r, sel.allPaid, now) : null;
  const period = r ? activePeriod(r, now) : null;
  return (
    <Page>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-wrap gap-2">{states.map((s) => <button key={s} onClick={() => nav({ status: live.scope.status === s ? null : s, restrictionId: null })} aria-pressed={live.scope.status === s} className={`rounded-full px-2.5 py-1 text-xs ${live.scope.status === s ? "bg-primary-soft text-primary" : "bg-surface2"}`}><span className="text-muted">{stateLabel[s]}</span> <b>{live.counts[s]}</b></button>)}</div>
        {live.canWrite && <Btn size="sm" variant="primary" onClick={() => setScheduling(true)}>+ Schedule restriction</Btn>}
      </div>
      {live.full && (
        <div className="flex flex-wrap items-end gap-3">
          <Field label="Contract"><Select value={live.scope.contractId ?? ""} onChange={(e) => nav({ contractId: e.target.value || null, restrictionId: null })}><option value="">All contracts</option>{live.contracts.map((c) => <option key={c.id} value={c.id}>{c.label}</option>)}</Select></Field>
          {live.scope.invoiceId && <Btn size="sm" onClick={() => nav({ invoiceId: null, restrictionId: null })}>Cause invoice {live.scope.invoiceId.slice(0, 8)} ✕</Btn>}
        </div>
      )}
      {!live.full && <Banner>Release view (restriction.override only): billing fields are not shown (IR03).</Banner>}
      <div className="split-rev">
        <Card title="Restrictions" sub="newest first" className="self-start">
          {live.rows.length === 0 ? <EmptyState title="No restrictions">No restriction matches this scope.</EmptyState> : <div className="flex flex-col gap-2">{live.rows.map((x) => <ListRow key={x.id} selected={r?.id === x.id} onClick={() => { setConfirmRules(false); nav({ restrictionId: x.id }); }}><div className="min-w-0 flex-1"><div className="flex items-center justify-between gap-2"><b className="truncate text-[13px]">{x.id.slice(0, 8)}</b><Badge tone={stateTone(x.state)}>{stateLabel[x.state]}</Badge></div><div className="text-[11px] text-muted">{x.label}</div><div className="text-[11px] text-muted">{x.policy} · {x.units} unit{x.units === 1 ? "" : "s"} · {x.progress}</div></div></ListRow>)}</div>}
        </Card>
        {!r || !sel ? <Card title="Restriction"><EmptyState title="Nothing selected">Choose a restriction.</EmptyState></Card> : (
          <div className="flex min-w-0 flex-col gap-4">
            <Card title={`Restriction ${r.id.slice(0, 8)}`} sub={`${sel.contract} · rules ${r.rulesVersion} · version ${r.version}`} action={<Link className="text-xs font-semibold text-primary" href={`/admin/restrictions/${r.id}`}>Exception / override →</Link>}>
              {step >= 0 ? <Steps steps={lifecycle} current={step} /> : <Banner tone="muted">Cancelled — no further commands.</Banner>}
              <div className="mt-4"><SummaryList cols={2} items={[
                ["Policy", policyText(r.policy)], ["Notice", `${klStamp(r.noticeAt)} · ${r.noticeNotificationIds?.length ?? 0} client notice(s)`],
                ["Execute after", klStamp(r.executeAfter)], ["Grace / exception", period ?? "none"],
                ...(live.full ? [["Reason shown to the customer", r.reason ? `“${r.reason}”` : "—"] as [string, string]] : []),
                ["Release intent", r.releaseIntent ? `${r.releaseIntent.source} · ${klStamp(r.releaseIntent.at)}` : "none"],
              ]} /></div>
              {live.canWrite && (
                <div className="mt-3 flex flex-col gap-2">
                  {r.state === "scheduled" && <>
                    <Check label={`I confirm the rules version ${r.rulesVersion} still applies`} checked={confirmRules} disabled={!!exec} onChange={setConfirmRules} />
                    <div className="flex flex-wrap items-center gap-2"><Btn variant="primary" disabled={pending || !!exec || !confirmRules} onClick={() => run(() => executeRestriction(r.id, r.version, r.rulesVersion), "Executed — apply requests sent per unit", () => setConfirmRules(false))}>Execute</Btn>{exec && <span className="text-xs text-muted">{exec}</span>}</div>
                  </>}
                  {(r.state === "requested" || r.state === "applied") && <div className="flex flex-wrap items-center gap-2"><Btn disabled={pending || !!release} onClick={() => run(() => requestRelease(r.id, r.version), "Release requested — remove commands sent per unit")}>Request release</Btn>{release && <span className="text-xs text-muted">{release}</span>}</div>}
                </div>
              )}
            </Card>
            {live.full && (
              <Card title="Cause invoices" action={<Link className="text-xs font-semibold text-primary" href="/admin/billing?overdueOnly=true">Open in Billing →</Link>}>
                {sel.causes.map((c) => <p key={c.id} className="text-[13px]"><Link className="font-semibold text-primary" href={`/admin/billing?invoiceId=${c.id}`}>{c.number}</Link> · {c.amountMinor === null ? "—" : amount(c.amountMinor, c.currency)} <Badge tone={c.paid ? "ok" : "warn"}>{c.paid ? "paid" : "unpaid"}</Badge></p>)}
                <p className="mt-1 text-xs text-muted">{sel.causes.filter((c) => !c.paid).length} of {sel.causes.length} cause invoices unpaid. When every cause invoice is paid, a scheduled restriction is cancelled and an applied one moves to release_requested.</p>
              </Card>
            )}
            <Card title="Units">
              <DataTable rows={sel.units} rowKey={(u) => u.unitId} cols={[
                { key: "u", label: "Unit", render: (u) => <b>{u.name}</b> }, { key: "a", label: "Apply", render: (u) => u.apply }, { key: "r", label: "Release", render: (u) => u.release },
                { key: "o", label: "Observed", render: (u) => <span className="text-xs">{u.observed}{u.pending ? ` · pending: ${u.pending}` : ""}</span>, hideBelow: "sm" },
                { key: "c", label: "Commands", render: (u) => <span className="text-xs">{u.commands}</span>, hideBelow: "md" },
                { key: "x", label: "", render: (u) => live.canWrite && <span className="flex flex-wrap gap-1">{u.retryApply && <Btn size="sm" onClick={() => setRetry({ phase: "apply", unitIds: [u.unitId], reason: "" })}>Retry apply</Btn>}{u.retryRelease && <Btn size="sm" onClick={() => setRetry({ phase: "release", unitIds: [u.unitId], reason: "" })}>Retry release</Btn>}{u.reconcile && <Btn size="sm" disabled={pending} onClick={() => run(() => reconcileUnits(r.id, r.version, [u.unitId]), "Reconciled with the latest observation")}>Reconcile</Btn>}</span> },
              ]} />
              <p className="mt-2 text-[11px] text-muted">Retry and Reconcile appear only for units that are pending, failed or waiting for reconciliation (SR26). Reaching the deadline never stops real equipment by itself.</p>
            </Card>
            {r.recoveryCases.length > 0 && <Card title="Recovery cases" sub="Earlier observed restrictions to reconcile explicitly (SR26)">{r.recoveryCases.map((c) => <p key={c.id} className="text-[13px]">{c.unitId.slice(0, 8)} · <Badge tone={c.state === "resolved" ? "ok" : "warn"}>{c.state}</Badge></p>)}</Card>}
          </div>
        )}
      </div>
      <Modal open={!!retry} onClose={() => setRetry(null)} title={`Retry ${retry?.phase ?? ""}`} footer={<><Btn onClick={() => setRetry(null)}>Cancel</Btn><Btn variant="primary" disabled={pending || !retry?.reason.trim()} onClick={() => r && retry && run(() => retryUnits(r.id, r.version, retry.unitIds, retry.phase, r.rulesVersion, retry.reason.trim()), "Retry sent", () => setRetry(null))}>Retry</Btn></>}>
        <p className="text-xs text-muted">Sends a new {retry?.phase === "apply" ? "apply" : "remove"} command for the unit with rules version {r?.rulesVersion}. An undeliverable unit returns Offline with nothing changed.</p>
        <Field label="Reason"><Textarea value={retry?.reason ?? ""} maxLength={1000} onChange={(e) => retry && setRetry({ ...retry, reason: e.target.value })} /></Field>
      </Modal>
      {live.schedule && <ScheduleModal open={scheduling} onClose={() => setScheduling(false)} contracts={live.schedule} now={now} onDone={(id) => { setScheduling(false); nav({ restrictionId: id, status: null }); }} />}
    </Page>
  );
}

function ScheduleModal({ open, onClose, contracts, now, onDone }: { open: boolean; onClose: () => void; contracts: ScheduleContract[]; now: Date; onDone: (id: string) => void }) {
  const [pending, run] = useAction();
  const [contractId, setContractId] = useState("");
  const [unitIds, setUnitIds] = useState<string[]>([]);
  const [kind, setKind] = useState<Policy["kind"]>("temperature_limit");
  const [setpoint, setSetpoint] = useState("24");
  const [executeAfter, setExecuteAfter] = useState(klLocal(new Date(now.getTime() + 25 * 3600 * 1000).toISOString()).slice(0, 14) + "00");
  const [reason, setReason] = useState("");
  const [tried, setTried] = useState(false);
  const k = contracts.find((c) => c.id === contractId);
  const chosen = k?.units.filter((u) => unitIds.includes(u.id)) ?? [];
  const range = chosen.reduce<{ min: number; max: number } | null>((acc, u) => (u.temperature ? { min: Math.max(acc?.min ?? -Infinity, u.temperature.min), max: Math.min(acc?.max ?? Infinity, u.temperature.max) } : acc), null);
  const sp = Number(setpoint);
  const errors: Record<string, string> = {};
  if (!k) errors.contractId = "Choose an eligible RTO contract";
  else if (k.invoices.length === 0) errors.contractId = "This contract has no overdue unpaid invoice";
  else if (k.recipients === 0) errors.contractId = "No active client can receive the notice (IR05)";
  if (unitIds.length === 0) errors.unitIds = "Choose at least one unit";
  if (kind === "temperature_limit" && (!range || !(sp >= range.min && sp <= range.max))) errors.setpoint = range ? `Within ${range.min}–${range.max} °C for the chosen units` : "The chosen units need a temperature range";
  if (!executeAfter || Date.parse(klInstant(executeAfter)) < now.getTime() + 24 * 3600 * 1000) errors.executeAfter = "At least 24 hours from now";
  if (reason.trim().length < 1 || reason.length > 1000) errors.reason = "1–1000 characters, shown to the customer";
  const save = () => {
    setTried(true);
    if (!k || Object.keys(errors).length > 0) return;
    run(() => scheduleRestriction({
      contractId: k.id, expectedContractVersion: k.version, causeInvoiceIds: k.invoices.map((i) => i.id), unitIds,
      policy: kind === "power_off" ? { kind } : { kind, minimumCoolingSetpoint: sp }, executeAfter: klInstant(executeAfter), reason: reason.trim(), rulesVersion: k.rulesVersion,
    }), "Restriction scheduled — the notice was sent", (id) => { setTried(false); setUnitIds([]); setReason(""); onDone(id); });
  };
  return (
    <Modal open={open} onClose={onClose} title="Schedule restriction" wide footer={<><Btn onClick={onClose}>Cancel</Btn><Btn variant="primary" disabled={pending} onClick={save}>Schedule</Btn></>}>
      <Field label="Contract (eligible RTO)" error={tried ? errors.contractId : undefined}><Select value={contractId} onChange={(e) => { setContractId(e.target.value); setUnitIds([]); }}><option value="">Select…</option>{contracts.map((c) => <option key={c.id} value={c.id}>{c.label} · rules {c.rulesVersion}</option>)}</Select></Field>
      {k && <>
        <p className="text-xs">Cause invoices (all overdue unpaid of the contract, fixed): {k.invoices.length ? k.invoices.map((i) => i.number).join(", ") : "none"} · notice to {k.recipients} active client{k.recipients === 1 ? "" : "s"}</p>
        <div><p className="mb-1 text-[13px] font-semibold">Units</p><div className="flex flex-col gap-1">{k.units.map((u) => <Check key={u.id} disabled={!!u.blocked} checked={unitIds.includes(u.id)} onChange={(on) => setUnitIds(on ? [...unitIds, u.id] : unitIds.filter((x) => x !== u.id))} label={<span>{u.name} <span className="text-xs text-muted">{u.blocked ? `— ${u.blocked}` : u.temperature ? `${u.temperature.min}–${u.temperature.max} °C` : "no temperature setpoint"}</span></span>} />)}</div>{tried && errors.unitIds && <p className="mt-1 text-xs text-crit">{errors.unitIds}</p>}</div>
      </>}
      <div className="grid-fluid" style={{ ["--min" as string]: "190px" }}>
        <Field label="Policy"><Select value={kind} onChange={(e) => setKind(e.target.value as Policy["kind"])}><option value="temperature_limit">Temperature limit</option><option value="power_off">Power off</option></Select></Field>
        {kind === "temperature_limit" && <Field label="Minimum cooling setpoint (°C)" error={tried ? errors.setpoint : undefined}><Input type="number" value={setpoint} onChange={(e) => setSetpoint(e.target.value)} /></Field>}
        <Field label="Execute after (Kuala Lumpur)" error={tried ? errors.executeAfter : undefined} hint="The notice is sent now; at least 24 h ahead"><Input type="datetime-local" value={executeAfter} onChange={(e) => setExecuteAfter(e.target.value)} /></Field>
      </div>
      <Field label="Reason shown to the customer" error={tried ? errors.reason : undefined}><Textarea value={reason} maxLength={1000} onChange={(e) => setReason(e.target.value)} placeholder="Demo: invoice overdue since 2026-09-10" /></Field>
    </Modal>
  );
}
